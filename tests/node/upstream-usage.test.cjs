'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const cases = require('../fixtures/upstream-usage.json');
const { parseUpstreamUsage, createUpstreamUsage, resolveUpstreamUsage } = require('../../internal/js/chat-stream/upstream_usage');
const { parseChunkForContent } = require('../../internal/js/chat-stream/sse_parse');
const { createUsageAccounting } = require('../../internal/js/chat-stream/usage_accounting');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream');

for (const fixture of cases) test(`shared usage fixture: ${fixture.name}`, () => {
  assert.deepEqual(parseUpstreamUsage(fixture.chunk), fixture.expected);
  const parsed = parseChunkForContent(fixture.chunk, false, 'text');
  if (fixture.name === 'output count string is metadata') assert.deepEqual(parsed.parts, []);
});

test('continuations replace cumulative snapshots and regeneration resets them', () => {
  const usage = createUsageAccounting({ final_prompt: 'task', prompt_tokens: 100, usage_accounting_version: 1 });
  usage.observe({ message_id: 2, accumulated_tokens: 0 });
  for (const n of [2413, 2413, 2551, 2413]) usage.observe({ message_id: 2, accumulated_tokens: n });
  assert.equal(usage.metadata().upstream_usage.messages[0].accumulated_tokens, 2551);
  assert.equal(usage.build('', 'answer').ds2api.source, 'estimated');
  const wire = usage.metadata(); wire.upstream_usage.messages[0].accumulated_tokens = 7;
  assert.equal(usage.metadata().upstream_usage.messages[0].accumulated_tokens, 2551);
  usage.resetUpstream();
  assert.deepEqual(usage.metadata().upstream_usage.messages, []);
  usage.observe({ input_tokens: 0, output_tokens: 0, reasoning_tokens: 0 });
  const zero = usage.build('', 'answer', null, false);
  assert.equal(zero.total_tokens, 0); assert.equal(zero.ds2api.source, 'upstream'); assert.equal(zero.ds2api.complete, false);
  const capped = createUpstreamUsage();
  for (let id = 1; id <= 80; id++) capped.observe({ message_id: id, accumulated_tokens: id });
  assert.equal(capped.snapshot().messages.length, 64); assert.equal(capped.snapshot().truncated, true);
});

test('Vercel sends upstream counts to preview, release and the client', async t => {
  const req = Object.assign(new EventEmitter(), { url: '/v1/chat/completions', headers: { host: 'test.example' } });
  const res = Object.assign(new EventEmitter(), { chunks: [], setHeader() {}, flushHeaders() {}, flush() {}, write(value) { this.chunks.push(value); return true; }, end() { this.writableEnded = true; } });
  const reports = [];
  const json = body => new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } });
  t.mock.method(global, 'fetch', async (url, init) => {
    if (String(url).includes('__stream_prepare')) return json({ model: 'model', lease_id: 'lease', session_id: 'session', deepseek_token: 'token', pow_header: 'pow', payload: { prompt: 'task', chat_session_id: 'session' }, final_prompt: 'task', thinking_enabled: false, usage_accounting_version: 1, prompt_tokens: 100 });
    if (String(url).includes('__stream_release')) { reports.push(JSON.parse(init.body)); return json({ success: true }); }
    if (String(url).includes('/chat/completion')) return new Response('data: {"v":{"response":{"message_id":2,"accumulated_token_usage":0}}}\n\ndata: {"p":"response/content","v":"answer"}\n\ndata: {"p":"response","o":"BATCH","v":[{"p":"token_usage","v":{"input_tokens":11,"output_tokens":29,"reasoning_tokens":9}},{"p":"accumulated_token_usage","v":2551},{"p":"status","v":"FINISHED"}]}\n\ndata: [DONE]\n\n', { headers: { 'content-type': 'text/event-stream' } });
    throw new Error(`unexpected fetch ${url}`);
  });
  const payload = { model: 'model', messages: [{ role: 'user', content: 'task' }], stream: true };
  await handleVercelStream(req, res, JSON.stringify(payload), payload, global.fetch);
  assert.ok(reports.some(report => report.usage_only));
  for (const report of reports) assert.equal(report.upstream_usage.messages[0].accumulated_tokens, 2551);
  const usage = res.chunks.join('').split('\n').filter(line => line.startsWith('data: {')).map(line => JSON.parse(line.slice(6))).find(chunk => chunk.usage)?.usage;
  assert.equal(usage.prompt_tokens, 11); assert.equal(usage.completion_tokens, 29); assert.equal(usage.total_tokens, 40);
  assert.equal(usage.ds2api.source, 'upstream');
});

test('an upstream reasoning count replaces the estimated reasoning portion once', () => {
 const base = { prompt_tokens: 100, completion_tokens: 20, total_tokens: 120, completion_tokens_details: { reasoning_tokens: 5 } };
 const usage = resolveUpstreamUsage(base, { messages: [{ reasoning_tokens: 9 }] });
 assert.equal(usage.completion_tokens, 24); assert.equal(usage.total_tokens, 124); assert.equal(usage.ds2api.source, 'mixed');
 assert.equal(usage.completion_tokens_details.reasoning_tokens, 9);
 const unsafe = resolveUpstreamUsage(base, { messages: [{ input_tokens: Number.MAX_SAFE_INTEGER, output_tokens: 29 }] });
 assert.equal(unsafe.prompt_tokens, 100); assert.equal(unsafe.completion_tokens, 20); assert.equal(unsafe.ds2api.source, 'estimated');
});

test('validated upstream history counts retain their provenance if preview is unavailable', () => {
 const accounting = createUsageAccounting({ final_prompt: 'task', prompt_tokens: 100, usage_accounting_version: 1 });
 const validated = { prompt_tokens: 58, completion_tokens: 23, total_tokens: 81, ds2api: { source: 'upstream', input_source: 'upstream', output_source: 'upstream', total_source: 'upstream', complete: true, upstream: { request: { input_tokens: 58, output_tokens: 23, total_tokens: 81 } } } };
 const result = accounting.build('', 'answer', validated);
 assert.deepEqual(result, validated);
 assert.equal(accounting.build('', 'answer', validated, false).ds2api.complete, false);
});
