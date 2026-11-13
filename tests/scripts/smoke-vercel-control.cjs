'use strict';
const assert = require('node:assert/strict');
const { createLocalControl } = require('../../internal/js/chat-stream/local_control');
process.env.DS2API_CONFIG_JSON = '{}';
process.env.DS2API_ENV_WRITEBACK = '';
process.env.DS2API_VERCEL_INTERNAL_SECRET = 'control-smoke-test-only';
const control = createLocalControl();
(async () => {
  try {
    const [first, second] = await Promise.all([control.start(), control.start()]);
    assert.equal(first, second);
    const health = await fetch(`${first}/healthz`);
    assert.equal(health.status, 200);
    await health.body.cancel();
    const released = await fetch(`${second}/v1/chat/completions?__stream_release=1`, {
      method: 'POST', headers: { 'content-type': 'application/json', 'x-ds2-internal-token': 'control-smoke-test-only' },
      body: JSON.stringify({ lease_id: 'not-a-real-lease' }),
    });
    assert.equal(released.status, 404);
    await released.body.cancel();
    const unauthenticated = await fetch(`${second}/v1/chat/completions?__stream_prepare=1`, {
      method: 'POST', headers: { 'content-type': 'application/json' }, body: '{}',
    });
    assert.equal(unauthenticated.status, 401);
    await unauthenticated.body.cancel();
    console.log('Local control smoke passed: one owner, live HTTP server, protected control routes');
  } finally { control.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
