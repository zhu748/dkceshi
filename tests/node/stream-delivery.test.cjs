'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream');
const { createChatCompletionEmitter } = require('../../internal/js/chat-stream/stream_emitter');

const json = body => new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } });

for (const boundary of ['success', 'tool_write_failure', 'terminal_write_failure', 'close_after_tools']) {
  test(`stream delivery evidence: ${boundary}`, async t => {
    t.mock.method(console, 'warn', () => {});
    const req = Object.assign(new EventEmitter(), { url: '/v1/chat/completions', headers: { host: 'example.test' } });
    const res = Object.assign(new EventEmitter(), {
      chunks: [], destroyed: false, writableEnded: false,
      setHeader() {}, flushHeaders() {}, flush() {},
      write(data) {
        data = String(data);
        if (boundary === 'tool_write_failure' && data.includes('"tool_calls":[') || boundary === 'terminal_write_failure' && data.includes('"finish_reason":')) throw new Error('test write failure');
        this.chunks.push(data);
        if (boundary === 'close_after_tools' && data.includes('"tool_calls":[')) { this.destroyed = true; this.emit('close'); }
        return true;
      },
      destroy() { this.destroyed = true; this.emit('close'); },
      end(data) { if (data) this.write(data); this.writableEnded = true; },
    });
    const requests = [];
    t.mock.method(global, 'fetch', async (url, init) => {
      url = String(url);
      if (url.includes('__stream_prepare')) return json({ model: 'model', lease_id: 'lease', session_id: 'session', payload: { prompt: 'task' }, final_prompt: 'task', deepseek_token: 'token', pow_header: 'pow', tool_names: ['read'], validate_tools: true, usage_accounting_version: 1, prompt_tokens: 10 });
      if (url.includes('__stream_validate')) return json({ content: '', tool_calls: [{ index: 0, id: 'call_read', type: 'function', function: { name: 'read', arguments: '{}' } }] });
      if (url.includes('__stream_release')) {
        const body = JSON.parse(init.body);
        requests.push(body);
        if (body.usage_only) assert.equal(res.chunks.some(frame => frame.includes('"finish_reason":')), false);
        else if (boundary === 'success') assert.ok(res.chunks.includes('data: [DONE]\n\n'));
        return json({ success: true, usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 } });
      }
      throw new Error(`unexpected control ${url}`);
    });
    const raw = '<tool_calls><invoke name="read"></invoke></tool_calls>';
    await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => new Response('data: ' + JSON.stringify({ p: 'response/content', v: raw }) + '\n\ndata: [DONE]\n\n'));
    const releases = requests.filter(body => !body.usage_only);
    assert.equal(releases.length, 1);
    assert.equal(releases[0].delivery_version, 1);
    assert.equal(releases[0].tools_written, boundary !== 'tool_write_failure');
    assert.equal(releases[0].response_written, boundary === 'success');
    if (boundary.includes('write_failure')) assert.equal(releases[0].error_code, 'client_write_error');
    if (boundary === 'success') assert.equal(releases[0].finish_reason, 'tool_calls');
  });
}

test('emitter does not consume role state on skipped or failed writes', () => {
  let closed = true;
  const frames = [];
  const res = { write(frame) { frames.push(frame); return false; }, flush() {} };
  const emitter = createChatCompletionEmitter({ res, isClosed: () => closed });
  assert.equal(emitter.sendDeltaFrame({ content: 'skipped' }), false);
  closed = false;
  assert.equal(emitter.sendDeltaFrame({ content: 'written with backpressure' }), true);
  assert.equal(JSON.parse(frames[0].slice(6)).choices[0].delta.role, 'assistant');
});
