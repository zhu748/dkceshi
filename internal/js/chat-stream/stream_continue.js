'use strict';

const { asString } = require('./http_internal');
const { shouldSkipPath } = require('./sse_parse');

const { streamLimit } = require('./stream_limits');
const AUTO_CONTINUE_MAX_ROUNDS = streamLimit('auto_continue_max_rounds');

function createContinueState(sessionID) {
  return {
    sessionID: asString(sessionID),
    responseMessageID: 0,
    lastStatus: '',
    finished: false,
    sawContent: false,
    sawDone: false,
  };
}

function prepareContinueStateForNextRound(state) {
  return {
    ...state,
    lastStatus: '',
    finished: false,
    // Resume the same answer: an empty continuation cannot erase prior output.
    sawDone: false,
  };
}

function observeContinueState(state, chunk) {
  if (!state || !chunk || typeof chunk !== 'object') {
    return;
  }
  if (!state.sawContent && chunkHasVisibleContent(chunk)) {
    state.sawContent = true;
  }
  const topID = numberValue(chunk.response_message_id);
  if (topID > 0) {
    state.responseMessageID = topID;
  }
  observeContinueDirectPatch(state, chunk.p, chunk.v);
  if (chunk.p === 'response') {
    observeContinueBatchPatches(state, 'response', chunk.v);
  } else {
    observeContinueBatchPatches(state, '', chunk.v);
  }
  const response = chunk.v && typeof chunk.v === 'object' ? chunk.v.response : null;
  observeContinueResponseObject(state, response);
  const messageResponse = chunk.message && typeof chunk.message === 'object' && chunk.message.response;
  observeContinueResponseObject(state, messageResponse);
}

function observeContinueDirectPatch(state, path, value) {
  if (!state) {
    return;
  }
  switch (asString(path).trim().replace(/^\/+|\/+$/g, '')) {
    case 'response/status':
    case 'status':
    case 'response/quasi_status':
    case 'quasi_status':
      setContinueStatus(state, asString(value));
      break;
    case 'response/auto_continue':
    case 'auto_continue':
      if (value === true) {
        state.lastStatus = 'AUTO_CONTINUE';
      }
      break;
    default:
      break;
  }
}

function observeContinueResponseObject(state, response) {
  if (!state || !response || typeof response !== 'object') {
    return;
  }
  const id = numberValue(response.message_id);
  if (id > 0) {
    state.responseMessageID = id;
  }
  setContinueStatus(state, asString(response.status));
  if (response.auto_continue === true) {
    state.lastStatus = 'AUTO_CONTINUE';
  }
}

function observeContinueBatchPatches(state, parentPath, raw) {
  if (!state || !Array.isArray(raw)) {
    return;
  }
  for (const patch of raw) {
    if (!patch || typeof patch !== 'object') {
      continue;
    }
    const path = asString(patch.p).trim();
    if (!path) {
      continue;
    }
    let fullPath = path;
    const parent = asString(parentPath).trim().replace(/^\/+|\/+$/g, '');
    if (parent && !path.includes('/')) {
      fullPath = `${parent}/${path}`;
    }
    switch (fullPath.replace(/^\/+|\/+$/g, '')) {
      case 'response/status':
      case 'status':
      case 'response/quasi_status':
      case 'quasi_status':
        setContinueStatus(state, asString(patch.v));
        break;
      case 'response/auto_continue':
      case 'auto_continue':
        if (patch.v === true) {
          state.lastStatus = 'AUTO_CONTINUE';
        }
        break;
      default:
        break;
    }
  }
}

function setContinueStatus(state, status) {
  const normalized = asString(status).trim();
  if (!normalized) {
    return;
  }
  state.lastStatus = normalized;
  if (['FINISHED', 'CONTENT_FILTER'].includes(normalized.toUpperCase())) {
    state.finished = true;
  }
}

function shouldAutoContinue(state) {
  if (!state || state.finished || !state.sessionID || state.responseMessageID <= 0) {
    return false;
  }
  const status = asString(state.lastStatus).trim().toUpperCase();
  if (['INCOMPLETE', 'AUTO_CONTINUE'].includes(status)) {
    return true;
  }
  // Relaxation: the stream ended (EOF or mid-body error) without reaching a
  // terminal status but the message produced visible content — treat it as an
  // interrupted response and resume via continue. A stream that already
  // signalled completion with [DONE] is never resumed.
  return !!state.sawContent && !state.sawDone;
}

const CONTINUE_METADATA_PATHS = new Set([
  'response/status', 'status',
  'response/quasi_status', 'quasi_status',
  'response/auto_continue', 'auto_continue',
  'conversation_mode', 'accumulated_token_usage',
  'response/message_id', 'message_id',
  'request_message_id', 'response/request_message_id',
  'response/has_pending_fragment', 'has_pending_fragment',
  'response/search_status',
]);

function isContinueMetadataPath(path) {
  return CONTINUE_METADATA_PATHS.has(asString(path).trim());
}

// Mirrors the Go-side heuristic: reports whether an SSE JSON chunk carries
// visible content (text / thinking / tool fragments) as opposed to envelope,
// status or heartbeat frames.
function chunkHasVisibleContent(chunk) {
  if (!chunk || typeof chunk !== 'object') {
    return false;
  }
  const path = asString(chunk.p);
  if (path && shouldSkipPath(path)) {
    return false;
  }
  if (!('v' in chunk)) {
    return false;
  }
  const v = chunk.v;
  if (typeof v === 'string') {
    const trimmed = v.trim();
    if (!trimmed) {
      return false;
    }
    const upper = trimmed.toUpperCase();
    if (['FINISHED', 'WIP', 'INCOMPLETE', 'AUTO_CONTINUE', 'CONTENT_FILTER'].includes(upper)) {
      return false;
    }
    return true;
  }
  if (Array.isArray(v)) {
    for (const item of v) {
      if (!item || typeof item !== 'object') {
        continue;
      }
      const itemPath = asString(item.p);
      if (itemPath) {
        if (shouldSkipPath(itemPath) || isContinueMetadataPath(itemPath)) {
          continue;
        }
        if (itemPath.includes('content')) {
          return true;
        }
      }
      if (item.content !== undefined || item.v !== undefined) {
        return true;
      }
    }
    return false;
  }
  if (typeof v === 'object') {
    if (v.response !== undefined) {
      return false; // initial envelope, not content
    }
    if (asString(v.text) !== '') {
      return true;
    }
    if (asString(v.content) !== '') {
      return true;
    }
    if (Array.isArray(v.fragments) && v.fragments.length > 0) {
      return true;
    }
    return false;
  }
  return false;
}

function numberValue(v) {
  if (typeof v === 'number' && Number.isFinite(v)) {
    return Math.trunc(v);
  }
  const parsed = Number.parseInt(asString(v), 10);
  return Number.isFinite(parsed) ? parsed : 0;
}

module.exports = { createContinueState, observeContinueState, shouldAutoContinue, AUTO_CONTINUE_MAX_ROUNDS, prepareContinueStateForNextRound };
