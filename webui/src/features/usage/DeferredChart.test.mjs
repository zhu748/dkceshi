import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { fileURLToPath } from 'node:url'
import React from 'react'
import TestRenderer, { act } from 'react-test-renderer'
import react from '@vitejs/plugin-react'
import { createServer } from 'vite'

let vite, DeferredChart
before(async () => {
    vite = await createServer({
        root: fileURLToPath(new URL('../../../', import.meta.url)), configFile: false,
        plugins: [react()], server: { middlewareMode: true, hmr: false }, appType: 'custom', logLevel: 'error',
    })
    DeferredChart = (await vite.ssrLoadModule('/src/features/usage/DeferredChart.jsx')).default
})
after(async () => { await vite?.close() })

test('offscreen charts reserve space and initialize only once when visible', async t => {
    let observe, observed, disconnected = 0, options
    const oldObserver = globalThis.IntersectionObserver, oldWindow = globalThis.window
    globalThis.IntersectionObserver = class {
        constructor(callback, config) { observe = callback; options = config }
        observe(target) { observed = target }
        disconnect() { disconnected++ }
    }
    t.after(() => {
        if (oldObserver === undefined) delete globalThis.IntersectionObserver
        else globalThis.IntersectionObserver = oldObserver
        if (oldWindow === undefined) delete globalThis.window
        else globalThis.window = oldWindow
    })
    const host = {}, createNodeMock = node => node.props.role === 'img' ? null : host
    let renderer
    await act(async () => { renderer = TestRenderer.create(React.createElement(DeferredChart, { loadingLabel: 'Loading', height: 240, option: {}, ariaLabel: 'Usage' }), { createNodeMock }) })
    assert.equal(observed, host)
    assert.equal(options.rootMargin, '200px')
    assert.equal(renderer.root.findAllByProps({ role: 'img' }).length, 0)
    assert.equal(renderer.root.findByProps({ role: 'status' }).props.style.height, 240)
    await vite.ssrLoadModule('/src/features/usage/EChart.jsx')
    globalThis.window = { addEventListener() {}, removeEventListener() {} }
    await act(async () => { observe([{ isIntersecting: true }]); await new Promise(resolve => setImmediate(resolve)) })
    assert.equal(renderer.root.findByProps({ role: 'img' }).props['aria-label'], 'Usage')
    assert.equal(disconnected, 1)
    await act(async () => observe([{ isIntersecting: true }]))
    assert.equal(disconnected, 1)
    await act(async () => renderer.unmount())
    assert.equal(disconnected, 2)
    await act(async () => observe([{ isIntersecting: true }]))
    assert.equal(disconnected, 2)
})
