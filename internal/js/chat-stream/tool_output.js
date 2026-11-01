'use strict';

const { findToolMarkupTagOutsideIgnored, findDSMLTagOutsideIgnored } = require('../helpers/stream-tool-sieve/parse_payload');
const { findToolProtocolPrefixOutsideIgnored } = require('../helpers/stream-tool-sieve/protocol_prefix');

function needsToolFinalization(text, thinking, prepared, state) {
  if (prepared.validate_tools === true || state.capture || state.pending) return true;
  for (const raw of [text, thinking]) {
    if (raw && (findToolProtocolPrefixOutsideIgnored(raw) || findToolMarkupTagOutsideIgnored(raw, 0) || findDSMLTagOutsideIgnored(raw, 0))) return true;
  }
  return false;
}

module.exports = { needsToolFinalization };
