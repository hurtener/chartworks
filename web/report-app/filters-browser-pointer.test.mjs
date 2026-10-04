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
