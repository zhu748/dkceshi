'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Readable, PassThrough } = require('node:stream');
const { IncomingMessage } = require('node:http');
const { readRawBody } = require('../../internal/js/chat-stream/http_internal');

const original = '{"messages":[{"role":"tool","content":{"id":9007199254740993,"x":"first","x":"last"}}]}';

test('raw stream bypasses parsed body and lazy JSON helpers', async () => {
  for (const getter of [true, false]) {
    const req = Readable.from([original.slice(0, 37), original.slice(37)]);
    if (getter) Object.defineProperty(req, 'body', { get() { throw new Error('must not parse'); } });
    else req.body = JSON.parse(original);
    assert.equal((await readRawBody(req)).toString(), original);
  }
});

test('Vercel restored IncomingMessage retains raw JSON', async () => {
  const req = new IncomingMessage(new PassThrough());
  req.push(Buffer.from(original)); req.push(null);
  for await (const _ of req) { /* Simulate helper prebuffering. */ }
  const restored = new PassThrough();
  const originalOn = req.on.bind(req);
  req.read = restored.read.bind(restored);
  req.on = req.addListener = (name, cb) => ['data', 'end'].includes(name) ? restored.on(name, cb) : originalOn(name, cb);
  restored.end(Buffer.from(original));
  Object.defineProperty(req, 'body', { get() { throw new Error('must not parse'); } });
  assert.equal((await readRawBody(req)).toString(), original);
});

test('raw buffers stay exact and parsed-only bodies fail explicitly', async () => {
  assert.equal((await readRawBody({ body: Buffer.from(original) })).toString(), original);
  assert.equal((await readRawBody({ rawBody: original, body: JSON.parse(original) })).toString(), original);
  await assert.rejects(readRawBody({ body: JSON.parse(original) }), err => err.status === 400);
});

test('raw body read failures release their event listeners', async () => {
  const req = new PassThrough();
  const pending = readRawBody(req);
  req.emit('error', new Error('read failed'));
  await assert.rejects(pending, /read failed/);
  for (const name of ['data', 'end', 'error', 'aborted']) assert.equal(req.listenerCount(name), 0);
});

test('request close before end rejects promptly and cleans listeners', async () => {
  const req = new PassThrough();
  const pending = readRawBody(req);
  req.write('partial JSON');
  req.destroy();
  try {
    await assert.rejects(Promise.race([
      pending,
      new Promise((_, reject) => {
        const timer = setTimeout(() => reject(new Error('request close was ignored')), 100);
        timer.unref();
      }),
    ]), err => err.status === 400);
  } finally {
    // Ensure a failing implementation does not retain its read timeout.
    req.emit('aborted');
  }
  for (const name of ['data', 'end', 'error', 'aborted', 'close']) assert.equal(req.listenerCount(name), 0);
});
