'use strict';

// Match completionruntime.LeadingText: whitespace does not commit an attempt.
function createLeadingText() {
  let pending = '', committed = false;
  return {
    write(text) {
      if (committed) return text;
      pending += text;
      if (!text.trim()) return '';
      committed = true;
      const ready = pending;
      pending = '';
      return ready;
    },
    reset() { pending = ''; committed = false; },
  };
}

module.exports = { createLeadingText };
