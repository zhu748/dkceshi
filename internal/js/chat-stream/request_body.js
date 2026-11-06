'use strict';
const { streamLimit } = require('./stream_limits');

const MAX_BODY_BYTES = 100 * 1024 * 1024;

// Never access a lazy body getter: Vercel restores the raw request stream but
// its JSON helper converts numbers to Number and collapses duplicate keys.
async function readRawBody(req) {
  for (const key of ['rawBody', 'body']) {
    const value = Object.getOwnPropertyDescriptor(req, key)?.value;
    if (Buffer.isBuffer(value)) return bounded(value);
    if (typeof value === 'string') return bounded(Buffer.from(value));
  }
  // Vercel restores data/end events, while IncomingMessage's ended state and
  // async iterator still describe the already-consumed original stream.
  if (typeof req.on === 'function' && typeof req.read === 'function') return readEventBody(req);
  if (typeof req[Symbol.asyncIterator] === 'function') {
    const chunks = [];
    let size = 0;
    for await (const chunk of req) {
      const bytes = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      size += bytes.length;
      if (size > MAX_BODY_BYTES) throw bodyError(413, 'request body too large');
      chunks.push(bytes);
    }
    if (size > 0 || !Object.getOwnPropertyDescriptor(req, 'body')) return Buffer.concat(chunks, size);
  }
  throw bodyError(400, 'Raw request body unavailable; forward the original JSON bytes.');
}

function readEventBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [], subscriptions = [];
    let size = 0, settled = false;
    const cleanup = () => {
      clearTimeout(timer);
      for (const [target, name, listener] of subscriptions) target.removeListener(name, listener);
    };
    const fail = error => {
      if (settled) return;
      settled = true;
      cleanup(); reject(error);
    };
    const timer = setTimeout(() => fail(bodyError(408, 'request body read timed out')), streamLimit('connect_timeout_ms'));
    const on = (name, listener) => subscriptions.push([req.on(name, listener) || req, name, listener]);
    on('error', fail);
    on('aborted', () => fail(bodyError(400, 'request body aborted')));
    on('close', () => fail(bodyError(400, 'request body closed before completion')));
    on('data', chunk => {
      const bytes = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      size += bytes.length;
      if (size > MAX_BODY_BYTES) { fail(bodyError(413, 'request body too large')); return; }
      chunks.push(bytes);
    });
    on('end', () => {
      if (settled) return;
      settled = true;
      cleanup(); resolve(Buffer.concat(chunks, size));
    });
    // A restored stream can retain stale aborted/ended flags from its original
    // IncomingMessage; only new stream events determine its terminal state.
  });
}

function bounded(bytes) {
  if (bytes.length > MAX_BODY_BYTES) throw bodyError(413, 'request body too large');
  return bytes;
}

function bodyError(status, message) {
  return Object.assign(new Error(message), { status });
}

module.exports = { readRawBody };
