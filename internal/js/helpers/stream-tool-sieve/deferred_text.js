'use strict';
const { findToolProtocolPrefixOutsideIgnored } = require('./protocol_prefix');

const { findToolMarkupTagOutsideIgnored, findDSMLTagOutsideIgnored, findMatchingDSMLClose } = require('./markup_scan');

function shouldDeferText(state, text) {
  return !!state.deferMalformed && (!!findToolProtocolPrefixOutsideIgnored(text) || !!findToolMarkupTagOutsideIgnored(text, 0) || !!findDSMLTagOutsideIgnored(text, 0));
}

// consumeDSMLHiddenCapture hides a DSML block from visible output: prefix is
// the text before the first DSML tag; suffix is the text after the matched
// closing tag. handled=true means a DSML tag was found; hold=true means the
// outer wrapper has not closed yet and the caller should keep buffering. When
// final is true an unclosed tail is discarded as well. Mirrors the Go side
// (internal/toolstream/tool_sieve_core.go) consumeDSMLHiddenCapture.
function consumeDSMLHiddenCapture(captured, final) {
  const open = findDSMLTagOutsideIgnored(captured, 0);
  if (!open) {
    return { prefix: '', suffix: '', handled: false, hold: false };
  }
  const prefix = captured.slice(0, open.start);
  const closeTag = findMatchingDSMLClose(captured, open);
  if (!closeTag) {
    if (final) {
      return { prefix, suffix: '', handled: true, hold: false };
    }
    return { prefix: '', suffix: '', handled: true, hold: true };
  }
  return { prefix, suffix: captured.slice(closeTag.end + 1), handled: true, hold: false };
}

module.exports = { shouldDeferText, consumeDSMLHiddenCapture };
