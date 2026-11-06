'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { findToolProtocolPrefixOutsideIgnored } = require('../../internal/js/helpers/stream-tool-sieve/protocol_prefix');
const { findToolMarkupTagOutsideIgnored } = require('../../internal/js/helpers/stream-tool-sieve/parse_payload');
const { createToolSieveState, processToolSieveChunk, flushToolSieve } = require('../../internal/js/helpers/stream-tool-sieve');

test('delimiter runs keep ignored literals and earliest executable boundaries', () => {
  for (const prefix of ['<'.repeat(512), '＜<'.repeat(256)]) {
    assert.equal(findToolProtocolPrefixOutsideIgnored(prefix + '>'), null);
    assert.equal(findToolProtocolPrefixOutsideIgnored(prefix + '|EPSE|invoke name="real">').start, 0);
    for (const body of ['![CDATA[<|EPSE|invoke name="literal">]]>', '!-- <|EPSE|invoke name="literal"> -->']) {
      const raw = prefix + body + ' <|EPSE|invoke name="real">';
      const expected = raw.lastIndexOf('<|EPSE|');
      assert.equal(findToolProtocolPrefixOutsideIgnored(raw).start, expected);
      assert.equal(findToolMarkupTagOutsideIgnored(raw, 0).start, expected);
    }
  }
});

test('large ordinary delimiter runs pass through the entire streaming guard', () => {
  const raw = '<'.repeat(131072) + '>';
  const state = createToolSieveState();
  state.deferMalformed = state.cleanVisible = true;
  const events = [...processToolSieveChunk(state, raw, []), ...flushToolSieve(state, [])];
  assert.equal(events.map(event => event.text || '').join(''), raw);
  assert.equal(events.flatMap(event => event.calls || []).length, 0);
});
