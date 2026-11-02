import test from 'node:test'
import assert from 'node:assert/strict'
import { createDiagnosticRequests } from './diagnosticRequests.js'
const response = body => ({ ok: true, json: async () => body })
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }

test('saved normalized settings supersede polling begun before the save', async () => {
    const resource = createDiagnosticRequests(), old = deferred()
    const polling = resource.list(() => old.promise, {}, new AbortController().signal)
    const saved = { success: true, revision: 2, settings: { repair: { model: 'normalized', has_api_key: true } } }
    let sent
    const result = await resource.mutate(async (_url, options) => { sent = JSON.parse(options.body); return response(saved) }, '/repair-settings', 'PUT', { model: ' normalized ', api_key: 'test-key' })
    assert.equal(result, saved)
    assert.equal(sent.api_key, 'test-key')
    assert.equal(result.settings.repair.api_key, undefined)
    old.resolve(response({ revision: 1, settings: { repair: { model: 'old' } } }))
    assert.equal(await polling, null)
    assert.equal(await resource.list(async () => response(saved), {}, new AbortController().signal), saved)
})

test('polls begun during saves and duplicate submissions cannot overwrite results', async () => {
    const resource = createDiagnosticRequests(), savingResponse = deferred(), pollResponse = deferred()
    const saving = resource.mutate(() => savingResponse.promise, '/settings', 'PUT', {})
    assert.equal(await resource.mutate(() => { throw new Error('duplicate') }, '/settings', 'PUT', {}), false)
    const polling = resource.list(() => pollResponse.promise, {}, new AbortController().signal)
    savingResponse.resolve(response({ success: true }))
    await saving
    pollResponse.resolve(response({ settings: { limit: 20 } }))
    assert.equal(await polling, null)
    assert.equal(resource.busy, false)
})

test('failed saves allow retry and ignore obsolete polling errors', async () => {
    const resource = createDiagnosticRequests(), old = deferred()
    const polling = resource.list(() => old.promise, {}, new AbortController().signal)
    await assert.rejects(resource.mutate(async () => ({ ok: false, json: async () => ({ detail: 'invalid config' }) }), '/settings', 'PUT', {}), /invalid config/)
    old.resolve({ ok: false, json: async () => ({ detail: 'obsolete error' }) })
    assert.equal(await polling, null)
    assert.equal(resource.busy, false)
    assert.deepEqual(await resource.mutate(async () => response({ success: true }), '/settings', 'PUT', {}), { success: true })
})

test('aborted polls cannot commit after their JSON body resolves', async () => {
    const resource = createDiagnosticRequests(), body = deferred(), controller = new AbortController()
    const loading = resource.list(async () => ({ ok: true, json: () => body.promise }), {}, controller.signal)
    controller.abort()
    body.resolve({ items: [] })
    assert.equal(await loading, null)
})

test('late saves after unmount or authentication changes cannot unlock a newer save', async () => {
    const resource = createDiagnosticRequests(), old = deferred(), next = deferred()
    let signal
    const first = resource.mutate((_url, options) => { signal = options.signal; return old.promise }, '/settings', 'PUT', {})
    const generation = resource.generation
    resource.cancel()
    assert.equal(signal.aborted, true)
    assert.equal(resource.isCurrent(generation), false)
    resource.activate()
    const second = resource.mutate(() => next.promise, '/settings', 'PUT', {})
    old.resolve(response({ success: true }))
    assert.equal(await first, false)
    assert.equal(resource.busy, true)
    next.resolve(response({ success: true, settings: { repair: { model: 'new' } } }))
    assert.equal((await second).settings.repair.model, 'new')
    assert.equal(resource.busy, false)
})

test('errors from cancelled saves cannot reach the next session', async () => {
    const resource = createDiagnosticRequests(), pending = deferred()
    const saving = resource.mutate(() => pending.promise, '/settings', 'PUT', {})
    resource.cancel()
    resource.activate()
    pending.resolve({ ok: false, json: async () => ({ detail: 'old session expired' }) })
    assert.equal(await saving, false)
})
