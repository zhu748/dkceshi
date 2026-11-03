'use strict';

// Optional module path supports comparing a saved previous implementation.
// node tests/tools/bench-pending-literal.cjs [path/to/pending_literal.cjs]
const assert = require('node:assert/strict');
const { performance } = require('node:perf_hooks');
const { resolve } = require('node:path');
const target = process.argv[2]
  ? resolve(process.argv[2])
  : resolve(__dirname, '../../internal/js/helpers/stream-tool-sieve/pending_literal.js');
const { pendingLiteral } = require(target);
const body = 'plain body '.repeat(6000);

for (const open of ['<!--', '<![CDATA[']) {
  const run = () => {
    const progress = pendingLiteral(open);
    for (let i = 0; i < body.length; i += 128) {
      assert.equal(progress.feed(body.slice(i, i + 128)), false);
    }
    assert.equal(progress.feed(open === '<!--' ? '-->' : ']]＞'), true);
  };
  for (let i = 0; i < 5; i++) run();
  const times = [];
  for (let repeat = 0; repeat < 7; repeat++) {
    const start = performance.now();
    for (let i = 0; i < 20; i++) run();
    times.push((performance.now() - start) / 20);
  }
  times.sort((a, b) => a - b);
  console.log(JSON.stringify({ fixture: open, bytes: body.length, chunk: 128, runs: times.length,
    median_ms: times[3], min_ms: times[0], max_ms: times.at(-1) }));
}
