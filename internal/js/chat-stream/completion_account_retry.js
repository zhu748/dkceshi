'use strict';

const { detectMutedOrBannedResponse } = require('./upstream_inspect');
const { readUpstreamError } = require('./upstream_body');
const { isTransientUpstreamStatus, isAccountAuthFailureStatus, switchOptionsForStatus, TRANSIENT_SWITCH_MAX_ATTEMPTS } = require('./stream_retry_policy');

// Inspect every completion, including regenerated and replacement-account
// responses. A JSON account error must never enter the SSE parser as silence.
async function fetchAccountCompletion(payload, hooks) {
  let current = payload, transientAttempts = 0;
  while (!hooks.closed()) {
    let failure, transient = false, terminal = false;
    try {
      const response = await hooks.fetch(current);
      if (!response || hooks.closed()) return null;
      const detection = await detectMutedOrBannedResponse(response, hooks.read);
      if (hooks.closed()) return null;
      if (detection.response) {
        const inspected = detection.response;
        if (inspected.ok && inspected.body) return inspected;
        const status = inspected.ok ? 502 : inspected.status;
        transient = isTransientUpstreamStatus(status) || isAccountAuthFailureStatus(status);
        failure = { status, code: isAccountAuthFailureStatus(status) ? 'account_unauthorized' : transient ? 'upstream_unavailable' : 'upstream_error',
          message: await readUpstreamError(inspected, hooks.read) || `Upstream returned HTTP ${status}.`, options: switchOptionsForStatus(status) };
        terminal = !transient;
      } else failure = detection.rateLimited
      ? { status: 429, code: 'rate_limit_exceeded', message: 'Upstream requests are rate limited.', options: { rate_limited: true } }
      : detection.unauthorized
        ? { status: 401, code: 'account_unauthorized', message: 'Account token is invalid.', options: { auth_failed: true } }
      : detection.banned
        ? { status: 403, code: 'account_banned', message: 'Account is banned by upstream.', options: { banned: true, banned_reason: '账户已被停用' } }
        : { status: 403, code: 'account_muted', message: 'Account is muted by upstream.', options: { mute_until: detection.mutedUntil } };
    } catch (error) {
      if (hooks.closed()) return null;
      const limited = error.code === 'output_limit_exceeded';
      failure = { status: limited ? 502 : 503, code: limited ? error.code : 'upstream_unavailable',
        message: limited ? error.message : 'Failed to get completion.', options: {} };
      terminal = limited;
      transient = true;
      console.warn('[vercel_stream_completion] upstream transport failed', error.message);
    }
    if (hooks.closed()) return null;
    if (terminal) { await hooks.fail(failure); return null; }
    const budget = !transient || hooks.budgetManaged === true || transientAttempts < TRANSIENT_SWITCH_MAX_ATTEMPTS;
    let switched;
    try { switched = budget && hooks.canSwitch() ? await hooks.switch(failure.options) : null; }
    catch (error) { console.warn('[vercel_stream_completion] recovery failed', error.message); }
    if (hooks.closed()) return null;
    if (!(switched?.ok && switched.body?.payload && typeof switched.body.payload === 'object')) {
      await hooks.fail(failure);
      return null;
    }
    if (transient) transientAttempts++;
    hooks.adopt(switched);
    current = switched.body.payload;
  }
  return null;
}

module.exports = { fetchAccountCompletion };
