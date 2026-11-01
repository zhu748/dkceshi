'use strict';

const { createToolSieveState } = require('./state');
const { processToolSieveChunk, flushToolSieve } = require('./sieve');
const { findToolProtocolPrefixOutsideIgnored } = require('./protocol_prefix');
const { findToolMarkupTagOutsideIgnored } = require('./markup_scan');
const { normalizeFullwidthASCIIChar, removeToolMarkupIgnorables } = require('./markup_syntax');
const { createMarkerNormalizer } = require('../../chat-stream/marker');

function visibleGuardText(events) {
  let text = '';
  for (const event of events) {
    if (event.type !== 'text' || !event.text) continue;
    if (event.deferred) {
      const normalized = Array.from(removeToolMarkupIgnorables(event.text), normalizeFullwidthASCIIChar).join('');
      const candidate = findToolProtocolPrefixOutsideIgnored(event.text);
      if ((candidate && candidate.protocol !== 'envelope_partial') || findToolMarkupTagOutsideIgnored(event.text, 0) || /<\/?(?:tool_calls|invoke|parameter)(?:\s|$)/i.test(normalized)) continue;
    }
    text += event.text;
  }
  return text;
}

function createVisibleGuard(marker) {
  const state = createToolSieveState();
  state.deferMalformed = true;
  state.cleanVisible = true;
  state.displayOnly = true;
  const normalizer = createMarkerNormalizer(marker);
  return {
    write: text => visibleGuardText(processToolSieveChunk(state, normalizer.write(text), [])),
    flush: () => visibleGuardText(processToolSieveChunk(state, normalizer.flush(), []).concat(flushToolSieve(state, []))),
  };
}

function filterVisible(text) {
  const guard = createVisibleGuard();
  return guard.write(text) + guard.flush();
}

module.exports = { createVisibleGuard, filterVisible };
