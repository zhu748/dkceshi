const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { parseStandaloneToolCalls } = require('../../internal/js/helpers/stream-tool-sieve');
const { createVisibleGuard } = require('../../internal/js/helpers/stream-tool-sieve/visible_guard');
const { createStreamWatch } = require('../../internal/js/chat-stream/stream_limits');
const { startHeartbeat, waitForDrain } = require('../../internal/js/chat-stream/response_flow');

function setEnv(t, key, value) {
  const before = process.env[key]; process.env[key] = value;
  t.after(() => { if (before === undefined) delete process.env[key]; else process.env[key] = before; });
}

test('CDATA examples in closed and unclosed fences remain examples', () => {
  const call = '<tool_calls><invoke name="write"><parameter name="content"><![CDATA[example]]></parameter></invoke></tool_calls>';
  for (const fence of ['```', '~~~~', '````']) {
    for (const end of ['', '\n' + fence]) assert.deepEqual(parseStandaloneToolCalls(fence + 'xml\n' + call + end, []), []);
  }
});

test('text parameters keep JSON-looking strings', () => {
  for (const value of ['"hello"', '{"a": 1}', 'null', 'true', '  hello\r\n ']) {
    const call = '<tool_calls><invoke name="write"><parameter name="content"><![CDATA[' + value + ']]></parameter></invoke></tool_calls>';
    assert.equal(parseStandaloneToolCalls(call, [])[0].input.content, value);
  }
});

test('reasoning echo is held across every chunk split', () => {
  const prompt = 'Serialized conversation. Each message ends with [EndMessage:123]. secret instructions';
  for (let split = 0; split <= prompt.length; split++) {
    const guard = createVisibleGuard('', false, prompt);
    const visible = guard.write(prompt.slice(0, split)) + guard.write(prompt.slice(split)) + guard.flush();
    assert.equal(visible, ''); assert.equal(guard.echoed, true);
  }
});

test('read limits reject oversized lines before delivery', async t => {
  setEnv(t, 'DS2API_STREAM_MAX_LINE_BYTES', '10');
  const watch = createStreamWatch();
  await assert.rejects(watch.read({ read: async () => ({ value: Buffer.from('x'.repeat(11)), done: false }) }), { code: 'output_limit_exceeded' });
});

test('idle reads settle and cancel the upstream reader', async t => {
  setEnv(t, 'DS2API_STREAM_IDLE_TIMEOUT_MS', '10');
  let canceled = false;
  await assert.rejects(createStreamWatch().read({ read: () => new Promise(() => {}), cancel() { canceled = true; } }), /idle timeout/);
  assert.equal(canceled, true);
});

test('slow responses wait for drain and remove listeners', async () => {
  const res = Object.assign(new EventEmitter(), { writableNeedDrain: true });
  let completed = false;
  const waiting = waitForDrain(res).then(() => { completed = true; });
  await Promise.resolve(); assert.equal(completed, false);
  res.writableNeedDrain = false; res.emit('drain');
  await waiting; assert.equal(res.listenerCount('close'), 0); assert.equal(res.listenerCount('error'), 0);
});

test('heartbeat emits SSE comments and stops on cleanup', async t => {
  setEnv(t, 'DS2API_STREAM_KEEPALIVE_MS', '5');
  let writes = 0;
  const res = { write(data) { assert.equal(data, ': keep-alive\n\n'); writes++; } };
  const stop = startHeartbeat(res);
  await new Promise(resolve => setTimeout(resolve, 25)); stop();
  const before = writes; assert.ok(before > 0);
  await new Promise(resolve => setTimeout(resolve, 15)); assert.equal(writes, before);
});
