import assert from 'node:assert/strict'
import {before, after, test} from 'node:test'
import {fileURLToPath} from 'node:url'
import React from 'react'
import TestRenderer from 'react-test-renderer'
import react from '@vitejs/plugin-react'
import {createServer} from 'vite'
import {EMPTY_PROXY_FORM, proxyRecordValue} from './proxyFields.js'
import {proxyPayload} from './proxyApi.js'
const {act, create} = TestRenderer
let vite, useProxyOperations
before(async () => {
    vite = await createServer({root:fileURLToPath(new URL('../../../', import.meta.url)), configFile:false, plugins:[react()], server:{middlewareMode:true, hmr:false}, appType:'custom', logLevel:'error'})
    useProxyOperations = (await vite.ssrLoadModule('/src/features/proxy/useProxyOperations.js')).default
})
after(async () => {await vite?.close()})
const deferred = () => {let resolve; const promise = new Promise(done => {resolve=done}); return {promise, resolve}}
const response = data => ({ok:true, status:200, headers:new Headers({'content-type':'application/json'}), text:async () => JSON.stringify(data)})
const node = {id:'p', type:'socks5h', host:'relay.example', port:1080}
const proxies = [node]
const form = {...EMPTY_PROXY_FORM, host:'relay.example', port:1080}
const t = key => key
const noop = () => {}
async function mount(extra) {
    let current, renderer
    const defaults = {apiFetch:noop, proxies, form, editingProxy:null, t, onMessage:noop, onSaved:noop, getModalGeneration:() => 0}
    function Harness({options}) {current=useProxyOperations({...defaults, ...options}); return null}
    await act(async () => {renderer=create(React.createElement(Harness, {options:extra}))})
    return {get current() {return current}, update:async options => act(async () => renderer.update(React.createElement(Harness, {options}))), close:() => act(() => renderer.unmount())}
}

test('numeric fields cannot silently discard invalid MTU input', () => {
    for (const mtu of ['abc', '0', '-1', '1400.5']) assert.throws(() => proxyPayload({...form, mtu}), /invalidMTU/)
    assert.equal(proxyPayload({...form, mtu:''}).mtu, 0)
    assert.equal(proxyPayload({...form, mtu:'1280'}).mtu, 1280)
    assert.throws(() => proxyPayload({...form, port:'1.5'}), /invalidPort/)
    assert.equal(proxyRecordValue({}, 'constructor'), undefined)
})

test('saving twice sends one request and cannot close a newly opened modal', async () => {
    const saved=deferred(); let sends=0, generation=0, closed=0, refreshed=0
    const harness=await mount({apiFetch:async () => {sends++; return saved.promise}, getModalGeneration:() => generation, onSaved:() => {closed++}, onRefresh:async () => {refreshed++}})
    try {
        let pending
        act(() => {pending=harness.current.saveProxy(); harness.current.saveProxy()})
        assert.equal(sends, 1)
        generation++
        await act(async () => {saved.resolve(response({success:true, proxy:node})); await pending})
        assert.equal(closed, 0)
        assert.equal(refreshed, 1)
        assert.equal(harness.current.saving, false)
    } finally {harness.close()}
})

test('saving after unmount cannot refresh or notify and aborts its request', async () => {
    const saved=deferred(); let signal, changes=0
    const harness=await mount({apiFetch:async (_url, options) => {signal=options.signal; return saved.promise}, onRefresh:async () => {changes++}, onMessage:() => {changes++}, onSaved:() => {changes++}})
    let pending
    act(() => {pending=harness.current.saveProxy()})
    harness.close()
    assert.equal(signal.aborted, true)
    await act(async () => {saved.resolve(response({success:true})); await pending})
    assert.equal(changes, 0)
})

test('a committed save with failed refresh is reported as saved and does not invite a retry', async () => {
    const messages=[]; let closed=0
    const harness=await mount({apiFetch:async () => response({success:true}), onRefresh:async () => {throw new Error('offline')}, onMessage:(...args) => messages.push(args), onSaved:() => {closed++}})
    try {await act(async () => harness.current.saveProxy()); assert.equal(closed, 1); assert.deepEqual(messages, [['error','proxyManager.savedRefreshFailed']])}
    finally {harness.close()}
})

test('a test response from before a config refresh cannot restore an old result', async () => {
    const tested=deferred(); let signal, sends=0
    const apiFetch=async (_url, options) => {signal=options.signal; sends++; return tested.promise}
    const messages=[]
    const harness=await mount({apiFetch, onMessage:(...args) => messages.push(args)})
    try {
        let pending
        act(() => {pending=harness.current.testProxy(node); harness.current.testProxy(node)})
        assert.equal(sends, 1)
        await harness.update({apiFetch, proxies:[{...node, host:'new.example'}], onMessage:(...args) => messages.push(args)})
        assert.equal(signal.aborted, true)
        await act(async () => {tested.resolve(response({success:true})); await pending})
        assert.equal(harness.current.testResults.p, undefined)
        assert.deepEqual(messages, [])
    } finally {harness.close()}
})

test('bulk import completion cannot close a new modal while refresh is pending', async () => {
    const refreshed=deferred(); let generation=0, closed=0
    const harness=await mount({onRefresh:() => refreshed.promise, getModalGeneration:() => generation, onSaved:() => {closed++}})
    try {
        let pending
        act(() => {pending=harness.current.finishImport({added:2, skipped:1})})
        generation++
        await act(async () => {refreshed.resolve(); await pending})
        assert.equal(closed, 0)
    } finally {harness.close()}
})
