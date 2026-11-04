import { test } from 'node:test'
import assert from 'node:assert/strict'
import React from '../../webui/node_modules/react/index.js'
import TestRenderer from '../../webui/node_modules/react-test-renderer/index.js'
import { createServer } from '../../webui/node_modules/vite/dist/node/index.js'
import { fileURLToPath } from 'node:url'
const { act, create }=TestRenderer
const server=await createServer({root:fileURLToPath(new URL('../../webui/',import.meta.url)),server:{middlewareMode:true,watch:null},appType:'custom',logLevel:'silent'})
const {useVercelSyncState}=await server.ssrLoadModule('/src/features/vercel/useVercelSyncState.js')
const {useSettingsForm}=await server.ssrLoadModule('/src/features/settings/useSettingsForm.js')
const noop=()=>{}, translate=key=>key

test('Vercel status polling schedules another successful poll',async t=>{
 const timers=new Map();let id=0,statusCalls=0,renderer
 const nativeTimeout=global.setTimeout,nativeClear=global.clearTimeout
 t.mock.method(global,'setTimeout',(fn,delay,...args)=>{if(delay>=1000){const n=++id;timers.set(n,fn);return n};return nativeTimeout(fn,delay,...args)})
 t.mock.method(global,'clearTimeout',n=>{if(timers.has(n))timers.delete(n);else nativeClear(n)})
 const apiFetch=async url=>{if(url.endsWith('/status'))statusCalls++;return {ok:true,json:async()=>({})}}
 function Harness(){useVercelSyncState({apiFetch,onMessage:noop,t:translate,isVercel:true});return null}
 try{
  await act(async()=>{renderer=create(React.createElement(Harness))})
  assert.equal(timers.size,1)
  const [timer,fn]=timers.entries().next().value;timers.delete(timer)
  await act(async()=>{fn();await Promise.resolve()})
  assert.equal(statusCalls,2)
  assert.equal(timers.size,1,'successful timer run did not schedule its successor')
 }finally{act(()=>renderer.unmount())}
})

test('changing UI language preserves unsaved settings',async()=>{
 let current,renderer,reads=0
 const apiFetch=async()=>{reads++;return {ok:true,headers:new Headers({'content-type':'application/json'}),json:async()=>({model_aliases:{old:'deepseek-v4.1-flash'},setting_specs:[],section_specs:[]})}}
 function Harness({t}){current=useSettingsForm({apiFetch,onMessage:noop,t});return null}
 try{
  await act(async()=>{renderer=create(React.createElement(Harness,{t:translate}))})
  act(()=>current.setForm(previous=>({...previous,model_aliases_text:'{"my-unsaved-edit":"deepseek-v4.1-flash"}'})))
  await act(async()=>renderer.update(React.createElement(Harness,{t:key=>'en:'+key})))
  assert.match(current.form.model_aliases_text,/my-unsaved-edit/,`language change triggered ${reads} fetches and discarded edits`)
 }finally{act(()=>renderer.unmount())}
})
test.after(()=>server.close())

function deferred() { let resolve; const promise=new Promise(yes=>{resolve=yes});return {promise,resolve} }
const settingsResponse = (aliases={})=>({ok:true,headers:new Headers({'content-type':'application/json'}),json:async()=>({model_aliases:aliases,setting_specs:[],section_specs:[]})})

test('settings ignores a late response from the previous request source',async()=>{
 const old=deferred();let current,renderer
 const first=()=>old.promise,second=async()=>settingsResponse({new:'model'})
 function Harness({apiFetch}){current=useSettingsForm({apiFetch,onMessage:noop,t:translate});return null}
 try{
  await act(async()=>{renderer=create(React.createElement(Harness,{apiFetch:first}))})
  await act(async()=>renderer.update(React.createElement(Harness,{apiFetch:second})))
  await act(async()=>{old.resolve(settingsResponse({old:'model'}));await old.promise})
  assert.match(current.form.model_aliases_text,/new/)
  assert.doesNotMatch(current.form.model_aliases_text,/old/)
 }finally{act(()=>renderer.unmount())}
})

test('settings saves coalesce and ignore results after unmount',async()=>{
 const saved=deferred(),messages=[];let current,renderer,puts=0,signal
 const apiFetch=async(_url,options)=>{
  if(options?.method==='PUT'){puts++;signal=options.signal;return saved.promise}
  return settingsResponse()
 }
 const onMessage=(...args)=>messages.push(args)
 function Harness(){current=useSettingsForm({apiFetch,onMessage,t:translate});return null}
 await act(async()=>{renderer=create(React.createElement(Harness))})
 let operation
 act(()=>{operation=current.saveSettings();current.saveSettings()})
 assert.equal(puts,1)
 act(()=>renderer.unmount())
 await act(async()=>{saved.resolve(settingsResponse());await operation})
 assert.equal(signal.aborted,true)
 assert.deepEqual(messages,[])
})
