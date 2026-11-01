'use strict';

// Mirror toolcall.LiteralState. This is a bounded scheduling hint, never a
// replacement for authoritative full-source parsing or argument validation.
class LiteralState {
  constructor() { this.tail = ''; this.mode = ''; this.quote = false; this.escaped = false; this.code = ''; this.ticks = 0; this.run = 0; this.runChar = ''; }
  feed(ch) {
    const normalized = ({ '＜': '<', '﹤': '<', '〈': '<', '＞': '>', '﹥': '>', '〉': '>', '！': '!', '、': '!' })[ch] || ch.toLowerCase();
    this.tail = (this.tail + normalized).slice(-9);
    if (this.mode) {
      if ((this.mode === 'd' && this.tail.endsWith(']]>')) || (this.mode === 'c' && this.tail.endsWith('-->'))) this.mode = '';
      return false;
    }
    if (this.quote) {
      if (this.escaped) this.escaped = false;
      else if (ch === '\\') this.escaped = true;
      else if (ch === '"') this.quote = false;
      return false;
    }
    if (this.run && ch !== this.runChar) {
      if (!this.code && (this.runChar === '`' || this.run >= 3)) { this.code = this.runChar; this.ticks = this.run; }
      else if (this.code === this.runChar && this.run >= this.ticks) { this.code = ''; this.ticks = 0; }
      this.run = 0;
    }
    if (ch === '`' || ch === '~') {
      if (this.runChar !== ch) this.run = 0;
      this.runChar = ch; this.run++; return false;
    }
    if (this.code) return false;
    if (this.tail.endsWith('<![cdata[')) { this.mode = 'd'; return false; }
    if (this.tail.endsWith('<!--')) { this.mode = 'c'; return false; }
    if (ch === '"') { this.quote = true; return false; }
    return true;
  }
}

module.exports = { LiteralState };
