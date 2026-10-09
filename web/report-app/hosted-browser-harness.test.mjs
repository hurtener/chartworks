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

const presentationSource=await readFile(new URL('./presentation.browser.mjs',import.meta.url),'utf8');
const formattingStart=presentationSource.indexOf('function formattingInspectorFits('),formattingEnd=presentationSource.indexOf('// End compact-inspector geometry contract.',formattingStart);
assert(formattingStart>=0&&formattingEnd>formattingStart,'Exact compact formatting geometry predicate is present in the hosted journey');
const formattingFits=vm.runInNewContext('('+presentationSource.slice(formattingStart,formattingEnd)+')');
const inspectorSample=(patch={})=>({viewportWidth:1440,viewportHeight:1000,requestedFrameWidth:1440,documentClientWidth:1440,documentScrollWidth:1440,scrollbarGutter:0,frameWidth:1440,frameLeft:0,frameRight:1440,
 editorHeight:620,editorWidth:280,editorClientWidth:280,editorScrollWidth:280,fieldCount:1,selectorCount:1,inputCount:2,
 save:{top:320,bottom:364,left:1100,right:1240,width:140,height:44,hit:true},
 cancel:{top:370,bottom:414,left:1100,right:1250,width:150,height:44,hit:true},...patch});

test('formatting inspector enforces compact single-field layout and visible primary actions at desktop and narrower iframe widths',()=>{
 assert.equal(formattingFits(inspectorSample()),true);
 assert.equal(formattingFits(inspectorSample({requestedFrameWidth:960,frameWidth:960,frameRight:960,editorWidth:240,editorClientWidth:240,editorScrollWidth:240,
  save:{top:410,bottom:454,left:700,right:840,width:140,height:44,hit:true},cancel:{top:460,bottom:504,left:700,right:850,width:150,height:44,hit:true}})),true);
 assert.equal(formattingFits(inspectorSample({editorHeight:760,editorWidth:360,editorClientWidth:360,editorScrollWidth:361,inputCount:1})),true,'Exact upper bounds and KPI one-input form are accepted');
 for(const patch of [{editorHeight:761},{editorHeight:1600},{editorWidth:361},{editorClientWidth:0},{editorScrollWidth:282},{fieldCount:0},{fieldCount:3},{selectorCount:0},{selectorCount:2},{inputCount:0},{inputCount:5},{viewportWidth:960},{viewportHeight:999},{frameWidth:959},{frameLeft:-1},{frameRight:1441}])assert.equal(formattingFits(inspectorSample(patch)),false,JSON.stringify(patch));
});

test('desktop inspector uses the actual document scrollbar gutter and rejects unexplained or clipped width changes',()=>{
 const desktop=inspectorSample({documentClientWidth:1425,documentScrollWidth:1425,scrollbarGutter:15,frameWidth:1425,frameRight:1425,
  editorHeight:675.640625,editorWidth:311,editorClientWidth:311,editorScrollWidth:311,
  save:{top:367.6875,bottom:411.6875,left:1094,right:1210,width:116,height:44,hit:true},
  cancel:{top:367.6875,bottom:411.6875,left:1218,right:1344,width:126,height:44,hit:true}});
 assert.equal(formattingFits(desktop),true,'Exact hosted Chrome 1425px content width fits the 1440px desktop viewport');
 const narrow={...desktop,requestedFrameWidth:960,frameWidth:960,frameRight:960,
  save:{...desktop.save,left:700,right:816},cancel:{...desktop.cancel,left:824,right:950}};
 assert.equal(formattingFits(narrow),true,'Narrow frame remains exactly the requested 960px despite the outer gutter');
 for(const patch of [{documentClientWidth:1440},{scrollbarGutter:0},{scrollbarGutter:-15},{documentScrollWidth:1440},{frameWidth:1440,frameRight:1440},{frameWidth:1424,frameRight:1424},{requestedFrameWidth:1425},{documentClientWidth:959,documentScrollWidth:959,scrollbarGutter:481,frameWidth:959,frameRight:959},{documentClientWidth:1441,scrollbarGutter:-1,frameWidth:1441,frameRight:1441}])assert.equal(formattingFits({...desktop,...patch}),false,JSON.stringify(patch));
 for(const sample of [desktop,narrow])for(const key of ['save','cancel'])assert.equal(formattingFits({...sample,[key]:{...sample[key],right:sample.frameRight+1}}),false,key+' cannot extend into the gutter or beyond the narrow frame');
 assert.equal(formattingFits({...narrow,frameWidth:945,frameRight:945}),false,'The outer gutter is not subtracted twice from the fixed narrow width');
 assert(presentationSource.includes("width===1440?'100%':'960px'"),'Desktop resize restores document-relative width rather than causing horizontal overflow');
 assert(presentationSource.includes('expected=Math.min(${width},available)'));
 assert(presentationSource.includes('f.clientWidth===expected&&f.contentWindow.innerWidth===expected'));
 assert(presentationSource.includes('documentClientWidth:document.documentElement.clientWidth'));
 assert(presentationSource.includes('scrollbarGutter:innerWidth-document.documentElement.clientWidth'));
});

test('formatting action reachability rejects scroll-dependent, clipped, occluded or missing evidence',()=>{
 assert.equal(formattingFits(null),false);assert.equal(formattingFits({}),false);
 for(const key of ['save','cancel'])for(const change of [{top:-1},{bottom:1001},{left:-1},{right:1441},{width:0},{height:0},{hit:false},{top:NaN},{left:Infinity},{bottom:319},{right:1099}]){
  const sample=inspectorSample();Object.assign(sample[key],change);assert.equal(formattingFits(sample),false,key+' '+JSON.stringify(change));
 }
 for(const key of ['documentClientWidth','documentScrollWidth','scrollbarGutter','frameWidth','frameLeft','frameRight','editorHeight','editorWidth','editorClientWidth','editorScrollWidth'])assert.equal(formattingFits(inspectorSample({[key]:NaN})),false,key);
 const absent=inspectorSample();delete absent.cancel;assert.equal(formattingFits(absent),false);
});

test('hosted presentation uses real keyboard selection, proves staged cross-field edits and keeps original exact native mutation invariants',()=>{
 for(const required of ['select[aria-label="Format field"]',"await key('Home','Home',36)","await key('ArrowDown','ArrowDown',40)","${focused}===${fieldSelector}","await chooseFormatField(c)","await chooseFormatField(column)","Reset '+label+' returns focus",'await resizeFormattingFrame(960)','await resizeFormattingFrame(1440)',"captureAs('table-inspector-narrow')",'draft','formattingGeometry','JSON.stringify(geometry)'])assert(presentationSource.includes(required),required);
 assert(presentationSource.includes('noCalls(\'Selecting \''),'Field selection is explicitly tool-free');
 assert(presentationSource.includes('Responsive host resizing preserves the selected field and all staged edits'));
 assert(presentationSource.includes('Cancel discards the other field precision stage'));
 assert(presentationSource.includes('Cancel discards the other field label stage'));
 for(const preserved of ["'conflict','unknown','late-read','late-save'",'repeat:2','mutation_request','Private preview','Percent delta: ',"captureAs('table-inspector')","captureAs('kpi-inspector')"])assert(presentationSource.includes(preserved),preserved);
});
