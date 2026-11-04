'use strict';
const { readErrorText } = require('./response_read');
const { isTransientUpstreamStatus, isAccountAuthFailureStatus, TRANSIENT_SWITCH_MAX_ATTEMPTS, switchOptionsForStatus } = require('./stream_retry_policy');
const { writeOpenAIError } = require('./error_shape');

async function resolveInitialResponse(response, { res, readOptions, prepared, switchAccount, adopt, fetchCompletion, closed }) {
  let attempts = 0;
  while (!response.ok || !response.body) {
    const status = response.ok ? 500 : response.status || 500;
    const detail = response.body ? await readErrorText(response, readOptions) : '';
    if (closed()) return null;
    if ((isTransientUpstreamStatus(status) || isAccountAuthFailureStatus(status))
      && (prepared.failover_budget_managed === true || attempts < TRANSIENT_SWITCH_MAX_ATTEMPTS)) {
      const switched = await switchAccount(switchOptionsForStatus(status));
      if (closed()) return null;
      if (switched?.ok && switched.body?.payload && typeof switched.body.payload === 'object') {
        attempts++;
        adopt(switched);
        response = await fetchCompletion(switched.body.payload);
        if (!response) return null;
        continue;
      }
    }
    writeOpenAIError(res, status, detail);
    return null;
  }
  return response;
}
module.exports = { resolveInitialResponse };
