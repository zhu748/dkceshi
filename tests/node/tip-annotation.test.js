'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { parseChunkForContent } = require('../../internal/js/chat-stream/sse_parse_impl');

test('HAR TIP annotation stays out of content and reasoning with thinking on or off', () => {
  // Share the reduced HAR sequence with the Go collector/pump regression.
  const raw = fs.readFileSync(path.join(__dirname, '../fixtures/ai_tip.sse'), 'utf8');
  for (const thinking of [false, true]) {
    const state = {};
    let type = thinking ? 'thinking' : 'text';
    let body = '';
    let stopped = false;
    for (const line of raw.trim().split('\n')) {
      const result = parseChunkForContent(JSON.parse(line.slice(6)), thinking, type, false, state);
      type = result.newType;
      assert.equal(result.errorMessage, '');
      assert.equal(result.detectionThinking || '', '');
      assert.ok(result.parts.every(part => part.type === 'text'));
      body += result.parts.map(part => part.text).join('');
      if (result.finished) { stopped = true; break; }
    }
    assert.equal(body, '正文继续');
    assert.equal(stopped, true);
  }
});

test('TIP metadata does not become the source of a later answer snapshot', () => {
  const state = {};
  const tip = { id: 4, type: 'TIP', content: '独立标注' };
  const chunks = [
    { p: 'response/content', v: '正文' },
    { p: 'response/fragments', v: [tip] },
    { v: { response: { fragments: [{ id: 3, type: 'RESPONSE', content: '正文继续' }, tip], status: 'FINISHED' } } },
  ];
  let body = '';
  for (const chunk of chunks) {
    const result = parseChunkForContent(chunk, false, 'text', false, state);
    assert.equal(result.errorMessage, '');
    body += result.parts.map(part => part.text).join('');
  }
  assert.equal(body, '正文继续');
});
