'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fixture = require('../fixtures/android_app_2_7_1_gif_reuse.json');
const latestFixture = require('../fixtures/android_app_2_7_1_2026_11_04.json');
const loginFixture = require('../fixtures/android_app_2_7_1_login_2026_11_04.json');
const { buildAppHeaders, fetchAppStream } = require('../../internal/js/shared/app-transport');
const { once } = require('node:events');
const http2 = require('node:http2');

const captures = [{ fixture, mode: '0' }, { fixture: latestFixture, mode: '0' }, { fixture: loginFixture, mode: '' }];

function requestForShape(shape, mode) {
  const headers = Object.fromEntries(shape.header_names.filter(name => !name.startsWith(':')).map(name => [name, shape.static_headers[name] || 'synthetic-test-value']));
  Object.assign(headers, { 'Accept-Language': '*', 'sec-fetch-mode': 'cors' });
  if (mode === '0') headers['x-device-id'] = 'local-installation';
  const multipart = shape.body_kind === 'multipart';
  let body = shape.body_kind === 'empty' ? '' : '{"prompt":"测试"}';
  if (shape.body_kind === 'encrypted') body = Buffer.from([0, 1, 2, 255, 128, 3]);
  if (multipart) body = '--fixture\r\nContent-Disposition: form-data; name=file; filename=test.txt\r\nContent-Type: application/octet-stream\r\n\r\n测试\r\n--fixture--\r\n';
  if (headers['content-type']) headers['content-type'] = multipart ? 'multipart/form-data; boundary=fixture' : shape.static_headers['content-type'] || 'application/json';
  if (multipart) headers['x-file-size'] = String(Buffer.byteLength('测试'));
  return { headers, body };
}

test('current HARs: Node HTTP/2 headers cover installation header presence and omission', () => {
  const original = process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
  try {
    for (const capture of captures) {
      process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = capture.mode;
      for (const shape of capture.fixture.request_shapes) {
        const { headers, body } = requestForShape(shape, capture.mode);
        const got = buildAppHeaders(`https://chat.deepseek.com${shape.path}`, { method: shape.method, headers, body });
        assert.deepEqual(Object.keys(got), shape.header_names, shape.path);
        for (const [name, value] of Object.entries(shape.static_headers)) assert.equal(got[name], value, `${shape.path}: ${name}`);
        if (got['content-length'] !== undefined) assert.equal(got['content-length'], String(Buffer.byteLength(body)));
      }
    }
  } finally {
    if (original === undefined) delete process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
    else process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = original;
  }
});

test('installation header defaults to preserving identity and supports explicit omission', () => {
  const original = process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
  const headers = { 'x-device-id': 'local-installation' };
  try {
    for (const value of ['', '0', 'false', 'no', 'off', 'invalid', '1', 'true', 'yes', 'on', ' TRUE ']) {
      process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = value;
        const got = buildAppHeaders('https://chat.deepseek.com/api/v0/file/fetch_files', { method: 'GET', headers });
      const enabled = !['0', 'false', 'no', 'off'].includes(value);
        assert.equal(got['x-device-id'], enabled ? 'local-installation' : undefined, value);
        assert.equal(headers['x-device-id'], 'local-installation');
    }
  } finally {
    if (original === undefined) delete process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
    else process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = original;
  }
});

test('November 4 upload and login HAR shapes survive the actual Node HTTP/2 sender', async () => {
  const original = process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
  const sessions = new Set();
  const observed = [];
  const server = http2.createServer();
  server.on('session', session => { sessions.add(session); session.on('close', () => sessions.delete(session)); });
  server.on('stream', (stream, headers) => {
    const chunks = [];
    stream.on('data', chunk => chunks.push(chunk));
    stream.on('end', () => {
      observed.push({ headers, body: Buffer.concat(chunks) });
      stream.respond({ ':status': 200 });
      stream.end(headers[':method'] === 'HEAD' ? undefined : 'ok');
    });
  });
  server.listen(0, '127.0.0.1');
  try {
    await once(server, 'listening');
    for (const capture of captures.slice(1)) {
      process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = capture.mode;
      const headShape = { ...capture.fixture.request_shapes.find(shape => shape.method === 'GET'), method: 'HEAD' };
      for (const shape of [...capture.fixture.request_shapes, headShape]) {
        const { headers, body } = requestForShape(shape, capture.mode);
        const response = await fetchAppStream(`http://127.0.0.1:${server.address().port}${shape.path}`, { method: shape.method, headers, body });
        assert.equal(await response.text(), shape.method === 'HEAD' ? '' : 'ok');
        const got = observed.at(-1);
        assert.deepEqual(Object.keys(got.headers), shape.header_names, shape.path);
        for (const [name, value] of Object.entries(shape.static_headers)) assert.equal(got.headers[name], value, `${shape.path}: ${name}`);
        assert.deepEqual(got.body, Buffer.from(body));
        if (got.headers['content-length'] !== undefined) assert.equal(got.headers['content-length'], String(Buffer.byteLength(body)));
      }
    }
  } finally {
    for (const session of sessions) session.destroy();
    await new Promise(resolve => server.close(resolve));
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
