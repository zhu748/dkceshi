import assert from 'node:assert/strict'
import test from 'node:test'

import { createAdminSession, readStoredAdminSession, saveStoredAdminSession } from './adminSession.js'
import { createLatestRequest } from '../utils/latestRequest.js'

function storage(token, expiry) {
    const items = new Map(token ? [['ds2api_token', token], ['ds2api_token_expires', String(expiry)]] : [])
    return { getItem: (key) => items.get(key) ?? null, removeItem: (key) => items.delete(key),
        setItem: (key, value) => items.set(key, value) }
}

function deferred() {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}

test('stored sessions keep token and expiry together and prefer a valid tab login', () => {
    assert.equal(readStoredAdminSession(storage('old', 100), storage('tab', 1000), 500), 'tab')
    assert.equal(readStoredAdminSession(storage('old', undefined), storage('tab', 1000), 500), 'tab')
    assert.equal(readStoredAdminSession(storage('remembered', 1000), storage('tab', 1000), 500), 'tab')
    assert.equal(readStoredAdminSession(storage('expired', 100), storage(), 500), null)
    const blocked = { getItem: () => { throw new Error('blocked') } }
    assert.equal(readStoredAdminSession(blocked, storage('tab', 1000), 500), 'tab')
})

test('a tab-only login clears an old remembered token and tolerates unavailable persistence', () => {
    const local = storage('old', 1000), session = storage()
    assert.equal(saveStoredAdminSession('tab', 60, false, local, session), true)
    assert.equal(local.getItem('ds2api_token'), null)
    assert.equal(readStoredAdminSession(local, session), 'tab')
    const blocked = { setItem: () => { throw new Error('quota') }, removeItem: () => {} }
    assert.equal(saveStoredAdminSession('new', 60, true, blocked, session), false)
    assert.equal(session.getItem('ds2api_token'), null)
})

test('a late verification cannot log a user back in after logout', async () => {
    const pending = deferred(), tokens = []
    let signal, checking
    const session = createAdminSession({ local: storage('old', Date.now() + 60000), session: storage(),
        fetcher: (_url, options) => { signal = options.signal; return pending.promise },
        onToken: (token) => tokens.push(token), onChecking: (value) => { checking = value } })
    const restoring = session.restore()
    session.logout()
    assert.equal(signal.aborted, true)
    pending.resolve({ ok: true })
    await restoring
    assert.deepEqual(tokens, [null])
    assert.equal(checking, false)
})

test('stale failures and old authenticated requests cannot log out a new session', async () => {
    for (const fail of [false, true]) {
        const pending = deferred(), tokens = []
        const session = createAdminSession({ local: storage('old', Date.now() + 60000), session: storage(),
            fetcher: () => pending.promise, onToken: (token) => tokens.push(token), onChecking: () => {} })
        const restoring = session.restore()
        session.login('new')
        assert.equal(session.logout('old'), false)
        if (fail) pending.reject(new Error('offline'))
        else pending.resolve({ ok: false })
        await restoring
        assert.deepEqual(tokens, ['new'])
        assert.equal(session.logout('new'), true)
        assert.deepEqual(tokens, ['new', null])
    }
})

test('current offline verification keeps its stored token and an invalid credential clears it', async () => {
    for (const offline of [true, false]) {
        const local = storage('stored', Date.now() + 60000), tokens = []
        const session = createAdminSession({ local, session: storage(),
            fetcher: async () => { if (offline) throw new Error('offline'); return { ok: false } },
            onToken: (token) => tokens.push(token), onChecking: () => {} })
        await session.restore()
        assert.deepEqual(tokens, [offline ? 'stored' : null])
        if (!offline) assert.equal(local.getItem('ds2api_token'), null)
    }
})

test('request generations invalidate earlier work while cancellation permits a fresh mount', () => {
    const requests = createLatestRequest()
    const first = requests.start(), second = requests.start()
    assert.equal(first.signal.aborted, true)
    assert.equal(first.isCurrent(), false)
    assert.equal(second.isCurrent(), true)
    requests.cancel()
    assert.equal(second.isCurrent(), false)
    assert.equal(requests.start().isCurrent(), true)
})

test('an in-memory login can be reverified when browser storage is unavailable', async () => {
    for (const local of [storage(), storage('older-remembered-session', Date.now() + 60000)]) {
        const tokens = []
        const session = createAdminSession({ local, session: storage(),
            fetcher: async (_url, options) => {
                assert.equal(options.headers.Authorization, 'Bearer memory-only')
                return { ok: true }
            }, onToken: (token) => tokens.push(token), onChecking: () => {} })
        session.login('memory-only')
        await session.restore()
        assert.deepEqual(tokens, ['memory-only', 'memory-only'])
    }
})
