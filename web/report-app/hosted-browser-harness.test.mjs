import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {runHostedBrowser, hostedPointerReady} from './hosted-browser-harness.mjs';
import {initializePublicationHost} from './publication-browser-fixture.mjs';

const sample=(patch={})=>({x:1100,y:600,viewportWidth:1440,viewportHeight:1000,resizeCount:2,quietMs:250,
  frameFits:true,frameHit:true,targetHit:true,geometry:[1000,580,200,40,0,20,1440,850,1440,834,0,0,0,0],...patch});
test('shared pointer requires two stable samples and both-document hit tests',()=>{
  const initial=sample();assert(hostedPointerReady(sample(),initial));assert(!hostedPointerReady(initial,null));
  for(const patch of [{quietMs:199},{resizeCount:3},{frameFits:false},{frameHit:false},{targetHit:false},{x:1440},{y:1000},{x:0},{y:0},{geometry:null}])assert(!hostedPointerReady(sample(patch),initial),JSON.stringify(patch));
  for(let i=0;i<initial.geometry.length;i++){const moved=sample();moved.geometry[i]+=1;assert(!hostedPointerReady(moved,initial));}
});

test('hosted-only guard rejects before HTML reads, servers or process creation',async()=>{
  const old=process.env.GITHUB_ACTIONS;delete process.env.GITHUB_ACTIONS;
  try{await assert.rejects(runHostedBrowser({htmlPath:'/not-read',screenshotPath:'/not-created.png',mode:'mcp'},()=>assert.fail('No journey')),/hosted CI job/);}
  finally{if(old===undefined)delete process.env.GITHUB_ACTIONS;else process.env.GITHUB_ACTIONS=old;}
});

function host(embedded,allocation){
  const listeners={},sent=[],calls=[],frameListeners={},frame={style:{},addEventListener:(name,fn)=>frameListeners[name]=fn,contentWindow:{postMessage:(value,origin)=>sent.push({value:structuredClone(value),origin})}};
  const sandbox={document:{getElementById:()=>frame},location:{origin:'https://report-host.example'},performance:{now:()=>1000},Map,Promise,JSON};
  sandbox.window=sandbox;sandbox.addEventListener=(name,fn)=>listeners[name]=fn;sandbox.publicationInvoke=wire=>calls.push(JSON.parse(wire));
  vm.createContext(sandbox);vm.runInContext(`(${initializePublicationHost.toString()})(${embedded},${JSON.stringify({version:'chartworks-host-tools-v1',names:['reporting_search']})},${allocation})`,sandbox);
  const send=(method,params,id=1)=>listeners.message({source:frame.contentWindow,origin:sandbox.location.origin,data:embedded?{protocol:'chartworks-report-app-v1',frame:'publication-frame',generation:1,id,method,params}:{jsonrpc:'2.0',id,method,params}});
  frameListeners.load();send(embedded?'initialize':'ui/initialize',embedded?{challenge:'synthetic-publication-challenge'}:{protocolVersion:'2026-01-26'});
  return {sandbox,frame,sent,calls,send};
}
for(const embedded of [false,true])test(`${embedded?'embedded':'MCP'} shared host retains exact handshake and optional allocation separation`,async()=>{
  const h=host(embedded,true),handshake=h.sent.at(-1).value.result;
  const capabilities=embedded?handshake.capabilities:handshake.hostContext;
  assert.equal(capabilities[embedded?'target_allocation':'chartworks/target-allocation'].version,'report-app-allocation-v1');
  h.send('tools/call',{name:'reporting_search',arguments:{query:''}},3);assert.equal(h.calls.length,1);assert.equal(h.calls[0].name,'reporting_search');
  h.sandbox.completePublicationCall(h.calls[0].id,{structuredContent:{result:{items:[]}}});await Promise.resolve();await Promise.resolve();
  assert.equal(h.sent.at(-1).value.id,3);assert.equal(h.sandbox.hostCalls.length,1);
  const allocation={version:'report-app-allocation-v1',kind:'block',intent:'copy_chart',idempotency_key:'stable-test',source:{block:'source'}};
  h.send('app/allocate-target',allocation,4);assert.equal(h.calls.at(-1).name,'app/allocate-target');assert.deepEqual(h.calls.at(-1).args,allocation);assert.equal(h.sandbox.hostAllocations.length,1);assert.equal(h.sandbox.hostCalls.length,1);
  const result={version:allocation.version,kind:allocation.kind,intent:allocation.intent,idempotency_key:allocation.idempotency_key,id:'copy'};
  h.sandbox.completePublicationCall(h.calls.at(-1).id,result);await Promise.resolve();await Promise.resolve();assert.deepEqual(h.sent.at(-1).value.result,result);
  assert.throws(()=>h.sandbox.completePublicationCall(999,{}),/Uncorrelated/);
  h.send('ui/notifications/size-changed',{height:620});assert.equal(h.frame.style.height,'636px');h.sandbox.closeApp();assert.equal(h.sent.at(-1).value.method,embedded?'close':'ui/resource-teardown');
  const legacy=host(embedded,false),cap=embedded?legacy.sent.at(-1).value.result.capabilities:legacy.sent.at(-1).value.result.hostContext;
  assert.equal(cap[embedded?'target_allocation':'chartworks/target-allocation'],undefined,'Existing publication fixture still advertises no allocation');
});

test('both presentation and publication use one bounded process and network driver',async()=>{
  const ci=await readFile(new URL('../../.github/workflows/ci.yml',import.meta.url),'utf8');
  assert(ci.includes('node --test web/report-app/hosted-browser-harness.test.mjs web/report-app/presentation-browser-fixture.test.mjs'),'New source contracts are registered in the ordinary hosted CI job');
  const source=await readFile(new URL('./hosted-browser-harness.mjs',import.meta.url),'utf8');
  assert(source.includes('cleanupBrowserFixture({browser, closed, directory'));
  assert(source.includes('120000'));
  assert(source.includes("url.origin !== origin || !['/', '/resource', '/favicon.ico'].includes(url.pathname) || url.search || request.method !== 'GET'"));
  assert(source.includes("equal(errors, [], 'No browser exceptions or unexpected requests')"));
  assert(source.includes('fixture.clockScript()'));
  for(const file of ['publication.browser.mjs','presentation.browser.mjs']){
    const driver=await readFile(new URL('./'+file,import.meta.url),'utf8');assert(driver.includes('runHostedBrowser'));assert(!driver.includes("from 'node:child_process'"));
  }
});

test('publication journey binds its host-side assertion recorder',async()=>{
 const driver=await readFile(new URL('./publication.browser.mjs',import.meta.url),'utf8');
 assert(driver.includes('assertions.push('),'Existing post-run authority assertions remain present');
 const parameters=/async\s*\(\{([^}]+)\}\)\s*=>/.exec(driver)?.[1].split(',').map(v=>v.trim());
 assert(parameters?.includes('assertions'),'Recorder must come from the shared harness, not an undefined global');
 const harness=await readFile(new URL('./hosted-browser-harness.mjs',import.meta.url),'utf8');
 assert(/await run\(\{[^}]*\bassertions\b/.test(harness),'Shared harness supplies the same recorder');
});
