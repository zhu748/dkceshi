'use strict';

const MIN_DELTA_FLUSH_CHARS = 16;
const MAX_DELTA_FLUSH_WAIT_MS = 20;

function createChatCompletionEmitter({ res, sessionID, created, model, isClosed }) {
  let firstChunkSent = false;

  const sendFrame = (obj) => {
    if (isClosed() || res.writableEnded || res.destroyed) {
      return;
    }
    res.write(`data: ${JSON.stringify(obj)}\n\n`);
    if (typeof res.flush === 'function') {
      res.flush();
    }
  };

  const sendDeltaFrame = (delta) => {
    const payloadDelta = { ...delta };
    if (!firstChunkSent) {
      payloadDelta.role = 'assistant';
      firstChunkSent = true;
    }
    sendFrame({
      id: sessionID,
      object: 'chat.completion.chunk',
      created,
      model,
      choices: [{ delta: payloadDelta, index: 0 }],
    });
  };

  return {
    sendFrame,
    sendDeltaFrame,
  };
}

function createDeltaCoalescer({ sendDeltaFrame, minFlushChars = MIN_DELTA_FLUSH_CHARS, maxFlushWaitMS = MAX_DELTA_FLUSH_WAIT_MS }) {
  let pendingField = '';
  let pendingText = '';
  let flushTimer = null;

  const clearFlushTimer = () => {
    if (flushTimer) {
      clearTimeout(flushTimer);
      flushTimer = null;
    }
  };

  const flush = () => {
    clearFlushTimer();
    if (!pendingField || !pendingText) {
      return;
    }
    const delta = { [pendingField]: pendingText };
    pendingField = '';
    pendingText = '';
    sendDeltaFrame(delta);
  };

  const scheduleFlush = () => {
    if (flushTimer || maxFlushWaitMS <= 0) {
      return;
    }
    flushTimer = setTimeout(flush, maxFlushWaitMS);
    if (typeof flushTimer.unref === 'function') {
      flushTimer.unref();
    }
  };

  const append = (field, text) => {
    if (!field || !text) {
      return;
    }
    if (pendingField && pendingField !== field) {
      flush();
    }
    pendingField = field;
    pendingText += text;
    // Spreading the whole pending string per append allocated an array on
    // every delta; count code points with an early exit at the threshold
    // instead (surrogate pairs are folded into a single point).
    if (codePointCountAtLeast(pendingText, minFlushChars)) {
      flush();
      return;
    }
    scheduleFlush();
  };

  return {
    append,
    flush,
  };
}

// codePointCountAtLeast reports whether str contains at least min code points.
// It stops as soon as the threshold is reached, so the coalescer's 16-char
// check costs at most ~17 iterations instead of a full array spread.
function codePointCountAtLeast(str, min) {
  if (!str || min <= 0) {
    return !str ? false : true;
  }
  let count = 0;
  for (let i = 0; i < str.length; i++) {
    const code = str.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff && i + 1 < str.length) {
      const lo = str.charCodeAt(i + 1);
      if (lo >= 0xdc00 && lo <= 0xdfff) {
        i++;
      }
    }
    count++;
    if (count >= min) {
      return true;
    }
  }
  return false;
}

module.exports = {
  createChatCompletionEmitter,
  createDeltaCoalescer,
};
