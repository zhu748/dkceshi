import assert from 'node:assert/strict'
import { after, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import React from '../../../node_modules/react/index.js'
import TestRenderer from '../../../node_modules/react-test-renderer/index.js'
import { createServer } from '../../../node_modules/vite/dist/node/index.js'

const { act, create } = TestRenderer
const server = await createServer({
    root: fileURLToPath(new URL('../../../', import.meta.url)),
    server: { middlewareMode: true, watch: null },
    appType: 'custom', logLevel: 'silent',
})
after(() => server.close())
const { useUsageSettings } = await server.ssrLoadModule('/src/features/usage/useUsageSettings.js')
const { useUsageRecords } = await server.ssrLoadModule('/src/features/usage/useUsageRecords.js')
const saved = (price = 4) => ({ enabled: true, models: { 'deepseek-v4.1-flash': { input_price: 0.02, output_price: price } } })
const response = (settings = saved()) => ({ ok: true, json: async () => ({ settings }) })

function deferred() {
    let resolve, reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
}

async function fixture(t, put) {
    const messages = []
    let current, renderer
    const authFetch = (url, options) => options?.method === 'PUT' ? put(url, options) : Promise.resolve(response())
    const onMessage = (...args) => messages.push(args)
    function Harness() { current = useUsageSettings({ authFetch, onMessage }); return null }
    await act(async () => { renderer = create(React.createElement(Harness)) })
    t.after(() => act(() => renderer.unmount()))
    act(() => current.setSettingsOpen(true))
    return { get current() { return current }, messages, unmount: () => act(() => renderer.unmount()) }
}

for (const phase of ['response', 'json', 'error']) {
    test(`late ${phase} from an old save cannot overwrite or close a reopened settings dialog`, async t => {
        const pending = deferred()
        let signal, old
        const f = await fixture(t, (_url, options) => {
            signal = options.signal
            return phase === 'json' ? { ok: true, json: () => pending.promise } : pending.promise
        })
        await act(async () => { old = f.current.closeSettings(true) })
        act(() => {
            f.current.dismissSettings()
            f.current.setSettingsOpen(true)
            f.current.setPricing(prev => ({ ...prev, peakMultiplier: 7 }))
        })
        await act(async () => {
            if (phase === 'error') pending.reject(new Error('late error'))
            else pending.resolve(phase === 'json' ? { settings: saved(99) } : response(saved(99)))
            await old
        })
        assert.equal(f.current.settingsOpen, true)
        assert.equal(f.current.pricing.peakMultiplier, 7)
        assert.equal(f.current.pricing.models['deepseek-v4.1-flash'].outputPrice, 4)
        assert.deepEqual(f.messages, [])
        assert.equal(signal?.aborted, true)
    })
}

test('unmounted settings save cannot emit a success or error message', async t => {
    const pending = deferred()
    const f = await fixture(t, () => pending.promise)
    let old
    await act(async () => { old = f.current.saveUsageSettings(f.current.pricing, true) })
    f.unmount()
    await act(async () => { pending.resolve(response()); await old })
    assert.deepEqual(f.messages, [])
})

test('a synchronous fetch failure does not block a later retry', async t => {
    let calls = 0
    const f = await fixture(t, () => { calls++; throw new Error('offline') })
    for (let i = 0; i < 2; i++) {
        await act(async () => { assert.equal(await f.current.saveUsageSettings(f.current.pricing, true), false) })
    }
    assert.equal(calls, 2)
    assert.equal(f.current.saving, false)
})

test('old save finalization cannot clear a new save or suppress its request', async t => {
    const first = deferred(), second = deferred()
    let calls = 0, old, next
    const f = await fixture(t, () => (++calls === 1 ? first.promise : second.promise))
    await act(async () => { old = f.current.closeSettings(true) })
    act(() => { f.current.dismissSettings(); f.current.setSettingsOpen(true) })
    await act(async () => { next = f.current.closeSettings(true) })
    assert.equal(calls, 2)
    await act(async () => { first.resolve(response(saved(99))); await old })
    assert.equal(f.current.saving, true)
    assert.equal(f.current.settingsOpen, true)
    await act(async () => { second.resolve(response(saved(8))); await next })
    assert.equal(f.current.saving, false)
    assert.equal(f.current.settingsOpen, false)
    assert.equal(f.current.pricing.models['deepseek-v4.1-flash'].outputPrice, 8)
    assert.deepEqual(f.messages, [['success', 'usage.settingsSaved']])
})

test('duplicate saves share the same mutation and keep explicit zero prices', async t => {
    const pending = deferred()
    let calls = 0, first, duplicate
    const f = await fixture(t, (_url, options) => {
        calls++
        const { settings } = JSON.parse(options.body)
        assert.equal(settings.enabled, false)
        assert.equal(settings.models['deepseek-v4.1-flash'].input_miss_price, 0)
        return pending.promise
    })
    const pricing = { ...f.current.pricing, models: { 'deepseek-v4.1-flash': { inputPrice: 0, inputMissPrice: 0, outputPrice: 0 } } }
    await act(async () => {
        first = f.current.saveUsageSettings(pricing, false)
        duplicate = f.current.saveUsageSettings(pricing, false)
    })
    assert.equal(first, duplicate)
    assert.equal(calls, 1)
    await act(async () => { pending.resolve(response()); await first })
    assert.equal(f.current.saving, false)
})

test('usage hook preserves the displayed array on 304 and invalidates it after clear', async t => {
    let current, renderer, calls = 0
    const authFetch = async (_url, options) => {
        calls++
        if (calls === 2) {
            assert.equal(options.headers['If-None-Match'], 'accepted')
            return { status: 304, json: () => assert.fail('304 must not parse JSON') }
        }
        assert.deepEqual(options.headers, {})
        return { status: 200, ok: true, headers: new Headers({ ETag: 'accepted' }), json: async () => ({ items: [
            { date: '2026-11-03', model: 'model', prompt_tokens: 10, total_tokens: 10 },
        ] }) }
    }
    function Harness() { current = useUsageRecords({ authFetch }); return null }
    await act(async () => { renderer = create(React.createElement(Harness)) })
    t.after(() => act(() => renderer.unmount()))
    const records = current.records
    await act(async () => { await current.loadRecords() })
    assert.equal(current.records, records)
    assert.equal(current.loading, false)
    act(() => current.clearRecords())
    assert.deepEqual(current.records, [])
    await act(async () => { await current.loadRecords() })
    assert.equal(current.records[0].usage.total_tokens, 10)
})

test('usage hook disposes old fetch sources so stale refresh callbacks cannot restart them', async t => {
    let current, renderer, oldCalls = 0
    const pending = deferred()
    const firstFetch = () => { oldCalls++; return pending.promise }
    const nextFetch = async () => ({ ok: true, status: 200, json: async () => ({ items: [] }) })
    function Harness({ authFetch }) { current = useUsageRecords({ authFetch }); return null }
    await act(async () => { renderer = create(React.createElement(Harness, { authFetch: firstFetch })) })
    t.after(() => act(() => renderer.unmount()))
    const staleRefresh = current.loadRecords
    await act(async () => { renderer.update(React.createElement(Harness, { authFetch: nextFetch })) })
    await staleRefresh()
    assert.equal(oldCalls, 1)
    await act(async () => { pending.resolve({ ok: true, json: async () => ({ items: [{ date: '2026-11-03', total_tokens: 99 }] }) }) })
    assert.deepEqual(current.records, [])
    assert.equal(current.initialized, true)
})
