'use strict';

const { needsToolFinalization } = require('./tool_output');
const { textBeforeToolBoundary } = require('../helpers/stream-tool-sieve/sieve');
const { buildInternalGoURL, buildInternalGoHeaders } = require('./http_internal');
const MAX_TEXT_BYTES = 256 * 1024;

function captureProgressText(text) {
  let bytes = Buffer.from(text || '', 'utf8');
  if (bytes.length <= MAX_TEXT_BYTES) return { text: text || '', bytes: bytes.length };
  const originalBytes = bytes.length;
  // Keep the candidate opener even after a long prose prefix. This is only a
  // progress snapshot; final validation always receives the complete source.
  const prefix = textBeforeToolBoundary(text);
  if (prefix) bytes = Buffer.from(text.slice(prefix.length), 'utf8');
  if (bytes.length <= MAX_TEXT_BYTES) return { text: bytes.toString('utf8'), bytes: originalBytes, truncated: true };
  let head = MAX_TEXT_BYTES / 2, tail = bytes.length - head;
  while ((bytes[head] & 0xc0) === 0x80) head--;
  while ((bytes[tail] & 0xc0) === 0x80) tail++;
  return { text: bytes.subarray(0, head).toString('utf8') + '\n…[truncated]…\n' + bytes.subarray(tail).toString('utf8'), bytes: originalBytes, truncated: true };
}

async function sendProgress(req, leaseID, snapshot, signal, deadline = Infinity) {
  const url = buildInternalGoURL(req);
  url.pathname = '/v1/chat/completions';
  url.searchParams.set('__stream_progress', '1');
  const response = await fetch(url.toString(), {
    method: 'POST', headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
    body: JSON.stringify({ lease_id: leaseID, ...snapshot }),
    signal: AbortSignal.any([signal, AbortSignal.timeout(Math.max(1, Math.min(5000, deadline - Date.now())))]),
  });
  try { if (!response.ok) throw new Error(`HTTP ${response.status}`); }
  finally { await response.body?.cancel(); }
}

// At most one bounded checkpoint per second. Progress is best effort and never
// starts repair or changes the public stream. Each generation has its own ID.
function createDiagnosticProgress({ enabled, req, leaseID, read, diagnostic, signal, deadline, send = sendProgress, now = Date.now }) {
  let active = false, pending = null, last = -Infinity;
  return {
    reset() { active = false; last = -Infinity; },
    observe(text, thinking, state) {
      try {
        if (!enabled || signal.aborted) return;
        active ||= needsToolFinalization(text, thinking, {}, state);
        if (!active || pending || now() - last < 1000) return;
        last = now();
        const raw = read();
        const snapshot = { ...diagnostic(), content: captureProgressText(raw.content), thinking: captureProgressText(raw.thinking) };
        pending = Promise.resolve().then(() => send(req, leaseID, snapshot, signal, deadline))
          .catch(error => { if (!signal.aborted) console.warn('[tool_diagnostics] progress failed', error.message); })
          .finally(() => { pending = null; });
      } catch (error) { console.warn('[tool_diagnostics] snapshot failed', error.message); }
    },
    async settle() { await pending; },
  };
}

module.exports = { captureProgressText, createDiagnosticProgress };
