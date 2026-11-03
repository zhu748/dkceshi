'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { pendingLiteral } = require('../../internal/js/helpers/stream-tool-sieve/pending_literal');

test('held comments and CDATA recognize every split closing delimiter', () => {
  for (const [open, close] of [['<!--', '-->'], ...[']]>', ']]＞', ']]﹥', ']]〉'].map(close => ['<![CDATA[', close])]) {
    for (let split = 0; split <= close.length; split++) {
      const progress = pendingLiteral(open + 'body');
      assert.ok(progress);
      assert.equal(progress.feed('ordinary ]-＞ bytes'), false);
      const first = progress.feed(close.slice(0, split));
      assert.equal(first, split === close.length, `${close}/${split}`);
      if (!first) assert.equal(progress.feed(close.slice(split) + 'after'), true, `${close}/${split}`);
    }
  }
});

test('held delimiter scan handles long bodies, empty chunks and false prefixes', () => {
  for (const open of ['<!--', '<![CDATA[']) {
    const progress = pendingLiteral(open);
    for (const chunk of ['x'.repeat(65536), ']', '-', ']', '＞', '--', 'x>', ']]', 'x>', '']) {
      assert.equal(progress.feed(chunk), false);
    }
    const close = open === '<!--' ? '-->' : ']]>';
    for (let i = 0; i < close.length; i++) {
      assert.equal(progress.feed(close[i]), i === close.length - 1);
    }
  }
});

test('quoted literal scan retains escapes and JSON control boundaries', () => {
  const progress = pendingLiteral('"ordinary');
  assert.equal(progress.feed('\\'), false);
  assert.equal(progress.feed('"escaped'), false);
  assert.equal(progress.feed('"after'), true);
  assert.equal(pendingLiteral('"body').feed('\n'), true);
  assert.equal(pendingLiteral('<!--closed-->'), null);
  assert.equal(pendingLiteral('<![CDATA[closed]]＞'), null);
});
