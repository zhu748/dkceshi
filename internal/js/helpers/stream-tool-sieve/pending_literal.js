'use strict';

// Only schedules full parsing when a held literal may close. Raw input remains
// in state.pending; escaped quotes and split Unicode closes retain their state.
class PendingLiteral {
  constructor(mode) { this.mode = mode; this.escaped = false; this.tail = ''; }
  feed(text) {
    if (this.mode !== 'q') return this.feedDelimited(text);
    for (const ch of text) {
      if (this.escaped) { this.escaped = false; continue; }
      if (ch === '\\') { this.escaped = true; continue; }
      if (ch === '"' || ch.charCodeAt(0) < 32) return true;
    }
    return false;
  }

  feedDelimited(text) {
    // All closers are three UTF-16 units; only two units can cross a chunk.
    const boundary = this.tail + text.slice(0, 2);
    const closes = this.mode === 'c' ? ['-->'] : [']]>', ']]＞', ']]﹥', ']]〉'];
    if (closes.some(close => text.includes(close) || boundary.includes(close))) return true;
    this.tail = text.length >= 2 ? text.slice(-2) : (this.tail + text).slice(-2);
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
  if (mode === 'q') progress.preview = raw;
  return progress.feed(raw.slice(start)) ? null : progress;
}

function appendPendingText(state, chunk) {
  if (chunk) {
    state.pending += chunk;
    if (state.literalWait) {
      if (state.literalWait.mode === 'q' && !state.literalWait.previewBlocked) state.literalWait.preview += chunk;
      if (!state.literalWait.feed(chunk)) return true;
    }
  }
  // An empty chunk is an explicit flush and must run the full parser.
  state.literalWait = null;
  return false;
}

module.exports = { pendingLiteral, appendPendingText };
