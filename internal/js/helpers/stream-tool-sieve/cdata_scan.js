'use strict';

// CDATA and ignored-section scanning. Keep original string offsets.
const { toRawString } = require('./state');
const {
  cdataOffsetIsInsideMarkdownFence,
} = require('./markdown_fence');
const {
  consumeToolKeyword,
  isToolMarkupSeparator,
  skipToolMarkupIgnorables,
  xmlTagEndDelimiterLenAt,
  xmlTagStartDelimiterLenAt,
} = require('./markup_syntax');

function skipXmlIgnoredSection(text, i) {
  if (text[i] === '"') {
    for (let end = i + 1; end < text.length; end += 1) {
      if (text[end] === '\\') { end += 1; continue; }
      if (text[end] === '"') {
        try { JSON.parse(text.slice(i, end + 1)); return { next: end + 1, advanced: true, blocked: false }; } catch (_err) { break; }
      }
      if (text.charCodeAt(end) < 32) break;
    }
  }
  const raw = toRawString(text);
  const openLen = toolCDATAOpenLenAt(raw, i);
  if (openLen > 0) {
    const end = findToolCDATAEnd(raw, i + openLen);
    if (end < 0) {
      return { advanced: false, blocked: true, next: i };
    }
    return { advanced: true, blocked: false, next: end + toolCDATACloseLenAt(raw, end) };
  }
  if (raw.startsWith('<!--', i)) {
    const end = raw.indexOf('-->', i + '<!--'.length);
    if (end < 0) {
      return { advanced: false, blocked: true, next: i };
    }
    return { advanced: true, blocked: false, next: end + '-->'.length };
  }
  return { advanced: false, blocked: false, next: i };
}

function toolCDATAOpenLenAt(text, idx) {
  const raw = toRawString(text);
  const start = skipToolMarkupIgnorables(raw, idx);
  const ltLen = xmlTagStartDelimiterLenAt(raw, start);
  if (!ltLen) {
    return 0;
  }
  let pos = start + ltLen;
  for (let skipped = 0; skipped <= 4 && pos < raw.length; skipped += 1) {
    pos = skipToolMarkupIgnorables(raw, pos);
    if (raw[pos] === '[') {
      pos += 1;
      const keyword = consumeToolKeyword(raw, pos, 'cdata');
      if (!keyword.ok) {
        return 0;
      }
      pos = skipToolMarkupIgnorables(raw, keyword.next);
      if (raw[pos] !== '[') {
        return 0;
      }
      pos += 1;
      return pos - idx;
    }
    const cp = raw.codePointAt(pos);
    if (cp === undefined) {
      return 0;
    }
    const ch = String.fromCodePoint(cp);
    if (!isToolMarkupSeparator(ch)) {
      return 0;
    }
    pos += ch.length;
  }
  return 0;
}

function toolCDATACloseLenAt(text, idx) {
  const raw = toRawString(text);
  const start = skipToolMarkupIgnorables(raw, idx);
  if (raw[start] !== ']') {
    return 0;
  }
  let pos = start + 1;
  pos = skipToolMarkupIgnorables(raw, pos);
  if (raw[pos] !== ']') {
    return 0;
  }
  pos += 1;
  const gtLen = xmlTagEndDelimiterLenAt(raw, pos);
  return gtLen ? (pos + gtLen) - idx : 0;
}

function cdataInParameter(text, from) {
  const prefix = text.slice(0, from);
  const open = Math.max(...['<', '＜', '﹤', '〈'].map(ch => prefix.lastIndexOf(ch)));
  if (open < 0) return false;
  const head = text.slice(0, open).replace(/[ \t\r\n]+$/, '');
  const start = Math.max(...['<', '＜', '﹤', '〈'].map(ch => head.lastIndexOf(ch)));
  if (start < 0) return false;
  // Resolve lazily: markup_scan imports the ignored-section scanner.
  const { scanToolMarkupTagAt } = require('./markup_scan');
  const tag = scanToolMarkupTagAt(head, start);
  return !!tag && !tag.closing && tag.name === 'parameter' && tag.end + 1 === head.length;
}

function findToolCDATAEnd(text, from) {
  const raw = toRawString(text);
  if (from < 0 || from >= raw.length) {
    return -1;
  }
  // Ordinary literals cannot borrow a later parameter's CDATA close.
  if (!cdataInParameter(raw, from)) {
    for (let i = from; i < raw.length; i += 1) if (toolCDATACloseLenAt(raw, i)) return i;
    return -1;
  }
  let firstNonFenceEnd = -1;
  // Direction 3 parity with Go: `]]>` is a hard CDATA boundary. An odd number
  // of markdown fences inside the CDATA body (e.g. an unclosed ``` opener in
  // file-writing / code parameters) makes cdataOffsetIsInsideMarkdownFence
  // report "inside fence" for the whole body and skip the only structural
  // `]]>`, so the real close would be lost. When the body carries no
  // tool-markup sample (plain code / text, not a fenced tool-call example),
  // fall back to the first structural `]]>` so the hard boundary survives.
  // When the body DOES carry tool-markup (a fenced tool-call example), keep
  // the conservative fence-aware path so a mid-stream buffer does not mistake
  // an example `]]>` for the real close.
  let firstStructuralEnd = -1;
  const bodyHasToolMarkup = toolMarkupPresent(raw.slice(from));
  for (let i = from; i < raw.length; i += 1) {
    const closeLen = toolCDATACloseLenAt(raw, i);
    if (!closeLen) {
      continue;
    }
    const end = i;
    const structural = cdataEndLooksStructural(raw, end + closeLen);
    if (structural && firstStructuralEnd < 0) {
      firstStructuralEnd = end;
    }
    if (cdataOffsetIsInsideMarkdownFence(raw.slice(from, end))) {
      i = end + closeLen - 1;
      continue;
    }
    if (structural) {
      return end;
    }
    if (firstNonFenceEnd < 0) {
      firstNonFenceEnd = end;
    }
    i = end + closeLen - 1;
  }
  if (firstNonFenceEnd >= 0) {
    return firstNonFenceEnd;
  }
  if (!bodyHasToolMarkup) {
    return firstStructuralEnd;
  }
  return -1;
}

// toolMarkupPresent (Go parity): reports whether s contains a recognizable
// tool-call markup fragment (EPSE or canonical XML). It distinguishes a fenced
// tool-call example (which must keep the fence-aware CDATA path so an example
// `]]>` is not taken for the real close) from plain code/text that merely
// carries an odd markdown fence (where the structural `]]>` really is the
// close).
function toolMarkupPresent(s) {
  const raw = toRawString(s);
  const markers = ['<|EPSE|', '<tool_calls', '<invoke', '<parameter', '<epse-'];
  for (const marker of markers) {
    if (raw.includes(marker)) {
      return true;
    }
  }
  return false;
}

function indexToolCDATAOpen(text, from = 0) {
  const raw = toRawString(text);
  for (let i = Math.max(0, from || 0); i < raw.length; i += 1) {
    if (toolCDATAOpenLenAt(raw, i)) {
      return i;
    }
  }
  return -1;
}

function findTrailingToolCDATACloseStart(text) {
  const raw = toRawString(text);
  for (let i = raw.length - 1; i >= 0; i -= 1) {
    const closeLen = toolCDATACloseLenAt(raw, i);
    if (closeLen && i + closeLen === raw.length) {
      return i;
    }
  }
  return -1;
}

function cdataEndLooksStructural(text, after) {
  const raw = toRawString(text);
  let pos = after;
  while (pos < raw.length) {
    const ch = raw[pos];
    if ([' ', '\t', '\r', '\n'].includes(ch)) {
      pos += 1;
      continue;
    }
    return raw.startsWith('</', pos) || raw.startsWith('<／', pos) || raw.startsWith('＜/', pos) || raw.startsWith('＜／', pos);
  }
  return true;
}

module.exports = {
  findToolCDATAEnd,
  findTrailingToolCDATACloseStart,
  indexToolCDATAOpen,
  skipXmlIgnoredSection,
  toolCDATACloseLenAt,
  toolCDATAOpenLenAt,
};
