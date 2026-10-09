import test from 'node:test';
import assert from 'node:assert/strict';
import {checkFieldCatalog,typedFieldIntent,fieldSelectionIssue} from './field-selection.js';
import {DatasetSession,datasetIntent,checkDatasetView} from './dataset.js';
import {datasetClone,datasetView,datasetTopic,datasetPublication,datasetOperation} from './dataset-fixture.mjs';

function fixture(){
 const view=datasetClone(datasetView);
 view.fields={compiler:'typed-dataset-postgres-v3',supported:true,max_columns:32,dimensions:[],columns:[
  {id:'category',name:'Specimen',source_name:'specimen_id',native_type:'text',type:'text',category:'text',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:[]},
  {id:'identifier',name:'Instrument',source_name:'instrument_id',native_type:'text',type:'text',category:'text',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:[]},
  {id:'time',name:'Observed',source_name:'observed_on',native_type:'date',type:'date',category:'temporal',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:['day','week','month','quarter','year']},
  {id:'value',name:'Reading',source_name:'reading',native_type:'numeric',type:'number',category:'numeric',nullable:true,supported:true,aggregations:['count','distinct_count','sum','average','minimum','maximum'],grains:[]},
  {id:'year',name:'Year',source_name:'year_id',native_type:'int4',type:'number',category:'numeric',nullable:false,supported:true,aggregations:['count','distinct_count','sum','average','minimum','maximum'],grains:[]},
  {id:'instant',name:'Recorded',source_name:'recorded_at',native_type:'timestamptz',type:'instant',category:'temporal',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:['hour','day','week','month','quarter','year']},
 ]};
 const draft={kind:'table',title:'Observations',pageSize:20,fields:{mode:'aggregate',dimensions:[{kind:'column',field:'category'},{kind:'column',field:'identifier'},{kind:'column',field:'time',grain:'month',calendar:'gregorian'}],measures:[{kind:'column',field:'value',aggregation:'average'},{kind:'column',field:'value',aggregation:'maximum'},{kind:'count'}]}};
 return {view,draft};
}

test('schema-driven selection supports three groups and three measures without a business name contract',()=>{
 const {view,draft}=fixture();checkDatasetView(view,view.topic,view.dataset);
 const intent=datasetIntent(view,draft);assert.equal(intent.fields.dimensions.length,3);assert.equal(intent.fields.measures.length,3);assert.deepEqual(intent.mapping.bindings.columns,['group_1','group_2','group_3','value_1','value_2','value_3']);assert(!('sql' in intent));
 for(const column of view.fields.columns){column.name=`Renamed ${column.id}`;column.source_name=`new_${column.id}`;}
 assert.deepEqual(datasetIntent(view,draft),intent);
});

test('multiple numeric measures bind actual rich chart slots and keep the category explicit',()=>{
 const {view,draft}=fixture();draft.kind='bar';draft.fields.dimensions.splice(1);
 assert.deepEqual(typedFieldIntent(view,draft).mapping.bindings,{category:'group_1',values:['value_1','value_2','value_3']});
 draft.kind='line';assert.match(fieldSelectionIssue(view,draft),/date or timestamp/);
 draft.fields.dimensions=[{kind:'column',field:'time'}];assert.deepEqual(typedFieldIntent(view,draft).mapping.order,[{column:'group_1',direction:'asc'}]);
 draft.kind='pie';assert.throws(()=>typedFieldIntent(view,draft),/invalid_request/);
});

test('raw tables preserve fields without adding a measure or silently aggregating rows',()=>{
 const {view,draft}=fixture();draft.fields.mode='rows';draft.fields.measures=[];delete draft.fields.dimensions[2].grain;delete draft.fields.dimensions[2].calendar;
 const intent=typedFieldIntent(view,draft);assert.equal(intent.fields.mode,'rows');assert.equal(intent.fields.measures.length,0);assert.equal(intent.mapping.table.columns.length,3);
 draft.kind='bar';assert.throws(()=>typedFieldIntent(view,draft),/invalid_request/);
});

test('type, date, reviewed aggregation and capacity errors are caught without changing the selection',()=>{
 for(const change of [
  d=>{d.fields.dimensions[2].field='year';},
  d=>{d.fields.measures[0].field='category';},
  d=>{d.fields.dimensions[2].timezone='UTC';},
  d=>{d.fields.dimensions[2].field='instant';},
  d=>{d.fields.measures[0]={kind:'measure',field:'revenue',aggregation:'average'};},
  d=>{d.fields.measures[0].aggregation='arbitrary()';},
  d=>{d.fields.dimensions.push({...d.fields.dimensions[0]});},
 ]){const {view,draft}=fixture();change(draft);const before=datasetClone(draft);assert.throws(()=>typedFieldIntent(view,draft),/invalid_request/);assert.deepEqual(draft,before);}
 const {view,draft}=fixture();view.fields.max_columns=5;assert.throws(()=>typedFieldIntent(view,draft),/invalid_request/);assert.match(fieldSelectionIssue(view,draft),/5 result columns/);
});

test('instant grouping has an explicit zone and reviewed calendar cannot be overridden',()=>{
 const {view,draft}=fixture();draft.fields.dimensions=[{kind:'column',field:'instant',grain:'month',calendar:'gregorian',timezone:'America/Argentina/Buenos_Aires'}];
 assert.equal(typedFieldIntent(view,draft).fields.dimensions[0].timezone,'America/Argentina/Buenos_Aires');
 view.fields.dimensions=[{id:'calendar',name:'Reviewed calendar',column:'instant',supported:true,temporal:{calendar:'gregorian',grains:['month'],timezone:'UTC'}}];
 draft.fields.dimensions=[{kind:'dimension',field:'calendar',grain:'month',calendar:'gregorian',timezone:'UTC'}];assert.doesNotThrow(()=>typedFieldIntent(view,draft));
 draft.fields.dimensions[0].timezone='Asia/Tokyo';assert.throws(()=>typedFieldIntent(view,draft),/invalid_request/);
});

test('corrupt capability metadata fails closed and never invents physical types',()=>{
 for(const change of [v=>v.fields.columns.push({...v.fields.columns[0]}),v=>v.fields.max_columns=257,v=>v.fields.columns[4].grains=['year'],v=>v.fields.columns[0].aggregations.push('execute'),v=>v.fields.compiler='unknown']){const {view}=fixture();change(view);assert.throws(()=>checkFieldCatalog(view),/stale_validation/);}
});

test('new sessions use typed selection only when the server advertises it; changing topics clears it',async()=>{
 const {view}=fixture(),calls=[];
 const s=new DatasetSession(async(name,args)=>{calls.push(name);if(name==='describe_topic')return datasetClone(datasetPublication);if(name==='reporting_authoring_dataset_v1')return view;throw Error(name);},{operation:()=>datasetOperation});
 await s.selectTopic(datasetTopic);await s.selectDataset('orders');assert.deepEqual(s.draft.fields,{mode:'aggregate',dimensions:[],measures:[]});
 s.newBlock='private-chart';s.edit(d=>{d.fields.measures.push({kind:'count'});d.kind='kpi';});assert(s.valid());assert(!calls.some(c=>c.includes('prepare')));
 await s.selectTopic(datasetTopic);assert.equal(s.draft.fields,undefined);
});
