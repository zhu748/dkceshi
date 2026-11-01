'use strict';

const { createToolSieveState } = require('./state');
const { processToolSieveChunk, flushToolSieve } = require('./sieve');
const { findToolProtocolPrefixOutsideIgnored } = require('./protocol_prefix');
const { findToolMarkupTagOutsideIgnored } = require('./markup_scan');
const { normalizeFullwidthASCIIChar, removeToolMarkupIgnorables } = require('./markup_syntax');
const { createPromptEchoGuard, feedPromptEchoGuard, flushPromptEchoGuard } = require('../../chat-stream/prompt_echo');
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

function createVisibleGuard(marker, stripReferences = false, prompt = '') {
  const state = createToolSieveState();
  state.deferMalformed = true;
  state.cleanVisible = true;
  state.stripReferences = stripReferences;
  state.displayOnly = true;
  const normalizer = createMarkerNormalizer(marker);
  const echo = createPromptEchoGuard(prompt);
  return {
    write: text => visibleGuardText(processToolSieveChunk(state, normalizer.write(feedPromptEchoGuard(echo, text)), [])),
    get echoed() { return echo.tripped === true; },
    flush: () => visibleGuardText(processToolSieveChunk(state, normalizer.write(flushPromptEchoGuard(echo)) + normalizer.flush(), []).concat(flushToolSieve(state, []))),
  };
}

function filterVisible(text, stripReferences = false) {
  const guard = createVisibleGuard(undefined, stripReferences);
  return guard.write(text) + guard.flush();
}

module.exports = { createVisibleGuard, filterVisible };
