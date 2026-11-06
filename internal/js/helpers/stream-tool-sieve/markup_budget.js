'use strict';

const { streamLimit } = require('../stream-limits');
const { skipXmlIgnoredSection } = require('./cdata_scan');
const { markdownCodeSpanEnd } = require('./markdown_fence');
const { scanToolMarkupTagAt, scanDSMLTagAt } = require('./markup_scan');
const { nextToolMarkupProbe } = require('./opener_scan');

function markupLimitExceeded(text) {
  const maxCalls = streamLimit('max_tool_calls');
  let depth = 0, nodes = 0, calls = 0;
  for (let i = 0; i < text.length;) {
    const skipped = skipXmlIgnoredSection(text, i);
    if (skipped.blocked) return false;
    if (skipped.advanced) { i = skipped.next; continue; }
    const code = markdownCodeSpanEnd(text, i);
    if (code.ok) { i = code.end; continue; }
    const tag = scanToolMarkupTagAt(text, i) || scanDSMLTagAt(text, i);
    if (!tag) { i = nextToolMarkupProbe(text, i); continue; }
    i = tag.end + 1;
    nodes++;
    if (tag.closing) depth = Math.max(0, depth - 1);
    else if (!tag.selfClosing) { depth++; if (tag.name === 'invoke') calls++; }
    if (depth > 128 || nodes > 32768 || calls > maxCalls) return true;
  }
  return false;
}

module.exports = { markupLimitExceeded };
