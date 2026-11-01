'use strict';

const {
  consumeToolMarkupLessThan, consumeToolMarkupClosingSlash,
  skipToolMarkupIgnorables, normalizeFullwidthASCIIChar, toolMarkupWhitespaceLikeLenAt,
} = require('./markup_syntax');

// Mirror protocol_envelope.go: classify the shell, never infer an executable
// tool name or rewrite a parameter. An incomplete shell remains a candidate.
function protocolEnvelopeAt(text, start, partial = false) {
  let open = consumeToolMarkupLessThan(text, start);
  if (!open.ok) return false;
  let i = open.next;
  while ((open = consumeToolMarkupLessThan(text, i)).ok) i = open.next;
  i = skipToolMarkupIgnorables(text, consumeToolMarkupClosingSlash(text, i).next);
  const leading = consumeDelimiters(text, i);
  if (leading === i) return false;
  i = leading;
  const markerStart = i;
  if (i === text.length || !/[\p{L}\p{N}]/u.test(String.fromCodePoint(text.codePointAt(i)))) return false;
  while (i < text.length) {
    const next = skipToolMarkupIgnorables(text, i);
    if (next !== i) { i = next; continue; }
    const ch = String.fromCodePoint(text.codePointAt(i));
    if (!/[\p{L}\p{N}_-]/u.test(ch)) break;
    i += ch.length;
  }
  if (i === markerStart) return false;
  if (i === text.length) return partial;
  const next = consumeDelimiters(text, i);
  if (next === i) return false;
  i = next;
  while (i < text.length) {
    const size = toolMarkupWhitespaceLikeLenAt(text, i);
    if (!size) break;
    i += size;
  }
  i = skipToolMarkupIgnorables(text, i);
  return i === text.length ? partial : /[\p{L}_]/u.test(String.fromCodePoint(text.codePointAt(i)));
}

function consumeDelimiters(text, i) {
  while (i < text.length) {
    const pos = skipToolMarkupIgnorables(text, i);
    if (pos === text.length) return pos;
    const ch = String.fromCodePoint(text.codePointAt(pos));
    if (!['|', '!', '、'].includes(normalizeFullwidthASCIIChar(ch))) return i;
    i = pos + ch.length;
  }
  return i;
}

module.exports = { protocolEnvelopeAt };
