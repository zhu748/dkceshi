'use strict';

// Mirrors sse.PromptEchoGuard: bind a small leading fingerprint to the actual
// generated prelude, without filtering words in ordinary answers or quotes.
function createPromptEchoGuard(prompt = '') {
  const prefixes = [];
  const text = String(prompt).trim();
  if (text.startsWith('<｜System｜>')) prefixes.push(Buffer.from(text.split('\n', 1)[0]).subarray(0, 256));
  const intro = 'Serialized conversation. Each message ends with ';
  if (text.includes(intro + '[EndMessage:')) prefixes.push(Buffer.from(intro));
  return { prefixes, pending: '', done: prefixes.length === 0, tripped: false };
}

function feedPromptEchoGuard(guard, text) {
  if (guard.tripped) return '';
  if (guard.done) return text;
  const data = guard.pending + text;
  const start = Buffer.from(data.replace(/^[ \r\n\t]+/, ''));
  if (guard.prefixes.some(prefix => start.length >= prefix.length && start.subarray(0, prefix.length).equals(prefix))) {
    guard.pending = ''; guard.tripped = true; return '';
  }
  if (Buffer.byteLength(data) <= 512 && guard.prefixes.some(prefix => start.length <= prefix.length && prefix.subarray(0, start.length).equals(start))) {
    guard.pending = data; return '';
  }
  guard.pending = ''; guard.done = true;
  return data;
}

function flushPromptEchoGuard(guard) {
  const text = guard.pending;
  guard.pending = ''; guard.done = true;
  return text;
}

module.exports = { createPromptEchoGuard, feedPromptEchoGuard, flushPromptEchoGuard };
