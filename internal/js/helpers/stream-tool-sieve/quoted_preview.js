'use strict';

// Keep the original quoted source for authoritative classification. Preview
// ordinary text only; markup and incomplete control prefixes remain buffered.
function previewQuotedText(state) {
  if (state.literalWait?.mode !== 'q') return [];
  const progress = state.literalWait;
  if (progress.previewBlocked) return [];
  const emitted = state.previewEmitted || 0;
  const skipped = emitted - (progress.previewAccounted || 0);
  if (skipped > 0) progress.preview = progress.preview.slice(skipped);
  progress.previewAccounted = emitted;
  const raw = progress.preview;
  let end = 0;
  while (end < raw.length) {
    const ch = raw[end];
    if ('<＜﹤〈[`~'.includes(ch)) { progress.previewBlocked = true; break; }
    if ((ch === 'C' || ch === 'c') && 'CONTENT_FILTER'.startsWith(raw.slice(end, end + 14).toUpperCase())) {
      progress.previewBlocked = raw.length - end >= 14;
      break;
    }
    // Do not deliver half a UTF-16 surrogate pair to the SSE serializer.
    if (end + 1 === raw.length && raw.charCodeAt(end) >= 0xd800 && raw.charCodeAt(end) <= 0xdbff) break;
    end++;
  }
  const text = raw.slice(0, end);
  progress.preview = raw.slice(end);
  progress.previewAccounted += end;
  state.previewEmitted = emitted + end;
  return text ? [{ type: 'text', text, preview: true }] : [];
}

function consumePreview(state, text) {
  if (text && state.previewEmitted) {
    state.previewSkip = (state.previewSkip || 0) + state.previewEmitted;
    state.previewEmitted = 0;
  }
  const n = Math.min(state.previewSkip || 0, text.length);
  state.previewSkip = (state.previewSkip || 0) - n;
  return text.slice(n);
}

module.exports = { previewQuotedText, consumePreview };
