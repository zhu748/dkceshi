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
