'use strict';

const { detectMutedOrBannedResponse } = require('./upstream_inspect');

// Inspect every completion, including regenerated and replacement-account
// responses. A JSON account error must never enter the SSE parser as silence.
async function fetchAccountCompletion(payload, hooks) {
  let response = await hooks.fetch(payload);
  while (response && !hooks.closed()) {
    const detection = await detectMutedOrBannedResponse(response);
    if (hooks.closed()) return null;
    if (detection.response) return detection.response;
    const failure = detection.rateLimited
      ? { status: 429, code: 'rate_limit_exceeded', message: 'Upstream requests are rate limited.', options: { rate_limited: true } }
      : detection.unauthorized
        ? { status: 401, code: 'account_unauthorized', message: 'Account token is invalid.', options: { auth_failed: true } }
      : detection.banned
        ? { status: 403, code: 'account_banned', message: 'Account is banned by upstream.', options: { banned: true, banned_reason: '账户已被停用' } }
        : { status: 403, code: 'account_muted', message: 'Account is muted by upstream.', options: { mute_until: detection.mutedUntil } };
    const switched = hooks.canSwitch() ? await hooks.switch(failure.options) : null;
    if (hooks.closed()) return null;
    if (!(switched?.ok && switched.body?.payload && typeof switched.body.payload === 'object')) {
      await hooks.fail(failure);
      return null;
    }
    hooks.adopt(switched);
    response = await hooks.fetch(switched.body.payload);
  }
  return null;
}

module.exports = { fetchAccountCompletion };
