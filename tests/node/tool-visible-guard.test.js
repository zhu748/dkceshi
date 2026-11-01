'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { VisibleCleaner } = require('../../internal/js/helpers/visible-cleaner');
const { createVisibleGuard } = require('../../internal/js/helpers/stream-tool-sieve/visible_guard');
const { needsToolFinalization } = require('../../internal/js/chat-stream/tool_output');
const { createToolSieveState, processToolSieveChunk, flushToolSieve } = require('../../internal/js/helpers/stream-tool-sieve');

test('display guard never leaks tool protocol across any chunk split', () => {
  const call = '<|EPSE|tool_calls><|EPSE|invoke name="run"><|EPSE|parameter name="cmd"><![CDATA[secret]]></|EPSE|parameter></|EPSE|invoke></|EPSE|tool_calls>';
  const samples = [
    ['before ' + call + ' after', 'before  after'],
    ['before <|EPSE|tool_calls>', 'before '],
    ['before <|epse|invoke name="run"', 'before '],
    ['before <｜ＥＰＳＥ｜call name="run">secret</｜ＥＰＳＥ｜call>', 'before '],
    ['before <｜EPSE｜tool_ca', 'before '],
    ['before ＜｜DSML｜ calls', 'before '],
    ['before <invoke name="run"', 'before '],
    ['1 < 2 and x > y; a trailing <', '1 < 2 and x > y; a trailing <'],
    ['`' + call + '`', '`' + call + '`'],
    ['```xml\n' + call + '\n```', '```xml\n' + call + '\n```'],
    ['~~~xml\n' + call + '\n~~~', '~~~xml\n' + call + '\n~~~'],
    ['{"example":' + JSON.stringify(call) + '}', '{"example":' + JSON.stringify(call) + '}'],
    ['<![CDATA[<|EPSE|tool_calls>]]>', '<![CDATA[<|EPSE|tool_calls>]]>'],
    ['<!-- <|EPSE|tool_calls> -->', '<!-- <|EPSE|tool_calls> -->'],
  ];
  for (const [raw, expected] of samples) {
    const chars = Array.from(raw);
    for (let split = 0; split <= chars.length; split += 1) {
      const guard = createVisibleGuard();
      const actual = guard.write(chars.slice(0, split).join('')) + guard.write(chars.slice(split).join('')) + guard.flush();
      assert.equal(actual, expected, `split=${split}, raw=${raw}`);
    }
    const guard = createVisibleGuard();
    assert.equal(chars.map(ch => guard.write(ch)).join('') + guard.flush(), expected, `one codepoint per chunk: ${raw}`);
  }
});

test('display guard handles caller markers without changing normal reasoning', () => {
  const raw = 'think <|Q7ZK3M|invoke name="run"><|Q7ZK3M|parameter name="cmd">secret</|Q7ZK3M|parameter></|Q7ZK3M|invoke> done';
  const guard = createVisibleGuard('Q7ZK3M');
  assert.equal(Array.from(raw, ch => guard.write(ch)).join('') + guard.flush(), 'think  done');
  const ordinary = createVisibleGuard();
  assert.equal(ordinary.write('normal reasoning') + ordinary.flush(), 'normal reasoning');
});

test('unrecognized branded markup still requires authoritative final validation', () => {
  for (const raw of ['<｜ｅｐｓｅ｜call name="run">secret', '＜｜EPSE｜tool_ca', '＜｜DSML｜ calls']) {
    assert.equal(needsToolFinalization(raw, '', {}, {}), true, raw);
  }
});

test('production body sieve keeps split literals before a real call', () => {
  const call = '<tool_calls><invoke name="run"></invoke></tool_calls>';
  for (const literal of [JSON.stringify(call), '<![CDATA[' + call + ']]>', '<!-- ' + call + ' -->', '`' + call + '`']) {
    const raw = literal + '\n' + call;
    for (let split = 0; split <= raw.length; split += 1) {
      const state = createToolSieveState();
      state.deferMalformed = true;
      const events = processToolSieveChunk(state, raw.slice(0, split), []).concat(processToolSieveChunk(state, raw.slice(split), []), flushToolSieve(state, []));
      assert.equal(events.map(event => event.text || '').join(''), literal + '\n', `split=${split}, raw=${raw}`);
      assert.equal(events.filter(event => event.type === 'tool_calls').length, 1, `split=${split}, raw=${raw}`);
    }
  }
});
test('display markers are filtered after classification and preserve literal examples', () => {
  for (const [raw,want] of [
    ['hello CONTENT_FILTERblocked','hello '],
    ['CONTENT_FILTER=enabled;','CONTENT_FILTER=enabled;'],
    ['mention CONTENT_FILTER in docs','mention CONTENT_FILTER in docs'],
    ['before [citation:7] after','before  after'],
    ['`[citation:7] CONTENT_FILTERblocked`','`[citation:7] CONTENT_FILTERblocked`'],
    ['"[citation:7] CONTENT_FILTERblocked"','"[citation:7] CONTENT_FILTERblocked"'],
  ]) {
    for (let split=0;split<=raw.length;split++) {
      const cleaner = new VisibleCleaner(true);
      assert.equal(cleaner.write(raw.slice(0,split))+cleaner.write(raw.slice(split))+cleaner.flush(),want,`split=${split} raw=${raw}`);
    }
  }
});
