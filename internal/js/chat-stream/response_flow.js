'use strict';

const { streamLimit } = require('./stream_limits');

// Pause upstream consumption when the response socket cannot accept more data.
function waitForDrain(res) {
  if (!res.writableNeedDrain) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const cleanup = () => {
      clearTimeout(timer);
      res.removeListener('drain', drain); res.removeListener('close', close); res.removeListener('error', failed);
    };
    const drain = () => { cleanup(); resolve(); };
    const failed = error => { cleanup(); reject(error); };
    const close = () => failed(Object.assign(new Error('Response connection closed.'), { name: 'AbortError' }));
    const timer = setTimeout(() => {
      res.destroy?.();
      failed(Object.assign(new Error('Response write timed out.'), { name: 'AbortError' }));
    }, streamLimit('connect_timeout_ms'));
    res.once('drain', drain); res.once('close', close); res.once('error', failed);
    if (res.destroyed || res.writableEnded) close();
    else if (!res.writableNeedDrain) drain();
  });
}

function startHeartbeat(res) {
  const timer = setInterval(() => {
    if (res.destroyed || res.writableEnded || res.writableNeedDrain) return;
    try { res.write(': keep-alive\n\n'); res.flush?.(); }
    catch { res.destroy?.(); }
  }, streamLimit('keepalive_ms'));
  timer.unref?.();
  return () => clearInterval(timer);
}

module.exports = { startHeartbeat, waitForDrain };
