'use strict';

// Classify body, fragment, status and metadata patch paths.
const { SKIP_PATTERNS, SKIP_EXACT_PATHS } = require('../shared/deepseek-constants');
const { asString } = require('./sse_text');

function isStatusPath(pathValue) {
  return ['response/status', 'status', 'response/quasi_status', 'quasi_status'].includes(pathValue);
}

// isToolFragmentType reports whether a fragment type carries search/tool
// metadata (TOOL_SEARCH / TOOL_OPEN / ...). Its content is progress narration
// ("正在搜索图片", "已搜索到相关图片"), not the answer body, so it is routed to
// reasoning instead of visible content.
function isToolFragmentType(typeName) {
  return typeof typeName === 'string' && typeName.startsWith('TOOL_');
}

// isFragmentScopedPath reports whether a patch path targets one specific
// fragment, e.g. "response/fragments/-1" or "fragments/8". Ops nested under such
// a path are relative to that fragment: a {"p":"status","v":"FINISHED"} there
// marks the fragment finished, not the whole response. Collection appends
// ("response/fragments") and deeper paths ("fragments/-1/content") are not
// fragment-scoped array parents.
function isFragmentScopedPath(pathValue) {
  let p = asString(pathValue);
  if (p.startsWith('response/')) {
    p = p.slice('response/'.length);
  }
  if (!p.startsWith('fragments/')) {
    return false;
  }
  const rest = p.slice('fragments/'.length);
  if (!rest) {
    return false;
  }
  return rest.indexOf('/') === -1;
}

function isFinishedStatus(value) {
  return asString(value).toUpperCase() === 'FINISHED';
}

function shouldSkipPath(pathValue) {
  if (isFragmentStatusPath(pathValue)) {
    return true;
  }
  if (SKIP_EXACT_PATHS.has(pathValue)) {
    return true;
  }
  for (const p of SKIP_PATTERNS) {
    if (pathValue.includes(p)) {
      return true;
    }
  }
  return false;
}

function isFragmentStatusPath(pathValue) {
  if (!pathValue || pathValue === 'response/status') {
    return false;
  }
  return /^(?:(?:response\/)?fragments\/)?-?\d+\/status$/i.test(pathValue);
}

function isCitation(text) {
  return asString(text).trim().startsWith('[citation:');
}

module.exports = {
  isCitation,
  isFinishedStatus,
  isFragmentScopedPath,
  isFragmentStatusPath,
  isStatusPath,
  isToolFragmentType,
  shouldSkipPath,
};
