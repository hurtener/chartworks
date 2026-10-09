import test from 'node:test';
import assert from 'node:assert/strict';
import {PublicationSession} from './publication.js';
import {multiChartFixture,clone} from './publication-fixture.mjs';
import {appError,authoringTool} from './model.js';

async function setup(wrap=fn=>fn){const fixture=multiChartFixture(),session=new PublicationSession(wrap(fixture.invoke));await session.open('report-a',1);return {fixture,session};}
const mutations=f=>f.calls.filter(c=>c.name===authoringTool('block_publish'));

test('one exact confirmation publishes twelve revisions, preserving all outputs and private report pins',async()=>{
  const {fixture:f,session:s}=await setup();assert.equal(s.chartBatchTargets().length,12);await assert.rejects(s.publishChartBatch());s.confirmChartBatch(true);const inspected=clone(s.view),progress=[];
  const last=await s.publishChartBatch(()=>progress.push(s.message));assert.equal(last.args.block,'chart-12');assert.equal(mutations(f).length,12);assert.equal(progress.length,12);
  for(const [index,op] of s.records('report-a').entries()){const b=inspected.blocks[index].block;assert.equal(op.status,'confirmed');assert.deepEqual(op.args,{block:b.state.id,expected_version:b.state.version,revision:b.revision,digest:b.digest,evidence:b.validation.id});assert.deepEqual(op.disclosure.map(o=>o.id),b.outputs.map(o=>o.id));}
  assert(f.report(1).private);assert(f.report(1).definition.report_pages.flatMap(p=>p.widgets).every(w=>w.block.policy==='private_preview'));assert(f.calls.every(c=>[authoringTool('lifecycle'),authoringTool('block_publish')].includes(c.name)));assert.equal(s.chartBatchConfirmed(),false);
});

test('changing any reviewed output, version, evidence, report or eligible set invalidates batch consent',async()=>{
  for(const change of [v=>v.blocks[11].block.outputs[1].id='changed',v=>v.blocks[11].block.state.version++,v=>v.blocks[11].block.validation.id='new-evidence',v=>v.report.state.version++,v=>v.blocks[11].can_publish=false]){const {fixture:f,session:s}=await setup();s.confirmChartBatch(true);change(s.view);assert.equal(s.chartBatchConfirmed(),false);await assert.rejects(s.publishChartBatch());assert.equal(mutations(f).length,0);}
});

test('a second uncertain result stops the batch, preserving first confirmation and only sent custody',async()=>{
  let writes=0;const {fixture:f,session:s}=await setup(fn=>async(name,args)=>{if(name===authoringTool('block_publish')&&++writes===2)throw appError('unavailable',true);return fn(name,args);});s.confirmChartBatch(true);
  await assert.rejects(s.publishChartBatch());const records=s.records('report-a');assert.equal(writes,2);assert.equal(records.length,2);assert.equal(records[0].status,'confirmed');assert.equal(records[1].status,'unknown');assert.equal(s.view,null);assert.match(s.message,/1 of 12 charts confirmed/);assert.match(s.message,/remaining charts were not sent/);assert.equal(s.blocked('report-a'),true);assert.equal(f.blocks.get('chart-3:2').block.private,true);
  await s.inspectOperation(records[1]);assert.equal(records[1].status,'unknown');assert.equal(s.retryConfirmed(records[1]),false);assert.equal(writes,2);
});

test('a definite refusal stops independent publications without reverting confirmed charts',async()=>{
  let writes=0;const {fixture:f,session:s}=await setup(fn=>async(name,args)=>{if(name===authoringTool('block_publish')&&++writes===2)throw appError('forbidden');return fn(name,args);});s.confirmChartBatch(true);await assert.rejects(s.publishChartBatch());assert.equal(writes,2);assert.deepEqual(s.records('report-a').map(o=>o.status),['confirmed','failed']);assert.equal(f.blocks.get('chart-1:2').block.private,false);assert.equal(f.blocks.get('chart-3:2').block.private,true);assert.equal(s.view,null);
});

test('navigation stops unsent publications and late completion cannot change the new inspection',async()=>{
  let release;const {fixture:f,session:s}=await setup(fn=>async(name,args)=>{if(name===authoringTool('block_publish'))await new Promise(resolve=>release=resolve);return fn(name,args);});s.confirmChartBatch(true);const pending=s.publishChartBatch();await assert.rejects(s.publishChartBatch());await assert.rejects(s.open('report-a',1));s.invalidate();release();assert.equal(await pending,null);assert.equal(mutations(f).length,1);assert.equal(s.view,null);assert.equal(s.message,'');assert.equal(s.records('report-a')[0].status,'confirmed');assert.equal(s.batching,false);
});

test('unready revisions are excluded and adding a ready revision needs fresh consent',async()=>{
  const {fixture:f,session:s}=await setup();s.view.blocks[11].validation_fresh=false;assert.equal(s.chartBatchTargets().length,11);s.confirmChartBatch(true);await s.publishChartBatch();assert.equal(mutations(f).length,11);assert.equal(f.blocks.get('chart-12:2').block.private,true);
});
