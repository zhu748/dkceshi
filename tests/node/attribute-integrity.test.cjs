'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { parseToolCalls, createToolSieveState, processToolSieveChunk, flushToolSieve } = require('../../internal/js/helpers/stream-tool-sieve');

test('unknown attribute payloads cannot execute at any chunk split', () => {
  for (const raw of [
    '<tool_calls><invoke name="read"><parameter name="path" value="TARGET.txt"></parameter></invoke></tool_calls>',
    '<tool_calls><invoke name="read" arguments=\'{"path":"TARGET.txt"}\'></invoke></tool_calls>',
    '<tool_calls><invoke arguments=\'name="read"\'></invoke></tool_calls>',
    '<tool_calls note="SOURCE"><invoke name="read"></invoke></tool_calls>',
    '<tool_calls note="ONE" note="TWO"><invoke name="read"></invoke></tool_calls>',
    '<tool_calls><invoke name="read"><parameter name="path" value TARGET></parameter></invoke></tool_calls>',
    '<tool_calls><invoke name="read" arguments TARGET></invoke></tool_calls>',
  ]) {
    assert.deepEqual(parseToolCalls(raw, []), []);
    for (let split = 0; split <= raw.length; split++) {
      const state = createToolSieveState();
      const events = [...processToolSieveChunk(state, raw.slice(0, split), []), ...processToolSieveChunk(state, raw.slice(split), []), ...flushToolSieve(state, [])];
      assert.equal(events.flatMap(e => e.calls || []).length, 0, `split=${split}`);
    }
  }
});

test('attribute-looking content inside CDATA remains literal', () => {
  const value = '<parameter name="path" value="literal">';
  const raw = `<tool_calls id="batch" version="1"><invoke name="read" call_id="history"><parameter name="content"><![CDATA[${value}]]></parameter></invoke></tool_calls>`;
  assert.deepEqual(parseToolCalls(raw, []), [{ name: 'read', input: { content: value } }]);
});
