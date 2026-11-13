'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { PassThrough } = require('node:stream');
const { createLocalControl } = require('../../internal/js/chat-stream/local_control');
const { buildInternalGoURL, buildInternalGoHeaders, setInternalGoOrigin } = require('../../internal/js/chat-stream/http_internal');

function fakeChild() {
  const child = new EventEmitter();
  child.stdout = new PassThrough();
  child.stdin = new PassThrough();
  child.kill = () => { child.killed = true; child.stdout.end(); child.emit('exit', 0); };
  return child;
}

test('concurrent streams share one local process and restart only after it exits', async () => {
  const children = [];
  const control = createLocalControl({ launch: () => { const child = fakeChild(); children.push(child); return child; } });
  const a = control.start(), b = control.start();
  assert.equal(children.length, 1);
  children[0].stdout.write('startup log\n{"origin":"http://127.0.0.1:38001"}\n');
  assert.equal(await a, await b);
  assert.equal(await control.start(), 'http://127.0.0.1:38001');
  children[0].kill();
  const next = control.start();
  assert.equal(children.length, 2);
  children[1].stdout.write('{"origin":"http://127.0.0.1:38002"}\n');
  assert.equal(await next, 'http://127.0.0.1:38002');
  control.close();
});

test('a failed process never poisons the startup cache', async () => {
  let child;
  const control = createLocalControl({ launch: () => { child = fakeChild(); return child; } });
  const failed = control.start();
  child.emit('exit', 1);
  await assert.rejects(failed, /exited/);
  const restarted = control.start();
  child.stdout.write('{"origin":"http://127.0.0.1:38003"}\n');
  assert.equal(await restarted, 'http://127.0.0.1:38003');
  control.close();
});

test('invalid readiness origins time out and the failed child is killed', async () => {
  const child = fakeChild();
  const control = createLocalControl({ launch: () => child, timeoutMs: 20 });
  const pending = control.start();
  child.stdout.write('{"origin":"https://untrusted.example"}\n');
  await assert.rejects(pending, /timed out/);
  assert.equal(child.killed, true);
});

test('all control calls stay on their local owner and preserve auth and audit headers', () => {
  const req = { url: '/v1/chat/completions?key=key', headers: { host: 'deployment.example', 'x-forwarded-proto': 'https',
    'x-goog-api-key': 'google-key', 'user-agent': 'client-sdk', 'x-ds2-source': 'sdk', 'x-vercel-protection-bypass': 'bypass-secret' } };
  setInternalGoOrigin(req, 'http://127.0.0.1:38000');
  const url = buildInternalGoURL(req);
  assert.equal(url.origin, 'http://127.0.0.1:38000');
  assert.equal(url.searchParams.get('__go'), '1');
  assert.equal(url.searchParams.has('x-vercel-protection-bypass'), false);
  const headers = buildInternalGoHeaders(req);
  assert.equal(headers['x-goog-api-key'], 'google-key');
  assert.equal(headers['user-agent'], 'client-sdk');
  assert.equal(headers['x-ds2-source'], 'sdk');
  // An HTTP header cannot opt another request into this process's private origin.
  assert.equal(buildInternalGoURL({ ...req, headers: { ...req.headers, 'x-ds2-go-origin': 'http://127.0.0.1:38000' } }).origin, 'https://deployment.example');
});
