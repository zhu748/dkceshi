const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { afterEach, test } = require('node:test');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream');
const { fetchStreamSwitch } = require('../../internal/js/chat-stream/http_internal');
const { detectMutedOrBannedResponse } = require('../../internal/js/chat-stream/upstream_inspect');

const originalFetch = global.fetch;
afterEach(() => { global.fetch = originalFetch; });
const json = body => new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } });
const prepared = () => ({ model: 'model', session_id: 'session', lease_id: 'lease', deepseek_token: 'token', pow_header: 'pow', payload: { prompt: 'hello' }, final_prompt: 'hello' });
const deferred = () => {
  let resolve;
  const promise = new Promise(yes => { resolve = yes; });
  return { promise, resolve };
};
function surfaces() {
  const req = Object.assign(new EventEmitter(), { headers: { host: 'example.test' }, url: '/v1/chat/completions' });
  const res = Object.assign(new EventEmitter(), {
    writableEnded: false, destroyed: false, chunks: [],
    setHeader() {}, flushHeaders() {}, flush() {},
    write(text) { this.chunks.push(text); return true; },
    end(text) { if (text) this.write(text); this.writableEnded = true; },
  });
  return { req, res };
}
const sse = () => new Response('data: {"v":"answer"}\n\ndata: [DONE]\n\n');

test('account auth failure reaches the Go switch recovery endpoint', async () => {
  let sent;
  global.fetch = async (_url, options) => { sent = JSON.parse(String(options.body)); return json({}); };
  await fetchStreamSwitch(surfaces().req, 'lease', { disable: true, auth_failed: true });
  assert.deepEqual(sent, { lease_id: 'lease', disable: true, auth_failed: true });
  await fetchStreamSwitch(surfaces().req, 'lease', { disable: true });
  assert.deepEqual(sent, { lease_id: 'lease', disable: true });
});

test('invalid prepare data still releases a lease returned by Go', async () => {
  const { req, res } = surfaces();
  let releases = 0;
  global.fetch = async url => {
    if (String(url).includes('__stream_release')) { releases++; return json({}); }
    return json({ ...prepared(), pow_header: '' });
  };
  await handleVercelStream(req, res, Buffer.from('{}'), {}, () => assert.fail('must not call upstream'));
  assert.equal(res.statusCode, 500);
  assert.equal(releases, 1);
  assert.equal(req.listenerCount('aborted'), 0);
  assert.equal(res.listenerCount('close'), 0);
});

test('disconnect during prepare skips upstream work and releases the late lease once', async () => {
  const { req, res } = surfaces(), pending = deferred();
  let releases = 0, completions = 0;
  global.fetch = async url => {
    if (String(url).includes('__stream_prepare')) return pending.promise;
    if (String(url).includes('__stream_release')) { releases++; return json({}); }
    throw new Error('unexpected internal request');
  };
  const running = handleVercelStream(req, res, Buffer.from('{}'), {}, async () => { completions++; return sse(); });
  res.destroyed = true;
  res.emit('close');
  pending.resolve(json(prepared()));
  await running;
  assert.equal(completions, 0);
  assert.equal(releases, 1);
  assert.equal(res.chunks.length, 0);
  assert.equal(req.listenerCount('aborted'), 0);
  assert.equal(res.listenerCount('close'), 0);
});

test('an already aborted request never starts prepare', async () => {
  const { req, res } = surfaces();
  req.aborted = true;
  let requests = 0;
  global.fetch = async () => { requests++; return json(prepared()); };
  await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => sse());
  assert.equal(requests, 0);
});

test('a late upstream response is canceled after disconnect, even if cleanup fails', async t => {
  const warnings = t.mock.method(console, 'warn', () => {});
  for (const failCancel of [false, true]) {
    const { req, res } = surfaces();
    let canceled = 0, releases = 0;
    global.fetch = async url => {
      if (String(url).includes('__stream_release')) { releases++; return json({}); }
      return json(prepared());
    };
    await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => {
      res.destroyed = true;
      res.emit('close');
      return new Response(new ReadableStream({ cancel() {
        canceled++;
        if (failCancel) throw new Error('cancel failed');
      } }));
    });
    assert.equal(canceled, 1);
    assert.equal(releases, 1);
    assert.equal(req.listenerCount('aborted'), 0);
    assert.equal(res.listenerCount('close'), 0);
  }
  assert.equal(warnings.mock.calls.length, 1);
  assert.match(warnings.mock.calls[0].arguments[0], /late response cancel failed/);
});

test('disconnect during PoW refresh never starts a second completion', async () => {
  const { req, res } = surfaces();
  let completions = 0, releases = 0;
  global.fetch = async url => {
    if (String(url).includes('__stream_prepare')) return json(prepared());
    if (String(url).includes('__stream_release')) { releases++; return json({}); }
    if (String(url).includes('__stream_pow')) {
      res.destroyed = true;
      res.emit('close');
      return json({ pow_header: 'fresh-pow' });
    }
    throw new Error('unexpected internal request');
  };
  await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => { completions++; return new Response('data: [DONE]\n\n'); });
  assert.equal(completions, 1);
  assert.equal(releases, 1);
});

test('disconnect while reading a failed response skips account switching', async () => {
  const { req, res } = surfaces();
  let switches = 0, releases = 0;
  global.fetch = async url => {
    if (String(url).includes('__stream_prepare')) return json(prepared());
    if (String(url).includes('__stream_release')) { releases++; return json({}); }
    if (String(url).includes('__stream_switch')) { switches++; return json({}); }
    throw new Error('unexpected internal request');
  };
  await handleVercelStream(req, res, Buffer.from('{}'), {}, async () => {
    const response = new Response('temporary error', { status: 503 });
    response.text = async () => {
      res.destroyed = true;
      res.emit('close');
      return 'temporary error';
    };
    return response;
  });
  assert.equal(switches, 0);
  assert.equal(releases, 1);
  assert.equal(res.chunks.length, 0);
});

test('JSON response inspection releases its source reader', async () => {
  for (const body of [{ code: 1 }, { data: { biz_code: 10 } }]) {
    const response = json(body);
    await detectMutedOrBannedResponse(response);
    assert.equal(response.body.locked, false);
  }
});

test('an oversized first JSON chunk is canceled without another read', async () => {
  let reads = 0, cancels = 0, releases = 0;
  const source = { ok: true, body: { getReader: () => ({
    read: async () => ++reads === 1 ? { value: Buffer.from('{' + ' '.repeat(1 << 20)), done: false } : { done: true },
    cancel: async () => { cancels++; }, releaseLock: () => { releases++; },
  }) } };
  const result = await detectMutedOrBannedResponse(source);
  assert.ok(result.response);
  assert.equal(reads, 1);
  assert.equal(cancels, 1);
  assert.equal(releases, 1);
});

test('JSON read failure still cancels and unlocks the source', async () => {
  let reads = 0, cancels = 0, releases = 0;
  const source = { ok: true, body: { getReader: () => ({
    read: async () => { if (++reads === 1) return { value: Buffer.from('{'), done: false }; throw new Error('read failed'); },
    cancel: async () => { cancels++; }, releaseLock: () => { releases++; },
  }) } };
  await assert.rejects(detectMutedOrBannedResponse(source), /read failed/);
  assert.equal(cancels, 1);
  assert.equal(releases, 1);
});

test('continuations retain split role markers and CDATA state', async () => {
  const prefix = '<tool_calls><invoke name="write_file"><parameter name="content"><![CDATA[prefix ';
  const suffix = '[Tool]: literal]]></parameter></invoke></tool_calls>';
  for (const [first, second, isTool] of [['prefix [To', 'ol]: invented result', false], [prefix, suffix, true]]) {
    const { req, res } = surfaces();
    let validations = 0, continuations = 0;
    global.fetch = async (url, options) => {
      if (String(url).includes('__stream_release')) return json({});
      if (String(url).includes('__stream_validate')) {
        validations++;
        assert.equal(JSON.parse(options.body).content, first + second);
        return json({ tool_calls: [{ index: 0, id: 'call_safe', type: 'function', function: { name: 'write_file', arguments: '{"content":"prefix [Tool]: literal"}' } }] });
      }
      return json({ ...prepared(), tool_names: isTool ? ['write_file'] : [], validate_tools: isTool });
    };
    const upstream = async url => {
      if (String(url).includes('/continue')) {
        continuations++;
        return new Response('data: ' + JSON.stringify({ p: 'response/content', v: second }) + '\n\ndata: [DONE]\n\n');
      }
      return new Response('data: ' + JSON.stringify({ response_message_id: 92, p: 'response/content', v: first }) + '\n\n');
    };
    await handleVercelStream(req, res, Buffer.from('{}'), {}, upstream);
    const body = res.chunks.join('');
    assert.equal(continuations, 1); assert.equal(body.includes('invented result'), false);
    assert.equal(body.includes('"error"'), false);
    assert.equal(validations, isTool ? 1 : 0);
    if (isTool) assert.ok(body.includes('"tool_calls"'));
  }
});

test('prepare retains the long-operation budget for file uploads', async t => {
  const { fetchStreamPrepare } = require('../../internal/js/chat-stream/http_internal');
  const savedConnect = process.env.DS2API_STREAM_CONNECT_TIMEOUT_MS;
  const savedDuration = process.env.DS2API_STREAM_MAX_DURATION_MS;
  process.env.DS2API_STREAM_CONNECT_TIMEOUT_MS = '1';
  process.env.DS2API_STREAM_MAX_DURATION_MS = '1000';
  t.after(() => {
    if (savedConnect === undefined) delete process.env.DS2API_STREAM_CONNECT_TIMEOUT_MS;
    else process.env.DS2API_STREAM_CONNECT_TIMEOUT_MS = savedConnect;
    if (savedDuration === undefined) delete process.env.DS2API_STREAM_MAX_DURATION_MS;
    else process.env.DS2API_STREAM_MAX_DURATION_MS = savedDuration;
  });
  global.fetch = async (_url, init) => {
    await new Promise(resolve => setTimeout(resolve, 10));
    assert.equal(init.signal.aborted, false);
    return json(prepared());
  };
  assert.equal((await fetchStreamPrepare(surfaces().req, Buffer.from('{}'))).ok, true);
});
