'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fixture = require('../fixtures/android_app_2_7_1.json');
const { BASE_HEADERS } = require('../../internal/js/shared/deepseek-constants');
const { serializeAppPayload } = require('../../internal/js/shared/deepseek-wire');

test('Node static client profile matches the Android HAR profile', () => {
  const headers = Object.fromEntries(Object.entries(BASE_HEADERS).map(([key, value]) => [key.toLowerCase(), value]));
  for (const [key, value] of Object.entries(fixture.client)) assert.equal(headers[key], value, key);
});

test('Node completion payload follows HAR ordering and continuation values', () => {
  for (const parent of [null, 42]) {
    const payload = { model_type: 'default', prompt: 'hello <工具>', search_enabled: false, thinking_enabled: true, ref_file_ids: ['one', 'two', 'three', 'four'], preempt: false, action: null, parent_message_id: parent, chat_session_id: 'session' };
    const actual = JSON.parse(serializeAppPayload('https://chat.deepseek.com/api/v0/chat/completion', payload));
    assert.deepEqual(Object.keys(actual), fixture.requests[0].body_keys);
    assert.equal(actual.audio_id, null);
    assert.equal(actual.model_type, parent === null ? 'default' : null);
    assert.equal(actual.ref_file_ids.length, 4);
    assert.equal(payload.model_type, 'default');
  }
});
