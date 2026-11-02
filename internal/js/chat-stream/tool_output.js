'use strict';

const { findToolMarkupTagOutsideIgnored, findDSMLTagOutsideIgnored } = require('../helpers/stream-tool-sieve/parse_payload');
const { findToolProtocolPrefixOutsideIgnored } = require('../helpers/stream-tool-sieve/protocol_prefix');
const { createToolSieveState, processToolSieveChunk, flushToolSieve } = require('../helpers/stream-tool-sieve');
const { filterVisible } = require('../helpers/stream-tool-sieve/visible_guard');

function needsToolFinalization(text, thinking, prepared, state) {
  if (prepared.validate_tools === true || state.capture || state.pending) return true;
  for (const raw of [text, thinking]) {
    if (raw && (findToolProtocolPrefixOutsideIgnored(raw) || findToolMarkupTagOutsideIgnored(raw, 0) || findDSMLTagOutsideIgnored(raw, 0))) return true;
  }
  return false;
}

// Prefer Go's validated content. This fallback supports older validation
// responses while preserving the same prefix-only stream rendering contract.
function validatedToolText(text, toolNames, hasCalls) {
  if (!hasCalls) return filterVisible(text);
  const state = createToolSieveState();
  state.cleanVisible = state.deferMalformed = true;
  const events = processToolSieveChunk(state, text, toolNames).concat(flushToolSieve(state, toolNames));
  let result = '';
  for (const event of events) {
    if (event.type === 'tool_calls' || event.type === 'tool_call_deltas') break;
    result += event.text || '';
  }
  return filterVisible(result);
}

module.exports = { needsToolFinalization, validatedToolText };
