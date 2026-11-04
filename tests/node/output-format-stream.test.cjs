'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream');

for (const valid of [true, false]) test(`JSON answer waits for final validation: valid=${valid}`, async t => {
  const req = Object.assign(new EventEmitter(), { url: '/v1/chat/completions', headers: { host: 'example.test' } });
  const res = Object.assign(new EventEmitter(), {
    chunks: [], writableEnded: false, destroyed: false,
    setHeader() {}, flushHeaders() {}, flush() {},
    write(data) { this.chunks.push(String(data)); return true; },
    end(data) { if (data) this.write(data); this.writableEnded = true; },
  });
  const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
  const source = valid ? '{"answer":42}' : 'not JSON';
  let validations = 0;
  t.mock.method(global, 'fetch', async (url, init) => {
    if (String(url).includes('__stream_prepare')) return json({ model: 'model', session_id: 'session', lease_id: 'lease', deepseek_token: 'token', pow_header: 'pow', payload: { prompt: 'JSON please' }, final_prompt: 'JSON please', validate_tools: true, validate_output: true, tool_choice: { Mode: 'auto' } });
    if (String(url).includes('__stream_release')) return json({ success: true });
    if (String(url).includes('__stream_validate')) {
      validations++;
      assert.equal(JSON.parse(init.body).content, source);
      assert.equal(res.chunks.some(frame => frame.includes('"content":')), false, 'unvalidated text escaped');
      return valid ? json({ tool_calls: [], content: source }) : json({ error: { code: 'invalid_response_format', message: 'response must contain one JSON value' } }, 422);
    }
    throw new Error('unexpected control request');
  });
  const wire = `data: ${JSON.stringify({ p: 'response/content', v: source })}\n\ndata: [DONE]\n\n`;
  await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => new Response(wire));
  const output = res.chunks.join('');
  assert.equal(validations, 1);
  if (valid) assert.ok(output.includes('"content":"{\\"answer\\":42}"'));
  else {
    assert.ok(output.includes('invalid_response_format'));
    assert.equal(output.includes('"content":"not JSON"'), false);
  }
  assert.ok(output.includes('[DONE]'));
});
