'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { parseToolCalls, createToolSieveState, processToolSieveChunk, flushToolSieve } = require('../../internal/js/helpers/stream-tool-sieve');
const { resolveToolcallPolicy } = require('../../internal/js/chat-stream/toolcall_policy');

function wrapper(body) {
  return `<tool_calls><invoke name="write">${body}</invoke></tool_calls>`;
}

test('valid JSON objects retain item and same-name keys at every stream split', () => {
  for (const value of [{ item: 42 }, { item: [] }, { options: [1, 2] }, { item: { item: [1] } }]) {
    for (const body of [JSON.stringify(value), `<![CDATA[${JSON.stringify(value)}]]>`]) {
      const raw = wrapper(`<parameter name="options">${body}</parameter>`);
      const expected = [{ name: 'write', input: { options: value } }];
      assert.deepEqual(parseToolCalls(raw, []), expected);
      for (let split = 0; split <= raw.length; split++) {
        const state = createToolSieveState();
        const events = [...processToolSieveChunk(state, raw.slice(0, split), []), ...processToolSieveChunk(state, raw.slice(split), []), ...flushToolSieve(state, [])];
        assert.deepEqual(events.filter(e => e.type === 'tool_calls').flatMap(e => e.calls), expected, `split=${split}`);
      }
    }
  }
});

test('adjacent CDATA sections round trip literal terminators and original whitespace', () => {
  for (const name of ['content', 'payload']) {
    for (const value of ['a]]>b', 'a\x00]]>b', ']]>]]>', '  原文\r\n\t]]> &amp; tail ']) {
      const encoded = `<![CDATA[${value.replaceAll(']]>', ']]]]><![CDATA[>')}]]>`;
      const raw = wrapper(`<parameter name="${name}">${encoded}</parameter>`);
      assert.deepEqual(parseToolCalls(raw, []), [{ name: 'write', input: { [name]: value } }]);
      for (let split = 0; split <= raw.length; split++) {
        const state = createToolSieveState();
        const events = [...processToolSieveChunk(state, raw.slice(0, split), []), ...processToolSieveChunk(state, raw.slice(split), []), ...flushToolSieve(state, [])];
        assert.deepEqual(events.filter(e => e.type === 'tool_calls').flatMap(e => e.calls), [{ name: 'write', input: { [name]: value } }]);
      }
    }
  }
});

test('JSON output constraints hold text until authoritative Go validation', () => {
  const policy = resolveToolcallPolicy({ validate_tools: true, validate_output: true, tool_choice: { Mode: 'auto' } }, []);
  assert.equal(policy.holdText, true);
  assert.equal(policy.constrained, true);
  assert.equal(policy.emitEarlyToolDeltas, false);
  assert.equal(resolveToolcallPolicy({ tool_choice: { Mode: 'auto' } }, []).holdText, false);
});
