import assert from 'node:assert/strict'
import test from 'node:test'

import { createBatchAccountTests } from './batchAccountTests.js'

function deferred() {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}
const response = data => ({ json: async () => data })
function fixture() {
    const progress = [], running = [], completed = []
    const tester = createBatchAccountTests({ onProgress: value => progress.push(value), onRunning: value => running.push(value) })
    const options = {
        accounts: [{ id: 'one' }, { id: 'two' }], resolveAccountIdentifier: account => account.id,
        invalidIdentifier: 'invalid identifier', onCompleted: value => completed.push(value),
        apiFetch: async () => response({ success: true, message: 'ok', response_time: 3 }),
    }
    return { tester, options, progress, running, completed }
}

test('batch tests publish ordered progress, keep snapshots and continue after failures', async () => {
    const f = fixture(), calls = []
    await f.tester.run({ ...f.options, accounts: [{ id: '' }, { id: 'offline' }, { id: 'one' }],
        apiFetch: async (url, options) => {
            assert.equal(url, '/admin/accounts/test')
            assert.equal(options.signal.aborted, false)
            const id = JSON.parse(options.body).identifier
            calls.push(id)
            if (id === 'offline') throw new Error('offline')
            return response({ success: true, message: 'ok', response_time: 3 })
        },
    })
    assert.deepEqual(calls, ['offline', 'one'])
    assert.deepEqual(f.running, [true, false])
    assert.deepEqual(f.completed, [{ success: 1, total: 3 }])
    assert.equal(f.progress[1].results.length, 1)
    assert.deepEqual(f.progress.at(-1).results, [
        { id: '-', success: false, message: 'invalid identifier' },
        { id: 'offline', success: false, message: 'offline' },
        { id: 'one', success: true, message: 'ok', time: 3 },
    ])
})

test('cancel during response, JSON parsing or failure stops the rest of the batch', async () => {
    for (const phase of ['response', 'json', 'failure']) {
        const f = fixture(), pending = deferred()
        let calls = 0, signal, parsing = false
        const batch = f.tester.run({ ...f.options, apiFetch: async (_url, options) => {
            calls++; signal = options.signal
            if (phase === 'json') return { json: () => { parsing = true; return pending.promise } }
            return pending.promise
        } })
        await Promise.resolve()
        await Promise.resolve()
        if (phase === 'json') assert.equal(parsing, true)
        assert.deepEqual(f.tester.cancel(), { current: 0, total: 2, results: [] })
        assert.equal(signal.aborted, true)
        const count = f.progress.length
        if (phase === 'failure') pending.reject(new Error('late failure'))
        else pending.resolve(phase === 'json' ? { success: true } : response({ success: true }))
        await batch
        assert.equal(calls, 1)
        assert.equal(f.progress.length, count)
        assert.deepEqual(f.running, [true, false])
        assert.deepEqual(f.completed, [])
    }
})

test('unmount disposal stops callbacks and a new run keeps its own busy state', async () => {
    const f = fixture(), oldResponse = deferred(), newResponse = deferred()
    const first = f.tester.run({ ...f.options, apiFetch: () => oldResponse.promise })
    await Promise.resolve()
    f.tester.dispose()
    assert.deepEqual(f.running, [true], 'unmount must not update React state')
    const second = f.tester.run({ ...f.options, accounts: [{ id: 'new' }], apiFetch: () => newResponse.promise })
    await Promise.resolve()
    oldResponse.resolve(response({ success: true }))
    await first
    assert.equal(f.tester.running, true)
    assert.deepEqual(f.running, [true, true])
    newResponse.resolve(response({ success: true }))
    await second
    assert.deepEqual(f.running, [true, true, false])
    assert.deepEqual(f.completed, [{ success: 1, total: 1 }])
})

test('duplicate clicks share the active run rather than repeating account requests', async () => {
    const f = fixture(), pending = deferred()
    let calls = 0
    const options = { ...f.options, accounts: [{ id: 'one' }], apiFetch: () => { calls++; return pending.promise } }
    const first = f.tester.run(options)
    const second = f.tester.run(options)
    assert.equal(first, second)
    await Promise.resolve()
    assert.equal(calls, 1)
    pending.resolve(response({ success: true }))
    await first
    assert.deepEqual(f.completed, [{ success: 1, total: 1 }])
})

test('a stopped batch retains completed results and never requests the next account', async () => {
    let tester, stopped, calls = 0
    const progress = []
    tester = createBatchAccountTests({ onRunning: () => {}, onProgress: value => {
        progress.push(value)
        if (value.current === 1) stopped = tester.cancel()
    } })
    await tester.run({ ...fixture().options, apiFetch: async () => { calls++; return response({ success: true }) }, onCompleted: () => assert.fail('stopped batch completed') })
    assert.equal(calls, 1)
    assert.equal(stopped.current, 1)
    assert.equal(stopped.total, 2)
    assert.equal(progress.at(-1).results.length, 1)
    assert.equal(tester.running, false)
})

test('account identifiers are snapshotted and completion exceptions release busy state', async () => {
    const f = fixture(), accounts = [{ id: 'one' }, { id: 'two' }], calls = []
    const batch = f.tester.run({ ...f.options, accounts, apiFetch: async (_url, options) => {
        calls.push(JSON.parse(options.body).identifier)
        return response({ success: true })
    }, onCompleted: () => { throw new Error('callback failed') } })
    accounts[1].id = 'changed-after-start'
    await assert.rejects(batch, /callback failed/)
    assert.deepEqual(calls, ['one', 'two'])
    assert.equal(f.tester.running, false)
    assert.deepEqual(f.running, [true, false])
})

test('empty or disposed-before-start batches make no requests', async () => {
    const f = fixture()
    await f.tester.run({ ...f.options, accounts: [] })
    assert.deepEqual(f.running, [])
    const batch = f.tester.run({ ...f.options, apiFetch: () => assert.fail('disposed run sent a request') })
    f.tester.dispose()
    await batch
    assert.deepEqual(f.completed, [])
    assert.equal(f.tester.running, false)
})
