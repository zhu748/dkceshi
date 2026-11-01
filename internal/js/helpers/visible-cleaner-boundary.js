'use strict';

const compactTag = text => text.replaceAll('｜', '|').replace(/\s+/g, '').toLowerCase();

function displayTagEnd(text) {
  let quote = '';
  for (let i = 1; i < text.length; i++) {
    const ch = text[i];
    if (quote) { if (ch === quote) quote = ''; continue; }
    if (ch === '"' || ch === "'") { quote = ch; continue; }
    if (ch === '>') return i + 1;
  }
  return 0;
}

function markerRun(text, marker) {
  let i = 0;
  while (i < text.length && text[i] === marker) i++;
  return i;
}

function inlineCodeEnd(text, run) {
  for (let i = run; i < text.length;) {
    if (text[i] !== '`') { i++; continue; }
    const n = markerRun(text.slice(i), '`');
    if (n === run) return i + n;
    i += n;
  }
  return 0;
}

// Use source offsets and nesting; literal closes cannot end a display wrapper.
function displayWrapperClose(text, from, open) {
  const closeTag = '</' + open.slice(1); let depth = 1;
  for (let i = from; i < text.length;) {
    const literal = displayLiteralEnd(text, i);
    if (literal > i) { i = literal; continue; }
    if (text[i] !== '<') { i++; continue; }
    const n = displayTagEnd(text.slice(i));
    if (!n) break;
    const tag = compactTag(text.slice(i, i + n));
    if (tag === open) depth++;
    if (tag === closeTag && --depth === 0) return { start: i, end: i + n };
    i += n;
  }
  return { start: -1, end: -1 };
}

function displayLiteralEnd(text, start) {
  const raw = text.slice(start);
  for (const [open, close] of [['<!--', '-->'], ['<![CDATA[', ']]>']]) {
    if (!raw.startsWith(open)) continue;
    const end = raw.indexOf(close, open.length);
    return end < 0 ? text.length : start + end + close.length;
  }
  if (raw[0] === '"') {
    for (let i = 1; i < raw.length; i++) {
      if (raw[i] === '\\') { i++; continue; }
      if (raw.charCodeAt(i) < 32) return start;
      if (raw[i] === '"') return start + i + 1;
    }
    return text.length;
  }
  if (raw[0] === '`' || raw[0] === '~') {
    const run = markerRun(raw, raw[0]);
    if (run >= 3 && displayLineStart(text, start)) return start + displayFenceEnd(raw, run);
    const end = raw[0] === '`' ? inlineCodeEnd(raw, run) : 0;
    if (end) return start + end;
    if (raw[0] === '`') return text.length;
  }
  return start;
}

function displayLineStart(text, i) {
  for (i--; i >= 0; i--) {
    if (text[i] === '\n' || text[i] === '\r') return true;
    if (text[i] !== ' ' && text[i] !== '\t') return false;
  }
  return true;
}

function displayFenceEnd(text, run) {
  for (let pos = text.indexOf('\n'); pos >= 0 && pos < text.length;) {
    pos++;
    let end = text.indexOf('\n', pos);
    if (end < 0) end = text.length;
    const line = text.slice(pos, end).replace(/^[ \t]+/, ''), n = markerRun(line, text[0]);
    if (n >= run && !line.slice(n).trim()) return end;
    if (end === text.length) break;
    pos = end;
  }
  return text.length;
}

module.exports = { compactTag, displayTagEnd, markerRun, inlineCodeEnd, displayWrapperClose };
