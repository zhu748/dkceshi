'use strict';

const defaults = require('../../streambudget/defaults.json');
const deployment = require('../../../vercel.json');

function streamLimit(key) {
  const raw = process.env['DS2API_STREAM_' + key.toUpperCase()];
  const value = Number(raw);
  return /^\d+$/.test(raw || '') && Number.isSafeInteger(value) && value > 0 && value <= 2 ** 30 ? value : defaults[key];
}

function resourceLimitError() {
  return Object.assign(new Error('Upstream output exceeded configured resource limits.'), { code: 'output_limit_exceeded' });
}

function createStreamWatch() {
  const maxLine = streamLimit('max_line_bytes'), maxOutput = streamLimit('max_output_bytes');
  const deployed = Boolean(process.env.VERCEL || process.env.NOW_REGION);
  const duration = deployed ? Math.min(streamLimit('max_duration_ms'), deployment.functions['api/chat-stream.js'].maxDuration * 1000) : streamLimit('max_duration_ms');
  const finalDeadline = Date.now() + duration;
  const deadline = finalDeadline - (deployed ? Math.min(20000, Math.floor(duration / 10)) : 0);
  const idleTimeout = streamLimit('idle_timeout_ms');
  let outputBytes = 0, lineBytes = 0;
  return {
    deadline, finalDeadline,
    async read(reader, { signal, countBytes = true, timeoutMs = idleTimeout } = {}) {
      let timer, onAbort;
      const cancel = () => {
        try { Promise.resolve(reader.cancel()).catch(error => console.warn('[stream_watch] cancel failed', error.message)); }
        catch (error) { console.warn('[stream_watch] cancel failed', error.message); }
      };
      const timeout = new Promise((_, reject) => {
        timer = setTimeout(() => {
          reject(Date.now() >= deadline ? resourceLimitError() : new Error('Upstream stream idle timeout.'));
          cancel();
        }, Math.max(1, Math.min(timeoutMs, idleTimeout, deadline - Date.now())));
        onAbort = () => { reject(signal.reason || new DOMException('Aborted', 'AbortError')); cancel(); };
        signal?.addEventListener('abort', onAbort, { once: true });
      });
      try {
        if (signal?.aborted) { onAbort(); return await timeout; }
        if (Date.now() >= deadline) { cancel(); throw resourceLimitError(); }
        const result = await Promise.race([reader.read(), timeout]);
        // Inspection replays its bytes; the consumer counts them exactly once.
        if (!countBytes) return result;
        const bytes = result.value || new Uint8Array();
        outputBytes += bytes.length;
        if (outputBytes > maxOutput) throw resourceLimitError();
        for (let offset = 0; offset < bytes.length;) {
          const newline = bytes.indexOf(10, offset);
          const end = newline < 0 ? bytes.length : newline + 1;
          lineBytes += end - offset;
          if (lineBytes > maxLine) throw resourceLimitError();
          if (newline >= 0) lineBytes = 0;
          offset = end;
        }
        if (result.done) lineBytes = 0;
        return result;
      } finally { clearTimeout(timer); signal?.removeEventListener('abort', onAbort); }
    },
  };
}

module.exports = { streamLimit, createStreamWatch, resourceLimitError };
