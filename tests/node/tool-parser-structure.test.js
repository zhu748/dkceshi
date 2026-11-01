const assert = require('node:assert/strict');
const path = require('node:path');
const test = require('node:test');

for (const [label, modulePath] of [
  ['tool parser', '../../internal/js/helpers/stream-tool-sieve/parse_payload'],
  ['SSE parser', '../../internal/js/chat-stream/sse_parse_impl'],
  ['Vercel stream', '../../internal/js/chat-stream/vercel_stream_impl'],
]) {
  const entry = require.resolve(modulePath);
  require(entry);
  test(`${label} modules load without circular dependencies`, () => {
    const directory = path.dirname(entry) + path.sep;
    const complete = new Set();
    const active = new Set();
    function visit(module, trail) {
      if (!module.filename.startsWith(directory)) return;
      assert.ok(!active.has(module.id), `circular parser import: ${[...trail, path.basename(module.id)].join(' -> ')}`);
      if (complete.has(module.id)) return;
      active.add(module.id);
      for (const child of module.children) visit(child, [...trail, path.basename(module.id)]);
      active.delete(module.id);
      complete.add(module.id);
    }
    visit(require.cache[entry], []);
    assert.ok(complete.size > 1, 'check the parser implementation as well as its facade');
  });
}

const { parseToolCallsDetailed } = require('../../internal/js/helpers/stream-tool-sieve/parse');
test('tool parameters decode exactly one entity layer and preserve valid JSON', () => {
  for (const [encoded, expected] of [['&amp;amp;', '&amp;'], ['&amp;lt;', '&lt;'], ['&#65;&#x1F600;', 'A😀'], ['&apos;&quot;', "'\""]]) {
    const raw = `<tool_calls><invoke name="write"><parameter name="content">${encoded}</parameter></invoke></tool_calls>`;
    assert.equal(parseToolCallsDetailed(raw, []).calls[0].input.content, expected);
  }
  const raw = '<tool_calls><invoke name="write"><parameter name="options">{"file_path":"first\\nsecond","literal":"&amp;","sentinel":"<noop>"}</parameter></invoke></tool_calls>';
  assert.deepEqual(parseToolCallsDetailed(raw, []).calls[0].input.options, { file_path: 'first\nsecond', literal: '&amp;', sentinel: '<noop>' });
});

const { parseChunkForContent } = require('../../internal/js/chat-stream/sse_parse');
test('terminal status batches retain their last content', () => {
  for (const status of ['status', 'quasi_status']) {
    const parsed = parseChunkForContent({p:'response',v:[{p:'content',v:'final tail'},{p:status,v:'FINISHED'}]}, false, 'text');
    assert.equal(parsed.finished, true);
    assert.equal(parsed.parts.map(part => part.text).join(''), 'final tail');
  }
});

test('nested XML parameter entities are decoded only by their leaf parser', () => {
  const raw = '<tool_calls><invoke name="write"><parameter name="options"><label>&amp;amp;</label></parameter></invoke></tool_calls>';
  assert.deepEqual(parseToolCallsDetailed(raw, []).calls[0].input.options, { label:'&amp;' });
});

test('only response snapshots carry a response-level terminal status', () => {
  const snapshot = { status:'FINISHED', fragments:[{type:'RESPONSE',content:'snapshot'}] };
  assert.equal(parseChunkForContent({v:snapshot}, false, 'text').finished, true);
  assert.equal(parseChunkForContent({p:'response/fragments/-1',v:{status:'FINISHED',text:'fragment'}}, false, 'text').finished, false);
});
