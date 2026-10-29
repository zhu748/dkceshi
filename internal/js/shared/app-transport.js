'use strict';

const http2 = require('node:http2');
const { Readable } = require('node:stream');
const zlib = require('node:zlib');

const headerOrder = [
  'x-ds-pow-response', 'x-thinking-enabled', 'x-file-size', 'x-model-type',
  'x-client-platform', 'x-client-version', 'x-client-locale', 'x-client-bundle-id',
  'x-client-timezone-offset', 'x-device-model', 'x-rangers-id', 'x-device-id',
  'x-hif-dliq', 'x-hif-leim', 'x-settings-token', 'user-agent', 'authorization',
  'accept', 'accept-charset', 'content-type', 'content-length', 'accept-encoding',
];

function buildAppHeaders(url, init) {
  const parsed = new URL(url);
  const normalized = new Map(Object.entries(init.headers || {}).map(([k, v]) => [k.toLowerCase(), String(v)]));
  for (const [name, expires] of Object.entries(init.headerExpirations || {})) {
    if (Number(expires) <= Date.now() / 1000) normalized.delete(name.toLowerCase());
  }
  for (const key of ['host', 'connection', 'transfer-encoding', 'keep-alive', 'upgrade', 'accept-language', 'sec-fetch-mode']) normalized.delete(key);
  normalized.set('content-length', String(Buffer.byteLength(init.body || '')));
  if (!normalized.has('accept-encoding')) normalized.set('accept-encoding', 'gzip');
  const headers = { ':method': init.method || 'POST', ':authority': parsed.host, ':path': parsed.pathname + parsed.search, ':scheme': parsed.protocol.slice(0, -1) };
  for (const name of [...headerOrder, ...[...normalized.keys()].sort()]) {
    if (!normalized.has(name) || name.startsWith(':')) continue;
    const value = normalized.get(name);
    if (value !== '') headers[name] = value;
    normalized.delete(name);
  }
  headers[http2.sensitiveHeaders] = ['authorization', 'x-ds-pow-response', 'x-settings-token'];
  return headers;
}

// Native HTTP/2 avoids fetch's browser-only default headers. TLS is still the
// Node runtime's TLS; this does not claim to reproduce an Android ClientHello.
function fetchAppStream(url, init = {}) {
  return new Promise((resolve, reject) => {
    if (init.signal?.aborted) { reject(init.signal.reason || new DOMException('Aborted', 'AbortError')); return; }
    const parsed = new URL(url);
    const session = http2.connect(parsed.origin);
    let stream;
    let resolved = false;
    const cleanup = () => {
      init.signal?.removeEventListener('abort', abort);
      session.destroy();
    };
    const fail = (error) => {
      if (!resolved) reject(error);
      else stream?.destroy(error);
      cleanup();
    };
    const abort = () => fail(init.signal.reason || new DOMException('Aborted', 'AbortError'));
    init.signal?.addEventListener('abort', abort, { once: true });
    session.once('error', fail);
    session.once('close', () => { if (!resolved) reject(new Error('HTTP/2 session closed before response headers')); });
    session.once('connect', () => {
      try {
        stream = session.request(buildAppHeaders(url, init));
        stream.once('error', fail);
        stream.once('response', (raw) => {
          const headers = new Headers();
          for (const [key, value] of Object.entries(raw)) {
            if (!key.startsWith(':') && value !== undefined) headers.set(key, Array.isArray(value) ? value.join(', ') : String(value));
          }
          let body = stream;
          if (headers.get('content-encoding')?.toLowerCase() === 'gzip') {
            body = stream.pipe(zlib.createGunzip());
            headers.delete('content-encoding');
            headers.delete('content-length');
          }
          body.once('close', cleanup);
          body.once('error', fail);
          const status = Number(raw[':status']);
          resolved = true;
          if (status === 204 || status === 205 || status === 304) {
            body.resume();
            resolve(new Response(null, { status, headers }));
          } else {
            resolve(new Response(Readable.toWeb(body), { status, headers }));
          }
        });
        stream.end(init.body || '');
      } catch (error) { fail(error); }
    });
  });
}

module.exports = { buildAppHeaders, fetchAppStream };
