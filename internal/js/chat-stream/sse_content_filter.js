'use strict';

// Upstream content-filter status and leaked suffix handling.
const { asString } = require('./sse_text');

function hasContentFilterStatus(chunk) {
  if (!chunk || typeof chunk !== 'object') {
    return false;
  }
  const code = asString(chunk.code);
  if (code && code.toLowerCase() === 'content_filter') {
    return true;
  }
  return hasContentFilterStatusValue(chunk);
}

function hasContentFilterStatusValue(v) {
  if (Array.isArray(v)) {
    for (const item of v) {
      if (hasContentFilterStatusValue(item)) {
        return true;
      }
    }
    return false;
  }
  if (!v || typeof v !== 'object') {
    return false;
  }
  const pathValue = asString(v.p);
  if (pathValue && pathValue.toLowerCase().includes('status')) {
    if (asString(v.v).toLowerCase() === 'content_filter') {
      return true;
    }
  }
  if (asString(v.code).toLowerCase() === 'content_filter') {
    return true;
  }
  for (const value of Object.values(v)) {
    if (hasContentFilterStatusValue(value)) {
      return true;
    }
  }
  return false;
}

module.exports = {
  hasContentFilterStatus,
};
