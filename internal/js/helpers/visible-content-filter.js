'use strict';

function contentFilterSuffix(raw, final) {
  const marker = 'CONTENT_FILTER', n = Math.min(raw.length, marker.length);
  if (raw.slice(0, n).toUpperCase() !== marker.slice(0, n)) return {};
  if (raw.length < marker.length) return { wait: !final };
  const tail = raw.slice(marker.length);
  for (const refusal of ['blocked', '你好，这个问题我暂时无法回答']) {
    if (tail.startsWith(refusal)) return { cut: true };
    if (!final && refusal.startsWith(tail)) return { wait: true };
  }
  return {};
}

function referenceDisplayUnit(raw, final) {
  if (raw[0] !== '[') return {};
  const match = /^\[(?:citation|reference):\s*\d+\]/i.exec(raw);
  if (match) return { end: match[0].length };
  if (final || raw.length > 64) return {};
  const lower = raw.toLowerCase();
  for (const prefix of ['[citation:', '[reference:']) {
    if (prefix.startsWith(lower)) return { wait: true };
    if (lower.startsWith(prefix) && /^\s*\d*$/.test(lower.slice(prefix.length))) return { wait: true };
  }
  return {};
}

module.exports = { contentFilterSuffix, referenceDisplayUnit };
