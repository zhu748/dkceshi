'use strict';

// Tool/DSML tag scanning, nesting and incomplete-tag detection.
const { toRawString } = require('./state');
const { unfinishedToolTagName, includeDuplicateLeadingLessThan } = require('./body_integrity');
const {
  skipXmlIgnoredSection,
} = require('./cdata_scan');
const {
  markdownCodeSpanEnd,
} = require('./markdown_fence');
const {
  parseCanonicalToolMarkupAttrs,
} = require('./markup_attributes');
const {
  consumeToolMarkupNamePrefix,
  consumeToolMarkupNamePrefixOnce,
  hasEPSENamePrefixOrPartial,
  hasPartialToolMarkupNameAfterArbitraryPrefix,
  hasToolMarkupNamePrefix,
  matchToolMarkupName,
  matchToolMarkupNameAfterArbitraryPrefix,
  toolMarkupPrefixContainsSlash,
} = require('./markup_names');
const {
  TOOL_MARKUP_NAMES,
  consumeToolKeyword,
  consumeToolMarkupClosingSlash,
  consumeToolMarkupLessThan,
  consumeToolMarkupSeparator,
  containsXmlTagTerminator,
  findXmlTagEnd,
  hasXmlTagBoundary,
  isSelfClosingXmlTag,
  isXmlTagStartDelimiter,
  lastIndexOfToolMarkupStartDelimiter,
  normalizeFullwidthASCIIChar,
  skipToolMarkupIgnorables,
  toolMarkupWhitespaceLikeLenAt,
  xmlTagStartDelimiterLenAt,
} = require('./markup_syntax');

function scanToolMarkupTagAt(text, start) {
  const raw = toRawString(text);
  const startDelimLen = xmlTagStartDelimiterLenAt(raw, start);
  if (!raw || start < 0 || start >= raw.length || !startDelimLen) {
    return null;
  }
  const lower = raw.toLowerCase();
  let i = start + startDelimLen;
  while (i < raw.length) {
    i = skipToolMarkupIgnorables(raw, i);
    const delimLen = xmlTagStartDelimiterLenAt(raw, i);
    if (!delimLen) {
      break;
    }
    i += delimLen;
  }
  const slash = consumeToolMarkupClosingSlash(raw, i);
  let closing = slash.closing;
  i = slash.next;
  const prefix = consumeToolMarkupNamePrefix(raw, lower, i);
  const prefixStart = i;
  i = prefix.next;
  let epseLike = prefix.epseLike;
  if (epseLike && !closing) {
    const field = scanFieldNamedParameter(raw, start, i);
    if (field) return field;
  }
  let { name, len } = matchToolMarkupName(raw, i, epseLike);
  let shorthandParameter = false;
  if (!name) {
    const fallback = matchToolMarkupNameAfterArbitraryPrefix(raw, prefixStart);
    if (!fallback.ok) {
      if (!epseLike || !isEPSEShorthandParameter(raw, i, closing)) {
        return null;
      }
      name = 'parameter';
      len = 0;
      shorthandParameter = true;
    } else {
      if (!closing && toolMarkupPrefixContainsSlash(raw.slice(prefixStart, fallback.start))) {
        closing = true;
      }
      name = fallback.name;
      i = fallback.start;
      len = fallback.len;
      epseLike = true;
    }
  }
  const originalNameEnd = i + len;
  let nameEnd = originalNameEnd;
  while (true) {
    const nextPipe = consumeToolMarkupSeparator(raw, nameEnd);
    if (!nextPipe.ok) {
      break;
    }
    nameEnd = nextPipe.next;
  }
  const hasTrailingSeparator = nameEnd > originalNameEnd;
  if (!shorthandParameter && !hasXmlTagBoundary(raw, nameEnd)) {
    return null;
  }
  let end = findXmlTagEnd(raw, nameEnd);
  if (end < 0) {
    if (!hasTrailingSeparator) {
      return null;
    }
    end = nameEnd - 1;
  }
  if (hasTrailingSeparator) {
    const nextLT = raw.indexOf('<', nameEnd);
    if (nextLT >= 0 && end >= nextLT) {
      end = nameEnd - 1;
    }
  }
  if (end < 0) {
    return null;
  }
  return {
    start,
    end,
    nameStart: i,
    nameEnd,
    name,
    closing,
    selfClosing: isSelfClosingXmlTag(raw.slice(start, end)),
    epseLike,
    canonical: !epseLike,
  };
}

// Go parity: parameter_shorthand.go. The field name is present in the source;
// only the missing parameter/name shell is restored, never the value.
function scanFieldNamedParameter(raw, start, from) {
  const match = /^([A-Za-z_][A-Za-z0-9_.:-]*)["'][ \t\r\n]*>/.exec(raw.slice(from));
  if (!match || TOOL_MARKUP_NAMES.some(tag => tag.raw === match[1].toLowerCase())) return null;
  return {
    start, end: from + match[0].length - 1, nameStart: from, nameEnd: from + match[1].length,
    name: 'parameter', closing: false, selfClosing: false, epseLike: true, canonical: false,
    parameterName: match[1],
  };
}

function isEPSEShorthandParameter(text, i, closing) {
  const raw = toRawString(text);
  if (i < 0 || i >= raw.length) {
    return false;
  }
  const end = findXmlTagEnd(raw, i);
  if (end < 0) {
    return false;
  }
  if (closing) {
    return raw.slice(i, end).trim() === '';
  }
  for (const attr of parseCanonicalToolMarkupAttrs(raw, i)) {
    if (attr && attr.key === 'name') {
      return true;
    }
  }
  return false;
}

function findToolMarkupTagOutsideIgnored(text, from) {
  const raw = toRawString(text);
  for (let i = Math.max(0, from || 0); i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      i = spanEnd.end;
      continue;
    }
    const tag = scanToolMarkupTagAt(raw, i);
    if (tag) {
      return tag;
    }
    i += 1;
  }
  return null;
}

function findMatchingToolMarkupClose(text, openTag) {
  const raw = toRawString(text);
  if (!raw || !openTag || !openTag.name || openTag.closing) {
    return null;
  }
  let depth = 1;
  for (let pos = openTag.end + 1; pos < raw.length;) {
    const tag = findToolMarkupTagOutsideIgnored(raw, pos);
    if (!tag) {
      return null;
    }
    if (tag.name !== openTag.name) {
      pos = tag.end + 1;
      continue;
    }
    if (tag.closing) {
      depth -= 1;
      if (depth === 0) {
        return tag;
      }
    } else if (!tag.selfClosing) {
      depth += 1;
    }
    pos = tag.end + 1;
  }
  return null;
}

function findPartialToolMarkupStart(text) {
  const raw = toRawString(text);
  const lastLT = lastIndexOfToolMarkupStartDelimiter(raw);
  if (lastLT < 0) {
    return -1;
  }
  const start = includeDuplicateLeadingLessThan(raw, lastLT);
  const tail = raw.slice(start);
  if (containsXmlTagTerminator(tail)) {
    return -1;
  }
  return isPartialToolMarkupTagPrefix(tail) ? start : -1;
}

function isPartialToolMarkupTagPrefix(text) {
  const raw = toRawString(text);
  if (!raw || !isXmlTagStartDelimiter(raw[0]) || containsXmlTagTerminator(raw)) {
    return false;
  }
  if (isPartialDSMLTagPrefix(raw)) {
    return true;
  }
  if (unfinishedToolTagName(raw)) return true;
  const lower = raw.toLowerCase();
  let i = 1;
  while (i < raw.length && isXmlTagStartDelimiter(raw[i])) {
    i += 1;
  }
  if (i >= raw.length) {
    return true;
  }
  const slash = consumeToolMarkupClosingSlash(raw, i);
  if (slash.closing) {
    i = slash.next;
  }
  while (i <= raw.length) {
    if (i === raw.length) {
      return true;
    }
    if (hasToolMarkupNamePrefix(raw, i)) {
      return true;
    }
    if (hasEPSENamePrefixOrPartial(raw, i)) {
      return true;
    }
    if (hasPartialToolMarkupNameAfterArbitraryPrefix(raw, i)) {
      return true;
    }
    const next = consumeToolMarkupNamePrefixOnce(raw, lower, i);
    if (!next.ok) {
      return false;
    }
    i = next.next;
  }
  return false;
}

// isPartialDSMLTagPrefix reports whether text is an as-yet-unterminated
// DSML-prefixed tag. Any DSML tag is a streaming interception trigger, so a
// half-arrived `<DSML ...` (with or without a local-name suffix) must be held
// back instead of leaking as visible text. Mirrors the Go side
// (internal/toolcall/toolcalls_scan.go) isPartialDSMLTagPrefix.
function isPartialDSMLTagPrefix(text) {
  const raw = toRawString(text);
  if (!raw || !isXmlTagStartDelimiter(raw[0]) || containsXmlTagTerminator(raw)) {
    return false;
  }
  let i = 1;
  while (i < raw.length && isXmlTagStartDelimiter(raw[i])) {
    i += 1;
  }
  if (i >= raw.length) {
    return true;
  }
  const slash = consumeToolMarkupClosingSlash(raw, i);
  if (slash.closing) {
    i = slash.next;
  }
  while (true) {
    const sep = consumeToolMarkupSeparator(raw, i);
    if (sep.ok) {
      i = sep.next;
      continue;
    }
    const spacingLen = toolMarkupWhitespaceLikeLenAt(raw, i);
    if (spacingLen > 0) {
      i += spacingLen;
      continue;
    }
    break;
  }
  if (i >= raw.length) {
    return true;
  }
  const dsml = consumeToolKeyword(raw, i, 'dsml');
  if (!dsml.ok) {
    return hasPartialDSMLKeyword(raw, i);
  }
  const tail = normalizeFullwidthASCIIChar(raw[dsml.next] || '');
  if (/^[A-Za-z0-9]$/.test(tail || '')) {
    return false;
  }
  return true;
}

function hasPartialDSMLKeyword(raw, start) {
  const expected = 'dsml';
  let idx = start;
  let matched = 0;
  while (matched < expected.length && idx < raw.length) {
    const ch = normalizeFullwidthASCIIChar(raw[idx]).toLowerCase();
    if (ch !== expected[matched]) {
      return false;
    }
    idx += 1;
    matched += 1;
  }
  return matched > 0 && matched < expected.length && idx === raw.length;
}

// scanDSMLTagAt parses a complete DSML-prefixed markup tag (local name
// included) beginning at start, returning { start, end, name, closing } or
// null. The local name is the token after the `dsml` literal, normalized to
// lowercase ASCII; it may be empty (e.g. `<DSML>`). Mirrors the Go side
// (internal/toolcall/toolcalls_intent.go) scanDSMLTagAt.
function scanDSMLTagAt(text, start) {
  const raw = toRawString(text);
  const lt = consumeToolMarkupLessThan(raw, start);
  if (!lt.ok) {
    return null;
  }
  let i = lt.next;
  while (true) {
    const next = consumeToolMarkupLessThan(raw, i);
    if (!next.ok) {
      break;
    }
    i = next.next;
  }
  let closing = false;
  const slash = consumeToolMarkupClosingSlash(raw, i);
  if (slash.closing) {
    closing = true;
    i = slash.next;
  }
  while (true) {
    const sep = consumeToolMarkupSeparator(raw, i);
    if (sep.ok) {
      i = sep.next;
      continue;
    }
    const spacingLen = toolMarkupWhitespaceLikeLenAt(raw, i);
    if (spacingLen > 0) {
      i += spacingLen;
      continue;
    }
    break;
  }
  const dsml = consumeToolKeyword(raw, i, 'dsml');
  if (!dsml.ok) {
    return null;
  }
  // Guard against words that merely start with "dsml" (e.g. "dsmlfoo").
  const tail = normalizeFullwidthASCIIChar(raw[dsml.next] || '');
  if (/^[A-Za-z0-9]$/.test(tail || '')) {
    return null;
  }
  let nameStart = dsml.next;
  while (true) {
    const sep = consumeToolMarkupSeparator(raw, nameStart);
    if (sep.ok) {
      nameStart = sep.next;
      continue;
    }
    const spacingLen = toolMarkupWhitespaceLikeLenAt(raw, nameStart);
    if (spacingLen > 0) {
      nameStart += spacingLen;
      continue;
    }
    break;
  }
  let nameEnd = nameStart;
  while (nameEnd < raw.length) {
    const ch = normalizeFullwidthASCIIChar(raw[nameEnd]);
    if (ch.charCodeAt(0) > 0x7f) {
      break;
    }
    if (!/[A-Za-z0-9_-]/.test(ch)) {
      break;
    }
    nameEnd += 1;
  }
  const tagEnd = findXmlTagEnd(raw, nameStart);
  if (tagEnd < 0) {
    return null;
  }
  let name = '';
  for (let k = nameStart; k < nameEnd; k += 1) {
    name += normalizeFullwidthASCIIChar(raw[k]).toLowerCase();
  }
  return { start, end: tagEnd, name, closing };
}

// findDSMLTagOutsideIgnored returns the first complete (terminator already
// arrived) DSML-prefixed markup tag at or after from, skipping CDATA, XML
// comments and markdown inline code spans. It mirrors
// findToolMarkupTagOutsideIgnored but matches the DSML prefix instead of the
// tool markup name table.
function findDSMLTagOutsideIgnored(text, from) {
  const raw = toRawString(text);
  for (let i = Math.max(0, from || 0); i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      i = spanEnd.end;
      continue;
    }
    const tag = scanDSMLTagAt(raw, i);
    if (tag) {
      return tag;
    }
    i += 1;
  }
  return null;
}

// findMatchingDSMLClose performs local-name depth pairing to find the closing
// tag for open. Empty local names pair with empty local names. Mirrors the Go
// side (internal/toolcall/toolcalls_intent.go) FindMatchingDSMLClose.
function findMatchingDSMLClose(text, open) {
  const raw = toRawString(text);
  if (!raw || !open || open.closing || open.end >= raw.length) {
    return null;
  }
  let depth = 1;
  for (let pos = open.end + 1; pos < raw.length;) {
    const tag = findDSMLTagOutsideIgnored(raw, pos);
    if (!tag) {
      return null;
    }
    if (tag.name !== open.name) {
      pos = tag.end + 1;
      continue;
    }
    if (tag.closing) {
      depth -= 1;
      if (depth === 0) {
        return tag;
      }
    } else {
      depth += 1;
    }
    pos = tag.end + 1;
  }
  return null;
}

module.exports = {
  findDSMLTagOutsideIgnored,
  findMatchingDSMLClose,
  findMatchingToolMarkupClose,
  findPartialToolMarkupStart,
  findToolMarkupTagOutsideIgnored,
  hasPartialDSMLKeyword,
  isPartialDSMLTagPrefix,
  scanToolMarkupTagAt,
};
