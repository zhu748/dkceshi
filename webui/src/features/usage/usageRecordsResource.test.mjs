import assert from 'node:assert/strict'
import test from 'node:test'
import { createUsageRecordsResource } from './usageRecordsResource.js'

const entry = (tokens = 10) => ({ date: '2026-11-03', model: 'model', prompt_tokens: tokens, total_tokens: tokens })
const response = (tag, tokens = 10) => ({ status: 200, ok: true, headers: new Headers(tag ? { ETag: tag } : {}), json: async () => ({ items: [entry(tokens)] }) })
function deferred() {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}
function fixture() {
    const state = { records: [], loading: true, initialized: false, error: '' }, changes = [], errors = []
    const resource = createUsageRecordsResource({
        onChange: patch => { changes.push(patch); Object.assign(state, patch) },
        onError: message => errors.push(message),
    })
    return { resource, state, changes, errors }
}

test('unchanged usage polls send the accepted ETag and never parse a 304 body', async () => {
    const f = fixture()
    await f.resource.load(async () => response('W/"v1"'), 'failed')
    const records = f.state.records
    await f.resource.load(async (_url, options) => {
        assert.equal(options.headers['If-None-Match'], 'W/"v1"')
        return { status: 304, ok: false, json: () => assert.fail('304 has no JSON body') }
    }, 'failed')
    assert.equal(f.state.records, records)
    assert.equal(f.state.initialized, true)
    assert.equal(f.state.loading, false)
    assert.deepEqual(f.errors, [])
})

for (const phase of ['response', 'json']) {
    test(`stale usage ${phase} cannot replace data or poison the next validator`, async () => {
        const f = fixture(), pending = deferred()
        const old = f.resource.load(async () => phase === 'json'
            ? { ...response('old'), json: () => pending.promise } : pending.promise, 'failed')
        await Promise.resolve()
        await f.resource.load(async () => response('new', 30), 'failed')
        pending.resolve(phase === 'json' ? { items: [entry(99)] } : response('old', 99))
        await old
        assert.equal(f.state.records[0].usage.total_tokens, 30)
        await f.resource.load(async (_url, options) => {
            assert.equal(options.headers['If-None-Match'], 'new')
            return { status: 304 }
        }, 'failed')
    })
}

test('clear cancels a pending poll and drops its validator before the next read', async () => {
    const f = fixture(), pending = deferred()
    await f.resource.load(async () => response('old'), 'failed')
    let signal
    const old = f.resource.load((_url, options) => { signal = options.signal; return pending.promise }, 'failed')
    f.resource.clear()
    assert.equal(signal.aborted, true)
    pending.resolve(response('late', 99))
    await old
    assert.deepEqual(f.state.records, [])
    await f.resource.load(async (_url, options) => {
        assert.equal(options.headers['If-None-Match'], undefined)
        return response('new', 20)
    }, 'failed')
    assert.equal(f.state.records[0].usage.total_tokens, 20)
})

test('failed reads keep the displayed snapshot and its accepted validator', async () => {
    const f = fixture()
    await f.resource.load(async () => response('accepted'), 'failed')
    const records = f.state.records
    await f.resource.load(async () => ({ ok: false, status: 503, json: async () => ({ detail: 'offline' }) }), 'failed')
    assert.equal(f.state.records, records)
    assert.deepEqual(f.errors, ['offline'])
    await f.resource.load(async (_url, options) => {
        assert.equal(options.headers['If-None-Match'], 'accepted')
        return { status: 304 }
    }, 'failed')
})

test('dispose prevents late callbacks and new requests; activation starts a new request generation', async () => {
    const f = fixture(), pending = deferred()
    const old = f.resource.load(() => pending.promise, 'failed')
    f.resource.dispose()
    const count = f.changes.length
    await f.resource.load(() => assert.fail('disposed resource requested data'), 'failed')
    f.resource.activate()
    await f.resource.load(async () => response('current', 20), 'failed')
    const latest = f.changes.length
    pending.reject(new Error('late failure'))
    await old
    assert.ok(latest > count)
    assert.equal(f.changes.length, latest)
    assert.equal(f.state.records[0].usage.total_tokens, 20)
    assert.deepEqual(f.errors, [])
})

test('a successful response without ETag clears the previous backend validator', async () => {
    const f = fixture()
    await f.resource.load(async () => response('old'), 'failed')
    await f.resource.load(async () => response('', 20), 'failed')
    await f.resource.load(async (_url, options) => {
        assert.deepEqual(options.headers, {})
        return response('new')
    }, 'failed')
})
