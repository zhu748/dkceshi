'use strict';

const { findToolMarkupTagOutsideIgnored, findDSMLTagOutsideIgnored } = require('../helpers/stream-tool-sieve/parse_payload');
const { findToolProtocolPrefixOutsideIgnored } = require('../helpers/stream-tool-sieve/protocol_prefix');
const { textBeforeToolBoundary } = require('../helpers/stream-tool-sieve/sieve');
const { filterVisible } = require('../helpers/stream-tool-sieve/visible_guard');

function needsToolFinalization(text, thinking, prepared, state) {
  // New prepares resolve this policy in Go. Retain old prepares' format and
  // required-tool contracts; a catalog alone never requires prose validation.
  const mode = prepared.tool_choice?.Mode;
  const validateProse = prepared.validate_prose ?? (prepared.validate_output === true || mode === 'required' || mode === 'forced');
  if (validateProse || state.capture || state.pending) return true;
  const sources = filterVisible(text).trim() ? [text] : [text, thinking];
  for (const raw of sources) {
    if (raw && (findToolProtocolPrefixOutsideIgnored(raw) || findToolMarkupTagOutsideIgnored(raw, 0) || findDSMLTagOutsideIgnored(raw, 0))) return true;
  }
  return false;
}

// Prefer Go's validated content. This fallback supports older validation
// responses while preserving the same prefix-only stream rendering contract.
function validatedToolText(text, toolNames, hasCalls) {
  if (!hasCalls) return filterVisible(text);
  return filterVisible(textBeforeToolBoundary(text));
}

module.exports = { needsToolFinalization, validatedToolText };
