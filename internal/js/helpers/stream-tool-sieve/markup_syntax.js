'use strict';

// Unicode delimiters, keyword folding and XML character syntax.
const { toRawString } = require('./state');

const TOOL_MARKUP_NAMES = [
  { raw: 'tool_calls', canonical: 'tool_calls' },
  { raw: 'tool-calls', canonical: 'tool_calls', epseOnly: true },
  { raw: 'toolcalls', canonical: 'tool_calls', epseOnly: true },
  { raw: 'invoke', canonical: 'invoke' },
  { raw: 'parameter', canonical: 'parameter' },
];

function isXmlTagStartDelimiter(ch) {
  return ['<', '＜', '﹤', '〈'].includes(ch);
}

function isToolMarkupSeparator(ch) {
  if (isToolMarkupWhitespaceLike(ch)) {
    return false;
  }
  const normalized = normalizeFullwidthASCIIChar(ch || '');
  if (!normalized || ['<', '>', '/', '=', '"', "'", '['].includes(normalized)) {
    return false;
  }
  if ([' ', '\t', '\n', '\r'].includes(normalized)) {
    return false;
  }
  return !/^[A-Za-z0-9]$/.test(normalized);
}

function isToolMarkupWhitespaceLike(ch) {
  return !!ch && (/\s/u.test(ch) || ch === '▁');
}

function consumeToolMarkupSeparator(raw, idx) {
  idx = skipToolMarkupIgnorables(raw, idx);
  if (idx >= raw.length) {
    return { next: idx, ok: false };
  }
  const cp = raw.codePointAt(idx);
  const ch = String.fromCodePoint(cp);
  if (!isToolMarkupSeparator(ch)) {
    return { next: idx, ok: false };
  }
  return { next: idx + ch.length, ok: true };
}

function hasToolMarkupBoundary(text, idx) {
  idx = skipToolMarkupIgnorables(text, idx);
  if (idx >= text.length) {
    return true;
  }
  if (toolMarkupWhitespaceLikeLenAt(text, idx) > 0) {
    return true;
  }
  if (consumeToolMarkupClosingSlash(text, idx).closing) {
    return true;
  }
  return xmlTagEndDelimiterLenAt(text, idx) > 0;
}

function consumeToolMarkupLessThan(raw, idx) {
  idx = skipToolMarkupIgnorables(raw, idx);
  if (idx < 0 || idx >= raw.length) {
    return { next: idx, ok: false };
  }
  const delimLen = xmlTagStartDelimiterLenAt(raw, idx);
  if (!delimLen) {
    return { next: idx, ok: false };
  }
  return { next: idx + delimLen, ok: true };
}

function removeToolMarkupIgnorables(rawValue) {
  const raw = toRawString(rawValue);
  let out = '';
  for (let i = 0; i < raw.length;) {
    const ignorableLen = toolMarkupIgnorableLenAt(raw, i);
    if (ignorableLen) {
      i += ignorableLen;
      continue;
    }
    const cp = raw.codePointAt(i);
    const ch = String.fromCodePoint(cp);
    out += ch;
    i += ch.length;
  }
  return out;
}

function skipToolMarkupIgnorables(text, idx) {
  const raw = toRawString(text);
  let pos = Math.max(0, idx || 0);
  while (pos < raw.length) {
    const next = toolMarkupIgnorableLenAt(raw, pos);
    if (!next) {
      break;
    }
    pos += next;
  }
  return pos;
}

function toolMarkupIgnorableLenAt(text, idx) {
  const raw = toRawString(text);
  if (idx < 0 || idx >= raw.length) {
    return 0;
  }
  const cp = raw.codePointAt(idx);
  if (cp === undefined) {
    return 0;
  }
  const ch = String.fromCodePoint(cp);
  const isFormat = /[\u00AD\u200B-\u200F\u202A-\u202E\u2060-\u206F\uFE00-\uFE0F\uFEFF]/u.test(ch);
  const isControl = /[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F]/u.test(ch);
  return isFormat || isControl ? ch.length : 0;
}

function toolMarkupEqualsLenAt(text, idx) {
  const raw = toRawString(text);
  const pos = skipToolMarkupIgnorables(raw, idx);
  for (const variant of ['=', '＝', '﹦', '꞊']) {
    if (raw.startsWith(variant, pos)) {
      return (pos + variant.length) - idx;
    }
  }
  return 0;
}

function toolMarkupDashLenAt(text, idx) {
  const raw = toRawString(text);
  const pos = skipToolMarkupIgnorables(raw, idx);
  for (const variant of ['-', '‐', '‑', '‒', '–', '—', '―', '−', '﹣', '－']) {
    if (raw.startsWith(variant, pos)) {
      return (pos + variant.length) - idx;
    }
  }
  return 0;
}

function toolMarkupUnderscoreLenAt(text, idx) {
  const raw = toRawString(text);
  const pos = skipToolMarkupIgnorables(raw, idx);
  for (const variant of ['_', '＿', '﹍', '﹎', '﹏']) {
    if (raw.startsWith(variant, pos)) {
      return (pos + variant.length) - idx;
    }
  }
  return 0;
}

function consumeToolKeyword(text, idx, keyword) {
  const raw = toRawString(text);
  let next = idx;
  for (const ch of keyword.toLowerCase()) {
    next = skipToolMarkupIgnorables(raw, next);
    if (next >= raw.length) {
      return { next: idx, ok: false };
    }
    if (ch === '_') {
      const len = toolMarkupUnderscoreLenAt(raw, next);
      if (!len) {
        return { next: idx, ok: false };
      }
      next += len;
      continue;
    }
    if (ch === '-') {
      const len = toolMarkupDashLenAt(raw, next);
      if (!len) {
        return { next: idx, ok: false };
      }
      next += len;
      continue;
    }
    const cp = raw.codePointAt(next);
    const folded = foldToolKeywordRune(String.fromCodePoint(cp));
    if (!folded || folded !== ch) {
      return { next: idx, ok: false };
    }
    next += cp > 0xFFFF ? 2 : 1;
  }
  return { next, ok: true };
}

function foldToolKeywordRune(ch) {
  if (!ch) {
    return '';
  }
  const cp = ch.codePointAt(0);
  if (cp >= 0xFF21 && cp <= 0xFF3A) {
    return String.fromCharCode(cp - 0xFEE0).toLowerCase();
  }
  if (cp >= 0xFF41 && cp <= 0xFF5A) {
    return String.fromCharCode(cp - 0xFEE0);
  }
  const lower = ch.toLowerCase();
  if ('acdeiklmnoprstv'.includes(lower)) {
    return lower;
  }
  const mapped = {
    'а': 'a',
    'α': 'a',
    'с': 'c',
    'ϲ': 'c',
    'ԁ': 'd',
    'ⅾ': 'd',
    'е': 'e',
    'ε': 'e',
    'і': 'i',
    'ι': 'i',
    'ı': 'i',
    'к': 'k',
    'κ': 'k',
    'ⅼ': 'l',
    'м': 'm',
    'μ': 'm',
    'ո': 'n',
    'о': 'o',
    'ο': 'o',
    'р': 'p',
    'ρ': 'p',
    'ѕ': 's',
    'т': 't',
    'τ': 't',
    'ν': 'v',
    'ѵ': 'v',
    'ⅴ': 'v',
  };
  return mapped[lower] || '';
}

function toolMarkupWhitespaceLikeLenAt(text, idx) {
  const raw = toRawString(text);
  const pos = skipToolMarkupIgnorables(raw, idx);
  if (pos < 0 || pos >= raw.length) {
    return 0;
  }
  if ([' ', '\t', '\n', '\r'].includes(raw[pos])) {
    return (pos + 1) - idx;
  }
  if (raw.startsWith('▁', pos)) {
    return (pos + '▁'.length) - idx;
  }
  const cp = raw.codePointAt(pos);
  const ch = String.fromCodePoint(cp);
  return /\s/u.test(ch) ? (pos + ch.length) - idx : 0;
}

function consumeToolMarkupPipe(raw, idx) {
  const pos = skipToolMarkupIgnorables(raw, idx);
  if (pos >= raw.length) {
    return { next: idx, ok: false };
  }
  for (const variant of ['|', '│', '∣', '❘', 'ǀ', '￨']) {
    if (raw.startsWith(variant, pos)) {
      return { next: pos + variant.length, ok: true };
    }
  }
  return { next: idx, ok: false };
}

function consumeToolMarkupClosingSlash(raw, idx) {
  const pos = skipToolMarkupIgnorables(raw, idx);
  if (pos >= raw.length) {
    return { next: idx, closing: false };
  }
  for (const variant of ['/', '／', '∕', '⁄', '⧸']) {
    if (raw.startsWith(variant, pos)) {
      return { next: pos + variant.length, closing: true };
    }
  }
  return { next: idx, closing: false };
}

function xmlTagStartDelimiterLenAt(text, idx) {
  const raw = toRawString(text);
  const pos = skipToolMarkupIgnorables(raw, idx);
  if (pos < 0 || pos >= raw.length) {
    return 0;
  }
  for (const variant of ['<', '＜', '﹤', '〈']) {
    if (raw.startsWith(variant, pos)) {
      return (pos + variant.length) - idx;
    }
  }
  return 0;
}

function xmlTagEndDelimiterLenAt(text, idx) {
  const raw = toRawString(text);
  const pos = skipToolMarkupIgnorables(raw, idx);
  if (pos < 0 || pos >= raw.length) {
    return 0;
  }
  for (const variant of ['>', '＞', '﹥', '〉']) {
    if (raw.startsWith(variant, pos)) {
      return (pos + variant.length) - idx;
    }
  }
  return 0;
}

function xmlTagEndDelimiterLenEndingAt(text, end) {
  const raw = toRawString(text);
  if (end < 0 || end >= raw.length) {
    return 0;
  }
  for (const variant of ['>', '＞', '﹥', '〉']) {
    if (end + 1 >= variant.length && raw.slice(end + 1 - variant.length, end + 1) === variant) {
      return variant.length;
    }
  }
  return 0;
}

function xmlQuotePairAt(text, idx) {
  const raw = toRawString(text);
  const pos = skipToolMarkupIgnorables(raw, idx);
  if (pos < 0 || pos >= raw.length) {
    return { close: '', len: 0 };
  }
  if (raw[pos] === '"') {
    return { close: '"', len: (pos + 1) - idx };
  }
  if (raw[pos] === "'") {
    return { close: "'", len: (pos + 1) - idx };
  }
  if (raw.startsWith('“', pos)) {
    return { close: '”', len: (pos + '“'.length) - idx };
  }
  if (raw.startsWith('‘', pos)) {
    return { close: '’', len: (pos + '‘'.length) - idx };
  }
  if (raw.startsWith('＂', pos)) {
    return { close: '＂', len: (pos + '＂'.length) - idx };
  }
  if (raw.startsWith('＇', pos)) {
    return { close: '＇', len: (pos + '＇'.length) - idx };
  }
  if (raw.startsWith('„', pos)) {
    return { close: '”', len: (pos + '„'.length) - idx };
  }
  if (raw.startsWith('‟', pos)) {
    return { close: '”', len: (pos + '‟'.length) - idx };
  }
  return { close: '', len: 0 };
}

function xmlQuoteCloseDelimiterLenAt(text, idx, close) {
  const raw = toRawString(text);
  if (!close) {
    return 0;
  }
  return raw.startsWith(close, idx) ? close.length : 0;
}

function lastIndexOfToolMarkupStartDelimiter(raw) {
  const text = toRawString(raw);
  let best = -1;
  for (const variant of ['<', '＜', '﹤', '〈']) {
    const idx = text.lastIndexOf(variant);
    if (idx > best) {
      best = idx;
    }
  }
  return best;
}

function containsXmlTagTerminator(raw) {
  const text = toRawString(raw);
  return text.includes('>') || text.includes('＞') || text.includes('﹥') || text.includes('〉');
}

function findXmlTagEnd(text, from) {
  const raw = toRawString(text);
  let quote = '';
  for (let i = Math.max(0, from || 0); i < raw.length;) {
    if (quote) {
      const closeLen = xmlQuoteCloseDelimiterLenAt(raw, i, quote);
      if (closeLen) {
        quote = '';
        i += closeLen;
        continue;
      }
      const cp = raw.codePointAt(i);
      i += cp > 0xFFFF ? 2 : 1;
      continue;
    }
    const nextQuote = xmlQuotePairAt(raw, i);
    if (nextQuote.len) {
      quote = nextQuote.close;
      i += nextQuote.len;
      continue;
    }
    const endLen = xmlTagEndDelimiterLenAt(raw, i);
    if (endLen > 0) {
      return i + endLen - 1;
    }
    const cp = raw.codePointAt(i);
    i += cp > 0xFFFF ? 2 : 1;
  }
  return -1;
}

function hasXmlTagBoundary(text, idx) {
  const pos = skipToolMarkupIgnorables(text, idx);
  if (pos >= text.length) {
    return true;
  }
  return toolMarkupWhitespaceLikeLenAt(text, pos) > 0
    || consumeToolMarkupClosingSlash(text, pos).closing
    || xmlTagEndDelimiterLenAt(text, pos) > 0;
}

function isSelfClosingXmlTag(startTag) {
  const trimmed = toRawString(startTag).trim();
  return trimmed.endsWith('/') || trimmed.endsWith('／');
}

function normalizeFullwidthASCIIChar(ch) {
  if (!ch) {
    return ch;
  }
  if (ch === '〈') {
    return '<';
  }
  if (ch === '〉') {
    return '>';
  }
  if (ch === '“' || ch === '”') {
    return '"';
  }
  if (ch === '‘' || ch === '’') {
    return "'";
  }
  const code = ch.charCodeAt(0);
  if (code >= 0xff01 && code <= 0xff5e) {
    return String.fromCharCode(code - 0xfee0);
  }
  return ch;
}

function normalizedASCIITailAt(raw, start) {
  let out = '';
  for (let i = Math.max(0, start || 0); i < raw.length; i += 1) {
    const ch = normalizeFullwidthASCIIChar(raw[i]).toLowerCase();
    if (ch.charCodeAt(0) > 0x7f) {
      break;
    }
    out += ch;
  }
  return out;
}

module.exports = {
  TOOL_MARKUP_NAMES,
  consumeToolKeyword,
  consumeToolMarkupClosingSlash,
  consumeToolMarkupLessThan,
  consumeToolMarkupPipe,
  consumeToolMarkupSeparator,
  containsXmlTagTerminator,
  findXmlTagEnd,
  foldToolKeywordRune,
  hasXmlTagBoundary,
  isSelfClosingXmlTag,
  isToolMarkupSeparator,
  isXmlTagStartDelimiter,
  lastIndexOfToolMarkupStartDelimiter,
  normalizeFullwidthASCIIChar,
  normalizedASCIITailAt,
  removeToolMarkupIgnorables,
  skipToolMarkupIgnorables,
  toolMarkupDashLenAt,
  toolMarkupEqualsLenAt,
  toolMarkupUnderscoreLenAt,
  toolMarkupWhitespaceLikeLenAt,
  xmlQuoteCloseDelimiterLenAt,
  xmlQuotePairAt,
  xmlTagEndDelimiterLenAt,
  xmlTagEndDelimiterLenEndingAt,
  xmlTagStartDelimiterLenAt,
};
