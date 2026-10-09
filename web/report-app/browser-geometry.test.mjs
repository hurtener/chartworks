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

// Test the same measured-geometry predicate that the hosted page-settings
// journey invokes, including the prior horizontally stretched screenshot shape.
const pageStart=source.indexOf('function pageSettingsFit('),pageEnd=source.indexOf('async function checkPageSettingsGeometry(',pageStart);
assert(pageStart>=0&&pageEnd>pageStart,'Hosted page-settings geometry predicate must exist');
const pageContext=vm.createContext({});vm.runInContext(source.slice(pageStart,pageEnd),pageContext);
const box=(left,top,width,height)=>({left,top,right:left+width,bottom:top+height,width,height,scrollWidth:width,clientWidth:width});
const pageGeometry=()=>({bar:box(24,230,1392,416),tabs:box(24,234,1380,34),buttons:[box(24,234,85,34),box(129,234,72,34),box(221,234,110,34),box(351,234,88,34)],settings:box(24,272,1392,370),fields:[box(24,326,480,44),box(24,400,480,44),box(24,474,480,44)],viewportWidth:1440});
const pageFits=sample=>{pageContext.sample=sample;return vm.runInContext('pageSettingsFit(sample)',pageContext);};
test('hosted page-settings geometry accepts a compact top tab row and readable settings below',()=>{
 assert.equal(pageFits(pageGeometry()),true);
 assert(source.includes("if(selector==='.page-controls')await checkPageSettingsGeometry()"),'expanded proof must invoke the measured check');
 assert(source.includes("await captureExpandedProof('.page-controls',['Page title','Page locale','Page timezone'],screenshotPath?"),'the geometry journey must run even when no screenshot path is requested');
});
test('hosted page-settings geometry rejects hidden tabs, stretched rows and clipped settings',()=>{
 for(const mutate of [
  g=>{g.tabs=box(24,400,140,34);g.buttons=[box(24,400,85,34),box(129,400,72,34),box(221,400,110,34),box(351,400,88,34)];g.settings=box(178,234,1238,370);},
  g=>{g.tabs.scrollWidth=1600;},g=>{g.buttons.pop();},g=>{g.buttons[2]=box(221,300,110,34);},g=>{g.buttons[2]=box(1400,234,110,34);},
  g=>{g.settings=box(24,272,1392,2000);g.bar=box(24,230,1392,2100);},g=>{g.fields[1]=box(24,400,240,44);},g=>{g.fields[1]=box(24,400,1500,44);},
  g=>{g.fields[2].scrollWidth=600;},g=>{g.fields[0]=null;},g=>{g.fields[0].height=0;},g=>{g.tabs.height=100;},g=>{g.bar.right=1600;}
 ]){const sample=pageGeometry();mutate(sample);assert.equal(pageFits(sample),false);}
});
