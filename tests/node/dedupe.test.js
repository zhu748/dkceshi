'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { trimContinuationOverlap } = require('../../internal/js/chat-stream/dedupe');

test('continuation snapshots count Unicode code points and preserve short repeated deltas', () => {
  for (const char of ['a', '字', '😀']) {
    for (const size of [31, 32, 33]) {
      const incoming = char.repeat(size);
      for (const suffix of ['', 'more']) {
        assert.equal(trimContinuationOverlap(incoming + suffix, incoming), size >= 32 ? '' : incoming,
          `${char}/${size}/${suffix}`);
      }
    }
  }
});

test('continuation keeps independent deltas and strips only a replayed prefix', () => {
  const prefix = '😀'.repeat(32);
  assert.equal(trimContinuationOverlap(prefix, prefix + ' next'), ' next');
  assert.equal(trimContinuationOverlap(prefix, 'another '.repeat(32)), 'another '.repeat(32));
  assert.equal(trimContinuationOverlap('', prefix), prefix);
  assert.equal(trimContinuationOverlap(prefix, ''), '');
});
