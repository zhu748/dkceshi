'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { parseToolCalls, createToolSieveState, processToolSieveChunk, flushToolSieve } = require('../../internal/js/helpers/stream-tool-sieve');
const { resolveToolcallPolicy } = require('../../internal/js/chat-stream/toolcall_policy');

test('parameter names preserve whitespace, empty keys and XML entities', () => {
  for (const [encoded, key] of [[' key ', ' key '], ['', ''], ['a&amp;b', 'a&b']]) {
    const raw = wrapper(`<parameter name="${encoded}"><![CDATA[exact]]></parameter>`);
    assert.deepEqual(parseToolCalls(raw, []), [{ name: 'write', input: { [key]: 'exact' } }]);
  }
});

test('JSON Unicode checks reject unpaired escapes but preserve literal examples', () => {
  const { parseJSONStrict } = require('../../internal/js/helpers/stream-tool-sieve/strict_json');
  for (const raw of ['"\\ud800"', '"\\udc00"', '{"\\udfff":1}']) {
    assert.throws(() => parseJSONStrict(raw), /Unicode/);
    assert.deepEqual(parseToolCalls(wrapper(`<parameter name="value">${raw}</parameter>`), []), []);
    assert.deepEqual(parseToolCalls(wrapper(`<parameter name="value"><![CDATA[${raw}]]></parameter>`), []), [{ name: 'write', input: { value: raw } }]);
  }
  assert.equal(parseJSONStrict('"\\ud83d\\ude00"'), '😀');
  assert.equal(parseJSONStrict('"\\\\ud800"'), '\\ud800');
});

test('large unclosed markup is bounded without interpreting CDATA text', () => {
  const { markupLimitExceeded } = require('../../internal/js/helpers/stream-tool-sieve/markup_budget');
  const body = '<invoke name="literal">'.repeat(6400);
  const broken = `<tool_calls>${body}</tool_calls>`;
  assert.equal(markupLimitExceeded(broken), true);
  assert.deepEqual(parseToolCalls(broken, []), []);
  const literal = wrapper(`<parameter name="content"><![CDATA[${body}]]></parameter>`);
  assert.equal(markupLimitExceeded(literal), false);
  assert.deepEqual(parseToolCalls(literal, []), [{ name: 'write', input: { content: body } }]);
});

function wrapper(body) {
  return `<tool_calls><invoke name="write">${body}</invoke></tool_calls>`;
}

test('nested XML depth is bounded while deep CDATA text remains exact', () => {
  for (const name of ['node', '中文', 'ns:node']) {
    const body = `<${name}>`.repeat(150) + 'value' + `</${name}>`.repeat(150);
    assert.deepEqual(parseToolCalls(wrapper(`<parameter name="payload">${body}</parameter>`), []), []);
    assert.deepEqual(parseToolCalls(wrapper(`<parameter name="payload"><![CDATA[${body}]]></parameter>`), []), [{ name: 'write', input: { payload: body } }]);
  }
});

test('closed CDATA cannot create parameters or sibling calls at any split', () => {
  for (const value of [
    'prefix </parameter><parameter name="description"><![CDATA[example',
    'prefix </parameter></invoke><invoke name="remove"><parameter name="path"><![CDATA[/tmp/example',
  ]) {
    const raw = wrapper(`<parameter name="content"><![CDATA[${value}]]></parameter>`);
    const expected = [{ name: 'write', input: { content: value } }];
    assert.deepEqual(parseToolCalls(raw, []), expected);
    for (let split = 0; split <= raw.length; split++) {
      const state = createToolSieveState();
      const events = [...processToolSieveChunk(state, raw.slice(0, split), []), ...processToolSieveChunk(state, raw.slice(split), []), ...flushToolSieve(state, [])];
      assert.deepEqual(events.filter(e => e.type === 'tool_calls').flatMap(e => e.calls), expected, `split=${split}`);
    }
  }
});

test('duplicate JSON keys stay non-executable, including escaped aliases', () => {
  for (const body of ['{"path":"a","path":"b"}', '{"nested":[{"path":"a","\\u0070ath":"b"}]}']) {
    const raw = wrapper(`<parameter name="content">${body}</parameter>`);
    assert.deepEqual(parseToolCalls(raw, []), []);
    for (let split = 0; split <= raw.length; split++) {
      const state = createToolSieveState();
      const events = [...processToolSieveChunk(state, raw.slice(0, split), []), ...processToolSieveChunk(state, raw.slice(split), []), ...flushToolSieve(state, [])];
      assert.equal(events.filter(e => e.type === 'tool_calls').flatMap(e => e.calls).length, 0);
    }
  }
});

for (const entry of require('../fixtures/tool-context-fidelity.json')) {
  test(`shared context integrity: ${entry.name}`, () => {
    const raw = wrapper(`<parameter name="${entry.parameter}">${entry.body}</parameter>`);
    const expected = [{ name: 'write', input: { [entry.parameter]: entry.expected } }];
    assert.deepEqual(parseToolCalls(raw, []), expected);
    for (let split = 0; split <= raw.length; split++) {
      const state = createToolSieveState();
      const events = [...processToolSieveChunk(state, raw.slice(0, split), []), ...processToolSieveChunk(state, raw.slice(split), []), ...flushToolSieve(state, [])];
      assert.deepEqual(events.filter(e => e.type === 'tool_calls').flatMap(e => e.calls), expected, `split=${split}`);
    }
  });
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

test('duplicate tool and parameter identities remain non-executable at every split', () => {
  for (const raw of [
    '<tool_calls><invoke name="read" name="delete"></invoke></tool_calls>',
    wrapper('<parameter name="path" name="other">value</parameter>'),
  ]) {
    assert.deepEqual(parseToolCalls(raw, []), []);
    for (let split = 0; split <= raw.length; split++) {
      const state = createToolSieveState();
      const events = [...processToolSieveChunk(state, raw.slice(0, split), []), ...processToolSieveChunk(state, raw.slice(split), []), ...flushToolSieve(state, [])];
      assert.equal(events.filter(e => e.type === 'tool_calls').flatMap(e => e.calls).length, 0);
    }
  }
});


test('mixed XML retains source instead of dropping text beside fields', () => {
  for (const body of ['<_text>ORIGINAL</_text>TAIL', 'HEAD<_text><![CDATA[ORIGINAL]]></_text>TAIL']) {
    const calls = parseToolCalls(wrapper(`<parameter name="payload">${body}</parameter>`), []);
    assert.equal(calls.length, 1);
    assert.equal(calls[0].input.payload, body);
  }
});
