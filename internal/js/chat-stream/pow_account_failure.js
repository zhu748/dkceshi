'use strict';

function powAccountFailure(pow) {
  const error = pow?.body?.error;
  if (!['rate_limit_exceeded', 'account_banned', 'account_muted', 'account_unauthorized'].includes(error?.code)) return null;
  const options = error.code === 'rate_limit_exceeded' ? { rate_limited: true }
    : error.code === 'account_banned' ? { banned: true }
      : error.code === 'account_unauthorized' ? { auth_failed: true } : {};
  return { status: pow.status, message: error.message, code: error.code, options };
}

module.exports = { powAccountFailure };
