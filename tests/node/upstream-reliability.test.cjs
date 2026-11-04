'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { fetchAccountCompletion } = require('../../internal/js/chat-stream/completion_account_retry');
const { createStreamWatch } = require('../../internal/js/chat-stream/stream_limits');
const { detectMutedOrBannedResponse } = require('../../internal/js/chat-stream/upstream_inspect');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream');

const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
const prepared = { model: 'model', session_id: 'session', lease_id: 'lease', deepseek_token: 'token', pow_header: 'pow', payload: { prompt: 'hello' }, final_prompt: 'hello', managed_account: true };
const answer = () => new Response('data: {"p":"response/content","v":"answer"}\n\ndata: [DONE]\n\n');
function surfaces() {
  const req = Object.assign(new EventEmitter(), { url: '/v1/chat/completions', headers: { host: 'example.test' } });
  const res = Object.assign(new EventEmitter(), { chunks: [], writableEnded: false, destroyed: false,
    setHeader() {}, flushHeaders() {}, write(text) { this.chunks.push(String(text)); return true; },
    end(text) { if (text) this.write(text); this.writableEnded = true; },
  });
  return { req, res };
}
function limits(t) {
  for (const [key, value] of Object.entries({ IDLE_TIMEOUT_MS: '15', CONNECT_TIMEOUT_MS: '15', MAX_DURATION_MS: '150' })) {
    const name = `DS2API_STREAM_${key}`, old = process.env[name];
    process.env[name] = value;
    t.after(() => { if (old === undefined) delete process.env[name]; else process.env[name] = old; });
  }
}

test('transport failures recover through the replacement account payload and credentials', async t => {
  let calls = 0, switches = 0, releases = 0;
  t.mock.method(global, 'fetch', async url => {
    if (String(url).includes('__stream_prepare')) return json(prepared);
    if (String(url).includes('__stream_switch')) { switches++; return json({ ...prepared, deepseek_token: 'replacement', payload: { prompt: 'rebuilt' } }); }
    if (String(url).includes('__stream_release')) { releases++; return json({}); }
    throw new Error('unexpected control');
  });
  const { req, res } = surfaces();
  await handleVercelStream(req, res, Buffer.from('{}'), {}, async (_url, init) => {
    if (++calls === 1) throw new Error('ECONNRESET');
    assert.equal(init.headers.authorization, 'Bearer replacement');
    assert.match(init.body, /rebuilt/);
    return answer();
  });
  assert.equal(calls, 2); assert.equal(switches, 1); assert.equal(releases, 1);
  assert.match(res.chunks.join(''), /answer/);
});

for (const source of ['no bytes', 'partial JSON', 'partial error body']) test(`inspection is bounded and releases its lease: ${source}`, async t => {
  limits(t);
  let canceled = 0, releases = 0, switches = 0;
  t.mock.method(global, 'fetch', async url => {
    if (String(url).includes('__stream_prepare')) return json(prepared);
    if (String(url).includes('__stream_switch')) { switches++; return json({}, 503); }
    if (String(url).includes('__stream_release')) { releases++; return json({}); }
    throw new Error('unexpected control');
  });
  const { req, res } = surfaces();
  await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => new Response(new ReadableStream({
    start(controller) {
      if (source !== 'no bytes') controller.enqueue(Buffer.from(source === 'partial JSON' ? '{"code":' : 'error'));
    }, cancel() { canceled++; },
  }), { status: source === 'partial error body' ? 503 : 200 }));
  assert.equal(canceled, 1); assert.equal(releases, 1); assert.equal(switches, 1);
  assert.equal(res.statusCode, 503); assert.match(res.chunks.join(''), /upstream_unavailable/);
});

test('deadline exhaustion is terminal and does not bench or switch an account', async t => {
  limits(t);
  const watch = createStreamWatch();
  const failure = [];
  let switched = 0;
  await fetchAccountCompletion({}, {
    fetch: async () => { throw Object.assign(new Error('budget exhausted'), { code: 'output_limit_exceeded' }); },
    closed: () => false, canSwitch: () => true, switch: () => { switched++; }, fail: detail => failure.push(detail),
  });
  assert.equal(switched, 0); assert.equal(failure[0].code, 'output_limit_exceeded');
  const controller = new AbortController(); controller.abort();
  let canceled = 0;
  await assert.rejects(watch.read({ cancel() { canceled++; }, read() { throw new Error('must not read'); } }, { signal: controller.signal }), { name: 'AbortError' });
  assert.equal(canceled, 1);
});

test('legacy bridges have a bounded transport recovery budget', async () => {
  let calls = 0, switches = 0, failure;
  await fetchAccountCompletion({}, {
    fetch: async () => { calls++; throw new Error('TLS handshake failed'); },
    closed: () => false, canSwitch: () => true,
    switch: async () => { switches++; return { ok: true, body: { payload: {} } }; },
    adopt() {}, fail: detail => { failure = detail; },
  });
  assert.equal(calls, 4); assert.equal(switches, 3); assert.equal(failure.code, 'upstream_unavailable');
});

test('already delivered output prevents transport-error regeneration', async () => {
  let switches = 0, failure;
  await fetchAccountCompletion({}, {
    fetch: async () => { throw new Error('ECONNRESET'); }, closed: () => false, canSwitch: () => false,
    switch: () => { switches++; }, fail: detail => { failure = detail; },
  });
  assert.equal(switches, 0); assert.equal(failure.code, 'upstream_unavailable');
});

test('inspection replay counts SSE bytes only once', async t => {
  const name = 'DS2API_STREAM_MAX_OUTPUT_BYTES', old = process.env[name];
  process.env[name] = '8'; t.after(() => { if (old === undefined) delete process.env[name]; else process.env[name] = old; });
  const watch = createStreamWatch();
  const inspected = await detectMutedOrBannedResponse(new Response('data: x\n'), reader => watch.read(reader, { countBytes: false }));
  const reader = inspected.response.body.getReader();
  assert.equal((await watch.read(reader)).value.length, 8);
  assert.equal((await watch.read(reader)).done, true);
  reader.releaseLock();
});

test('the total deadline cancels stalled response headers without switching accounts', async t => {
  limits(t);
  // AbortSignal.timeout is unref'd; a real HTTP socket keeps this wait alive.
  const keepAlive = setTimeout(() => {}, 1000); t.after(() => clearTimeout(keepAlive));
  let releases = 0, switches = 0;
  t.mock.method(global, 'fetch', async url => {
    if (String(url).includes('__stream_prepare')) return json(prepared);
    if (String(url).includes('__stream_release')) { releases++; return json({}); }
    if (String(url).includes('__stream_switch')) { switches++; return json({}, 503); }
    throw new Error('unexpected control');
  });
  const { req, res } = surfaces();
  await handleVercelStream(req, res, Buffer.from('{}'), {}, async (_url, init) => new Promise((_resolve, reject) => {
    init.signal.addEventListener('abort', () => reject(init.signal.reason), { once: true });
  }));
  assert.equal(releases, 1); assert.equal(switches, 0);
  assert.equal(res.statusCode, 502); assert.match(res.chunks.join(''), /output_limit_exceeded/);
});
