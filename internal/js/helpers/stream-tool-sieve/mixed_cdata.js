'use strict';

const { toolCDATAOpenLenAt, findToolCDATAEnd, toolCDATACloseLenAt, findTrailingToolCDATACloseStart } = require('./cdata_scan');
const { unescapeHtml } = require('./entities');
const { toRawString } = require('./state');

// Decode text mixed with CDATA without dropping the text after a close. Real
// XML children remain in the structured parser; entities decode only outside
// CDATA. This follows the Go XML reader for flat mixed values.
function parseMixedCDATAString(raw) {
  if (!toolCDATAOpenLenAt(raw, 0)) return { ok: false };
  let position = 0, output = '', seen = false;
  while (position < raw.length) {
    const open = toolCDATAOpenLenAt(raw, position);
    if (open) {
      const end = findToolCDATAEnd(raw, position + open);
      if (end < 0) return { ok: false };
      output += raw.slice(position + open, end);
      position = end + toolCDATACloseLenAt(raw, end);
      seen = true;
    } else {
      const next = raw.indexOf('<', position);
      const end = next < 0 ? raw.length : next;
      if (end === position) return { ok: false };
      output += unescapeHtml(raw.slice(position, end));
      position = end;
    }
  }
  return { ok: seen, value: output };
}

function extractStandaloneCDATA(inner) {
  const s = toRawString(inner).trim();
  const openLen = toolCDATAOpenLenAt(s, 0);
  if (!openLen) {
    return { ok: false, value: '' };
  }
  const closeStart = findTrailingToolCDATACloseStart(s);
  if (closeStart >= openLen) {
    return { ok: true, value: s.slice(openLen, closeStart) };
  }
  const end = findToolCDATAEnd(s, openLen);
  if (end >= 0) {
    return { ok: false, value: '' };
  }
  return { ok: true, value: s.slice(openLen) };
}

module.exports = { parseMixedCDATAString, extractStandaloneCDATA };
