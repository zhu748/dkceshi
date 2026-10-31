import assert from 'node:assert/strict'
import test from 'node:test'

import { createManualTokenResource } from './manualTokenResource.js'

function deferred() {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}

function fixture() {
    let state = { open: false, account: null, token: '', saving: false }
    const changes = [], events = []
    const resource = createManualTokenResource(patch => {
        changes.push(patch)
        state = { ...state, ...patch }
    })
    const callbacks = {
        onRequired: () => events.push('required'),
        onRejected: detail => events.push(['rejected', detail]),
        onSaved: () => events.push('saved'),
        onError: () => events.push('error'),
    }
    return { resource, changes, events, callbacks, get state() { return state } }
}

test('credential lookup encodes the account and preserves manual entry on failure', async () => {
    const f = fixture()
    await f.resource.open('user+tag@example.test', async (url, options) => {
        assert.equal(url, '/admin/accounts/user%2Btag%40example.test/credential')
        assert.equal(options.signal.aborted, false)
        return { ok: true, json: async () => ({ account_username: 'display', account_password: 'password' }) }
    })
    assert.deepEqual(f.state.account, { identifier: 'user+tag@example.test', username: 'display', password: 'password' })
    for (const result of ['offline', 'rejected', 'invalid-json']) {
        await f.resource.open(result, async () => {
            if (result === 'offline') throw new Error('offline')
            return { ok: result !== 'rejected', json: async () => { throw new Error('invalid json') } }
        })
        assert.equal(f.state.open, true)
        assert.deepEqual(f.state.account, { identifier: result, username: result, password: '' })
    }
    assert.deepEqual(f.events, [])
})

test('late responses and late JSON cannot overwrite a newer account', async () => {
    for (const phase of ['response', 'json']) {
        const f = fixture(), pending = deferred()
        let signal, jsonStarted = false
        const old = f.resource.open('old', async (_url, options) => {
            signal = options.signal
            if (phase === 'response') return pending.promise
            return { ok: true, json: () => { jsonStarted = true; return pending.promise } }
        })
        await Promise.resolve()
        if (phase === 'json') assert.equal(jsonStarted, true)
        await f.resource.open('new', async () => ({ ok: true, json: async () => ({ account_password: 'new-password' }) }))
        assert.equal(signal.aborted, true)
        const before = f.changes.length
        const data = { account_username: 'old-user', account_password: 'old-password' }
        pending.resolve(phase === 'response' ? { ok: true, json: async () => data } : data)
        await old
        assert.equal(f.changes.length, before)
        assert.equal(f.state.account.identifier, 'new')
        assert.equal(f.state.account.password, 'new-password')
    }
})

test('close, unmount cancellation and supplied test credentials invalidate old lookups', async () => {
    for (const action of ['close', 'cancel', 'offer']) {
        const f = fixture(), pending = deferred()
        let signal
        const old = f.resource.open('old', (_url, options) => { signal = options.signal; return pending.promise })
        if (action === 'offer') f.resource.offer({ identifier: 'test-account', username: 'test-user', password: 'test-password' })
        else f.resource[action]()
        const before = f.changes.length
        assert.equal(signal.aborted, true)
        pending.reject(new Error('late failure'))
        await old
        assert.equal(f.changes.length, before)
        assert.deepEqual(f.events, [])
        if (action === 'close') assert.equal(f.state.account, null)
        if (action === 'offer') assert.equal(f.state.account.password, 'test-password')
        // The same resource can be used after a Strict Mode effect remount.
        await f.resource.open('reopened', async () => ({ ok: false }))
        assert.equal(f.state.account.identifier, 'reopened')
    }
})

test('saving validates input, trims tokens and closes only the completed session', async () => {
    const f = fixture()
    const unexpected = () => assert.fail('must not send invalid input')
    await f.resource.submit('token', unexpected, f.callbacks)
    f.resource.offer({ identifier: 'user@example.test' })
    await f.resource.submit('  ', unexpected, f.callbacks)
    assert.deepEqual(f.events, ['required', 'required'])
    await f.resource.submit('  supplied-token  ', async (url, options) => {
        assert.equal(url, '/admin/accounts/user%40example.test')
        assert.equal(options.method, 'PUT')
        assert.equal(options.headers['Content-Type'], 'application/json')
        assert.deepEqual(JSON.parse(options.body), { token: 'supplied-token' })
        assert.equal(f.state.saving, true)
        return { ok: true, json: async () => ({}) }
    }, f.callbacks)
    assert.deepEqual(f.state, { open: false, account: null, token: '', saving: false })
    assert.equal(f.events.at(-1), 'saved')
})

test('save errors leave the active modal editable and allow a retry', async () => {
    const f = fixture()
    f.resource.offer({ identifier: 'active' })
    for (const outcome of ['rejected', 'offline', 'invalid-json']) {
        await f.resource.submit('token', async () => {
            if (outcome === 'offline') throw new Error('offline')
            return { ok: false, json: async () => {
                if (outcome === 'invalid-json') throw new Error('invalid json')
                return { detail: 'upstream rejected token' }
            } }
        }, f.callbacks)
        assert.equal(f.state.open, true)
        assert.equal(f.state.saving, false)
    }
    assert.deepEqual(f.events, [['rejected', 'upstream rejected token'], 'error', 'error'])
})

test('stale save completion, JSON or error cannot close or change a reopened modal', async () => {
    for (const phase of ['response', 'json', 'error']) {
        const f = fixture(), pending = deferred()
        let signal, jsonStarted = false
        f.resource.offer({ identifier: 'same-account' })
        const old = f.resource.submit('old-token', async (_url, options) => {
            signal = options.signal
            if (phase !== 'json') return pending.promise
            return { ok: true, json: () => { jsonStarted = true; return pending.promise } }
        }, f.callbacks)
        await Promise.resolve()
        if (phase === 'json') assert.equal(jsonStarted, true)
        f.resource.close()
        // Reopening the same account still starts a different modal session.
        f.resource.offer({ identifier: 'same-account', password: 'current-password' })
        assert.equal(signal.aborted, true)
        const before = f.changes.length
        if (phase === 'error') pending.reject(new Error('late error'))
        else pending.resolve(phase === 'json' ? {} : { ok: true, json: async () => ({}) })
        await old
        assert.equal(f.changes.length, before)
        assert.equal(f.state.open, true)
        assert.equal(f.state.account.password, 'current-password')
        assert.deepEqual(f.events, [])
    }
})

test('a second submit while saving sends no duplicate mutation', async () => {
    const f = fixture(), pending = deferred()
    f.resource.offer({ identifier: 'active' })
    let calls = 0
    const fetcher = () => { calls++; return pending.promise }
    const first = f.resource.submit('token', fetcher, f.callbacks)
    await f.resource.submit('token', fetcher, f.callbacks)
    assert.equal(calls, 1)
    pending.resolve({ ok: true, json: async () => ({}) })
    await first
    assert.deepEqual(f.events, ['saved'])
})

test('a completion callback can open another session without an old finally clearing it', async () => {
    const f = fixture(), nextSave = deferred()
    let next
    f.resource.offer({ identifier: 'first' })
    await f.resource.submit('token', async () => ({ ok: true, json: async () => ({}) }), {
        ...f.callbacks,
        onSaved: () => {
            f.resource.offer({ identifier: 'second' })
            next = f.resource.submit('next-token', () => nextSave.promise, f.callbacks)
        },
    })
    assert.equal(f.state.account.identifier, 'second')
    assert.equal(f.state.saving, true)
    nextSave.resolve({ ok: true, json: async () => ({}) })
    await next
    assert.equal(f.state.open, false)
})
