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
const {useAccountsData}=await server.ssrLoadModule('/src/features/account/useAccountsData.js')
const noop=()=>{}, translate=key=>key

test('Vercel status polling schedules another successful poll',async t=>{
 const timers=new Map();let id=0,statusCalls=0,renderer
 const nativeTimeout=global.setTimeout,nativeClear=global.clearTimeout
 // Track the successful poll cadence only. Vite's SSR maintenance timers
 // may run between React flushes and are unrelated to status polling.
 t.mock.method(global,'setTimeout',(fn,delay,...args)=>{if(delay===15000){const n=++id;timers.set(n,fn);return n};return nativeTimeout(fn,delay,...args)})
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

const accountResponse = name => ({ok:true,json:async()=>({items:[{identifier:name}],page:1,total:1,total_pages:1})})

test('slow queue polling completes without being repeatedly aborted',async t=>{
 const intervals=new Set(),queue=deferred();let current,renderer,calls=0,signal
 t.mock.method(global,'setInterval',fn=>{intervals.add(fn);return fn})
 t.mock.method(global,'clearInterval',id=>intervals.delete(id))
 const apiFetch=async(url,options)=>{
  if(url==='/admin/queue/status'){calls++;signal=options.signal;return queue.promise}
  return accountResponse('account')
 }
 function Harness(){current=useAccountsData({apiFetch,t:translate,onMessage:noop,onConfigRefresh:noop});return null}
 try{
  await act(async()=>{renderer=create(React.createElement(Harness))})
  await act(async()=>{for(let i=0;i<3;i++)for(const tick of intervals)tick()})
  assert.equal(calls,1,'polling started a replacement while the queue request was pending')
  assert.equal(signal.aborted,false)
  await act(async()=>{queue.resolve({ok:true,json:async()=>({in_use:2})});await queue.promise})
  assert.equal(current.queueStatus.in_use,2)
 }finally{act(()=>renderer.unmount())}
 assert.equal(intervals.size,0)
})

test('account polling replaces its request source and ignores late old responses',async t=>{
 const intervals=new Set(),old=deferred(),signals=[];let current,renderer
 t.mock.method(global,'setInterval',fn=>{intervals.add(fn);return fn})
 t.mock.method(global,'clearInterval',id=>intervals.delete(id))
 const first=async(_url,options)=>{signals.push(options.signal);return old.promise}
 const second=async url=>url==='/admin/queue/status'
  ? {ok:true,json:async()=>({in_use:3})} : accountResponse('new')
 function Harness({apiFetch}){current=useAccountsData({apiFetch,t:translate,onMessage:noop,onConfigRefresh:noop});return null}
 try{
  await act(async()=>{renderer=create(React.createElement(Harness,{apiFetch:first}))})
  await act(async()=>renderer.update(React.createElement(Harness,{apiFetch:second})))
  assert.ok(signals.every(signal=>signal.aborted))
  assert.equal(intervals.size,1)
  await act(async()=>{old.resolve(accountResponse('old'));await old.promise})
  assert.equal(current.accounts[0].identifier,'new')
  assert.equal(current.queueStatus.in_use,3)
  assert.equal(current.loadingAccounts,false)
 }finally{act(()=>renderer.unmount())}
 assert.equal(intervals.size,0)
})


test('temporary admin config stays in memory and erases legacy stored keys',async t=>{
 const {useAdminConfig}=await server.ssrLoadModule('/src/app/useAdminConfig.js')
 const items=new Map([['ds2api_env_config_draft_v1','{"keys":["old-secret"]}']])
 const previous=global.localStorage
 global.localStorage={getItem:k=>items.get(k)??null,setItem:(k,v)=>items.set(k,v),removeItem:k=>items.delete(k)}
 t.mock.method(global,'fetch',async()=>({ok:true,json:async()=>({env_backed:true,keys:['current-key']})}))
 let current,renderer
 function Harness(){current=useAdminConfig({token:'session-token',showMessage:noop,t:translate});return null}
 try{
  await act(async()=>{renderer=create(React.createElement(Harness))})
  assert.deepEqual(current.config.keys,['current-key'])
  assert.equal(items.has('ds2api_env_config_draft_v1'),false)
 }finally{act(()=>renderer?.unmount());if(previous===undefined)delete global.localStorage;else global.localStorage=previous}
})

test('TTS unmount aborts synthesis and discards late audio without creating a blob URL',async t=>{
 const {default:TtsTester}=await server.ssrLoadModule('/src/features/tts/TtsTesterContainer.jsx')
 const synthesis=deferred();let signal,renderer,operation,blobReads=0
 t.mock.method(global,'fetch',(_url,options)=>{signal=options.signal;return synthesis.promise})
 const authFetch=async()=>({ok:true,json:async()=>({tts:{enabled:true}})})
 try{
  await act(async()=>{renderer=create(React.createElement(TtsTester,{config:{keys:['test-key']},authFetch}))})
  act(()=>renderer.root.findByType('textarea').props.onChange({target:{value:'test synthesis'}}))
  act(()=>{operation=renderer.root.findByType('button').props.onClick()})
  act(()=>renderer.unmount())
  assert.equal(signal.aborted,true)
  await act(async()=>{synthesis.resolve({ok:true,blob:async()=>{blobReads++;return new Blob(['audio'])}});await operation})
  assert.equal(blobReads,0)
 }finally{if(renderer)act(()=>renderer.unmount())}
})

test('late password updates cannot log out a replacement session',async()=>{
 const pending=deferred(),messages=[];let current,renderer,operation,signal,logouts=0
 const apiFetch=async(url,options)=>{
  if(url.endsWith('/password')){signal=options.signal;return pending.promise}
  return settingsResponse()
 }
 function Harness(){current=useSettingsForm({apiFetch,t:translate,onMessage:(...args)=>messages.push(args),onForceLogout:()=>{logouts++}});return null}
 await act(async()=>{renderer=create(React.createElement(Harness))})
 act(()=>current.setNewPassword('new-password'))
 act(()=>{operation=current.updatePassword()})
 act(()=>renderer.unmount())
 assert.equal(signal.aborted,true)
 await act(async()=>{pending.resolve(settingsResponse());await operation})
 assert.equal(logouts,0)
 assert.deepEqual(messages,[])
})
