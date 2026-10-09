import test from 'node:test';
import assert from 'node:assert/strict';
import {columnFilterKey,columnFilterValue,checkColumnFilterCapability} from './column-filters.js';
import {filterInputState,filterInputValue,renderFilterInput} from './filter-controls.js';
import {DatasetSession,datasetIntent,checkDatasetView} from './dataset.js';
import {renderDatasetEditor} from './dataset-editor.js';
import {ReportFilterControls} from './report-filters.js';
import {physicalFilterCatalog} from './field-selection-fixture.mjs';
import {datasetView,datasetClone} from './dataset-fixture.mjs';

const pin={source:'source',context:'source:v1',dataset:'samples',source_revision:1,schema_digest:'c'.repeat(64)};
function view(){return {...datasetClone(datasetView),topic:{topic:'',version:'',digest:''},source_dataset:pin,source:pin.source,context:pin.context,source_revision:1,dataset:pin.dataset,dimensions:[],measures:[],fields:structuredClone(physicalFilterCatalog)};}
function session(){const calls=[],s=new DatasetSession(async(name,args)=>{calls.push({name,args});throw new Error('unexpected source request');},{tables:true,topics:false});s.view=checkDatasetView(view(),pin,pin.dataset);s.draft={title:'Selected fields',kind:'table',pageSize:20,dimensions:[],measure:'',fields:{mode:'rows',dimensions:[{kind:'column',field:'specimen'}],measures:[]}};return {s,calls};}
const parameter=(type,codec,policy={})=>({type,column:{type:codec,...policy}});

test('typed ranges retain all digits and compare beyond floating-point precision',()=>{
 const p=parameter('column_range','number'),good={range:{start:'9007199254740993.125',end_exclusive:'9007199254740993.126'}};
 assert.deepEqual(columnFilterValue(p,good),good);
 assert.throws(()=>columnFilterValue(p,{range:{start:good.range.end_exclusive,end_exclusive:good.range.start}}),/invalid_request/);
 assert.throws(()=>columnFilterValue(p,{range:{start:'1e4',end_exclusive:'9999.999'}}),/invalid_request/);
 assert.deepEqual(columnFilterValue(parameter('column_value','integer'),{literal:'9223372036854775807'}),{literal:'9223372036854775807'});
 for(const value of ['9223372036854775808','1.5','01','-0'])assert.throws(()=>columnFilterValue(parameter('column_value','integer'),{literal:value}),/invalid_request/);
 for(const value of ['bad\rtext','bad\u0007text','bad\u007ftext'])assert.throws(()=>columnFilterValue(parameter('column_value','text'),{literal:value}),/invalid_request/);
 assert.deepEqual(columnFilterValue(parameter('column_value','text'),{literal:'line\nwith\ttabs'}),{literal:'line\nwith\ttabs'});
});
test('date inputs retain civil days, timestamp inputs retain explicit policy and browser-normalized seconds',()=>{
 const p=parameter('column_range','date',{calendar:'gregorian'}),saved={range:{start:'1700-01-01',end_exclusive:'2026-03-09'}},state=filterInputState(p,saved);
 assert.equal(state.end,'2026-03-08');state.end='2026-03-10';assert.deepEqual(filterInputValue(state),{range:{start:'1700-01-01',end_exclusive:'2026-03-11'}});assert.equal(saved.range.end_exclusive,'2026-03-09');
 const instant=filterInputState(parameter('column_range','instant',{calendar:'gregorian',timezone:'America/New_York'}),null);instant.start='2026-03-08T00:00';instant.end='2026-03-09T00:00:00.000000';
 assert.deepEqual(filterInputValue(instant),{range:{start:'2026-03-08T00:00:00',end_exclusive:'2026-03-09T00:00:00'}});
 for(const timezone of ['', 'Local','invented/zone']){instant.column.timezone=timezone;assert.throws(()=>filterInputValue(instant),/invalid_request/);}
 const civil=filterInputState(parameter('column_range','timestamp',{calendar:'gregorian'}),null);civil.start='2026-01-01T00:00:00Z';civil.end='2026-02-01T00:00:00Z';assert.throws(()=>filterInputValue(civil),/invalid_request/);
});
test('column origin and type stay explicit and distinct from similarly named reviewed dimensions',()=>{
 const {s,calls}=session();s.addFilter(columnFilterKey('specimen'),'multi_select');s.commitFilter(columnFilterKey('specimen'),{items:['','alpha']});
 s.addFilter(columnFilterKey('reading'),'range');s.commitFilter(columnFilterKey('reading'),{range:{start:'1.25',end_exclusive:'9.75'}});
 s.addFilter(columnFilterKey('accepted'),'select');s.commitFilter(columnFilterKey('accepted'),{literal:'true'});
 s.addFilter(columnFilterKey('recorded'),'range');const stage=s.filterStages.get(columnFilterKey('recorded'));stage.column.timezone='America/New_York';stage.start='2026-03-08T00:00';stage.end='2026-03-09T00:00';s.commitFilter(columnFilterKey('recorded'),filterInputValue(stage));
 const intent=datasetIntent(s.view,s.draft);assert.deepEqual(intent.source_dataset,pin);assert.equal(intent.topic,undefined);assert.equal(intent.filters.length,4);assert(intent.filters.every(f=>f.column&&!Object.hasOwn(f,'dimension')));assert.equal(intent.filters[3].timezone,'America/New_York');assert.equal(calls.length,0);assert.equal(s.intentValid(),true);
 assert.throws(()=>s.addFilter(columnFilterKey('year'),'select'),/invalid_request/);
 const original=structuredClone(s.draft.filters[3]);s.beginFilter(columnFilterKey('recorded')).column.timezone='UTC';s.cancelFilter(columnFilterKey('recorded'));assert.deepEqual(s.draft.filters[3],original);
 s.beginFilter(columnFilterKey('recorded')).column.timezone='';assert.throws(()=>s.commitFilter(columnFilterKey('recorded'),original.default),/invalid_request/);assert.deepEqual(s.draft.filters[3],original);
});
test('types, stale capabilities and malformed unions fail before preparation',()=>{
 for(const change of [c=>c.type='date',c=>c.kinds=['date_range'],c=>c.max_set_size=17,c=>c.timezone_required=true]){const c=structuredClone(physicalFilterCatalog.columns[0]);change(c.filters);assert.throws(()=>checkColumnFilterCapability(c),/stale_validation/);}
 for(const [type,codec,value] of [['column_value','boolean',{literal:'yes'}],['column_set','text',{items:['a','a']}],['column_value','identifier',{literal:'not-uuid'}],['column_range','number',{range:{start:'1',end_exclusive:'2'},literal:'3'}]])assert.throws(()=>columnFilterValue(parameter(type,codec),value),/invalid_request/);
 const {s,calls}=session();s.addFilter(columnFilterKey('year'),'select');assert.throws(()=>s.changeFilterKind(columnFilterKey('year'),'date_range'),/invalid_request/);assert.throws(()=>s.commitFilter(columnFilterKey('year'),{literal:'2026-01-01'}),/invalid_request/);assert.equal(s.intentValid(),false);assert.equal(calls.length,0);
});

class Node{constructor(tag){this.tagName=tag.toUpperCase();this.children=[];this.listeners={};this.attributes={};this.value='';this.disabled=false;}append(...nodes){for(const node of nodes){node.parent=this;this.children.push(node);}}setAttribute(k,v){this.attributes[k]=v;}addEventListener(k,fn){(this.listeners[k]||=[]).push(fn);}fire(k){for(const fn of this.listeners[k]||[])fn();}replaceWith(node){node.parent=this.parent;this.parent.children[this.parent.children.indexOf(this)]=node;}querySelectorAll(tag){return this.children.flatMap(c=>[...(c.tagName===tag.toUpperCase()?[c]:[]),...c.querySelectorAll(tag)]);}focus(){} }
const find=(n,fn)=>[n,...n.children.flatMap(c=>find(c,()=>true))].filter(fn);
function root(){globalThis.document={createElement:tag=>new Node(tag)};return new Node('main');}
const byLabel=(r,label)=>find(r,n=>n.attributes['aria-label']===label)[0];
const button=(r,text)=>find(r,n=>n.tagName==='BUTTON'&&n.textContent===text)[0];
test('rendered physical set allows exact values and empty text without querying; Done alone commits',()=>{
 const r=root(),state=filterInputState(parameter('column_set','text'),null);let saved;renderFilterInput(r,state,{onDone:v=>saved=v});assert.equal(button(r,'Done').disabled,true);
 const input=byLabel(r,'Exact value to add');input.value='alpha';input.fire('input');button(r,'Add value').fire('click');assert.deepEqual(state.items,['alpha']);assert.equal(saved,undefined);
 button(r,'Add value').fire('click');assert.deepEqual(state.items,['alpha','']);button(r,'Done').fire('click');assert.deepEqual(saved,{items:['alpha','']});assert.equal(button(r,'Search options'),undefined);
});
test('rendered physical fields stay available without option search; saved temporal policies are immutable in report controls',()=>{
 const {s,calls}=session(),r=root();renderDatasetEditor(r,s,{canAllocate:true,change(){},read(){},prepare(){},create(){},recover(){},inspect(){},cancel(){}});
 assert.equal(byLabel(r,'Filter field').children.find(c=>c.value===columnFilterKey('specimen')).disabled,false);assert.equal(calls.length,0);
 const p=parameter('column_range','instant',{calendar:'gregorian',timezone:'UTC'}),state=filterInputState(p,{range:{start:'2026-01-01T00:00:00',end_exclusive:'2026-02-01T00:00:00'}}),output=root();renderFilterInput(output,state,{});assert.equal(byLabel(output,'Filter timezone'),undefined);
 const edit=root();renderFilterInput(edit,state,{editPolicy:true});const zone=byLabel(edit,'Filter timezone');zone.value='America/New_York';zone.fire('input');assert.equal(p.column.timezone,'UTC');assert.equal(state.column.timezone,'America/New_York');
});

test('physical report defaults, temporary preview and reader inputs stay separate through Done and Cancel',()=>{
 const filter={label:'Recorded at',parameter:{name:'recorded',...parameter('column_range','instant',{calendar:'gregorian',timezone:'America/New_York',source_dataset:pin,name:'recorded_at'}),default:{range:{start:'2026-03-08T00:00:00',end_exclusive:'2026-03-09T00:00:00'}}}};
 const definition={schema_version:3,report_pages:[{id:'main',filters:[filter],widgets:[]}]},calls=[];
 const app={closed:false,epoch:1,_uiPageGeneration:1,activePageID:'main',session:{definition},description:{filters:[{page:'main',...structuredClone(filter)}]},render(){},editPage(change,id){change(definition.report_pages.find(p=>p.id===id));},invoke(...args){calls.push(args);}};
 const controls=new ReportFilterControls(app),before=structuredClone(definition);
 controls.open(filter,'main','default');controls.editor.state.end='2026-03-10T00:00';controls.cancel(controls.editor);assert.deepEqual(definition,before);
 controls.open(filter,'main','preview');controls.editor.state.end='2026-03-10T00:00';const value=filterInputValue(controls.editor.state);controls.done(controls.editor,value);value.range.end_exclusive='mutated';
 const expected=[{page:'main',filters:[{name:'recorded',value:{range:{start:'2026-03-08T00:00:00',end_exclusive:'2026-03-10T00:00:00'}}}],overrides:[]}];
 assert.deepEqual(controls.pageInputs(definition),expected);assert.deepEqual(definition,before);
 controls.open(filter,'main','run');assert.equal(controls.editor.state.column.timezone,'America/New_York');controls.done(controls.editor,filterInputValue(controls.editor.state));assert.deepEqual(controls.publishedInputs(),expected);assert.deepEqual(app.description.filters[0].parameter,before.report_pages[0].filters[0].parameter);
 controls.open(filter,'main','default');controls.editor.state.end='2026-03-11T00:00';controls.done(controls.editor,filterInputValue(controls.editor.state));assert.equal(filter.parameter.default.range.end_exclusive,'2026-03-11T00:00:00');assert.equal(filter.parameter.column.timezone,'America/New_York');assert.deepEqual(controls.publishedInputs(),expected);assert.equal(calls.length,0);
});

test('physical option search keeps source pins and exact values, never commits a filter until Done',async()=>{
 const {s}=session(),calls=[];s.invoke=async(name,args)=>{calls.push({name,args});return {operation:args.operation,input_digest:'a'.repeat(64),status:'completed',values_available:true,new_operation_allowed:true,complete:true,options:[{value:'9007199254740993.125',label:'9007199254740993.125'}]};};
 s.addFilter(columnFilterKey('reading'),'select');const key=columnFilterKey('reading');assert.equal(calls.length,0);const stage=s.filterStages.get(key);
 await s.searchFilter(key,'allocated-chart','9007199254740993.125');assert.equal(calls.length,1);assert.deepEqual(calls[0].args.target.dataset,{source_dataset:pin,dataset:pin.dataset,column:'reading',new_block:'allocated-chart'});assert.equal(s.draft.filters[0].default,null);
 const r=root();let committed;renderFilterInput(r,stage,{lookup:s.filterLookups.get(key),onSearch(){throw Error('unexpected search');},onDone:value=>committed=value});
 const choice=find(r,n=>n.type==='radio'&&n.value==='9007199254740993.125')[0];choice.checked=true;choice.fire('change');assert.equal(committed,undefined);button(r,'Done').fire('click');assert.deepEqual(committed,{literal:'9007199254740993.125'});assert.equal(calls.length,1);
 s.commitFilter(key,committed);assert.equal(datasetIntent(s.view,s.draft).filters[0].default.literal,'9007199254740993.125');
});

test('mismatched typed option values remain unconfirmed and require original lookup inspection',async()=>{
 const {s}=session();s.addFilter(columnFilterKey('year'),'select');s.invoke=async(_name,args)=>({operation:args.operation,input_digest:'a'.repeat(64),status:'completed',values_available:true,new_operation_allowed:true,complete:true,options:[{value:'1.25',label:'1.25'}]});
 await assert.rejects(s.searchFilter(columnFilterKey('year'),'allocated-chart',''),/invalid_request/);const lookup=s.filterLookups.get(columnFilterKey('year'));assert.equal(lookup.unknown,true);assert.deepEqual(lookup.values,[]);assert.equal(s.hasUnsettledFilterLookups,true);assert.equal(s.draft.filters[0].default,null);
});
