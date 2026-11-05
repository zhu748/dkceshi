'use strict';

// Recovery of unclosed CDATA; closed literal boundaries are never inferred.
const { toRawString } = require('./state');
const {
  findToolCDATAEnd,
  indexToolCDATAOpen,
  toolCDATACloseLenAt,
  toolCDATAOpenLenAt,
} = require('./cdata_scan');
function sanitizeLooseCDATA(text) {
  const raw = toRawString(text);
  if (!raw) {
    return '';
  }

  let out = '';
  let pos = 0;
  let changed = false;
  while (pos < raw.length) {
    const start = indexToolCDATAOpen(raw, pos);
    if (start < 0) {
      out += raw.slice(pos);
      break;
    }
    const openLen = toolCDATAOpenLenAt(raw, start);
    const contentStart = start + openLen;
    out += raw.slice(pos, start);

    const endRel = findToolCDATAEnd(raw, contentStart);
    if (endRel >= 0) {
      const end = endRel + toolCDATACloseLenAt(raw, endRel);
      out += raw.slice(start, end);
      pos = end;
      continue;
    }

    changed = true;
    out += raw.slice(contentStart);
    pos = raw.length;
  }

  return changed ? out : raw;
}

module.exports = { sanitizeLooseCDATA };
