// Execute the hosted runner's exact screenshot/convergence helpers in a VM.
// No browser, server, image or production data is created by these contracts.
import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {publicationFrameFits} from './publication-browser-fixture.mjs';

const source=await readFile(new URL('./browser.test.mjs',import.meta.url),'utf8');
const start=source.indexOf('async function checkResizeConvergence('),end=source.indexOf('let checks=0,runError;',start);
assert(start>=0&&end>start,'Exact screenshot and convergence helpers must exist');
const helpers=source.slice(start,end);
const geometry=(height,frameHeight)=>({root:{top:0,bottom:height,left:0,right:1440,height},frameHeight,frameWidth:1440,resizeCount:1,quietMs:200});
async function capture(sample,{childBottom=sample.root.bottom,contentWidth=1440,contentHeight=1000}={}){
 const frame={clientHeight:sample.frameHeight,clientWidth:sample.frameWidth},captures=[],writes=[],checks=[];
 const root={getBoundingClientRect:()=>sample.root,children:[{getClientRects:()=>[{}],getBoundingClientRect:()=>({bottom:childBottom})}]};
 let context;
 const globals={assert,Buffer,publicationFrameFits,body:'appRoot',appRoot:root,document:{getElementById:()=>frame},getComputedStyle:()=>({paddingBottom:'0'}),resizeCount:sample.resizeCount,lastResizeAt:1000-sample.quietMs,resizeMessages:[{}],performance:{now:()=>1000},pause:async()=>{},resetProofScroll:async()=>{},
  evaluate:async expression=>vm.runInContext(expression,context),
  until:async(fn,message)=>assert.equal(await fn(),true,message),
  check:async(expression,message)=>{assert.equal(vm.runInContext(expression,context),true,message);checks.push(message);},
  rpc:async(method,params)=>{if(method==='Page.getLayoutMetrics')return {cssContentSize:{width:contentWidth,height:contentHeight}};assert.equal(method,'Page.captureScreenshot');captures.push(params);return {data:''};},
  writeFile:async path=>writes.push(path)};
 context=vm.createContext(globals);vm.runInContext(helpers,context);
 await vm.runInContext("captureProof('catalog.png')",context);
 assert.equal(captures.length,1);assert.deepEqual(writes,['catalog.png']);assert.equal(checks.length,2,'both convergence assertions ran');
 assert.equal(captures[0].captureBeyondViewport,true);assert.equal(captures[0].clip.width,contentWidth);assert.equal(captures[0].clip.height,contentHeight);
}

test('actual report screenshot helpers accept short catalog content and the exact host-floor boundary',async()=>{
 for(const [height,frame] of [[300,500],[446,500],[483,500],[484,500],[484.25,501],[1160,1176],[1160.25,1177],[2399,2415]])await capture(geometry(height,frame));
});

test('actual report screenshot helpers retain clipping, stale-size and settled-resize guards',async()=>{
 for(const sample of [geometry(510,500),geometry(446,600),geometry(1160,1180),geometry(2400,2416),geometry(NaN,500),{...geometry(446,500),quietMs:199},{...geometry(446,500),resizeCount:0}])await assert.rejects(capture(sample),/screenshot frame did not fit/);
 for(const patch of [{top:-1},{left:-1},{right:1442},{bottom:501}]){const sample=geometry(446,500);Object.assign(sample.root,patch);await assert.rejects(capture(sample),/screenshot frame did not fit/);}
});

test('actual report screenshot helpers still reject intrinsic blank tails and image cropping',async()=>{
 await assert.rejects(capture(geometry(446,500),{childBottom:442}),/iframe follows intrinsic visible content/);
 await assert.rejects(capture(geometry(446,500),{contentWidth:1601}),/must not silently crop/);
 await assert.rejects(capture(geometry(446,500),{contentHeight:6001}),/must not silently crop/);
});
