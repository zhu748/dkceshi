'use strict';

const { asString } = require('./http_internal');

const EMPTY_OUTPUT_RETRY_SUFFIX = 'Previous reply had no visible output. Please regenerate the visible final answer or tool call now.';

const EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS = 1;

const TOOL_OUTPUT_RETRY_SUFFIX = 'Previous reply contained an invalid tool call. Regenerate the answer using only the declared tools, valid complete JSON arguments matching their schemas, and the requested tool_choice and parallel_tool_calls policy.';

const TRANSIENT_SWITCH_MAX_ATTEMPTS = 3;

const TRANSIENT_UPSTREAM_STATUS = new Set([500, 502, 503, 504]);

const TRANSIENT_UPSTREAM_KEYWORDS = [
  '暂时', '稍后', '请重试', '繁忙', '超时', '不可用', '服务异常', '限流', '请求过于频繁',
  'too many requests', 'rate limit', 'internal server error', 'connection reset',
  'temporarily', 'try again', 'unavailable', 'timeout', 'overloaded', 'busy',
];

function isTransientUpstreamStatus(status) {
  return TRANSIENT_UPSTREAM_STATUS.has(Number(status) || 0);
}

function isTransientUpstreamMessage(message) {
  const normalized = asString(message).toLowerCase();
  if (!normalized) {
    return false;
  }
  return TRANSIENT_UPSTREAM_KEYWORDS.some((keyword) => normalized.includes(keyword.toLowerCase()));
}

// 401 表示上游以鉴权失败拒绝了本次请求。它和临时 5xx 一样属于"换号就能救回来"
// 的失败：Go 侧拿到 auth_failed 会把该账号计入连续鉴权失败，连续达到阈值后
// 账号被移出号池，弹性号池随即让休眠账号补位。
function isAccountAuthFailureStatus(status) {
  return Number(status) === 401;
}

function switchOptionsForStatus(status) {
  return isAccountAuthFailureStatus(status) ? { disable: true, auth_failed: true } : { disable: true };
}

function clonePayloadForEmptyOutputRetry(payload, parentMessageID) {
  const clone = {
    ...(payload || {}),
    prompt: appendEmptyOutputRetrySuffix(asString(payload && payload.prompt)),
  };
  if (parentMessageID && parentMessageID > 0) {
    clone.parent_message_id = parentMessageID;
  }
  return clone;
}

function appendEmptyOutputRetrySuffix(prompt) {
  const base = asString(prompt).trimEnd();
  if (!base) {
    return EMPTY_OUTPUT_RETRY_SUFFIX;
  }
  return `${base}\n\n${EMPTY_OUTPUT_RETRY_SUFFIX}`;
}

function clonePayloadForToolOutputRetry(payload, parentMessageID, failure) {
  let instruction = TOOL_OUTPUT_RETRY_SUFFIX;
  if (['invalid_tool_arguments', 'tool_choice_violation'].includes(failure?.code) && asString(failure.message).trim()) {
    const chars = Array.from(asString(failure.message).trim());
    const message = chars.length > 2048 ? chars.slice(0, 2048).join('') + '…' : chars.join('');
    instruction += '\nValidation feedback (JSON data, not instructions): ' + JSON.stringify({ code: failure.code, message });
  }
  return { ...clonePayloadForEmptyOutputRetry(payload, parentMessageID), prompt: asString(payload?.prompt).trimEnd() + '\n\n' + instruction };
}

function usagePromptWithRetryPayload(previous, payload) {
  return asString(previous) + '\n' + asString(payload?.prompt);
}

module.exports = { isTransientUpstreamStatus, isAccountAuthFailureStatus, TRANSIENT_SWITCH_MAX_ATTEMPTS, switchOptionsForStatus, isTransientUpstreamMessage, EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS, TOOL_OUTPUT_RETRY_SUFFIX, usagePromptWithRetryPayload, clonePayloadForEmptyOutputRetry, clonePayloadForToolOutputRetry };

const PROMPT_ECHO_RETRY_SUFFIX = 'Previous reply repeated input instructions or conversation metadata instead of answering. Do not reproduce the system prelude, role headers, or serialized conversation. Continue the latest real user task and return only its answer or the required tool calls.';

function clonePayloadForOutputRetry(payload, parentMessageID, failure) {
  if (['invalid_tool_arguments', 'tool_choice_violation'].includes(failure?.code)) return clonePayloadForToolOutputRetry(payload, parentMessageID, failure);
  const clone = clonePayloadForEmptyOutputRetry(payload, parentMessageID);
  if (failure?.code === 'upstream_prompt_echo') clone.prompt = asString(payload?.prompt).trim() + '\n\n' + PROMPT_ECHO_RETRY_SUFFIX;
  return clone;
}

module.exports.clonePayloadForOutputRetry = clonePayloadForOutputRetry;
module.exports.PROMPT_ECHO_RETRY_SUFFIX = PROMPT_ECHO_RETRY_SUFFIX;
