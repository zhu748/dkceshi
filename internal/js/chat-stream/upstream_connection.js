'use strict';
const { BASE_HEADERS } = require('../shared/deepseek-constants');
const { serializeAppPayload } = require('../shared/deepseek-wire');
const { asString, fetchStreamPow } = require('./http_internal');
const { powAccountFailure } = require('./pow_account_failure');
const { resourceLimitError } = require('./stream_limits');

// Credentials and PoW are adopted together after refresh and account failover.
function createUpstreamConnection({ prepared, req, leaseID, lifecycle, deadline, appFetch }) {
  const signal = deadline === undefined ? lifecycle.signal : AbortSignal.any([
    lifecycle.signal, AbortSignal.timeout(Math.max(1, deadline - Date.now())),
  ]);
  const prep = { body: prepared };
  let deepseekToken = asString(prepared.deepseek_token);
  const initialPowHeader = asString(prepared.pow_header);
  let baseHeaders = prep.body.base_headers && typeof prep.body.base_headers === 'object'
    ? { ...BASE_HEADERS, ...prep.body.base_headers }
    : { ...BASE_HEADERS };
  let headerExpirations = prep.body.base_header_expirations || {};
  const updateBaseHeaders = (switchedBody) => {
    if (switchedBody && switchedBody.base_headers && typeof switchedBody.base_headers === 'object') {
      baseHeaders = { ...BASE_HEADERS, ...switchedBody.base_headers };
      headerExpirations = switchedBody.base_header_expirations || {};
    }
  };

  let currentPowHeader = initialPowHeader;
  let lastPowFailure = null;
  const refreshPowHeader = async (roundType) => {
    lastPowFailure = null;
    if (lifecycle.closed) return '';
    try {
      const pow = await fetchStreamPow(req, leaseID, lifecycle.signal, deadline);
      if (lifecycle.closed) return '';
      const nextPowHeader = asString(pow.body && pow.body.pow_header);
      if (pow.ok && nextPowHeader) {
        deepseekToken = asString(pow.body.deepseek_token) || deepseekToken;
        updateBaseHeaders(pow.body);
        currentPowHeader = nextPowHeader;
        return currentPowHeader;
      }
      lastPowFailure = powAccountFailure(pow);
      if (lastPowFailure) return '';
      console.warn('[vercel_stream_pow] refresh failed, reusing previous PoW', {
        round_type: roundType,
        status: pow.status || 0,
      });
    } catch (err) {
      if (lifecycle.closed || lifecycle.signal.aborted) {
        return '';
      }
      console.warn('[vercel_stream_pow] refresh failed, reusing previous PoW', {
        round_type: roundType,
        error: err,
      });
    }
    return currentPowHeader;
  };

  const fetchDeepSeekStream = async (url, bodyPayload, powHeader) => {
    if (lifecycle.closed) return null;
    try {
      const response = await appFetch(url, {
        method: 'POST',
        headerExpirations,
        headers: {
          ...baseHeaders,
          authorization: `Bearer ${deepseekToken}`,
          ...(powHeader ? { 'x-ds-pow-response': powHeader } : {}),
        },
        body: serializeAppPayload(url, bodyPayload),
        signal,
      });
      if (lifecycle.closed) {
        try { await response.body?.cancel(); }
        catch (error) { console.warn('[vercel_stream] late response cancel failed', error.message); }
        return null;
      }
      return response;
    } catch (err) {
      if (lifecycle.closed || lifecycle.signal.aborted) {
        return null;
      }
      if (deadline !== undefined && Date.now() >= deadline) throw resourceLimitError();
      throw err;
    }
  };
  return {
    fetch: fetchDeepSeekStream,
    refreshPow: refreshPowHeader,
    get powHeader() { return currentPowHeader; },
    set powHeader(value) { currentPowHeader = value; },
    get powFailure() { return lastPowFailure; },
    adopt(body) {
      deepseekToken = asString(body.deepseek_token) || deepseekToken;
      currentPowHeader = asString(body.pow_header) || currentPowHeader;
      updateBaseHeaders(body);
    },
  };
}
module.exports = { createUpstreamConnection };
