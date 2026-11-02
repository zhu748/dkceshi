'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { createVisibleGuard } = require('../../internal/js/helpers/stream-tool-sieve/visible_guard');
const { createToolSieveState } = require('../../internal/js/helpers/stream-tool-sieve');
const { appendCapture, captureReady } = require('../../internal/js/helpers/stream-tool-sieve/capture_progress');
const { buildFinalUsage } = require('../../internal/js/chat-stream/token_usage');
const { createStreamReport } = require('../../internal/js/chat-stream/stream_report');
const { createStreamWatch } = require('../../internal/js/chat-stream/stream_limits');
const { fetchStreamPrepare, fetchStreamPow, fetchStreamSwitch, fetchStreamValidation } = require('../../internal/js/chat-stream/http_internal');

test('ordinary quoted prose streams before closure without duplicating its source', () => {
  const guard = createVisibleGuard();
  for (const part of ['I will explain "', 'current streaming behavior', '" then continue.']) assert.equal(guard.write(part), part);
  assert.equal(guard.flush(), '');
});

test('quoted Unicode tool prefixes stay buffered until their literal closes', () => {
  for (const delimiter of ['<', '＜', '﹤', '〈']) {
    const guard = createVisibleGuard();
    const markup = delimiter + '|EPSE|tool_calls>literal';
    assert.equal(guard.write('"ordinary prefix '), '"ordinary prefix ');
    assert.equal(guard.write(markup), '');
    assert.equal(guard.write('"') + guard.flush(), markup + '"');
  }
});

test('a complete batch schedules parsing once instead of after every argument', () => {
  for (const open of ['<tool_calls>', '<tool_calls version="1">']) {
  const state = createToolSieveState();
  appendCapture(state, open);
  assert.equal(captureReady(state), true);
  for (let i = 0; i < 64; i++) {
    appendCapture(state, '<invoke name="read"><parameter name="path"><![CDATA[a.txt]]></parameter></invoke>');
    assert.equal(captureReady(state), false, 'inner close triggered a full parse');
  }
  for (const ch of '</tool_calls>') appendCapture(state, ch);
  assert.equal(captureReady(state), true);
  }
});

test('authoritative input and output usage replace local estimates including zero', () => {
  for (const usage of [
    { prompt_tokens: 1234, completion_tokens: 20, total_tokens: 1254, completion_tokens_details: { reasoning_tokens: 2 } },
    { prompt_tokens: 0, completion_tokens: 0, total_tokens: 0 },
  ]) assert.deepEqual(buildFinalUsage('hello', 'thinking', 'long output', usage), usage);
  const fallback = buildFinalUsage('hello', '', 'answer', { completion_tokens: 9 });
  assert.equal(fallback.completion_tokens, 9);
  assert.equal(fallback.total_tokens, fallback.prompt_tokens + 9);
});

test('internal control requests inherit client cancellation', async t => {
  const old = global.fetch;
  t.after(() => { global.fetch = old; });
  const req = { headers: { host: 'example.test' }, url: '/v1/chat/completions' };
  for (const start of [signal => fetchStreamPrepare(req, Buffer.from('{}'), signal), signal => fetchStreamPow(req, 'lease', signal), signal => fetchStreamSwitch(req, 'lease', {}, signal)]) {
    let captured;
    global.fetch = async (_url, init) => new Promise((_resolve, reject) => {
      captured = init.signal;
      captured.addEventListener('abort', () => reject(captured.reason), { once: true });
    });
    const controller = new AbortController();
    const request = start(controller.signal);
    assert.equal(captured.aborted, false);
    controller.abort();
    await assert.rejects(request, { name: 'AbortError' });
    assert.equal(captured.aborted, true);
  }
});

test('Vercel stream budgets leave time for validation and cleanup', t => {
  const old = process.env.VERCEL;
  process.env.VERCEL = '1';
  t.after(() => { if (old === undefined) delete process.env.VERCEL; else process.env.VERCEL = old; });
  const watch = createStreamWatch();
  assert.ok(watch.finalDeadline - Date.now() <= 300000);
  assert.equal(watch.finalDeadline - watch.deadline, 20000);
});

test('request reports retain stage times and original terminal error', async () => {
  let now = 1000;
  const report = createStreamReport(() => now);
  now = 1050; report.mark('first_visible_ms'); report.mark('upstream_done_ms');
  now = 1060; report.mark('first_visible_ms');
  await report.measure('validation_ms', async () => { now = 1080; });
  report.mark('upstream_done_ms', true);
  report.fail({ writableEnded: true }, 422, 'missing path', 'invalid_tool_arguments');
  const record = report.snapshot({ error: 'missing path' });
  assert.equal(record.started_at_ms, 1000);
  assert.equal(record.elapsed_ms, 80);
  assert.deepEqual(record.timings, { first_visible_ms: 50, upstream_done_ms: 80, validation_ms: 20 });
  assert.equal(record.error_status, 422);
  assert.equal(record.error_code, 'invalid_tool_arguments');
});

test('tool diagnostic attempt metadata survives Node regeneration and release', () => {
  const report = createStreamReport();
  assert.equal(report.diagnostic().diagnostic_attempt, 1);
  report.regenerate({ code: 'invalid_tool_arguments', message: 'missing path' });
  assert.deepEqual(report.diagnostic(), { diagnostic_attempt: 2, diagnostic_retry_reason: 'missing path' });
  const release = report.snapshot({ finish_reason: 'tool_calls' });
  assert.equal(release.diagnostic_attempt, 2);
  assert.equal(release.diagnostic_retry_reason, 'missing path');
});


test('validation uses configured repair timeout and honors the remaining request deadline', async t => {
  const oldFetch = global.fetch, oldTimeout = global.setTimeout;
  t.after(() => { global.fetch = oldFetch; global.setTimeout = oldTimeout; });
  let delay, body;
  global.setTimeout = (fn, ms, ...args) => { delay = ms; return oldTimeout(fn, 120000, ...args); };
  global.fetch = async (_url, init) => { body = JSON.parse(init.body); return { ok: true, status: 200, json: async () => ({ tool_calls: [] }) }; };
  const req = { headers: { host: 'example.test' }, url: '/v1/chat/completions' };
  await fetchStreamValidation(req, 'lease', 'tool', '', '', undefined, Date.now() + 120000, { diagnostic_attempt: 2 }, 60000);
  assert.equal(delay, 65000);
  assert.equal(body.diagnostic_attempt, 2);
  assert.equal(body.content, 'tool');
  await fetchStreamValidation(req, 'lease', 'tool', '', '', undefined, Date.now() + 100, {}, 60000);
  assert.ok(delay >= 1 && delay <= 100);
});
