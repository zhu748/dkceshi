'use strict';

const MAX_MESSAGES = 64;
const count = value => {
  if (typeof value === 'string' && /^\d+$/.test(value.trim())) value = Number(value.trim());
  return Number.isSafeInteger(value) && value >= 0 ? value : null;
};
const fields = ['input_tokens', 'output_tokens', 'reasoning_tokens', 'total_tokens', 'accumulated_tokens'];

// Match tokenusage.Parse: inspect response metadata, never arbitrary tool JSON,
// file metadata, search content or fragment counters.
function parseUpstreamUsage(chunk) {
  const out = {};
  const field = (key, value) => {
    key = key.replace(/^usage\//, '').replace(/^token_usage\//, '');
    const aliases = { prompt_tokens: 'input_tokens', completion_tokens: 'output_tokens', accumulated_token_usage: 'accumulated_tokens', response_message_id: 'message_id', 'completion_tokens_details/reasoning_tokens': 'reasoning_tokens', 'output_tokens_details/reasoning_tokens': 'reasoning_tokens' };
    if (key === 'completion_tokens_details' || key === 'output_tokens_details') { field('reasoning_tokens', value?.reasoning_tokens); return; }
    key = aliases[key] || key;
    if (fields.includes(key) || key === 'message_id') { const n = count(value); if (n !== null) out[key] = n; }
  };
  const visit = (value, path = '', depth = 0) => {
    if (depth > 16 || !value || typeof value !== 'object') return;
    if (Array.isArray(value)) { for (const child of value) visit(child, path, depth + 1); return; }
    if (typeof value.p === 'string') {
      const p = value.p.replace(/^\/+|\/+$/g, '');
      path = p === 'response' || p.startsWith('response/') ? p : path ? `${path}/${p}` : p;
      if (path.includes('fragments')) return;
      if (['', 'response', 'usage', 'token_usage', 'response/usage', 'response/token_usage'].includes(path)) visit(value.v, path, depth + 1);
      else field(path.replace(/^response\//, ''), value.v);
      return;
    }
    if (!['', 'response', 'usage', 'token_usage', 'response/usage', 'response/token_usage'].includes(path)) return;
    for (const [key, v] of Object.entries(value)) {
      if (key === 'response') visit(v, 'response', depth + 1);
      else if (key === 'v') visit(v, path, depth + 1);
      else if (key === 'usage' || key === 'token_usage') visit(v, key, depth + 1);
      else field(key, v);
    }
  };
  visit(chunk);
  return out;
}

function createUpstreamUsage() {
  let messages = [], truncated = false;
  return {
    reset() { messages = []; truncated = false; },
    snapshot() { return { messages: messages.map(item => ({ ...item })), ...(truncated ? { truncated: true } : {}) }; },
    observe(observed) {
      if (!observed || !fields.some(key => count(observed[key]) !== null) && !(count(observed.message_id) > 0)) return;
      let index = messages.length - 1;
      if (observed.message_id > 0) while (index >= 0 && messages[index].message_id !== observed.message_id && messages[index].message_id) index--;
      if (index < 0) {
        if (messages.length >= MAX_MESSAGES) { truncated = true; return; }
        messages.push({}); index = messages.length - 1;
      }
      if (index < messages.length - 1) { const [current] = messages.splice(index, 1); messages.push(current); index = messages.length - 1; }
      const target = messages[index];
      if (count(observed.message_id) > 0) target.message_id = observed.message_id;
      for (const key of fields) {
        const n = count(observed[key]);
        if (n !== null && (target[key] === undefined || n >= target[key])) target[key] = n;
      }
    },
  };
}

function resolveUpstreamUsage(base, upstream, complete = true) {
  const usage = { ...base, completion_tokens_details: { ...base.completion_tokens_details } };
  const meta = { source: 'estimated', input_source: 'estimated', output_source: 'estimated', reasoning_source: 'estimated', total_source: 'derived', complete };
  if (upstream.messages.length) meta.upstream = upstream;
  if (upstream.messages.length === 1 && !upstream.truncated) {
    const o = upstream.messages[0];
    const consistent = o.input_tokens === undefined || o.output_tokens === undefined || o.total_tokens === undefined || Number.isSafeInteger(o.input_tokens + o.output_tokens) && o.total_tokens === o.input_tokens + o.output_tokens;
    if (consistent) {
      if (count(o.input_tokens) !== null) { usage.prompt_tokens = o.input_tokens; meta.input_source = 'upstream'; }
      if (count(o.output_tokens) !== null) { usage.completion_tokens = o.output_tokens; meta.output_source = 'upstream'; }
      if (count(o.reasoning_tokens) !== null && o.output_tokens === undefined) {
        const output = usage.completion_tokens - (usage.completion_tokens_details.reasoning_tokens || 0) + o.reasoning_tokens;
        if (count(output) !== null) { usage.completion_tokens = output; usage.completion_tokens_details.reasoning_tokens = o.reasoning_tokens; meta.output_source = 'mixed'; meta.reasoning_source = 'upstream'; }
      }
      if (count(o.reasoning_tokens) !== null && o.reasoning_tokens <= usage.completion_tokens) { usage.completion_tokens_details.reasoning_tokens = o.reasoning_tokens; meta.reasoning_source = 'upstream'; }
      else if (usage.completion_tokens_details.reasoning_tokens > usage.completion_tokens) { delete usage.completion_tokens_details.reasoning_tokens; meta.reasoning_source = 'unknown'; }
    }
  }
  if (!Number.isSafeInteger(usage.prompt_tokens + usage.completion_tokens)) {
    usage.prompt_tokens = base.prompt_tokens; usage.completion_tokens = base.completion_tokens;
    usage.completion_tokens_details = { ...base.completion_tokens_details };
    meta.input_source = meta.output_source = meta.reasoning_source = 'estimated';
  }
  usage.total_tokens = usage.prompt_tokens + usage.completion_tokens;
  if (meta.input_source === 'upstream' && meta.output_source === 'upstream') meta.source = 'upstream';
  else if (meta.input_source === 'upstream' || meta.output_source === 'upstream' || meta.output_source === 'mixed' || meta.reasoning_source === 'upstream') meta.source = 'mixed';
  return { ...usage, ds2api: meta };
}

module.exports = { count, parseUpstreamUsage, createUpstreamUsage, resolveUpstreamUsage };
