'use strict';

const { findToolMarkupTagOutsideIgnored, findDSMLTagOutsideIgnored } = require('./markup_scan');

// Inspect new fragments directly. Indexing the accumulated rope would flatten
// and copy the entire long body for every SSE fragment, even without parsing it.
function appendCapture(state, chunk) {
  state.capture += chunk;
  const progress = state.captureProgress || (state.captureProgress = { ready: true, tag: '', oversized: false });
  for (const ch of chunk) {
    if ('<＜﹤〈'.includes(ch)) {
      progress.tag = ch;
      progress.oversized = false;
      continue;
    }
    if (!progress.tag) continue;
    if (progress.tag.length < 256) progress.tag += ch;
    else progress.oversized = true;
    if (!'>＞﹥〉|｜！、'.includes(ch)) continue;
    const terminator = '>＞﹥〉'.includes(ch);
    if (progress.oversized ? terminator : findToolMarkupTagOutsideIgnored(progress.tag, 0)?.closing || findDSMLTagOutsideIgnored(progress.tag, 0)?.closing) progress.ready = true;
    if (terminator) progress.tag = '';
  }
}

function captureReady(state) {
  const ready = !state.captureProgress || state.captureProgress.ready;
  if (state.captureProgress) state.captureProgress.ready = false;
  return ready;
}

module.exports = { appendCapture, captureReady };
