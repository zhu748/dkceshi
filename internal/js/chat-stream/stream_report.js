'use strict';

const { sendFailedChunk, finishStreamFailure } = require('./stream_failure');

// One request clock spans prepare, upstream retries, validation and release.
// This is internal metadata, not an extension of any public SSE protocol.
function createStreamReport(now = Date.now) {
  const started = now(), timings = {};
  let failure = {};
  const mark = (name, replace = false) => { if (replace || timings[name] === undefined) timings[name] = Math.max(0, now() - started); };
  const setFailure = detail => { failure = { error_status: detail.status, error_code: detail.code }; };
  return {
    mark,
    snapshot(meta) { return { ...meta, ...(meta.error ? failure : {}), started_at_ms: started, elapsed_ms: Math.max(0, now() - started), timings: { ...timings } }; },
    async measure(name, action) {
      const begin = now();
      try { return await action(); }
      finally { timings[name] = (timings[name] || 0) + Math.max(0, now() - begin); }
    },
    fail(res, status, message, code) {
      setFailure({ status, code });
      sendFailedChunk(res, status, message, code);
    },
    finishFailure(res, release, detail) {
      setFailure(detail);
      return finishStreamFailure(res, release, detail);
    },
  };
}

module.exports = { createStreamReport };
