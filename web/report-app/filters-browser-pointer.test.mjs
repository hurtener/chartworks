// Exercises the exact hosted-runner pure predicate without importing its
// process/server startup. These are geometry regressions, not browser evidence.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const source=await readFile(new URL('./filters.browser.mjs',import.meta.url),'utf8');
const start=source.indexOf('function filterPointerReady('),end=source.indexOf('// End pointer readiness contract.',start);
assert(start>=0&&end>start,'Exact runner pointer readiness function must exist');
const ready=vm.runInNewContext('('+source.slice(start,end)+')');
const sample=(patch={})=>({x:1180,y:500,viewportWidth:1440,viewportHeight:1000,resizeCount:10,quietMs:250,frameFits:true,frameHit:true,targetHit:true,
  geometry:[1100,880,160,40,0,-400,1440,1372,1440,1356,0,400,0,0],...patch});

test('filter pointer requires stable resize, scroll and layout before dispatch',()=>{
  const previous=sample();assert.equal(ready(sample(),previous),true);
  assert.equal(ready(previous,null),false,'one fresh sample is insufficient');
  assert.equal(ready(sample({quietMs:199}),previous),false,'host resize has not settled');
  assert.equal(ready(sample({resizeCount:11}),previous),false,'equal target point does not hide an intervening iframe resize');
  for(const index of [0,1,2,3,4,5,6,7,8,9,10,11,12,13]){
    const moved=sample();moved.geometry[index]+=1;assert.equal(ready(moved,previous),false,'stale geometry coordinate '+index);
  }
  const collapsed=sample({resizeCount:11,geometry:[1100,880,160,40,0,-100,1440,690,1440,674,0,100,0,0]});
  const expanded=sample({resizeCount:12});
  assert.equal(ready(collapsed,previous),false,'observed 1356 to 674 collapse cannot reuse the old pointer');
  assert.equal(ready(expanded,collapsed),false,'observed re-expansion requires a new stable sample');
  assert.equal(ready(sample({resizeCount:12}),expanded),true,'settled expanded geometry becomes usable');
});

test('filter pointer hit-tests both realms and remains strict about clipping',()=>{
  const previous=sample();
  for(const patch of [{frameHit:false},{targetHit:false},{frameFits:false},{x:0},{y:0},{x:1440},{y:1000},{quietMs:NaN},{geometry:null}])assert.equal(ready(sample(patch),previous),false);
  // An outer document scroll legitimately gives the frame a negative top. The
  // actual center and both hit-tests decide pointer reach; root.top is not zero.
  assert(previous.geometry[5]<0);assert.equal(ready(sample(),previous),true);
  const bad=sample();bad.geometry[0]=NaN;assert.equal(ready(bad,previous),false);
});

const inspectorStart=source.indexOf('function filterInspectorFits('),inspectorEnd=source.indexOf('// End compact inspector contract.',inspectorStart);
assert(inspectorStart>=0&&inspectorEnd>inspectorStart,'Exact hosted compact-inspector predicate must exist');
const inspectorFits=vm.runInNewContext('('+source.slice(inspectorStart,inspectorEnd)+')');
const inspectorSample=(patch={})=>({viewportWidth:1440,viewportHeight:1000,rootHeight:850,rootWidth:1440,
  panelWidth:352,panelHeight:660,inspectorHeight:644,editorHeight:470,inspectorTopOffset:0,
  panelLeft:1088,panelRight:1440,editorLeft:1109,editorRight:1420,editorWidth:311,
  inspectorScrollWidth:351,inspectorClientWidth:351,editorScrollWidth:309,editorClientWidth:309,...patch});

test('focused filter geometry enforces desktop compactness rather than a merely uncropped full page',()=>{
  assert.equal(inspectorFits(inspectorSample()),true);
  for(const patch of [{viewportWidth:1439},{viewportHeight:999},{rootHeight:1001},{rootWidth:1441},
    {panelWidth:361},{panelHeight:801},{inspectorHeight:761},{editorHeight:661},{inspectorTopOffset:33},
    {inspectorTopOffset:-1},{editorLeft:1087},{editorRight:1442},{editorWidth:361},
    {inspectorScrollWidth:353},{editorScrollWidth:311}])assert.equal(inspectorFits(inspectorSample(patch)),false,JSON.stringify(patch));
  assert.equal(inspectorFits(inspectorSample({rootHeight:1000,panelWidth:360,panelHeight:800,inspectorHeight:760,editorHeight:660,inspectorTopOffset:32})),true,'Exact inclusive upper bounds are accepted');
});

test('focused filter geometry rejects absent, nonfinite or zero-sized evidence',()=>{
  assert.equal(inspectorFits(null),false);assert.equal(inspectorFits({}),false);
  for(const key of Object.keys(inspectorSample()))assert.equal(inspectorFits(inspectorSample({[key]:NaN})),false,key);
  for(const key of ['rootHeight','rootWidth','panelWidth','panelHeight','inspectorHeight','editorHeight','editorWidth']){
    assert.equal(inspectorFits(inspectorSample({[key]:0})),false,key);
    assert.equal(inspectorFits(inspectorSample({[key]:-1})),false,key);
  }
});
