'use strict';

const { buildUsage, buildFinalUsage, estimateTokens } = require('./token_usage');
const { usagePromptWithRetryPayload } = require('./stream_retry_policy');

function createUsageAccounting(prepared) {
  let prompt, baseTokens, enabled, retries;
  const reset = body => {
    prompt = body.final_prompt || '';
    baseTokens = Number.isSafeInteger(body.prompt_tokens) && body.prompt_tokens >= 0 ? body.prompt_tokens : null;
    enabled = body.usage_accounting_version === 1;
    retries = [];
  };
  reset(prepared);
  return {
    reset,
    get enabled() { return enabled; },
    retry(payload) { retries.push(payload.prompt || ''); prompt = usagePromptWithRetryPayload(prompt, payload); },
    metadata() { return enabled ? { usage_accounting_version: 1, retry_prompts: retries.slice() } : {}; },
    validation() { return { retry_prompts: retries.slice() }; },
    build(thinking, content, validated) {
      if (validated) return buildFinalUsage(prompt, thinking, content, validated);
      // The release response supplies the exact shared Go count. This is the
      // bounded fallback for an old bridge or a lost release response.
      const tokens = baseTokens === null ? 0 : baseTokens + estimateTokens(retries.join('\n'));
      return buildUsage(prompt, thinking, content, 0, tokens);
    },
    authoritative(fallback, released) {
      const usage = released?.usage;
      return usage && Number.isSafeInteger(usage.prompt_tokens) && usage.prompt_tokens >= 0
        && Number.isSafeInteger(usage.completion_tokens) && usage.completion_tokens >= 0
        ? { ...usage, total_tokens: usage.prompt_tokens + usage.completion_tokens } : fallback;
    },
  };
}

module.exports = { createUsageAccounting };
