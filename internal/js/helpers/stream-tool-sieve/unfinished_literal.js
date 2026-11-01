'use strict';

const { insideCodeFenceWithState } = require('./state');
const { skipXmlIgnoredSection } = require('./cdata_scan');
const { markdownCodeSpanEnd } = require('./markdown_fence');
const { findPartialXMLToolTagStart } = require('./sieve-xml');

function findUnfinishedToolLiteralStart(text) {
  for (let i = 0; i < text.length;) {
    if (text[i] === '<' && ('<![CDATA['.startsWith(text.slice(i).toUpperCase()) || '<!--'.startsWith(text.slice(i)))) return i;
    const ignored = skipXmlIgnoredSection(text, i);
    if (ignored.blocked) return i;
    if (ignored.advanced) { i = ignored.next; continue; }
    const code = markdownCodeSpanEnd(text, i);
    if (code.ok) { i = code.end; continue; }
    if (text[i] === '"') {
      let unfinished = true;
      for (let end = i + 1; end < text.length; end += 1) {
        if (text[end] === '\\') end += 1;
        else if (text[end] === '"' || text.charCodeAt(end) < 32) { unfinished = false; break; }
      }
      if (unfinished) return i;
    }
    i += 1;
  }
  return -1;
}

function holdUnfinishedDisplayLiteral(state, pending, start) {
  if (!state.deferMalformed) return null;
  const literal = findUnfinishedToolLiteralStart(pending);
  const partial = findPartialXMLToolTagStart(pending);
  return literal >= 0 && !insideCodeFenceWithState(state, pending.slice(0, literal)) && (start < 0 || literal < start) && (partial < 0 || literal < partial)
    ? { prefix: pending.slice(0, literal), tail: pending.slice(literal) } : null;
}

module.exports = { holdUnfinishedDisplayLiteral };
