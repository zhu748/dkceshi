'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { detectMutedOrBannedResponse } = require('../../internal/js/chat-stream/upstream_inspect');
const { createUsageAccounting } = require('../../internal/js/chat-stream/usage_accounting');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream');
const cases = require('../fixtures/completion-account-failures.json');
const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
const sse = content => new Response('data: ' + JSON.stringify(content) + '\n\ndata: [DONE]\n\n');

for (const entry of cases) test(`Go/Node completion contract: ${entry.name}`, async () => {
  const result = await detectMutedOrBannedResponse(json(entry.body, entry.status));
  const kind = result.unauthorized ? 'unauthorized' : result.rateLimited ? 'limited' : result.banned ? 'banned' : result.mutedUntil ? 'muted' : 'passthrough';
  assert.equal(kind, entry.kind);
  if (result.response) await result.response.body?.cancel();
});

test('accounting resets retry prompts and adopts the replacement account count', () => {
  const usage = createUsageAccounting({ final_prompt: 'first', prompt_tokens: 1005, usage_accounting_version: 1 });
  usage.retry({ prompt: 'follow-up' });
  assert.deepEqual(usage.metadata().retry_prompts, ['follow-up']);
  usage.reset({ final_prompt: 'second', prompt_tokens: 2005, usage_accounting_version: 1 });
  assert.deepEqual(usage.metadata().retry_prompts, []);
  assert.equal(usage.build('', 'answer').prompt_tokens, 2005);
});

function surfaces() {
  const req = Object.assign(new EventEmitter(), { url: '/v1/chat/completions', headers: { host: 'example.test' } });
  const res = Object.assign(new EventEmitter(), { chunks: [], writableEnded: false, destroyed: false,
    setHeader() {}, flushHeaders() {}, flush() {},
    write(data) { this.chunks.push(String(data)); return true; },
    end(data) { if (data) this.write(data); this.writableEnded = true; },
  });
  return { req, res };
}

test('PoW refresh adopts credentials and release supplies authoritative final usage', async () => {
  const originalFetch = global.fetch;
  let releases = 0, calls = 0, releaseBody;
  const authHeaders = [];
  const authoritative = { prompt_tokens: 2048, completion_tokens: 17, total_tokens: 2065 };
  global.fetch = async (url, init) => {
    if (String(url).includes('__stream_prepare')) return json({ model: 'model', session_id: 'session', lease_id: 'lease', deepseek_token: 'old', pow_header: 'old-pow', payload: { prompt: 'hello' }, final_prompt: 'hello', prompt_tokens: 1005, usage_accounting_version: 1, thinking_enabled: true });
    if (String(url).includes('__stream_pow')) return json({ pow_header: 'new-pow', deepseek_token: 'new', base_headers: { 'x-test-identity': 'new-identity' } });
    if (String(url).includes('__stream_release')) { releases++; releaseBody = JSON.parse(init.body); return json({ success: true, usage: authoritative }); }
    throw new Error(`unexpected control ${url}`);
  };
  try {
    const { req, res } = surfaces();
    await handleVercelStream(req, res, Buffer.from('{}'), {}, async (_url, init) => {
      authHeaders.push(init.headers);
      return ++calls === 1 ? sse({ response_message_id: 42, p: 'response/thinking_content', v: 'plan' }) : sse({ p: 'response/content', v: 'answer' });
    });
    assert.equal(authHeaders[0].authorization, 'Bearer old');
    assert.equal(authHeaders[1].authorization, 'Bearer new');
    assert.equal(authHeaders[1]['x-test-identity'], 'new-identity');
    assert.equal(releases, 1);
    assert.equal(releaseBody.retry_prompts.length, 1);
    assert.match(releaseBody.retry_prompts[0], /Previous reply/);
    const frames = res.chunks.join('').split('\n\n').filter(x => x.startsWith('data: {')).map(x => JSON.parse(x.slice(6)));
    assert.deepEqual(frames.find(x => x.usage).usage, authoritative);
    assert.equal(res.writableEnded, true);
  } finally { global.fetch = originalFetch; }
});

test('business auth JSON requests credential recovery without a rate-limit cooldown', async () => {
  const { fetchAccountCompletion } = require('../../internal/js/chat-stream/completion_account_retry');
  let adopted = false, switches = 0;
  const response = await fetchAccountCompletion({ prompt: 'hello' }, {
    fetch: async () => adopted ? sse({ v: 'answer' }) : json({ code: 40001, msg: 'token expired' }),
    closed: () => false, canSwitch: () => true,
    switch: async options => { switches++; assert.deepEqual(options, { auth_failed: true }); return { ok: true, body: { payload: { prompt: 'hello' } } }; },
    adopt: () => { adopted = true; }, fail: () => assert.fail('recoverable auth failure'),
  });
  assert.equal(switches, 1);
  assert.match(await response.text(), /answer/);
});

test('a failed release preserves the prepared full-input count and finishes the stream', async t => {
  t.mock.method(console, 'warn', () => {});
  const originalFetch = global.fetch;
  let releases = 0;
  global.fetch = async url => {
    if (String(url).includes('__stream_prepare')) return json({ model: 'model', session_id: 'session', lease_id: 'lease', deepseek_token: 'token', pow_header: 'pow', payload: { prompt: 'short' }, final_prompt: 'short', prompt_tokens: 1005, usage_accounting_version: 1 });
    if (String(url).includes('__stream_release')) { releases++; return json({ error: 'offline' }, 503); }
    throw new Error(`unexpected ${url}`);
  };
  try {
    const { req, res } = surfaces();
    await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => sse({ v: 'answer' }));
    const frames = res.chunks.join('').split('\n\n').filter(x => x.startsWith('data: {')).map(x => JSON.parse(x.slice(6)));
    assert.equal(frames.find(x => x.usage).usage.prompt_tokens, 1005);
    assert.equal(releases, 1);
    assert.equal(res.writableEnded, true);
  } finally { global.fetch = originalFetch; }
});

test('disconnect during PoW counts the partial generation without an unsent retry prompt', async () => {
  const originalFetch = global.fetch;
  const { req, res } = surfaces();
  let calls = 0, released;
  global.fetch = async (url, init) => {
    if (String(url).includes('__stream_prepare')) return json({ model: 'model', session_id: 'session', lease_id: 'lease', deepseek_token: 'token', pow_header: 'pow', payload: { prompt: 'hello' }, final_prompt: 'hello', thinking_enabled: true, prompt_tokens: 1005, usage_accounting_version: 1 });
    if (String(url).includes('__stream_pow')) { res.destroyed = true; res.emit('close'); return json({ pow_header: 'next' }); }
    if (String(url).includes('__stream_release')) { released = JSON.parse(init.body); return json({ success: true }); }
    throw new Error(`unexpected ${url}`);
  };
  try {
    await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => { calls++; return sse({ response_message_id: 42, p: 'response/thinking_content', v: 'plan' }); });
    assert.equal(calls, 1);
    assert.equal(released.finish_reason, 'stopped');
    assert.equal(released.thinking, 'plan');
    assert.deepEqual(released.retry_prompts, []);
    assert.equal(released.usage.prompt_tokens, 1005);
  } finally { global.fetch = originalFetch; }
});
