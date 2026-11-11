import assert from 'node:assert/strict'
import test from 'node:test'
import React from 'react'
import { act, create } from 'react-test-renderer'
import { useChatHistoryList } from './useChatHistoryList.js'

const deferred = () => {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}
const response = (ids, etag = 'v1', limit = 20) => ({ ok: true, status: 200,
    headers: new Headers({ ETag: etag }), json: async () => ({ items: ids.map(id => ({ id })), limit }) })

async function mount(t, source) {
    let state, renderer
    const messages = [], changes = [], timers = new Set(), renders = []
    t.mock.method(globalThis, 'setInterval', fn => { timers.add(fn); return fn })
    t.mock.method(globalThis, 'clearInterval', fn => timers.delete(fn))
    function Harness({ apiFetch }) {
        state = useChatHistoryList({ apiFetch, t: key => key,
            onMessage: (...args) => messages.push(args), onMutation: change => changes.push(change) })
        renders.push({ source: apiFetch, items: state.items })
        return null
    }
    await act(async () => { renderer = create(React.createElement(Harness, { apiFetch: source })) })
    const unmount = () => { if (renderer) { act(() => renderer.unmount()); renderer = null } }
    t.after(unmount)
    return { get state() { return state }, messages, changes, timers, renders, unmount,
        changeSource: apiFetch => act(async () => renderer.update(React.createElement(Harness, { apiFetch }))) }
}

test('history switches request source, cancels old reads and resets validators', async t => {
    const old = deferred(), seen = [], oldRequests = []
    let calls = 0
    const firstSource = async (_url, options) => {
        oldRequests.push(options)
        return ++calls === 1 ? response(['first'], 'first-etag') : old.promise
    }
    const ui = await mount(t, firstSource)
    let read
    act(() => { read = ui.state.loadList({ mode: 'manual' }) })
    const secondSource = async (_url, options) => { seen.push(options); return response(['second'], 'second-etag') }
    await ui.changeSource(secondSource)
    assert.equal(oldRequests[1].signal.aborted, true)
    assert.deepEqual(seen[0].headers, {})
    assert.ok(ui.renders.filter(render => render.source === secondSource).every(render => !render.items.some(item => item.id === 'first')))
    await act(async () => { old.resolve(response(['obsolete'], 'obsolete-etag')); await read })
    assert.deepEqual(ui.state.items.map(item => item.id), ['second'])
    assert.equal(ui.state.loading, false)
    assert.equal(ui.state.refreshing, false)
    assert.equal(ui.timers.size, 1)
})

for (const kind of ['delete', 'clear', 'limit']) {
    for (const phase of ['response', 'json']) {
        test(`${kind} is not undone by a stale poll finishing during ${phase}`, async t => {
            const late = deferred(), headers = []
            let reads = 0, oldSignal
            const expected = kind === 'clear' ? [] : ['b']
            const ui = await mount(t, async (_url, options) => {
                if (options.method) return kind === 'limit' ? response(expected, 'saved', 10)
                    : { ok: true, json: async () => ({ success: true }) }
                headers.push(options.headers)
                if (++reads === 1) return response(['a', 'b'])
                if (reads === 2) {
                    oldSignal = options.signal
                    return phase === 'response' ? late.promise : { ...response([]), json: () => late.promise }
                }
                return response(expected, 'fresh')
            })
            let read
            await act(async () => { read = ui.state.loadList({ mode: 'manual' }); await Promise.resolve() })
            await act(async () => {
                if (kind === 'delete') await ui.state.handleDeleteItem('a')
                if (kind === 'clear') await ui.state.handleClear()
                if (kind === 'limit') await ui.state.handleLimitChange(10)
            })
            assert.equal(oldSignal.aborted, true)
            await act(async () => {
                late.resolve(phase === 'response' ? response(['a', 'b', 'resurrected'], 'stale')
                    : { items: [{ id: 'a' }, { id: 'b' }, { id: 'resurrected' }], limit: 50 })
                await read
            })
            assert.deepEqual(ui.state.items.map(item => item.id), expected)
            assert.equal(ui.state.limit, kind === 'limit' ? 10 : 20)
            assert.equal(ui.state.refreshing, false)
            assert.equal(ui.state.busy, false)
            assert.equal(ui.changes[0].kind, kind)
            await act(async () => { await ui.state.loadList() })
            assert.deepEqual(headers[2], {}, 'mutation retained an invalid list ETag')
            assert.deepEqual(ui.state.items.map(item => item.id), expected)
        })
    }
}

test('history coalesces slow polls and keeps validators paired with accepted data', async t => {
    const slow = deferred(), seen = []
    let calls = 0
    const ui = await mount(t, async (_url, options) => {
        seen.push(options)
        if (++calls === 1) return response(['saved'], 'saved-etag')
        if (calls === 2) return slow.promise
        if (calls === 3) return { status: 304 }
        return response(['recovered'], 'recovered-etag')
    })
    let first, second
    act(() => { first = ui.state.loadList(); second = ui.state.loadList({ mode: 'manual', announceError: true }) })
    assert.equal(first, second)
    assert.equal(calls, 2)
    await act(async () => { for (const tick of ui.timers) tick() })
    assert.equal(calls, 2)
    await act(async () => { slow.resolve({ ok: false, status: 500, headers: new Headers({ ETag: 'invalid' }), json: async () => ({ detail: 'offline' }) }); await first })
    assert.equal(ui.state.detail, 'offline')
    assert.deepEqual(ui.state.items.map(item => item.id), ['saved'])
    await act(async () => { await ui.state.loadList() })
    assert.deepEqual(seen[2].headers, { 'If-None-Match': 'saved-etag' })
    assert.equal(ui.state.detail, '', 'a successful 304 kept a stale network error visible')
    await act(async () => { await ui.state.loadList() })
    assert.equal(ui.state.detail, '')
    assert.deepEqual(ui.state.items.map(item => item.id), ['recovered'])
})

test('a 304 without an accepted snapshot remains a visible load failure', async t => {
    const ui = await mount(t, async () => ({ status: 304 }))
    assert.equal(ui.state.detail, 'chatHistory.loadFailed')
    assert.deepEqual(ui.state.items, [])
    assert.deepEqual(ui.messages, [['error', 'chatHistory.loadFailed']])
    assert.equal(ui.state.loading, false)
})

test('writes coalesce synchronously, pause polls and allow retry after a failure', async t => {
    const saving = deferred()
    let writes = 0, reads = 0
    const ui = await mount(t, async (_url, options) => {
        if (options.method) return ++writes === 1 ? saving.promise : { ok: true, json: async () => ({ success: true }) }
        reads++
        return response(['a', 'b'])
    })
    let first, duplicate
    act(() => { first = ui.state.handleDeleteItem('a'); duplicate = ui.state.handleDeleteItem('a') })
    assert.equal(writes, 1)
    assert.equal(ui.state.busy, true)
    await act(async () => { for (const tick of ui.timers) tick(); await ui.state.loadList(); await duplicate })
    assert.equal(reads, 1)
    await act(async () => { saving.resolve({ ok: false, json: async () => ({ detail: 'denied' }) }); await first })
    assert.equal(ui.state.busy, false)
    assert.deepEqual(ui.state.items.map(item => item.id), ['a', 'b'])
    assert.deepEqual(ui.messages, [['error', 'denied']])
    await act(async () => { await ui.state.handleDeleteItem('a') })
    assert.equal(writes, 2)
    assert.deepEqual(ui.state.items.map(item => item.id), ['b'])
})

test('unmount aborts writes and suppresses late notifications', async t => {
    const body = deferred()
    let signal
    const ui = await mount(t, async (_url, options) => {
        if (!options.method) return response(['a'])
        signal = options.signal
        return { ok: true, json: () => body.promise }
    })
    let operation
    await act(async () => { operation = ui.state.handleClear(); await Promise.resolve() })
    ui.unmount()
    assert.equal(signal.aborted, true)
    await act(async () => { body.resolve({ success: true }); await operation })
    assert.deepEqual(ui.messages, [])
    assert.deepEqual(ui.changes, [])
    assert.equal(ui.timers.size, 0)
})

test('changing request source cancels a pending mutation without changing the new list', async t => {
    const saving = deferred()
    let oldSignal
    const ui = await mount(t, async (_url, options) => {
        if (!options.method) return response(['old'])
        oldSignal = options.signal
        return saving.promise
    })
    let operation
    act(() => { operation = ui.state.handleClear() })
    await ui.changeSource(async () => response(['new']))
    assert.equal(oldSignal.aborted, true)
    assert.equal(ui.state.busy, false)
    await act(async () => { saving.resolve({ ok: true, json: async () => ({ success: true }) }); await operation })
    assert.deepEqual(ui.state.items.map(item => item.id), ['new'])
    assert.deepEqual(ui.changes, [])
    assert.deepEqual(ui.messages, [])
})

test('disabling history stops list polling and source replacement resumes it', async t => {
    const ui = await mount(t, async (_url, options) => options.method ? response(['a'], 'disabled', 0) : response(['a']))
    assert.equal(ui.timers.size, 1)
    await act(async () => { await ui.state.handleLimitChange(0) })
    assert.equal(ui.state.limit, 0)
    assert.equal(ui.timers.size, 0)
    await ui.changeSource(async () => response(['new'], 'new', 20))
    assert.equal(ui.state.limit, 20)
    assert.equal(ui.timers.size, 1)
})
