'use strict';

const { streamLimit, resourceLimitError } = require('./stream_limits');

// Also used before the SSE pump starts and while reading error bodies.
async function readResponseChunk(reader, { deadline = Date.now() + streamLimit('max_duration_ms'), signal } = {}) {
  const failure = signal?.aborted ? signal.reason || new DOMException('Aborted', 'AbortError')
    : Date.now() >= deadline ? resourceLimitError() : null;
  if (failure) {
    try { Promise.resolve(reader.cancel(failure)).catch(() => {}); } catch (_) { /* Already released. */ }
    throw failure;
  }
  let timer, onAbort;
  const interrupted = new Promise((_, reject) => {
    const stop = error => {
      reject(error);
      try { Promise.resolve(reader.cancel(error)).catch(() => {}); } catch (_) { /* Already released. */ }
    };
    onAbort = () => stop(signal.reason || new DOMException('Aborted', 'AbortError'));
    if (signal?.aborted) { onAbort(); return; }
    signal?.addEventListener('abort', onAbort, { once: true });
    timer = setTimeout(() => stop(Date.now() >= deadline ? resourceLimitError()
      : Object.assign(new Error('Upstream stream idle timeout.'), { code: 'upstream_idle_timeout' })),
    Math.max(1, Math.min(streamLimit('idle_timeout_ms'), deadline - Date.now())));
  });
  try { return await Promise.race([reader.read(), interrupted]); }
  finally { clearTimeout(timer); signal?.removeEventListener('abort', onAbort); }
}

// Ownership of the reader moves to the replay stream. Never cancel an unread
// suffix merely because JSON inspection reached its bounded lookahead limit.
function replayResponse(res, chunks, reader, options) {
  let released = false;
  const release = () => { if (!released) { released = true; reader.releaseLock(); } };
  const body = new ReadableStream({
    start(controller) { for (const chunk of chunks) controller.enqueue(chunk); },
    async pull(controller) {
      try {
        const { done, value } = await readResponseChunk(reader, options);
        if (released) return;
        if (done) { controller.close(); release(); }
        else controller.enqueue(value);
      } catch (error) { if (!released) controller.error(error); release(); }
    },
    async cancel(reason) { try { if (!released) await reader.cancel(reason); } finally { release(); } },
  });
  return new Response(body, res);
}

module.exports = { readResponseChunk, replayResponse };

async function readErrorText(response, options) {
  const reader = response.body.getReader(), chunks = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await readResponseChunk(reader, options);
      if (done) return Buffer.concat(chunks).toString('utf8');
      const remaining = 65536 - size;
      chunks.push(Buffer.from(value.subarray(0, remaining)));
      size += value.length;
      if (size > 65536) return Buffer.concat(chunks).toString('utf8') + '\n[Upstream error body truncated at 64 KiB]';
    }
  } finally {
    try { await reader.cancel(); } finally { reader.releaseLock(); }
  }
}
module.exports.readErrorText = readErrorText;
