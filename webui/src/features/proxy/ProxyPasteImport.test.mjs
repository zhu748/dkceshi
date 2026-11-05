import assert from 'node:assert/strict'
import {after, before, test} from 'node:test'
import {fileURLToPath} from 'node:url'
import React from 'react'
import TestRenderer from 'react-test-renderer'
import react from '@vitejs/plugin-react'
import {createServer} from 'vite'
import {EMPTY_PROXY_FORM, PROXY_TYPES} from './proxyFields.js'
const {act, create} = TestRenderer
let vite, Paste, Modal
const translate = key => key
const noop = () => {}
before(async () => {
    vite = await createServer({root: fileURLToPath(new URL('../../../', import.meta.url)), configFile:false, plugins:[react()], server:{middlewareMode:true, hmr:false}, appType:'custom', logLevel:'error'})
    Paste = (await vite.ssrLoadModule('/src/features/proxy/ProxyPasteImport.jsx')).default
    Modal = (await vite.ssrLoadModule('/src/features/proxy/ProxyFormModal.jsx')).default
})
after(async () => {await vite?.close()})
const settle = () => new Promise(resolve => setTimeout(resolve, 390))

test('protocol selector exposes all nine supported protocols', async () => {
    let view
    await act(async () => {view = create(React.createElement(Modal, {show:true, t:translate, form:EMPTY_PROXY_FORM, setForm:noop, apiFetch:noop, onImported:noop}))})
    assert.deepEqual(view.root.findAllByType('option').map(node => node.props.value), PROXY_TYPES)
    act(() => view.unmount())
})

test('single input uses the selected type, preserves credentials and ignores a stale parse result', async () => {
    let form, setType, view, oldResolve
    const calls = []
    const apiFetch = async (_url, options) => {
        const body = JSON.parse(options.body)
        calls.push(body)
        if (body.text === 'old') return new Promise(resolve => {oldResolve = resolve})
        return {ok:true, json:async () => ({items:[{status:'ready', proxy:{type:body.type, host:'relay.example', port:3478, username:'u', password:' p '}}]})}
    }
    function Harness() {
        const [value, update] = React.useState({...EMPTY_PROXY_FORM, type:'turn', port:3478, tls_server_name:'old.example', insecure:true})
        form = value
        setType = type => update(previous => ({...previous, type}))
        return React.createElement(Paste, {t:translate, apiFetch, form:value, setForm:update, onImported:noop, onBlocksSave:noop})
    }
    await act(async () => {view = create(React.createElement(Harness))})
    try {
        act(() => view.root.findByType('textarea').props.onChange({target:{value:'old'}}))
        await act(settle)
        act(() => {setType('sstp'); view.root.findByType('textarea').props.onChange({target:{value:'u:p@relay.example:3478'}})})
        await act(settle)
        assert.equal(calls.at(-1).type, 'sstp')
        assert.equal(form.type, 'sstp')
        assert.equal(form.password, ' p ')
        assert.equal(form.tls_server_name, '')
        assert.equal(form.insecure, false)
        await act(async () => {oldResolve({ok:true, json:async () => ({items:[{status:'ready', proxy:{type:'http', host:'stale.example', port:80}}]})}); await Promise.resolve()})
        assert.equal(form.host, 'relay.example')
    } finally {act(() => view.unmount())}
})

test('bulk import submits only ready rows and blocks single-node save', async () => {
    let view, blocks = false, imported
    const saved = []
    const apiFetch = async (url, options) => {
        if (url.endsWith('/import')) {saved.push(JSON.parse(options.body)); return {ok:true, json:async () => ({added:1, skipped:0})}}
        return {ok:true, json:async () => ({items:[{source:'1', status:'ready', proxy:{type:'turn', host:'one', port:3478}}, {source:'2', status:'invalid', message:'bad'}, {source:'3', status:'duplicate'}]})}
    }
    const onBlocksSave = value => {blocks = value}
    await act(async () => {view = create(React.createElement(Paste, {t:translate, apiFetch, form:EMPTY_PROXY_FORM, setForm:noop, onImported:async value => {imported=value}, onBlocksSave}))})
    try {
        act(() => view.root.findByType('textarea').props.onChange({target:{value:'one\nbad\nduplicate'}}))
        await act(settle)
        assert.equal(blocks, true)
        await act(async () => view.root.findByType('button').props.onClick())
        assert.equal(saved[0].items.length, 1)
        assert.equal(saved[0].items[0].host, 'one')
        assert.equal(imported.added, 1)
    } finally {act(() => view.unmount())}
})

test('invalid parse results cannot overwrite a valid form or enable save', async () => {
    let view, updates = 0, blocked = false
    const apiFetch = async () => ({ok:true, json:async () => ({items:[{status:'invalid', message:'unsupported transport', proxy:{type:'turn', host:'bad.example', port:3478}}]})})
    await act(async () => {view = create(React.createElement(Paste, {t:translate, apiFetch, form:EMPTY_PROXY_FORM, setForm:() => {updates++}, onImported:noop, onBlocksSave:value => {blocked=value}}))})
    try {
        act(() => view.root.findByType('textarea').props.onChange({target:{value:'turn:bad.example?transport=udp'}}))
        assert.equal(blocked, true)
        await act(settle)
        assert.equal(updates, 0)
        assert.equal(blocked, true)
        act(() => view.root.findByType('textarea').props.onChange({target:{value:''}}))
        assert.equal(blocked, false)
    } finally {act(() => view.unmount())}
})

test('incomplete SSTP nodes fill address fields for manual credential completion', async () => {
    let view, filled, blocked = true
    const apiFetch = async () => ({ok:true, json:async () => ({items:[{status:'incomplete', proxy:{type:'sstp', host:'vpn.example', port:443}}]})})
    const setForm = value => {filled=value}
    await act(async () => {view = create(React.createElement(Paste, {t:translate, apiFetch, form:EMPTY_PROXY_FORM, setForm, onImported:noop, onBlocksSave:value => {blocked=value}}))})
    try {
        act(() => view.root.findByType('textarea').props.onChange({target:{value:'sstp://vpn.example'}}))
        await act(settle)
        assert.equal(filled.host, 'vpn.example')
        assert.equal(filled.type, 'sstp')
        assert.equal(blocked, false)
    } finally {act(() => view.unmount())}
})

test('closing a pending parse ignores late results even when the fetch source ignores abort', async () => {
    let view, resolve, updates = 0
    const apiFetch = async () => new Promise(done => {resolve=done})
    await act(async () => {view = create(React.createElement(Paste, {t:translate, apiFetch, form:EMPTY_PROXY_FORM, setForm:() => {updates++}, onImported:noop, onBlocksSave:noop}))})
    act(() => view.root.findByType('textarea').props.onChange({target:{value:'relay.example:1080'}}))
    await act(settle)
    act(() => view.unmount())
    await act(async () => {resolve({ok:true, json:async () => ({items:[{status:'ready', proxy:{type:'socks5', host:'late.example', port:1080}}]})}); await Promise.resolve()})
    assert.equal(updates, 0)
})

test('an import starts once and a late response after close cannot close a new modal', async () => {
    let view, resolve, sends = 0, imported = 0, signal
    const apiFetch = async (url, options) => {
        if (url.endsWith('/import')) {sends++; signal=options.signal; return new Promise(done => {resolve=done})}
        return {ok:true, json:async () => ({items:[{status:'ready', proxy:{type:'turn', host:'one', port:3478}}, {status:'ready', proxy:{type:'turn', host:'two', port:3478}}]})}
    }
    await act(async () => {view = create(React.createElement(Paste, {t:translate, apiFetch, form:EMPTY_PROXY_FORM, setForm:noop, onImported:async () => {imported++}, onBlocksSave:noop}))})
    act(() => view.root.findByType('textarea').props.onChange({target:{value:'one\ntwo'}}))
    await act(settle)
    let pending
    act(() => {const click=view.root.findByType('button').props.onClick; pending=click(); click()})
    assert.equal(sends, 1)
    act(() => view.unmount())
    assert.equal(signal.aborted, true)
    await act(async () => {resolve({ok:true, json:async () => ({added:2, skipped:0})}); await pending})
    assert.equal(imported, 0)
})

test('auto-detecting a different protocol fills once and preserves subsequent manual edits and language changes', async () => {
    let view, current, update, parses = 0
    const apiFetch = async () => {parses++; return {ok:true, json:async () => ({items:[{status:'incomplete', proxy:{type:'sstp', host:'vpn.example', port:443}}]})}}
    function Harness({t}) {
        const [form, setForm] = React.useState(EMPTY_PROXY_FORM)
        current = form
        update = setForm
        return React.createElement(Paste, {t, apiFetch, form, setForm, onImported:noop, onBlocksSave:noop})
    }
    await act(async () => {view = create(React.createElement(Harness, {t:translate}))})
    try {
        act(() => view.root.findByType('textarea').props.onChange({target:{value:'sstp://vpn.example'}}))
        await act(settle)
        act(() => update(form => ({...form, password:'manually filled', name:'edited name'})))
        await act(settle)
        await act(async () => view.update(React.createElement(Harness, {t:key => `en:${key}`})))
        await act(settle)
        assert.equal(parses, 1)
        assert.equal(current.password, 'manually filled')
        assert.equal(current.name, 'edited name')
    } finally {act(() => view.unmount())}
})

test('an import from an old authentication source cannot complete in the new session', async () => {
    let resolveImport, signal, imported=0, view
    const parsed={ok:true,json:async () => ({items:[{status:'ready',proxy:{type:'turn',host:'one',port:3478}},{status:'ready',proxy:{type:'turn',host:'two',port:3478}}]})}
    const oldFetch=async (url,options) => {
        if (url.endsWith('/import')) {signal=options.signal;return new Promise(resolve => {resolveImport=resolve})}
        return parsed
    }
    const newFetch=async () => parsed
    const props={t:translate,form:EMPTY_PROXY_FORM,setForm:noop,onImported:async () => {imported++},onBlocksSave:noop}
    await act(async () => {view=create(React.createElement(Paste,{...props,apiFetch:oldFetch}))})
    try {
        act(() => view.root.findByType('textarea').props.onChange({target:{value:'one\ntwo'}}))
        await act(settle)
        let pending
        act(() => {pending=view.root.findByType('button').props.onClick()})
        await act(async () => view.update(React.createElement(Paste,{...props,apiFetch:newFetch})))
        assert.equal(signal.aborted,true)
        await act(async () => {resolveImport({ok:true,json:async () => ({added:2,skipped:0})});await pending})
        assert.equal(imported,0)
        assert.equal(view.root.findByType('textarea').props.value,'one\ntwo')
    } finally {act(() => view.unmount())}
})
