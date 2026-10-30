'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { createContentPolicy, feedContentPolicy, contentPolicyCut } = require('../../internal/js/chat-stream/content_policy');
const { createToolSieveState, processToolSieveChunk, flushToolSieve, parseToolCalls } = require('../../internal/js/helpers/stream-tool-sieve');
const { findToolMarkupTagOutsideIgnored, findMatchingToolMarkupClose } = require('../../internal/js/helpers/stream-tool-sieve/parse_payload');

const boundary = '[EndMessage:3237101fbf162c60]';
const call = '<|EPSE|tool_calls><|EPSE|invoke name="read"><|EPSE|parameter name="file_path"><![CDATA[a.txt]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>';

function sieve(input, size) {
  const state = createToolSieveState();
  state.deferMalformed = true;
  const events = [];
  for (let i = 0; i < input.length; i += size) events.push(...processToolSieveChunk(state, input.slice(i, i + size), ['read']));
  events.push(...flushToolSieve(state, ['read']));
  return { text: events.map(e => e.text || '').join(''), calls: events.flatMap(e => e.calls || []) };
}

test('transcript boundary cuts fabricated continuation across every chunk size', () => {
  const input = 'done\n' + boundary + '\n[Assistant]:' + call;
  for (let size = 1; size <= input.length; size++) {
    const policy = createContentPolicy(); let out = '';
    for (let i = 0; i < input.length; i += size) out += feedContentPolicy(policy, input.slice(i, i + size));
    assert.equal(out, 'done\n', `size=${size}`);
    assert.equal(contentPolicyCut(policy), 'transcript_boundary');
  }
});

test('scanner coordinates and streaming arguments preserve leading whitespace', () => {
  const input = '\n  ' + call + '\n';
  const open = findToolMarkupTagOutsideIgnored(input, 0);
  const close = findMatchingToolMarkupClose(input, open);
  assert.equal(input.slice(open.start, open.end + 1), '<|EPSE|tool_calls>');
  assert.equal(input.slice(close.start, close.end + 1), '</|EPSE|tool_calls>');
  const formatted = call.replaceAll('><|', '>\n  <|').replace('a.txt', '  a.txt\n');
  for (let size = 1; size <= formatted.length; size++) {
    const result = sieve('\n' + formatted + '\n', size);
    assert.equal(result.text, '\n\n');
    assert.deepEqual(result.calls, [{ name: 'read', input: { file_path: '  a.txt\n' } }]);
  }
});

test('boundary literals in tool arguments, quotes and Markdown survive', () => {
  for (const literal of ['<![CDATA[' + boundary + ']]>', '"' + boundary + '"', '`' + boundary + '`', '```txt\n' + boundary + '\n```\n', '~~~txt\n' + boundary + '\n~~~\n']) {
    const input = literal + '\n' + boundary + '\nfabricated';
    for (let size = 1; size <= input.length; size++) {
      const policy = createContentPolicy(); let out = '';
      for (let i = 0; i < input.length; i += size) out += feedContentPolicy(policy, input.slice(i, i + size));
      assert.equal(out, literal + '\n', `size=${size} literal=${literal}`);
    }
  }
});

test('ordinary bracketed text and malformed boundaries are preserved', () => {
  for (const text of ['[EndMessage:short]', '[EndMessage:3237101fbf162c6g]', '[EndMessage:3237101fbf162c600]', '[EndMessage is documentation]']) {
    const input = text + '\nend';
    for (let size = 1; size <= input.length; size++) {
      const policy = createContentPolicy(); let out = '';
      for (let i = 0; i < input.length; i += size) out += feedContentPolicy(policy, input.slice(i, i + size));
      assert.equal(out, input);
      assert.equal(contentPolicyCut(policy), '');
    }
  }
});

test('orphan tool closing shells never leak as streaming content', () => {
  const shells = '</|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>';
  const input = 'before\n' + shells + '\nafter';
  for (let size = 1; size <= input.length; size++) {
    const result = sieve(input, size);
    assert.equal(result.text, 'before\n\nafter', `size=${size}`);
    assert.equal(result.calls.length, 0);
  }
});

test('extra closing shell after a real call neither leaks nor duplicates the call', () => {
  const input = call + '</|EPSE|tool_calls>\ndone';
  for (let size = 1; size <= input.length; size++) {
    const result = sieve(input, size);
    assert.equal(result.text, '\ndone', `size=${size}`);
    assert.deepEqual(result.calls, [{ name: 'read', input: { file_path: 'a.txt' } }]);
  }
});

test('closing shells in Markdown and CDATA values are preserved', () => {
  const shell = '</|EPSE|tool_calls>';
  for (const literal of ['`' + shell + '`', '```xml\n' + shell + '\n```\n']) {
    for (let size = 1; size <= literal.length; size++) assert.equal(sieve(literal, size).text, literal);
  }
  const input = call.replace('a.txt', shell);
  for (let size = 1; size <= input.length; size++) {
    const result = sieve(input, size);
    assert.equal(result.text, '');
    assert.equal(result.calls[0].input.file_path, shell);
  }
});

test('orphan DSML closing tags preserve the following prose and tool call', () => {
  for (const shell of ['</｜DSML｜calls>', '</｜｜DSML｜｜invoke>']) {
    const input = 'before\n' + shell + '\nafter';
    for (let size = 1; size <= input.length; size++) assert.equal(sieve(input, size).text, 'before\n\nafter');
    for (let size = 1; size <= call.length; size++) {
      assert.deepEqual(sieve(shell + call, size).calls, [{ name: 'read', input: { file_path: 'a.txt' } }]);
    }
  }
});

test('a damaged optional parameter cannot silently disappear from an executable call', () => {
  for (const broken of ['<parameter name="optional">lost', '<parameter>lost</parameter>', '<parameter name="optional"']) {
    const input = '<tool_calls><invoke name="read"><parameter name="file_path">a.txt</parameter>' + broken + '</invoke></tool_calls>';
    assert.deepEqual(parseToolCalls(input, ['read']), []);
    for (let size = 1; size <= input.length; size++) assert.deepEqual(sieve(input, size).calls, []);
  }
});

test('invalid JSON argument envelopes cannot silently become empty calls', () => {
  for (const body of ['{"path":"a.txt"}', '{"input":"a.txt"}', '{"input":null}', '{"parameters":[]}']) {
    assert.deepEqual(parseToolCalls('<tool_calls><invoke name="read">' + body + '</invoke></tool_calls>', ['read']), []);
  }
  assert.deepEqual(parseToolCalls('<tool_calls><invoke name="read">{"input":{},"parameters":{"ignored":true}}</invoke></tool_calls>', ['read']), [{ name: 'read', input: {} }]);
});

test('tool markup within a JSON string remains a literal argument across chunks', () => {
  const literal = '<|EPSE|invoke name="fake"></|EPSE|tool_calls>';
  const input = '<tool_calls><invoke name="read">' + JSON.stringify({ input: { text: literal } }) + '</invoke></tool_calls>';
  const expected = [{ name: 'read', input: { text: literal } }];
  assert.deepEqual(parseToolCalls(input, ['read']), expected);
  for (let size = 1; size <= input.length; size++) {
    const result = sieve(input, size);
    assert.deepEqual(result.calls, expected);
    assert.equal(result.text, '');
  }
});
