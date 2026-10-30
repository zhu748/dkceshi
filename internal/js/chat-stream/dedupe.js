'use strict';

const MIN_CONTINUATION_SNAPSHOT_LEN = 32;

function isContinuationSnapshot(text) {
  let count = 0;
  for (const char of text) {
    if (++count >= MIN_CONTINUATION_SNAPSHOT_LEN) return true;
  }
  return false;
}

function trimContinuationOverlap(existing, incoming) {
  if (!incoming) {
    return '';
  }
  if (!existing) {
    return incoming;
  }
  if (!isContinuationSnapshot(incoming)) return incoming;
  if (incoming.startsWith(existing)) {
    return incoming.slice(existing.length);
  }
  if (existing.startsWith(incoming)) {
    return '';
  }
  return incoming;
}

module.exports = {
  trimContinuationOverlap,
};
