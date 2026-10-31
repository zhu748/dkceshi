'use strict';

// Remove malformed DSML protocol residue outside literal code examples.
const {
  insideFencedCodeAt,
} = require('./markdown_fence');
const {
  findDSMLTagOutsideIgnored,
  findMatchingDSMLClose,
  hasPartialDSMLKeyword,
  isPartialDSMLTagPrefix,
} = require('./markup_scan');
const {
  consumeToolKeyword,
  consumeToolMarkupClosingSlash,
  consumeToolMarkupSeparator,
  isXmlTagStartDelimiter,
  toolMarkupWhitespaceLikeLenAt,
} = require('./markup_syntax');

// stripDSMLResidue is the JS mirror of the Go final safety net
// (internal/toolcall/toolcalls_deduct.go StripDSMLResidue): it removes every
// DSML-prefixed markup span (full wrapper, orphan closing tag, or unclosed
// opening tag plus its own tag run) plus a trailing unterminated DSML
// fragment, so a leftover `<｜｜DSML｜｜ calls>` never reaches the client.
// DSML inside fenced code blocks or inline code spans is documentation and is
// preserved. Used by the sieve flush release paths
// (internal/js/helpers/stream-tool-sieve/sieve.js flushToolSieve).
function stripDSMLResidue(text) {
  // Clean text is a byte-identical passthrough so flush releases keep the
  // verbatim-output contract, matching the Go side.
  if (typeof text !== 'string' || !text) {
    return text;
  }
  const spans = dsmlBadSpans(text);
  let stripped = text;
  if (spans.length > 0) {
    stripped = removeDSMLTextSpans(text, spans);
  }
  const tailStripped = stripTrailingPartialDSMLTag(stripped);
  if (tailStripped !== stripped) {
    return tailStripped.trim();
  }
  return tailStripped;
}

// dsmlBadSpans mirrors Go dsmlBadSpans: spans of DSML wrappers, skipping
// fenced-code documentation.
function dsmlBadSpans(text) {
  const spans = [];
  for (let pos = 0; pos < text.length;) {
    const tag = findDSMLTagOutsideIgnored(text, pos);
    if (!tag) {
      break;
    }
    if (insideFencedCodeAt(text, tag.start)) {
      pos = tag.end + 1;
      continue;
    }
    if (tag.closing) {
      spans.push({ start: tag.start, end: tag.end + 1 });
      pos = tag.end + 1;
      continue;
    }
    const closeTag = findMatchingDSMLClose(text, tag);
    if (closeTag) {
      spans.push({ start: tag.start, end: closeTag.end + 1 });
      pos = closeTag.end + 1;
      continue;
    }
    const end = dsmlTagRunEnd(text, tag);
    spans.push({ start: tag.start, end });
    pos = end;
  }
  return spans;
}

// dsmlTagRunEnd mirrors Go dsmlTagRunEnd: only whitespace-separated DSML tags
// join the run of an unclosed wrapper, so trailing prose is preserved.
function dsmlTagRunEnd(text, open) {
  let end = open.end + 1;
  for (let pos = end; pos < text.length;) {
    const tag = findDSMLTagOutsideIgnored(text, pos);
    if (!tag) {
      return end;
    }
    if (text.slice(end, tag.start).trim() !== '') {
      return end;
    }
    end = tag.end + 1;
    pos = tag.end + 1;
  }
  return end;
}

// removeDSMLTextSpans mirrors Go RemoveTextSpans: delete the merged spans and
// trim surrounding whitespace.
function removeDSMLTextSpans(text, spans) {
  const ordered = [...spans].sort((a, b) => (a.start !== b.start ? a.start - b.start : a.end - b.end));
  const merged = [];
  for (const span of ordered) {
    const last = merged[merged.length - 1];
    if (last && span.start <= last.end) {
      if (span.end > last.end) {
        last.end = span.end;
      }
      continue;
    }
    merged.push({ start: span.start, end: span.end });
  }
  let out = '';
  let prev = 0;
  for (const span of merged) {
    const start = Math.max(span.start, prev);
    const end = Math.min(span.end, text.length);
    if (start > end) {
      continue;
    }
    if (start > prev) {
      out += text.slice(prev, start);
    }
    prev = end;
  }
  if (prev < text.length) {
    out += text.slice(prev);
  }
  return out.trim();
}

// stripTrailingPartialDSMLTag mirrors Go stripTrailingPartialDSMLTag: cut a
// trailing unterminated DSML tag fragment, requiring DSML evidence so a bare
// trailing `<` in prose survives, and preserving fenced documentation.
function stripTrailingPartialDSMLTag(text) {
  const idx = text.lastIndexOf('<');
  if (idx < 0) {
    return text;
  }
  let start = idx;
  while (start > 0 && text[start - 1] === '<') {
    start -= 1;
  }
  const tail = text.slice(start);
  if (!isPartialDSMLTagPrefix(tail) || !partialDSMLTailHasEvidence(tail)) {
    return text;
  }
  if (insideFencedCodeAt(text, start)) {
    return text;
  }
  return text.slice(0, start);
}

// partialDSMLTailHasEvidence mirrors Go partialDSMLTailHasEvidence: the
// fragment must spell at least part of the `dsml` literal.
function partialDSMLTailHasEvidence(tail) {
  let i = 1;
  while (i < tail.length && isXmlTagStartDelimiter(tail[i])) {
    i += 1;
  }
  const slash = consumeToolMarkupClosingSlash(tail, i);
  if (slash.closing) {
    i = slash.next;
  }
  while (true) {
    const sep = consumeToolMarkupSeparator(tail, i);
    if (sep.ok) {
      i = sep.next;
      continue;
    }
    const spacingLen = toolMarkupWhitespaceLikeLenAt(tail, i);
    if (spacingLen > 0) {
      i += spacingLen;
      continue;
    }
    break;
  }
  if (i >= tail.length) {
    return false;
  }
  const kw = consumeToolKeyword(tail, i, 'dsml');
  if (kw.ok) {
    return true;
  }
  return hasPartialDSMLKeyword(tail, i);
}

module.exports = {
  stripDSMLResidue,
};
