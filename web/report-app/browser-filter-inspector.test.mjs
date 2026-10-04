// Execute the main hosted journey's exact accepted-default checks and navigation
// helper without importing its server/browser startup. This is not browser proof.
import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';

const source=await readFile(new URL('./browser.test.mjs',import.meta.url),'utf8');
const start=source.indexOf('async function inspectAcceptedDefault('),end=source.indexOf('// End accepted-default inspection contract.',start);
assert(start>=0&&end>start,'Exact accepted-default navigation helper must exist');
const helper=source.slice(start,end);
const cardDefinition=source.split('\n').find(line=>line.startsWith('const filterCard='));
const journeys=source.split('\n').filter(line=>line.trimStart().startsWith("await inspectAcceptedDefault('Rows shown'"));
assert.equal(journeys.length,2,'Both initial and retained-preview staged-default checks use the actual navigation flow');

async function journey(index,{onBack=()=>{},onResume=()=>{},duplicateSummary=false}={}){
 const state={phase:'editing',staged:index===0?'99':'31',accepted:index===0?'10':'30',retained:'exact retained amount'},calls=[],checks=[],clicks=[];
 const definition={filters:index===0?[]:[{parameter:{default:{literal:'30'}}}]};
 const label={value:'Rows shown'},input={get value(){return state.staged;},getAttribute:name=>name==='aria-label'?'Rows shown · saved default':null};
 const card={getClientRects:()=>state.phase==='paused'&&!state.hidden?[{}]:[],querySelector(selector){if(selector==='input[aria-label="Filter label"]')return label;if(selector==='p')return {textContent:'Saved: '+state.accepted};throw new Error('Unexpected summary selector '+selector);}};
 const root={
  querySelector(selector){if(selector==='.filter-inspector')return state.phase==='editing'?{}:null;if(selector==='.component-library')return state.phase==='paused'?{hidden:false}:null;throw new Error('Unexpected root selector '+selector);},
  querySelectorAll(selector){if(selector==='input')return state.phase==='editing'?[input]:[];if(selector==='.filter-editor > section')return state.phase==='paused'||duplicateSummary?[card]:[];throw new Error('Unexpected root collection '+selector);}
 };
 let context;
 context=vm.createContext({root,calls,definition,body:'root',defaultFilterCalls:0,beforeRedraw:0,exactAmount:'exact retained amount',output:()=> 'retainedOutput',retainedOutput:{querySelector:()=>({textContent:state.retained})},
  evaluate:async expression=>vm.runInContext(expression,context),
  check:async(expression,message)=>{assert.equal(vm.runInContext(expression,context),true,message);checks.push(message);},
  click:async title=>{clicks.push(title);if(title==='Back to Components'){assert.equal(state.phase,'editing');state.phase='paused';onBack({state,calls,definition});}else if(title==='Resume filter edits'){assert.equal(state.phase,'paused');state.phase='editing';onResume({state,calls,definition});}else assert.fail('Unexpected action '+title);}
 });
 vm.runInContext(cardDefinition+'\n'+helper,context);
 await vm.runInContext('(async()=>{'+journeys[index]+'})()',context);
 assert.deepEqual(clicks,['Back to Components','Resume filter edits']);assert.equal(checks.length,4,'Original accepted-value assertion runs between the three transition assertions');
 assert.equal(state.phase,'editing');assert.equal(state.staged,index===0?'99':'31');assert.equal(calls.length,0);
 return checks;
}

test('initial main journey inspects accepted 10 separately from staged 99 and persisted definition',async()=>{
 const checks=await journey(0);assert(checks.includes('typing a saved default leaves both the accepted draft default and persisted definition unchanged'));
});

test('retained main journey inspects accepted 30 separately from staged 31 without rereading values',async()=>{
 const checks=await journey(1);assert(checks.includes('uncommitted default edits preserve the accepted retained values without a tool call'));
});

test('main journey still rejects premature acceptance, persistence and changed retained evidence',async()=>{
 await assert.rejects(journey(0,{onBack:({state})=>{state.accepted='99';}}),/accepted draft default and persisted definition unchanged/);
 await assert.rejects(journey(0,{onBack:({definition})=>{definition.filters.push({});}}),/accepted draft default and persisted definition unchanged/);
 await assert.rejects(journey(1,{onBack:({state})=>{state.retained='changed';}}),/accepted retained values without a tool call/);
});

test('main journey rejects duplicate hidden summaries, lost stages and navigation tool calls',async()=>{
 await assert.rejects(journey(0,{duplicateSummary:true}),/stage is visible in its focused inspector/);
 await assert.rejects(journey(0,{onBack:({state})=>{state.hidden=true;}}),/Back exposes the accepted filter summary/);
 await assert.rejects(journey(0,{onResume:({state})=>{state.staged=state.accepted;}}),/Resume restores the exact staged default/);
 await assert.rejects(journey(0,{onBack:({calls})=>{calls.push({name:'unexpected'});}}),/Back exposes the accepted filter summary/);
 await assert.rejects(journey(1,{onResume:({calls})=>{calls.push({name:'unexpected'});}}),/Resume restores the exact staged default/);
});
