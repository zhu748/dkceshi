'use strict';

const defaults = require('../../streambudget/defaults.json');

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
  const deadline = Date.now() + streamLimit('max_duration_ms');
  const idleTimeout = streamLimit('idle_timeout_ms');
  let outputBytes = 0, lineBytes = 0;
  return {
    async read(reader) {
      let timer;
      const timeout = new Promise((_, reject) => {
        timer = setTimeout(() => {
          reject(Date.now() >= deadline ? resourceLimitError() : new Error('Upstream stream idle timeout.'));
          try { Promise.resolve(reader.cancel()).catch(error => console.warn('[stream_watch] cancel failed', error.message)); }
          catch (error) { console.warn('[stream_watch] cancel failed', error.message); }
        }, Math.max(1, Math.min(idleTimeout, deadline - Date.now())));
      });
      try {
        if (Date.now() >= deadline) throw resourceLimitError();
        const result = await Promise.race([reader.read(), timeout]);
        for (const byte of result.value || []) {
          outputBytes++; lineBytes++;
          if (outputBytes > maxOutput || lineBytes > maxLine) throw resourceLimitError();
          if (byte === 10) lineBytes = 0;
        }
        if (result.done) lineBytes = 0;
        return result;
      } finally { clearTimeout(timer); }
    },
  };
}

module.exports = { streamLimit, createStreamWatch, resourceLimitError };
