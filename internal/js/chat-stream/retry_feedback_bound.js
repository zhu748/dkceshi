'use strict';

// Keep work and allocation proportional to the feedback budget, rather than
// materializing every code point of a large rejected file/tool argument.
function boundRetryFeedback(text, limit, keepTail) {
  const prefix = [];
  let truncated = false;
  for (const character of text) {
    if (prefix.length === limit) { truncated = true; break; }
    prefix.push(character);
  }
  if (!truncated) return { text, truncated: false };
  if (!keepTail) return { text: prefix.join('') + '…', truncated: true };
  const separator = '\n…[truncated]…\n';
  const head = Math.floor(limit / 2), tail = limit - head - Array.from(separator).length;
  let offset = text.length;
  for (let i = 0; i < tail; i++) {
    offset--;
    const low = text.charCodeAt(offset), high = text.charCodeAt(offset - 1);
    if (low >= 0xdc00 && low <= 0xdfff && high >= 0xd800 && high <= 0xdbff) offset--;
  }
  return { text: prefix.slice(0, head).join('') + separator + text.slice(offset), truncated: true };
}

module.exports = { boundRetryFeedback };
