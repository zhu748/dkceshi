'use strict';

const { protocolEnvelopeAt } = require('./protocol_envelope');
const { unfinishedToolTagName } = require('./body_integrity');

const { skipXmlIgnoredSection } = require('./cdata_scan');
const { markdownCodeSpanEnd } = require('./markdown_fence');
const {
  consumeToolKeyword, consumeToolMarkupClosingSlash, consumeToolMarkupLessThan,
  consumeToolMarkupSeparator, normalizeFullwidthASCIIChar, toolMarkupWhitespaceLikeLenAt,
} = require('./markup_syntax');

// Visibility only: an unknown local name never becomes executable syntax.
function findToolProtocolPrefixOutsideIgnored(text, from = 0) {
  if (!/[<＜﹤〈]/.test(text)) return null;
  for (let i = Math.max(0, from); i < text.length;) {
    const ignored = skipXmlIgnoredSection(text, i);
    if (ignored.blocked) break;
    if (ignored.advanced) { i = ignored.next; continue; }
    const code = markdownCodeSpanEnd(text, i);
    if (code.ok) { i = code.end; continue; }
    const protocol = protocolPrefixAt(text, i);
    if (protocol) return { start: i, protocol };
    i += 1;
  }
  return null;
}

function protocolPrefixAt(text, start) {
  let open = consumeToolMarkupLessThan(text, start);
  if (!open.ok) return '';
  let i = open.next;
  while ((open = consumeToolMarkupLessThan(text, i)).ok) i = open.next;
  i = consumeToolMarkupClosingSlash(text, i).next;
  while (true) {
    const separator = consumeToolMarkupSeparator(text, i);
    const spacing = toolMarkupWhitespaceLikeLenAt(text, i);
    if (separator.ok) i = separator.next;
    else if (spacing) i += spacing;
    else break;
  }
  for (const protocol of ['epse', 'dsml']) {
    const keyword = consumeToolKeyword(text, i, protocol);
    if (keyword.ok && !/^[a-z0-9]$/i.test(normalizeFullwidthASCIIChar(text[keyword.next] || ''))) return protocol;
  }
  if (protocolEnvelopeAt(text, start)) return 'envelope';
  if (protocolEnvelopeAt(text, start, true) || (Buffer.byteLength(text.slice(start), 'utf8') > 128 && unfinishedToolTagName(text.slice(start)))) return 'envelope_partial';
  return '';
}

module.exports = { findToolProtocolPrefixOutsideIgnored };
