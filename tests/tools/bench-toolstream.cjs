'use strict';

// An optional checkout path allows identical before/after comparisons.
const { resolve } = require('node:path');
const { performance } = require('node:perf_hooks');
const assert = require('node:assert/strict');
const root = resolve(process.argv[2] || resolve(__dirname, '../..'));
const { createToolSieveState, processToolSieveChunk, flushToolSieve } = require(resolve(root, 'internal/js/helpers/stream-tool-sieve'));

function measure(count) {
  const raw = '<tool_calls>' + Array.from({ length: count }, (_, i) =>
    '<invoke name="write"><parameter name="content"><![CDATA[' + 'x'.repeat(1024) +
    ']]></parameter><parameter name="path"><![CDATA[f' + i + ']]></parameter></invoke>').join('') + '</tool_calls>';
  const state = createToolSieveState(); state.deferMalformed = state.cleanVisible = true;
  let calls = 0;
  const start = performance.now();
  for (let i = 0; i < raw.length; i += 256) {
    for (const event of processToolSieveChunk(state, raw.slice(i, i + 256), ['write'])) calls += event.calls?.length || 0;
  }
  for (const event of flushToolSieve(state, ['write'])) calls += event.calls?.length || 0;
  const ms = performance.now() - start;
  assert.equal(calls, count, 'benchmark must preserve every invocation');
  return { count, bytes: Buffer.byteLength(raw), chunk_size: 256, ms };
}

measure(4);
console.log(JSON.stringify({ node: process.version, root, iterations: 5 }));
for (const count of [8, 16, 32, 64]) {
  const runs = Array.from({ length: 5 }, () => measure(count)).sort((a, b) => a.ms - b.ms);
  console.log(JSON.stringify({ ...runs[2], ms: Math.round(runs[2].ms * 10) / 10 }));
}
