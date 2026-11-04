'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const http2 = require('node:http2');
const { once } = require('node:events');
const { fetchAppStream, closeIdleAppSessions } = require('../../internal/js/shared/app-transport');
const { createAppSessionPool } = require('../../internal/js/shared/app-session-pool');

async function fixture(t, onStream = stream => { stream.respond({ ':status': 200 }); stream.end('ok'); }) {
  const server = http2.createServer(), sessions = new Set();
  let connections = 0;
  server.on('session', session => { connections++; sessions.add(session); session.on('error', () => {}); session.on('close', () => sessions.delete(session)); });
  server.on('stream', (stream, headers) => { stream.on('error', () => {}); onStream(stream, headers); });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  t.after(async () => { closeIdleAppSessions(); for (const session of sessions) session.destroy(); await new Promise(resolve => server.close(resolve)); });
  return { url: `http://127.0.0.1:${server.address().port}`, get connections() { return connections; } };
}

test('sequential HTTP/2 requests reuse one connection within a credential and installation', async t => {
  const f = await fixture(t);
  for (let i = 0; i < 5; i++) {
    assert.equal(await (await fetchAppStream(f.url + '/stream', { headers: { authorization: 'Bearer one', 'x-device-id': 'install-a' } })).text(), 'ok');
  }
  assert.equal(f.connections, 1);
  for (const headers of [{ authorization: 'Bearer two', 'x-device-id': 'install-a' }, { authorization: 'Bearer one', 'x-device-id': 'install-b' }]) {
    await (await fetchAppStream(f.url, { headers })).text();
  }
  assert.equal(f.connections, 3);
});

test('omitting installation headers does not merge local installations', async t => {
  const original = process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER;
  process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = '0';
  t.after(() => { if (original === undefined) delete process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER; else process.env.DS2API_DEEPSEEK_SEND_INSTALLATION_HEADER = original; });
  const f = await fixture(t);
  for (const id of ['a', 'b', 'a']) await (await fetchAppStream(f.url, { headers: { authorization: 'token', 'x-device-id': id } })).text();
  assert.equal(f.connections, 2);
});

test('exclusive leases isolate cancellation from an active sibling', async t => {
  const peers = [];
  const f = await fixture(t, stream => { peers.push(stream); stream.respond({ ':status': 200 }); stream.write('partial'); });
  const abort = new AbortController();
  const first = await fetchAppStream(f.url, { signal: abort.signal });
  const second = await fetchAppStream(f.url);
  const firstRead = first.text();
  abort.abort();
  await assert.rejects(firstRead, { name: 'AbortError' });
  peers[1].end(' done');
  assert.equal(await second.text(), 'partial done');
  assert.equal(f.connections, 2);
});

test('pool bounds idle connections and expires them without affecting active leases', async t => {
  const f = await fixture(t), pool = createAppSessionPool({ maxIdle: 1, idleMs: 15 });
  t.after(() => pool.closeIdle());
  const first = pool.take(f.url, {}), second = pool.take(f.url, {});
  await Promise.all([once(first.session, 'connect'), once(second.session, 'connect')]);
  first.release(true); second.release(true);
  assert.equal(pool.size, 1);
  assert.equal(second.session.destroyed, true);
  await once(first.session, 'close');
  assert.equal(pool.size, 0);
});
