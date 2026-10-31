'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { createContentPolicy, feedContentPolicy, flushContentPolicy, contentPolicyCut } = require('../../internal/js/chat-stream/content_policy');
const { clonePayloadForOutputRetry, PROMPT_ECHO_RETRY_SUFFIX } = require('../../internal/js/chat-stream/stream_retry_policy');

const first = '<｜System｜>custom instruction';
const intro = 'Serialized conversation. Each message ends with ';
const prompt = first + '\n\n' + intro + '[EndMessage:0123456789abcdef].';

test('prompt echo is withheld across every delta size', () => {
  for (const prefix of [first, intro]) {
    const input = '\n\n' + prefix + '\nnot an answer';
    for (let size = 1; size <= input.length; size++) {
      const policy = createContentPolicy(prompt);
      let out = '';
      for (let i = 0; i < input.length; i += size) out += feedContentPolicy(policy, input.slice(i, i + size));
      out += flushContentPolicy(policy);
      assert.equal(out, '', `size=${size}`);
      assert.equal(contentPolicyCut(policy), 'prompt_echo');
    }
  }
});

test('ordinary answers, quotes, different instructions and unconfirmed EOF tails survive', () => {
  for (const input of ['answer', '<｜System｜>different instruction', '<｜System', 'Serialized conversation.', '\n\n', '`' + first + '`', '```text\n' + first + '\n```', 'The input contains ' + first, ' '.repeat(1024)]) {
    for (let size = 1; size <= input.length; size++) {
      const policy = createContentPolicy(prompt);
      let out = '';
      for (let i = 0; i < input.length; i += size) out += feedContentPolicy(policy, input.slice(i, i + size));
      out += flushContentPolicy(policy) + flushContentPolicy(policy);
      assert.equal(out, input, `size=${size}`);
      assert.equal(contentPolicyCut(policy), '');
    }
  }
});

test('prompt echo retry retains request fields, parent and precise feedback', () => {
  const original = { prompt, chat_session_id: 'session', thinking_enabled: true, ref_file_ids: ['history'] };
  const retry = clonePayloadForOutputRetry(original, 17, { code: 'upstream_prompt_echo' });
  assert.equal(retry.prompt, prompt + '\n\n' + PROMPT_ECHO_RETRY_SUFFIX);
  assert.equal(retry.parent_message_id, 17);
  assert.deepEqual(retry.ref_file_ids, ['history']);
  assert.equal(original.prompt, prompt);
});
