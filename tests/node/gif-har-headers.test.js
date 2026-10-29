'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fixture = require('../fixtures/android_app_2_7_1_gif_reuse.json');
const { buildAppHeaders } = require('../../internal/js/shared/app-transport');

test('latest GIF HAR: Node HTTP/2 header shapes match Go and observed App order', () => {
  const original = process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
  process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = '0';
  try {
    for (const shape of fixture.request_shapes) {
      const headers = {};
      for (const name of shape.header_names.filter(name => !name.startsWith(':'))) {
        headers[name] = shape.static_headers[name] || 'synthetic-test-value';
        if (name === 'content-type') headers[name] = shape.body_kind === 'multipart' ? 'multipart/form-data; boundary=fixture' : 'application/json';
      }
      Object.assign(headers, { 'x-device-id': 'local-installation', 'Accept-Language': '*', 'sec-fetch-mode': 'cors' });
      const body = shape.body_kind === 'empty' ? '' : '测试';
      const got = buildAppHeaders(`https://chat.deepseek.com${shape.path}`, { method: shape.method, headers, body });
      assert.deepEqual(Object.keys(got), shape.header_names, shape.path);
      for (const [name, value] of Object.entries(shape.static_headers)) assert.equal(got[name], value, `${shape.path}: ${name}`);
      if (got['content-length'] !== undefined) assert.equal(got['content-length'], String(Buffer.byteLength(body)));
    }
  } finally {
    if (original === undefined) delete process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
    else process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = original;
  }
});

test('device check authorization order and deterministic header case collisions', () => {
  const got = buildAppHeaders('https://chat.deepseek.com/api/v0/users/auth_token/check_device', {
    headers: { Authorization: 'old', authorization: 'new', 'x-client-platform': 'android' }, body: '{}',
  });
  assert.equal(Object.keys(got)[4], 'authorization');
  assert.equal(got.authorization, 'new');
});
