'use strict';

// Canonicalize recognized candidate shells while preserving every literal.
const { toRawString } = require('./state');
const { nextToolMarkupProbe } = require('./opener_scan');
const { skipXmlIgnoredSection } = require('./cdata_scan');
const { markdownCodeSpanEnd } = require('./markdown_fence');
const { scanToolMarkupTagAt } = require('./markup_scan');
const { parseCanonicalToolMarkupAttrs, quoteCanonicalXMLAttrValue } = require('./markup_attributes');
const { consumeToolMarkupNamePrefix, rawNameForTag } = require('./markup_names');
const { consumeToolKeyword, consumeToolMarkupClosingSlash, skipToolMarkupIgnorables, xmlTagStartDelimiterLenAt } = require('./markup_syntax');

function replaceEPSEToolMarkupOutsideIgnored(text) {
  const raw = toRawString(text);
  if (!raw) {
    return '';
  }
  let out = '';
  for (let i = 0; i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      out += raw.slice(i);
      break;
    }
    if (skipped.advanced) {
      out += raw.slice(i, skipped.next);
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      out += raw.slice(i, spanEnd.end);
      i = spanEnd.end;
      continue;
    }
    const tag = scanToolMarkupTagAt(raw, i);
    if (tag) {
      const attrs = tag.parameterName ? ` name="${tag.parameterName}"` : raw.slice(tag.nameEnd, tag.end);
      out += `<${tag.closing ? '/' : ''}${tag.name}${attrs}>`;
      i = tag.end + 1;
      continue;
    }
    const next = nextToolMarkupProbe(raw, i);
    out += raw.slice(i, next);
    i = next;
  }
  return out;
}

function canonicalizeToolCallCandidateSpans(text) {
  const raw = toRawString(text);
  if (!raw) {
    return '';
  }
  let out = '';
  for (let i = 0; i < raw.length;) {
    const skipped = skipXmlIgnoredSection(raw, i);
    if (skipped.blocked) {
      out += raw.slice(i);
      break;
    }
    if (skipped.advanced) {
      out += raw.slice(i, skipped.next);
      i = skipped.next;
      continue;
    }
    const spanEnd = markdownCodeSpanEnd(raw, i);
    if (spanEnd.ok) {
      out += raw.slice(i, spanEnd.end);
      i = spanEnd.end;
      continue;
    }
    const tag = scanToolMarkupTagAt(raw, i);
    if (!tag) {
      const next = nextToolMarkupProbe(raw, i);
      out += raw.slice(i, next);
      i = next;
      continue;
    }
    out += canonicalizeRecognizedToolMarkupTag(raw.slice(tag.start, tag.end + 1), tag);
    i = tag.end + 1;
  }
  return out;
}

function canonicalizeRecognizedToolMarkupTag(rawTag, tag) {
  const raw = toRawString(rawTag);
  if (!raw || !tag) {
    return raw;
  }
  let idx = 0;
  const startLen = xmlTagStartDelimiterLenAt(raw, idx);
  if (startLen > 0) {
    idx += startLen;
  }
  while (idx < raw.length) {
    idx = skipToolMarkupIgnorables(raw, idx);
    const delimLen = xmlTagStartDelimiterLenAt(raw, idx);
    if (!delimLen) {
      break;
    }
    idx += delimLen;
  }
  idx = skipToolMarkupIgnorables(raw, idx);
  if (tag.closing) {
    const slash = consumeToolMarkupClosingSlash(raw, idx);
    if (slash.closing) {
      idx = slash.next;
    }
  }
  const prefix = consumeToolMarkupNamePrefix(raw, raw.toLowerCase(), idx);
  idx = prefix.next;
  const nameMatch = consumeToolKeyword(raw, idx, rawNameForTag(tag));
  const afterName = nameMatch.ok ? nameMatch.next : idx;
  const attrs = tag.parameterName
    ? [{ key: 'name', value: tag.parameterName }]
    : parseCanonicalToolMarkupAttrs(raw, afterName);

  let out = '<';
  if (tag.closing) {
    out += '/';
  }
  if (tag.epseLike) {
    out += '|EPSE|';
  }
  out += tag.name;
  for (const attr of attrs) {
    if (!attr || !attr.key) {
      continue;
    }
    out += ` ${attr.key}="${quoteCanonicalXMLAttrValue(attr.value)}"`;
  }
  if (tag.selfClosing) {
    out += '/';
  }
  out += '>';
  return out;
}

module.exports = { replaceEPSEToolMarkupOutsideIgnored, canonicalizeToolCallCandidateSpans };
