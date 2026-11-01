'use strict';

// Only schedules full parsing when a held literal may close. Raw input remains
// in state.pending; escaped quotes and split Unicode closes retain their state.
class PendingLiteral {
  constructor(mode) { this.mode = mode; this.escaped = false; this.tail = ''; }
  feed(text) {
    for (const ch of text) {
      if (this.mode === 'q') {
        if (this.escaped) { this.escaped = false; continue; }
        if (ch === '\\') { this.escaped = true; continue; }
        if (ch === '"' || ch.charCodeAt(0) < 32) return true;
        continue;
      }
      this.tail = (this.tail + ch).slice(-3);
      if (this.mode === 'c' && this.tail === '-->') return true;
      if (this.mode === 'd' && /^\]\][>＞﹥〉]$/.test(this.tail)) return true;
    }
    return false;
  }
}

function pendingLiteral(raw) {
  let mode, start;
  if (raw.startsWith('"')) { mode = 'q'; start = 1; }
  else if (raw.startsWith('<!--')) { mode = 'c'; start = 4; }
  else if (raw.slice(0, 9).toUpperCase() === '<![CDATA[') { mode = 'd'; start = 9; }
  else return null;
  const progress = new PendingLiteral(mode);
  return progress.feed(raw.slice(start)) ? null : progress;
}

function appendPendingText(state, chunk) {
  if (chunk) {
    state.pending += chunk;
    if (state.literalWait && !state.literalWait.feed(chunk)) return true;
  }
  // An empty chunk is an explicit flush and must run the full parser.
  state.literalWait = null;
  return false;
}

module.exports = { pendingLiteral, appendPendingText };
