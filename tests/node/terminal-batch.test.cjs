'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { parseChunkForContent } = require('../../internal/js/chat-stream/sse_parse');

test('terminal BATCH retains content following the response status', () => {
  const result = parseChunkForContent({ p: 'response', o: 'APPEND', v: [
    { p: 'status', v: 'FINISHED' }, { p: 'content', v: 'final answer' },
  ] }, false, 'text');
  assert.equal(result.finished, true);
  assert.equal(result.parts.map(part => part.text).join(''), 'final answer');
});

test('response snapshots preserve filtering without misreading literal answer text', () => {
  for (const chunk of [
    { v: { response: { status: 'CONTENT_FILTER', fragments: [{ type: 'RESPONSE', content: 'blocked' }] } } },
    { p: 'response', v: { quasi_status: 'CONTENT_FILTER', fragments: [] } },
  ]) {
    const result = parseChunkForContent(chunk, false, 'text');
    assert.equal(result.finished, true);
    assert.equal(result.contentFilter, true);
    assert.deepEqual(result.parts, []);
  }
  const result = parseChunkForContent({ p: 'response/content', v: '{"status":"CONTENT_FILTER"}' }, false, 'text');
  assert.equal(result.contentFilter, false);
  assert.equal(result.finished, false);
  assert.equal(result.parts.length, 1);
});
