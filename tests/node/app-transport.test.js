'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const http2 = require('node:http2');
const zlib = require('node:zlib');
const { once } = require('node:events');
const { buildAppHeaders, fetchAppStream } = require('../../internal/js/shared/app-transport');
const { selectAnchoredArchive } = require('../../scripts/devid-generate.cjs');
test('HTTP/2 wire keeps App headers, gzip SSE and UTF-8 byte length', async () => {
 const server=http2.createServer();let observed;
 server.on('stream',(stream,headers)=>{let body='';stream.on('data',chunk=>{body+=chunk;});stream.on('end',()=>{observed={headers,body};stream.respond({':status':200,'content-type':'text/event-stream','content-encoding':'gzip'});stream.end(zlib.gzipSync('data: [DONE]\n\n'));});});
 server.listen(0,'127.0.0.1');await once(server,'listening');
 try {
  const response=await fetchAppStream(`http://127.0.0.1:${server.address().port}/api/v0/chat/completion`,{method:'POST',headers:{'x-client-platform':'android','x-ds-pow-response':'pow','User-Agent':'DeepSeek/2.7.1 Android/36'},body:'{"prompt":"你好"}'});
  assert.equal(await response.text(),'data: [DONE]\n\n');assert.equal(observed.headers['content-length'],String(Buffer.byteLength(observed.body)));
  assert.equal(observed.headers['accept-language'],undefined);assert.equal(observed.headers['sec-fetch-mode'],undefined);
  assert.deepEqual(Object.keys(observed.headers).slice(0,6),[':method',':authority',':path',':scheme','x-ds-pow-response','x-client-platform']);
 } finally {await new Promise(resolve=>server.close(resolve));}
});
test('already aborted stream makes no connection',async()=>{
 const controller=new AbortController();controller.abort();await assert.rejects(fetchAppStream('https://example.invalid',{signal:controller.signal}),{name:'AbortError'});
 const h=buildAppHeaders('https://example.test/a',{headers:{Host:'wrong','Accept-Language':'*','sec-fetch-mode':'cors'},body:'你'});assert.equal(h[':authority'],'example.test');assert.equal(h['content-length'],'3');assert.equal(h['accept-language'],undefined);
});
test('saved profile adopts locale and timezone changes',()=>{
 const profile={osFamily:'android',deviceModel:'Pixel 9',androidVersion:16,locale:'zh-CN',tzName:'Asia/Shanghai',tzOffset:-480,screenW:412};
 const next=selectAnchoredArchive({profile,smidV2:'same-device'},{device_model:'Pixel 9',android_version:16,locale:'en-US',tz:'America/Los_Angeles',tz_offset:420},{},null,1);
 assert.equal(next.profile.locale,'en-US');assert.equal(next.profile.tzOffset,420);assert.equal(next.profile.screenW,412);assert.equal(next.smidV2,'same-device');assert.equal(profile.locale,'zh-CN');
});
test('cached HIF headers expire before each outgoing request',()=>{
 const h=buildAppHeaders('https://example.test/a',{headers:{'x-hif-dliq':'expired','x-hif-leim':'fresh'},headerExpirations:{'x-hif-dliq':1,'x-hif-leim':Math.floor(Date.now()/1000)+60},body:'{}'});
 assert.equal(h['x-hif-dliq'],undefined);assert.equal(h['x-hif-leim'],'fresh');
});
