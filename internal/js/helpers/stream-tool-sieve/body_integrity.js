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

// Mirror partial_marker.go: length cannot turn an undecided name into prose.
function unfinishedToolTagName(raw) {
  return /^[<＜﹤〈]+[/／∕⁄⧸]?[\p{L}\p{N}_\-|｜！!、\p{Cf}]*$/u.test(raw);
}

function includeDuplicateLeadingLessThan(text, idx) {
  let out = idx;
  while (out > 0 && isXmlTagStartDelimiter(text[out - 1])) {
    out -= 1;
  }
  return out;
}

module.exports = { emptyToolMarkupGap, unfinishedToolTagName, includeDuplicateLeadingLessThan };
