import test from 'node:test';
import assert from 'node:assert/strict';
import {canPlace,moveWidget,resizeWidget,placeWidget,duplicateWidget,firstFreeCell,validCell,arrangeWidgets} from './grid.js';

const widgets=()=>[
  {id:'a',grid:{column:0,row:0,width:4,height:2},presentation:{title:'Revenue'},block:{block:'sales',revision:3,outputs:['kpi'],narrative:false}},
  {id:'b',grid:{column:6,row:3,width:6,height:3},presentation:{title:'Trend'}},
];

test('move and resize preserve unrelated custom geometry and nested data',()=>{
  const before=widgets(),moved=moveWidget(before,'a',1,1),resized=resizeWidget(moved,'a',5,2);
  assert.deepEqual(resized[0].grid,{column:1,row:1,width:5,height:2});
  assert.deepEqual(resized[1],before[1]);assert.deepEqual(before,widgets());
  resized[0].block.outputs.push('table');resized[1].presentation.title='changed';
  assert.deepEqual(before,widgets());
});

test('touching edges are allowed, collisions and out-of-bounds geometry reject without mutation',()=>{
  const before=widgets();
  assert.equal(canPlace(before,'a',{column:2,row:3,width:4,height:3}),true);
  for(const cell of [{column:3,row:3,width:4,height:3},{column:-1,row:0,width:4,height:2},{column:9,row:0,width:4,height:2},{column:0,row:9999,width:4,height:2},{column:0,row:0,width:4,height:101},{column:0.5,row:0,width:4,height:2}]) {
    assert.equal(canPlace(before,'a',cell),false);assert.throws(()=>placeWidget(before,'a',cell));
  }
  assert.deepEqual(before,widgets());assert.throws(()=>moveWidget(before,'missing',0,0));
});

test('duplicate uses a free rectangle and preserves all original placements',()=>{
  const before=widgets(),next=duplicateWidget(before,'a','new-a');
  assert.deepEqual(next.slice(0,2),before);assert.equal(next[2].id,'new-a');
  assert.deepEqual(next[2].grid,{column:4,row:0,width:4,height:2});
  assert.deepEqual(next[2].block,before[0].block);next[2].block.outputs.push('other');assert.deepEqual(before,widgets());
  assert.throws(()=>duplicateWidget(before,'a','b'));assert.throws(()=>duplicateWidget(before,'none','new'));
});

test('first-free search respects height, preferred position and document bounds',()=>{
  const occupied=[{id:'top',grid:{column:0,row:0,width:12,height:4}}];
  assert.deepEqual(firstFreeCell(occupied,{width:6,height:2,column:6}),{column:6,row:4,width:6,height:2});
  assert.throws(()=>firstFreeCell(Array.from({length:100},(_,i)=>({id:String(i),grid:{column:0,row:i*100,width:12,height:100}})),{width:1,height:1}));
  assert.equal(validCell({column:0,row:9900,width:12,height:100}),true);
  assert.throws(()=>firstFreeCell([{id:'end',grid:{column:0,row:9900,width:12,height:100}}],{column:0,row:9999,width:1,height:1}));
});

test('explicit arrangement preserves heights and starts subsequent rows below tallest cells',()=>{
  const before=[...widgets(),{id:'c',grid:{column:0,row:7,width:12,height:4}}],next=arrangeWidgets(before,2);
  assert.deepEqual(next.map(widget=>widget.grid),[
    {column:0,row:0,width:6,height:2},
    {column:6,row:0,width:6,height:3},
    {column:0,row:3,width:6,height:4},
  ]);
  assert.deepEqual(before.slice(0,2),widgets());assert.equal(before[2].grid.row,7);
  assert.deepEqual(arrangeWidgets([],1),[]);assert.throws(()=>arrangeWidgets(before,4));
});
