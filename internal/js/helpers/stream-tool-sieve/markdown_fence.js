'use strict';

// Markdown fence and inline-code boundaries; no tool grammar dependency.
const { toRawString } = require('./state');

function markdownCodeSpanEnd(text, start) {
  const raw = toRawString(text);
  if (start < 0 || start >= raw.length || raw[start] !== '`') {
    return { ok: false, end: start };
  }
  const count = countLeadingChars(raw, start, '`');
  if (!count) {
    return { ok: false, end: start };
  }
  let search = start + count;
  while (search < raw.length) {
    if (raw[search] !== '`') {
      search += 1;
      continue;
    }
    const run = countLeadingChars(raw, search, '`');
    if (run === count) {
      return { ok: true, end: search + run };
    }
    search += run;
  }
  return { ok: false, end: start };
}

function countLeadingChars(text, start, ch) {
  let count = 0;
  while (start + count < text.length && text[start + count] === ch) {
    count += 1;
  }
  return count;
}

function parseFenceOpenLine(trimmed) {
  if (trimmed.length < 3) return null;
  const ch = trimmed[0];
  if (ch !== '`' && ch !== '~') return null;
  let count = 0;
  while (count < trimmed.length && trimmed[count] === ch) count++;
  if (count < 3) return null;
  return { ch, count };
}

function isFenceCloseLine(trimmed, fenceChar, fenceLen) {
  if (!fenceChar || !trimmed || trimmed[0] !== fenceChar) return false;
  let count = 0;
  while (count < trimmed.length && trimmed[count] === fenceChar) count++;
  if (count < fenceLen) return false;
  return trimmed.slice(count).trim() === '';
}

function cdataOffsetIsInsideMarkdownFence(fragment) {
  const lines = toRawString(fragment).split('\n');
  let inFence = false;
  let fenceChar = '';
  let fenceLen = 0;
  for (const line of lines) {
    const trimmed = line.replace(/^[ \t]+/, '');
    if (!inFence) {
      const fence = parseFenceOpenLine(trimmed);
      if (fence) {
        inFence = true;
        fenceChar = fence.ch;
        fenceLen = fence.count;
      }
      continue;
    }
    if (isFenceCloseLine(trimmed, fenceChar, fenceLen)) {
      inFence = false;
      fenceChar = '';
      fenceLen = 0;
    }
  }
  return inFence;
}

// insideFencedCodeAt mirrors Go insideFencedCode: report whether idx sits
// inside a fenced code block of text.
function insideFencedCodeAt(text, idx) {
  if (!text || idx <= 0 || idx >= text.length) {
    return false;
  }
  let inFence = false;
  let fenceChar = '';
  let fenceLen = 0;
  let lineStart = 0;
  while (lineStart <= idx) {
    const nl = text.indexOf('\n', lineStart);
    const lineEnd = nl < 0 ? text.length : nl + 1;
    if (idx < lineEnd) {
      return inFence;
    }
    const line = text.slice(lineStart, nl < 0 ? text.length : nl);
    const trimmed = line.replace(/^[ \t]+/, '');
    if (!inFence) {
      const fence = parseFenceOpenLine(trimmed);
      if (fence) {
        inFence = true;
        fenceChar = fence.ch;
        fenceLen = fence.count;
      }
    } else if (isFenceCloseLine(trimmed, fenceChar, fenceLen)) {
      inFence = false;
      fenceChar = '';
      fenceLen = 0;
    }
    lineStart = lineEnd;
  }
  return false;
}

module.exports = {
  cdataOffsetIsInsideMarkdownFence,
  insideFencedCodeAt,
  isFenceCloseLine,
  markdownCodeSpanEnd,
  parseFenceOpenLine,
};
