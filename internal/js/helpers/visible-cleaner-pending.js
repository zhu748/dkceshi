'use strict';

const { displayTagEnd, compactTag } = require('./visible-cleaner-boundary');

// Keep new chunks separate until a structural boundary can finish the unit;
// indexing a growing string rope would otherwise repeatedly flatten it.
class PendingScan {
  constructor(mode) { this.mode = mode; this.parts = []; this.depth = 0; this.quoted = false; this.escaped = false; }
  append(text) {
    this.parts.push(text);
    return this.mode === 'j' ? this.feedJSON(text) : text.includes(this.mode);
  }
  feedJSON(text) {
    for (const ch of text) {
      if (this.quoted) {
        if (this.escaped) { this.escaped = false; continue; }
        if (ch === '\\') { this.escaped = true; continue; }
        if (ch === '"') { this.quoted = false; if (this.depth === 0) return true; }
        if (ch.charCodeAt(0) < 32) return true;
        continue;
      }
      if (ch === '"') this.quoted = true;
      else if (ch === '[' || ch === '{') this.depth++;
      else if (ch === ']' || ch === '}') { if (--this.depth <= 0) return true; }
    }
    return false;
  }
}

function newPendingScan(raw) {
  if (raw.length <= 4096) return null;
  let p;
  if (raw[0] === '"' || (raw[0] === '[' && raw.slice(1).trimStart().startsWith('{'))) {
    p = new PendingScan('j'); p.feedJSON(raw);
  } else if (raw[0] === '`') {
    if (raw.endsWith('`')) return null;
    p = new PendingScan('`');
  } else if (raw[0] === '<') {
    p = new PendingScan('>');
    const end = displayTagEnd(raw);
    if (end && compactTag(raw.slice(0, end)) === '<|tool|>') {
      const tail = raw.slice(end).trimStart();
      if (tail.startsWith('{')) { p.mode = 'j'; p.feedJSON(tail); }
    }
  }
  return p || null;
}

module.exports = { newPendingScan };
