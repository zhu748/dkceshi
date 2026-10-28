'use strict';

const completionKeys = ['chat_session_id', 'parent_message_id', 'prompt', 'ref_file_ids', 'thinking_enabled', 'search_enabled', 'audio_id', 'preempt', 'model_type', 'action'];

function serializeAppPayload(url, payload) {
  if (new URL(url).pathname !== '/api/v0/chat/completion' || !payload || typeof payload !== 'object' || Array.isArray(payload)) {
    return JSON.stringify(payload);
  }
  const values = { ...payload };
  if (!Object.hasOwn(values, 'audio_id')) values.audio_id = null;
  if (values.parent_message_id != null) values.model_type = null;
  const result = {};
  for (const key of completionKeys) {
    if (Object.hasOwn(values, key)) result[key] = values[key];
  }
  for (const key of Object.keys(values).filter((key) => !Object.hasOwn(result, key)).sort()) result[key] = values[key];
  return JSON.stringify(result);
}

module.exports = { serializeAppPayload };
