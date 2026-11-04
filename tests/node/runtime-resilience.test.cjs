'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { detectMutedOrBannedResponse } = require('../../internal/js/chat-stream/upstream_inspect');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream');

test('oversized inspected JSON preserves the unconsumed suffix', async () => {
  const prefix = Buffer.from('{"message":"' + 'x'.repeat((1 << 20) + 20));
  const suffix = Buffer.from('the-important-suffix"}');
  const stream = new ReadableStream({ start(c) { c.enqueue(prefix); c.enqueue(suffix); c.close(); } });
  const { response } = await detectMutedOrBannedResponse(new Response(stream, { headers: { 'content-type': 'application/json' } }));
  const text = await response.text();
  assert.equal(text.endsWith(suffix.toString()), true, `lost suffix: original=${prefix.length+suffix.length}, replayed=${text.length}`);
});

function surfaces() {
 const req=Object.assign(new EventEmitter(),{url:'/v1/chat/completions',headers:{host:'example.test'}});
 const res=Object.assign(new EventEmitter(),{chunks:[],writableEnded:false,destroyed:false,setHeader(){},flushHeaders(){},flush(){},write(v){this.chunks.push(String(v));return true},end(v){if(v)this.write(v);this.writableEnded=true}});
 return {req,res};
}
const json=body=>new Response(JSON.stringify(body),{headers:{'content-type':'application/json'}});
const prepared={model:'model',session_id:'session',lease_id:'lease',deepseek_token:'token',pow_header:'pow',payload:{prompt:'hello'},final_prompt:'hello',managed_account:true,failover_budget_managed:true};

test('management API paths route to the Go function in Vercel',()=>{
 const {rewrites}=require('../../vercel.json');
 const paths=['/admin/usage','/admin/usage/settings','/admin/tool-call-records','/admin/tool-call-records/repair-settings','/admin/mihomo/status'];
 const observed=paths.map(path=>({path,destination:rewrites.find(rule=>!rule.has && new RegExp('^'+rule.source+'$').test(path))?.destination}));
 assert.deepEqual(observed,paths.map(path=>({path,destination:'/api/index'})));
});

test('pre-SSE inspection obeys the stream idle deadline', async t => {
 const old=process.env.DS2API_STREAM_IDLE_TIMEOUT_MS;process.env.DS2API_STREAM_IDLE_TIMEOUT_MS='20';
 t.after(()=>{if(old===undefined)delete process.env.DS2API_STREAM_IDLE_TIMEOUT_MS;else process.env.DS2API_STREAM_IDLE_TIMEOUT_MS=old});
 t.mock.method(global,'fetch',async url=>String(url).includes('__stream_prepare')?json(prepared):json({success:true}));
 let controller;const source=new ReadableStream({start(c){controller=c}});
 const {req,res}=surfaces();let settled=false;
 const operation=handleVercelStream(req,res,Buffer.from('{}'),{},async()=>new Response(source)).then(()=>{settled=true},()=>{settled=true});
 await new Promise(resolve=>setTimeout(resolve,90));const before=settled;
 if (!before) controller.close();await operation;
 assert.equal(before,true,'headers arrived, no body bytes: 20ms idle limit did not apply during inspection');
});

test('initial network failure attempts account recovery', async t => {
 let switches=0;
 t.mock.method(global,'fetch',async url=>{
  if(String(url).includes('__stream_prepare'))return json(prepared);
  if(String(url).includes('__stream_switch')){switches++;return json(prepared)}
  return json({success:true});
 });
 const {req,res}=surfaces();let attempts=0,error;
 try {await handleVercelStream(req,res,Buffer.from('{}'),{},async()=>{attempts++;if(attempts===1)throw new Error('ECONNRESET');return new Response('data: {"p":"response/content","v":"ok"}\n\ndata: [DONE]\n\n')})}catch(e){error=e.message}
 assert.ok(switches>0,`network error escaped recovery: ${error}; switches=${switches}`);
});

const {readResponseChunk, readErrorText}=require('../../internal/js/chat-stream/response_read');
test('expired or cancelled inspection never consumes another body chunk',async()=>{
 for(const options of [{signal:AbortSignal.abort()},{deadline:Date.now()-1}]){
  let reads=0,cancels=0;
  const reader={read(){reads++;return Promise.resolve({done:true})},cancel(){cancels++;return Promise.resolve()}};
  await assert.rejects(readResponseChunk(reader,options));
  assert.equal(reads,0);assert.equal(cancels,1);
 }
});
test('HTTP error body reads are bounded and explicitly mark truncation',async()=>{
 const text=await readErrorText(new Response('x'.repeat(70000),{status:503}));
 assert.match(text,/truncated at 64 KiB/);
 assert.ok(text.length<66000);
});
