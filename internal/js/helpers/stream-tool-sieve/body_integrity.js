'use strict';
const { isXmlTagStartDelimiter } = require('./markup_syntax');

function emptyToolMarkupGap(text) {
  let remaining = text.trim();
  while (remaining) {
    if (!remaining.startsWith('<!--')) return false;
    const end = remaining.indexOf('-->', 4);
    if (end < 0) return false;
    remaining = remaining.slice(end + 3).trim();
  }
  return true;
}

// Mirror partial_marker.go: hold a short unfinished tag name even when the
// model misspelled the caller marker. Attributes/bodies use the normal scanner.
function unfinishedToolTagName(raw) {
  return Buffer.byteLength(raw, 'utf8') <= 128 && /^[<＜﹤〈]+[/／∕⁄⧸]?[\p{L}\p{N}_\-|｜！!、\p{Cf}]*$/u.test(raw);
}

function includeDuplicateLeadingLessThan(text, idx) {
  let out = idx;
  while (out > 0 && isXmlTagStartDelimiter(text[out - 1])) {
    out -= 1;
  }
  return out;
}

module.exports = { emptyToolMarkupGap, unfinishedToolTagName, includeDuplicateLeadingLessThan };
