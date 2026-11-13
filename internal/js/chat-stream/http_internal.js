'use strict';
const { streamLimit } = require('./stream_limits');
const { readRawBody } = require('./request_body');
const controlOrigins = new WeakMap();

function setInternalGoOrigin(req, origin) { controlOrigins.set(req, origin); }

const {
  writeOpenAIError,
} = require('./error_shape');
const {
  setCorsHeaders,
} = require('./cors');

function header(req, key) {
  if (!req || !req.headers) {
    return '';
  }
  return asString(req.headers[key.toLowerCase()]);
}

async function requestStreamControl(req, flag, body, signal, deadline) {
  const url = buildInternalGoURL(req);
  url.searchParams.set(flag, '1');
  const upstream = await fetch(url.toString(), {
    method: 'POST',
    headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
    body,
    signal: controlSignal(flag, signal, deadline),
  });
  const text = await upstream.text();
  let decoded = {};
  try { decoded = JSON.parse(text || '{}'); } catch (_err) {}
  return {
    ok: upstream.ok, status: upstream.status,
    contentType: upstream.headers.get('content-type') || 'application/json',
    text, body: decoded,
  };
}

function controlSignal(flag, signal, deadline) {
  const limit = streamLimit(flag === '__stream_pow' ? 'connect_timeout_ms' : 'max_duration_ms');
  const timeout = AbortSignal.timeout(Math.max(1, Math.min(limit, deadline === undefined ? limit : deadline - Date.now())));
  return signal ? AbortSignal.any([signal, timeout]) : timeout;
}

function fetchStreamPrepare(req, rawBody, signal, deadline) {
  return requestStreamControl(req, '__stream_prepare', rawBody, signal, deadline);
}

function fetchStreamPow(req, leaseID, signal, deadline) {
  return requestStreamControl(req, '__stream_pow', Buffer.from(JSON.stringify({ lease_id: leaseID })), signal, deadline);
}

async function fetchStreamSwitch(req, leaseID, options = {}, signal, deadline) {
  const switchBody = { lease_id: leaseID };
  if (options.disable) {
    switchBody.disable = true;
  }
  if (options.mute_until && typeof options.mute_until === 'number' && options.mute_until > 0) {
    switchBody.mute_until = options.mute_until;
  }
  if (options.banned) {
    switchBody.banned = true;
    if (options.banned_reason) {
      switchBody.banned_reason = asString(options.banned_reason);
    }
  }
  if (options.auth_failed === true) switchBody.auth_failed = true;
  if (options.rate_limited === true) switchBody.rate_limited = true;
  return requestStreamControl(req, '__stream_switch', Buffer.from(JSON.stringify(switchBody)), signal, deadline);
}

async function fetchStreamValidation(req, leaseID, content, thinking, detectionThinking, signal, deadline, diagnostic = {}, repairTimeoutMs = 10000) {
  const url = buildInternalGoURL(req);
  url.pathname = '/v1/chat/completions';
  url.searchParams.set('__stream_validate', '1');
  const controller = new AbortController();
  const budget = Number.isFinite(repairTimeoutMs) && repairTimeoutMs > 0 ? Math.min(repairTimeoutMs, 300000) + 5000 : 15000;
  const timeout = setTimeout(() => controller.abort(), Math.max(1, Math.min(budget, deadline === undefined ? budget : deadline - Date.now())));
  try {
    const response = await fetch(url.toString(), {
      method: 'POST', headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
      body: JSON.stringify({ lease_id: leaseID, content, thinking, detection_thinking: detectionThinking, ...diagnostic }),
      signal: signal ? AbortSignal.any([signal, controller.signal]) : controller.signal,
    });
    const body = await response.json();
    return { ok: response.ok, status: response.status, body };
  } finally { clearTimeout(timeout); }
}

function relayPreparedFailure(res, prep) {
  if (prep.status === 401 && looksLikeVercelAuthPage(prep.text)) {
    writeOpenAIError(
      res,
      401,
      'Vercel Deployment Protection blocked internal prepare request. Disable protection for this deployment or set VERCEL_AUTOMATION_BYPASS_SECRET.',
    );
    return;
  }
  res.statusCode = prep.status || 500;
  res.setHeader('Content-Type', prep.contentType || 'application/json');
  if (prep.text) {
    res.end(prep.text);
    return;
  }
  writeOpenAIError(res, prep.status || 500, 'vercel prepare failed');
}

async function safeReadText(resp) {
  if (!resp) {
    return '';
  }
  try {
    const text = await resp.text();
    return text.trim();
  } catch (_err) {
    return '';
  }
}

// Fail-closed: with neither DS2API_VERCEL_INTERNAL_SECRET nor DS2API_ADMIN_KEY
// configured the Go internal endpoints are disabled, so the bridge must not
// send a publicly known constant either (it would 401 regardless).
function internalSecret() {
  return asString(process.env.DS2API_VERCEL_INTERNAL_SECRET) || asString(process.env.DS2API_ADMIN_KEY);
}

function buildInternalGoURL(req) {
  const proto = asString(header(req, 'x-forwarded-proto')) || 'https';
  const host = asString(header(req, 'host'));
  const origin = controlOrigins.get(req) || `${proto}://${host}`;
  const url = new URL(`${origin}${req.url || '/v1/chat/completions'}`);
  url.searchParams.set('__go', '1');
  const protectionBypass = controlOrigins.has(req) ? '' : resolveProtectionBypass(req);
  if (protectionBypass) {
    url.searchParams.set('x-vercel-protection-bypass', protectionBypass);
  }
  return url;
}

function buildInternalGoHeaders(req, opts = {}) {
  const headers = {
    authorization: asString(header(req, 'authorization')),
    'x-api-key': asString(header(req, 'x-api-key')),
    'x-goog-api-key': asString(header(req, 'x-goog-api-key')),
    'user-agent': asString(header(req, 'user-agent')),
    'x-ds2-source': asString(header(req, 'x-ds2-source')),
    'x-ds2-target-account': asString(header(req, 'x-ds2-target-account')),
    'x-vercel-protection-bypass': resolveProtectionBypass(req),
  };
  if (opts.withInternalToken) {
    headers['x-ds2-internal-token'] = internalSecret();
  }
  if (opts.withContentType) {
    headers['content-type'] = asString(header(req, 'content-type')) || 'application/json';
  }
  return headers;
}

function createLeaseReleaser(req, leaseID, getMeta) {
  let released = false;
  return async () => {
    if (released || !leaseID) {
      return;
    }
    released = true;
    try {
      return await releaseStreamLease(req, leaseID, getMeta ? getMeta() : undefined);
    } catch (error) {
      // Go's lease TTL remains the fallback, but cleanup failures must be visible.
      console.warn('[vercel_stream] lease release failed', error.message);
    }
  };
}

async function releaseStreamLease(req, leaseID, meta) {
  const url = buildInternalGoURL(req);
  url.searchParams.set('__stream_release', '1');
  const releaseBody = meta && typeof meta === 'object' ? { ...meta, lease_id: leaseID } : { lease_id: leaseID };
  const body = Buffer.from(JSON.stringify(releaseBody));

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 1500);
  try {
    const response = await fetch(url.toString(), {
      method: 'POST',
      headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
      body,
      signal: controller.signal,
    });
    if (!response.ok) throw new Error(`lease release failed (${response.status})`);
    return await response.json();
  } finally {
    clearTimeout(timeout);
  }
}

function resolveProtectionBypass(req) {
  const fromHeader = asString(header(req, 'x-vercel-protection-bypass'));
  if (fromHeader) {
    return fromHeader;
  }
  return asString(process.env.VERCEL_AUTOMATION_BYPASS_SECRET) || asString(process.env.DS2API_VERCEL_PROTECTION_BYPASS);
}

function looksLikeVercelAuthPage(text) {
  const body = asString(text).toLowerCase();
  if (!body) {
    return false;
  }
  return body.includes('authentication required') && body.includes('vercel');
}

function asString(v) {
  if (typeof v === 'string') {
    return v.trim();
  }
  if (Array.isArray(v)) {
    return asString(v[0]);
  }
  if (v == null) {
    return '';
  }
  return String(v).trim();
}

function isAbortError(err) {
  if (!err || typeof err !== 'object') {
    return false;
  }
  return err.name === 'AbortError' || err.code === 'ABORT_ERR';
}

module.exports = {
  setCorsHeaders,
  header,
  readRawBody,
  fetchStreamPrepare,
  fetchStreamPow,
  fetchStreamSwitch,
  fetchStreamValidation,
  relayPreparedFailure,
  safeReadText,
  buildInternalGoURL,
  setInternalGoOrigin,
  buildInternalGoHeaders,
  createLeaseReleaser,
  releaseStreamLease,
  resolveProtectionBypass,
  looksLikeVercelAuthPage,
  asString,
  isAbortError,
};
