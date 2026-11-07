'use strict';

// Attribute scanning and canonical XML attribute formatting.
const { toRawString } = require('./state');
const {
  consumeToolKeyword,
  consumeToolMarkupClosingSlash,
  consumeToolMarkupPipe,
  removeToolMarkupIgnorables,
  skipToolMarkupIgnorables,
  toolMarkupEqualsLenAt,
  toolMarkupWhitespaceLikeLenAt,
  xmlQuoteCloseDelimiterLenAt,
  xmlQuotePairAt,
  xmlTagEndDelimiterLenAt,
} = require('./markup_syntax');

const XML_ATTR_PATTERN = /\b([a-z0-9_:-]+)\s*=\s*("([^"]*)"|'([^']*)')/gi;

function parseCanonicalToolMarkupAttrs(rawTag, startIdx, strict = false) {
  const raw = toRawString(rawTag);
  let idx = Math.max(0, startIdx || 0);
  const out = [];
  while (idx < raw.length) {
    idx = skipToolMarkupIgnorables(raw, idx);
    if (idx >= raw.length) {
      break;
    }
    const spacingLen = toolMarkupWhitespaceLikeLenAt(raw, idx);
    if (spacingLen > 0) {
      idx += spacingLen;
      continue;
    }
    if (xmlTagEndDelimiterLenAt(raw, idx) > 0) {
      break;
    }
    if (consumeToolMarkupPipe(raw, idx).ok) {
      idx = consumeToolMarkupPipe(raw, idx).next;
      continue;
    }
    if (consumeToolMarkupClosingSlash(raw, idx).closing) {
      idx = consumeToolMarkupClosingSlash(raw, idx).next;
      continue;
    }

    const keyStart = idx;
    while (idx < raw.length) {
      idx = skipToolMarkupIgnorables(raw, idx);
      if (idx >= raw.length) {
        break;
      }
      if (toolMarkupWhitespaceLikeLenAt(raw, idx) > 0) {
        break;
      }
      if (toolMarkupEqualsLenAt(raw, idx) > 0 || xmlTagEndDelimiterLenAt(raw, idx) > 0) {
        break;
      }
      if (consumeToolMarkupPipe(raw, idx).ok || consumeToolMarkupClosingSlash(raw, idx).closing) {
        break;
      }
      const cp = raw.codePointAt(idx);
      idx += cp > 0xFFFF ? 2 : 1;
    }
    let key = normalizeCanonicalToolAttrKey(raw.slice(keyStart, idx));

    idx = skipToolMarkupIgnorables(raw, idx);
    while (idx < raw.length) {
      const wsLen = toolMarkupWhitespaceLikeLenAt(raw, idx);
      if (!wsLen) {
        break;
      }
      idx += wsLen;
      idx = skipToolMarkupIgnorables(raw, idx);
    }
    const equalsLen = toolMarkupEqualsLenAt(raw, idx);
    if (!equalsLen) {
      if (strict && /[\p{L}\p{N}]/u.test(raw.slice(keyStart, idx))) out.push({ key: '__unsupported_attribute', value: '' });
      continue;
    }
    idx += equalsLen;
    idx = skipToolMarkupIgnorables(raw, idx);
    while (idx < raw.length) {
      const wsLen = toolMarkupWhitespaceLikeLenAt(raw, idx);
      if (!wsLen) {
        break;
      }
      idx += wsLen;
      idx = skipToolMarkupIgnorables(raw, idx);
    }
    if (!key) {
      key = '__unsupported_attribute';
    }

    let value = '';
    const quote = xmlQuotePairAt(raw, idx);
    if (quote.len) {
      const valueStart = idx + quote.len;
      idx = valueStart;
      while (idx < raw.length) {
        const closeLen = xmlQuoteCloseDelimiterLenAt(raw, idx, quote.close);
        if (closeLen) {
          value = raw.slice(valueStart, idx);
          idx += closeLen;
          break;
        }
        const cp = raw.codePointAt(idx);
        idx += cp > 0xFFFF ? 2 : 1;
      }
    } else {
      const valueStart = idx;
      while (idx < raw.length) {
        if (toolMarkupWhitespaceLikeLenAt(raw, idx) > 0 || xmlTagEndDelimiterLenAt(raw, idx) > 0 || toolMarkupEqualsLenAt(raw, idx) > 0) {
          break;
        }
        if (consumeToolMarkupPipe(raw, idx).ok || consumeToolMarkupClosingSlash(raw, idx).closing) {
          break;
        }
        const cp = raw.codePointAt(idx);
        idx += cp > 0xFFFF ? 2 : 1;
      }
      value = raw.slice(valueStart, idx);
    }
    out.push({ key, value });
  }
  return out;
}

function normalizeCanonicalToolAttrKey(rawKey) {
  const trimmed = toRawString(removeToolMarkupIgnorables(rawKey)).trim();
  if (!trimmed) {
    return '';
  }
  for (const key of ['name', 'call_id', 'id', 'version']) {
    const matched = consumeToolKeyword(trimmed, 0, key);
    if (matched.ok && skipToolMarkupIgnorables(trimmed, matched.next) === trimmed.length) return key;
  }
  return '';
}

function quoteCanonicalXMLAttrValue(rawValue) {
  return toRawString(rawValue).replace(/"/g, '&quot;');
}

function parseTagAttributes(raw) {
  const source = toRawString(raw);
  const out = {};
  if (!source) {
    return out;
  }
  for (const match of source.matchAll(XML_ATTR_PATTERN)) {
    const key = toRawString(match[1]).trim().toLowerCase();
    if (!key) {
      continue;
    }
    if (Object.hasOwn(out, key)) return {};
    out[key] = match.slice(3).find((value) => value !== undefined && value !== '') || '';
  }
  return out;
}

module.exports = {
  parseCanonicalToolMarkupAttrs,
  parseTagAttributes,
  quoteCanonicalXMLAttrValue,
};
