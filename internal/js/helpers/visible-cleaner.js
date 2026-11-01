'use strict';

const { cleanDisplayUnit } = require('./visible-cleaner-tokens');
const { markerRun, inlineCodeEnd } = require('./visible-cleaner-boundary');
const { newPendingScan } = require('./visible-cleaner-pending');
const { contentFilterSuffix, referenceDisplayUnit } = require('./visible-content-filter');

// Display-only cleanup. Keep the Go StreamCleaner contract: no tool execution,
// stable results across chunk partitions, and immediate fenced-code delivery.
class VisibleCleaner {
  constructor(stripReferences = false) { this.pending = ''; this.fence = ''; this.notLineStart = false; this.stripReferences = stripReferences; }
  write(text) {
    if (this.filtered) return '';
    if (this.waiting) {
      if (!this.waiting.append(text)) return '';
      this.pending += this.waiting.parts.join(''); this.waiting = null;
    } else this.pending += text;
    const out = this.drain(false);
    if (!this.fence) this.waiting = newPendingScan(this.pending);
    return out;
  }
  flush() {
    if (this.waiting) { this.pending += this.waiting.parts.join(''); this.waiting = null; }
    return this.drain(true);
  }
  drain(final) {
    let out = '';
    const emit = (text, consumed) => {
      out += text;
      for (const ch of this.pending.slice(0, consumed)) {
        if (ch === '\n' || ch === '\r') this.notLineStart = false;
        else if (ch !== ' ' && ch !== '\t') this.notLineStart = true;
      }
      this.pending = this.pending.slice(consumed);
    };
    while (this.pending) {
      const raw = this.pending;
      if (this.fence) {
        const n = this.fencedPrefix(final);
        if (!n) break;
        emit(raw.slice(0, n), n); continue;
      }
      if (raw[0] === '`' || raw[0] === '~') {
        const run = markerRun(raw, raw[0]);
        if (run === raw.length && !final) break;
        if (!this.notLineStart && run >= 3) {
          this.fence = raw.slice(0, run); emit(this.fence, run); continue;
        }
        if (raw[0] === '`') {
          let end = inlineCodeEnd(raw, run);
          if ((!end || end === raw.length) && !final) break;
          if (!end) end = raw.length;
          emit(raw.slice(0, end), end); continue;
        }
        emit(raw.slice(0, run), run); continue;
      }
      if (this.stripReferences) {
        const reference = referenceDisplayUnit(raw, final);
        if (reference.wait) break;
        if (reference.end) { emit('', reference.end); continue; }
      }
      const unit = cleanDisplayUnit(raw, final, cleanVisible);
      if (raw[0] === 'C' || raw[0] === 'c') {
        const suffix = contentFilterSuffix(raw, final);
        if (suffix.cut) { this.pending = ''; this.filtered = true; break; }
        if (suffix.wait) break;
      }
      if (unit.wait) break;
      if (unit.end) { emit(unit.text, unit.end); continue; }
      let end = 1;
      while (end < raw.length && !'`~<["Cc'.includes(raw[end])) end++;
      emit(raw.slice(0, end), end);
    }
    return out;
  }
  fencedPrefix(final) {
    const raw = this.pending;
    if (!this.notLineStart) {
      const trimmed = raw.replace(/^[ \t]+/, '');
      const run = markerRun(trimmed, this.fence[0]);
      if ((!trimmed || run === trimmed.length) && !final) return 0;
      if (run >= this.fence.length) {
        const tail = trimmed.slice(run), end = tail.indexOf('\n');
        if (end >= 0) { if (!tail.slice(0, end).trim()) this.fence = ''; }
        else if (!tail.trim()) { if (!final) return 0; this.fence = ''; }
      }
    }
    const end = raw.indexOf('\n');
    return end >= 0 ? end + 1 : raw.length;
  }
}

function cleanVisible(text) {
  const cleaner = new VisibleCleaner();
  return cleaner.write(text) + cleaner.flush();
}

module.exports = { VisibleCleaner, cleanVisible };
