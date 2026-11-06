'use strict';

const { consumeToolMarkupLessThan } = require('./markup_syntax');

// Failed delimiter-run probes share their suffixes. Keep the last opener so
// an embedded CDATA/comment boundary is still visited by the literal scanner.
function nextToolMarkupProbe(text, start) {
  if (text.charCodeAt(start) < 128 && text[start] !== '<') return start + 1;
  let last = start;
  let open = consumeToolMarkupLessThan(text, start);
  while (open.ok) {
    const next = consumeToolMarkupLessThan(text, open.next);
    if (!next.ok) break;
    last = open.next;
    open = next;
  }
  return last > start ? last : start + 1;
}

module.exports = { nextToolMarkupProbe };
