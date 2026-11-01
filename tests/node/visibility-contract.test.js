'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fixtures = require('../compat/fixtures/visibility/stream.json');
const { createToolSieveState, processToolSieveChunk, flushToolSieve } = require('../../internal/js/helpers/stream-tool-sieve');
const { createMarkerNormalizer } = require('../../internal/js/chat-stream/marker');
const { filterVisible } = require('../../internal/js/helpers/stream-tool-sieve/visible_guard');
const { VisibleCleaner, cleanVisible } = require('../../internal/js/helpers/visible-cleaner');

for (const fixture of fixtures) test('shared visibility: ' + fixture.name, () => {
  function check(chunks) {
    const state = createToolSieveState(); state.deferMalformed = state.cleanVisible = true;
    const marker = createMarkerNormalizer(fixture.marker), events = [];
    for (const chunk of chunks) events.push(...processToolSieveChunk(state, marker.write(chunk), ['read']));
    events.push(...processToolSieveChunk(state, marker.flush(), ['read']), ...flushToolSieve(state, ['read']));
    let visible = events.filter(e => !e.deferred).map(e => e.text || '').join('');
    if (!fixture.invalid && !fixture.calls.length) visible += filterVisible(events.filter(e => e.deferred).map(e => e.text || '').join(''));
    assert.equal(visible, fixture.visible, JSON.stringify(chunks));
    assert.deepEqual(events.flatMap(e => e.calls || []), fixture.calls);
  }
  for (let split = 0; split <= fixture.raw.length; split++) check([fixture.raw.slice(0, split), fixture.raw.slice(split)]);
  for (let size = 1; size <= fixture.raw.length; size++) {
    const chunks = [];
    for (let i = 0; i < fixture.raw.length; i += size) chunks.push(fixture.raw.slice(i, i + size));
    check(chunks);
  }
});

test('display cleanup is partition invariant and preserves literals', () => {
  const wire = '[{"function":{"name":"read","arguments":"{}"},"id":"call_demo","type":"function"}]';
  for (const [raw, expected] of [
    ['```json\n' + wire + '\n```', '```json\n' + wire + '\n```'],
    ['before' + wire + 'after', 'beforeafter'],
    ['< | Tool | >{"tool_call_id":"call_demo","content":"result"}', ''],
    ['<attempt_completion><result>answer</result></attempt_completion>', 'answer'],
    ['answer</result></attempt_completion>', 'answer'],
    ['A<think>B</think>C', 'ABC'], ['A<think>hidden', 'A'],
    ['<think>İ你好</think>after', 'İ你好after'],
    ['<think>outer<think>inner</think>tail</think>after', 'outerinnertailafter'],
    ['<think>"literal </think>" hidden', ''],
    ['<attempt_completion><result>Example: `</attempt_completion>` end</result></attempt_completion>', 'Example: `</attempt_completion>` end'],
    ['<think>before\n~~~xml\n</think>\n~~~\nafter</think>end', 'before\n~~~xml\n</think>\n~~~\nafterend'],
    ['<think>A<!-- </think> -->B<![CDATA[</think>]]>C</think>end', 'A<!-- </think> -->B<![CDATA[</think>]]>Cend'],
    ['``example ` <think> text``', '``example ` <think> text``'],
    ['<![CDATA[<|assistant|>]]>', '<![CDATA[<|assistant|>]]>'],
  ]) {
    assert.equal(cleanVisible(raw), expected);
    for (let size = 1; size <= raw.length; size++) {
      const cleaner = new VisibleCleaner(); let text = '';
      for (let i = 0; i < raw.length; i += size) text += cleaner.write(raw.slice(i, i + size));
      text += cleaner.flush();
      assert.equal(text, expected, `size=${size} raw=${raw}`);
    }
  }
  assert.equal(new VisibleCleaner().write('```xml\n<tool_calls>example'), '```xml\n<tool_calls>example');
});

test('large pending units preserve cleanup across scan boundaries', () => {
  const payload = 'a\\\"b'.repeat(4096);
  const wire = '[{"function":{"name":"read","arguments":"' + payload + '"},"id":"call_demo","type":"function"}]';
  const quote = '"' + payload + '<think>"';
  const samples = [
    [wire, ''], [wire.slice(0, -1), wire.slice(0, -1)],
    ['[{"example":' + quote + '}]', '[{"example":' + quote + '}]'],
    ['<|tool|>{"tool_call_id":"call_demo","content":' + quote + '}', ''],
    [quote, quote], ['`' + payload + '<think>`', '`' + payload + '<think>`'],
    ['<!--' + payload + '<think>-->', '<!--' + payload + '<think>-->'],
    ['<![CDATA[' + payload + '<think>]]>', '<![CDATA[' + payload + '<think>]]>'],
    ['<think>' + quote + '</think>', quote], ['<think>' + quote, ''],
  ];
  for (const [raw, expected] of samples) {
    assert.equal(cleanVisible(raw), expected);
    for (const size of [1, 7, 16, 4095, 4096, 4097, 8192, raw.length]) {
      const cleaner = new VisibleCleaner(); let text = '';
      for (let i = 0; i < raw.length; i += size) text += cleaner.write(raw.slice(i, i + size));
      text += cleaner.flush();
      assert.equal(text, expected, `size=${size} prefix=${raw.slice(0, 40)}`);
    }
  }
});
