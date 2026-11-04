'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fixture = require('../fixtures/android_app_2_7_1.json');
const latestFixture = require('../fixtures/android_app_2_7_1_2026_11_04.json');
const { BASE_HEADERS } = require('../../internal/js/shared/deepseek-constants');
const { serializeAppPayload } = require('../../internal/js/shared/deepseek-wire');

test('Node static headers and an explicitly configured device match the HAR', () => {
  const headers = Object.fromEntries(Object.entries(BASE_HEADERS).map(([key, value]) => [key.toLowerCase(), value]));
  for (const [key, value] of Object.entries(fixture.client)) {
    if (key !== 'x-device-model') assert.equal(headers[key], value, key);
  }
  assert.equal(headers['x-device-model'], undefined);
  const api = require('../../internal/js/shared/deepseek-constants').__test;
  const client = api.normalizeClient({ device_model: fixture.client['x-device-model'] });
  assert.equal(api.buildBaseHeaders({}, client)['x-device-model'], fixture.client['x-device-model']);
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

test('Node constants keep login SDK ID out of the installation header', () => {
 const api=require('../../internal/js/shared/deepseek-constants').__test;
 const client=api.normalizeClient({name:'DeepSeek',version:'2.7.1',device_id:'B-risk-sdk',installation_id:'android-installation'});
 assert.equal(api.buildBaseHeaders({},client)['x-device-id'],'android-installation');
 const onlyRisk=api.normalizeClient({device_id:'B-risk-sdk'});
 assert.equal(api.buildBaseHeaders({},onlyRisk)['x-device-id'],undefined);
});

test('November 4 completion variants preserve captured ordering and turn semantics', () => {
 const shape = latestFixture.request_shapes.find(shape => shape.path === '/api/v0/chat/completion');
 for (const variant of latestFixture.completion_variants) {
  const payload = { action: variant.action, model_type: 'default', preempt: variant.preempt,
   search_enabled: variant.search_enabled, thinking_enabled: variant.thinking_enabled,
   ref_file_ids: Array.from({ length: variant.ref_count }, (_, i) => `synthetic-file-${i}`),
   prompt: '测试\u2028<内容>', parent_message_id: variant.parent_is_null ? null : 42, chat_session_id: 'synthetic-session' };
  const encoded = serializeAppPayload('https://chat.deepseek.com/api/v0/chat/completion', payload);
  const actual = JSON.parse(encoded);
  assert.deepEqual(Object.keys(actual), shape.body_keys);
  for (const key of ['thinking_enabled', 'search_enabled', 'audio_id', 'preempt', 'model_type', 'action']) assert.equal(actual[key], variant[key], key);
  assert.equal(actual.ref_file_ids.length, variant.ref_count);
  assert.equal(actual.prompt, payload.prompt);
  assert.equal(payload.model_type, 'default');
 }
});
