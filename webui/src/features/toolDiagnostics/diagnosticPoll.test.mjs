import test from 'node:test'
import assert from 'node:assert/strict'
import { createDiagnosticPoll } from './diagnosticPoll.js'
function fixture(load) {
    let visible = false, id = 0
    const timers = new Map()
    const poll = createDiagnosticPoll({ load, isVisible: () => visible, setTimer: callback => { timers.set(++id, callback); return id }, clearTimer: key => timers.delete(key) })
    return { poll, timers, show(value) { visible = value; return poll.visibilityChanged() } }
}
test('hidden tabs pause polling and resume immediately', async () => {
    let calls = 0
    const { poll, timers, show } = fixture(async () => { calls++ })
    await poll.start()
    assert.equal(calls, 0)
    await show(true)
    assert.equal(calls, 1)
    assert.equal(timers.size, 1)
    await show(false)
    assert.equal(timers.size, 0)
    await show(true)
    assert.equal(calls, 2)
    poll.stop()
    assert.equal(timers.size, 0)
})
test('visibility changes do not overlap requests or schedule after unmount', async () => {
    let calls = 0, resolve
    const { poll, timers, show } = fixture(() => { calls++; return new Promise(done => { resolve = done }) })
    await poll.start()
    const running = show(true)
    await show(false)
    await show(true)
    assert.equal(calls, 1)
    poll.stop()
    resolve()
    await running
    assert.equal(timers.size, 0)
    await show(true)
    assert.equal(calls, 1)
})
test('a polling failure recovers at the next scheduled refresh', async () => {
    let calls = 0
    const errors = [], timers = []
    const poll = createDiagnosticPoll({ load: async () => { if (++calls === 1) throw new Error('offline') }, isVisible: () => true, onError: error => errors.push(error.message), setTimer: callback => { timers.push(callback); return timers.length }, clearTimer: () => {} })
    await poll.start()
    assert.deepEqual(errors, ['offline'])
    await timers[0]()
    assert.equal(calls, 2)
    poll.stop()
})
