import test from 'node:test';
import {readFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
import {DatasetSession,datasetIntent,checkDatasetView,checkPreparation,DATASET_CHART_KINDS} from './dataset.js';
import {datasetOperation,datasetClone,datasetTopic,datasetView,datasetPrepared,datasetCreated,datasetFixture} from './dataset-fixture.mjs';
import {appError} from './model.js';
async function opened(invoke){const s=new DatasetSession(invoke,{operation:()=>datasetOperation});await s.loadTopics();await s.selectTopic(s.topics[0]);await s.selectDataset('orders');s.newBlock='private-chart';s.edit(d=>{d.title='Revenue by region';d.dimensions=['region'];d.measure='revenue';});return s;}
test('catalog, dataset and all field edits are source-free with server binding IDs',async()=>{const f=datasetFixture(),s=await opened(f.invoke);s.edit(d=>{d.kind='table';d.pageSize=7;});const intent=datasetIntent(s.view,s.draft);assert.deepEqual(f.calls.map(c=>c.name),['list_topics','describe_topic','reporting_authoring_dataset_v1']);assert.deepEqual(intent.mapping.bindings,{columns:['d_region','m_revenue']});assert.equal(intent.mapping.table.page_size,7);assert.equal(intent.measure,'revenue');assert(!('aggregation' in intent));assert(!('sql' in intent));assert(!('columns' in intent.mapping));});
test('eight basic kinds keep reviewed shape and explicit temporal constraints',()=>{for(const kind of DATASET_CHART_KINDS){const dimensions=kind==='kpi'?[]:['line','area'].includes(kind)?['day']:['region'];assert.equal(datasetIntent(datasetView,{kind,dimensions,measure:'revenue',title:'Sales',pageSize:20}).mapping.kind,kind);}for(const draft of [{kind:'kpi',dimensions:['region']},{kind:'line',dimensions:['region']},{kind:'pie',dimensions:['region','day']},{kind:'bar',dimensions:['filtered']},{kind:'table',dimensions:['region','region']},{kind:'scatter',dimensions:[]}])assert.throws(()=>datasetIntent(datasetView,{measure:'revenue',title:'Sales',pageSize:20,...draft}),/invalid_request/);assert.throws(()=>datasetIntent(datasetView,{kind:'bar',dimensions:['region'],measure:'complete',title:'Sales'}),/invalid_request/);});
test('explicit Prepare and Create use immutable custody and return an unvalidated private revision',async()=>{const f=datasetFixture(),s=await opened(f.invoke);await s.prepare();assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);assert.throws(()=>s.edit(d=>{d.kind='kpi';}),/busy/);assert.equal(s.canCreate(),true);const v=await s.create();assert.equal(v.block.private,true);assert.equal(v.block.validation,undefined);assert.deepEqual(f.calls.at(-1).args,{new_block:'private-chart',preparation:'preparation-a',digest:'b'.repeat(64)});assert(!f.calls.some(c=>c.name.includes('validate')||c.name.includes('execute')));});
test('wrong publication pin, duplicate field bindings and unsupported shape cannot become valid choices',async()=>{const f=datasetFixture(),s=new DatasetSession(async(name,args)=>{const value=await f.invoke(name,args);if(name==='describe_topic')value.digest='e'.repeat(64);return value;});await s.loadTopics();await assert.rejects(s.selectTopic(s.topics[0]),/stale_validation/);assert.equal(s.publication,null);const view=datasetClone(datasetView);view.measures[0].binding=view.dimensions[0].binding;assert.throws(()=>checkDatasetView(view,datasetView.topic,'orders'),/stale_validation/);view.measures[0].binding='m_revenue';view.supported=false;view.reason='reviewed_rules_unsupported';assert.equal(checkDatasetView(view,datasetView.topic,'orders').supported,false);assert.throws(()=>datasetIntent(view,{kind:'kpi',dimensions:[],measure:'revenue',title:'Sales'}),/invalid_request/);});
test('unknown Prepare stays fenced and status inspection never starts another source read',async()=>{const f=datasetFixture();let lose=true;const s=await opened(async(name,args)=>{const v=await f.invoke(name,args);if(name.includes('prepare_chart')&&lose){lose=false;throw appError('unavailable',true);}return v;});await assert.rejects(s.prepare(),/unavailable/);assert.equal(s.unknown,'prepare');await assert.rejects(s.prepare(),/busy/);await s.inspect();assert.equal(s.canCreate(),true);assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);assert.deepEqual(f.calls.at(-1).args,{new_block:'private-chart',operation:datasetOperation});});
test('all post-dispatch errors preserve preparation custody for explicit inspection',async()=>{for(const code of ['forbidden','conflict','stale_validation','unavailable']){const f=datasetFixture(),s=await opened(async(name,args)=>{if(name.includes('prepare_chart'))throw appError(code);return f.invoke(name,args);});await assert.rejects(s.prepare());assert.equal(s.custody.operation,datasetOperation);assert.equal(s.unknown,'prepare');await assert.rejects(s.prepare(),/busy/);}});
test('unknown Create requires exact consumed status before explicit safe recovery',async()=>{const f=datasetFixture();let lose=true;const s=await opened(async(name,args)=>{const v=await f.invoke(name,args);if(name.includes('create_prepared')&&lose){lose=false;throw appError('unavailable',true);}return v;});await s.prepare();await assert.rejects(s.create(),/unavailable/);await assert.rejects(s.create(),/busy/);assert.equal(s.canRecover(),false);await s.inspect();assert.equal(s.unknown,'create');assert.equal(s.canRecover(),true);const v=await s.create(true);assert.equal(v.block.state.id,'private-chart');assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);assert.deepEqual(f.calls.filter(c=>c.name.includes('create_prepared')).map(c=>c.args),[f.calls.at(-1).args,f.calls.at(-1).args]);});
test('preparation identity/digest tampering and expired preparation fail closed',async()=>{const f=datasetFixture(),s=await opened(f.invoke);await s.prepare();for(const field of ['preparation','new_block','operation','digest']){const value=datasetClone(s.preparation);value[field]='foreign';assert.throws(()=>checkPreparation(value,s.custody,s.preparation),/stale_validation/);}s.preparation.expires_at='2020-01-01T00:00:00Z';assert.equal(s.canCreate(),false);await assert.rejects(s.create(),/busy/);});
test('created target, schema and mapping mismatches keep unknown mutation fence',async()=>{for(const mutate of [v=>{v.block.state.id='foreign';},v=>{v.block.expected_schema=[];},v=>{v.block.outputs[0].mapping.bindings.value='other';}]){const f=datasetFixture(),s=await opened(async(name,args)=>{const v=await f.invoke(name,args);if(name.includes('create_prepared'))mutate(v);return v;});await s.prepare();await assert.rejects(s.create(),e=>e.unknown===true);assert.equal(s.unknown,'create');await assert.rejects(s.create(),/busy/);}});
test('explicit original-attempt controls remain distinct from retained status reads',async()=>{const f=datasetFixture(),s=await opened(f.invoke);await s.prepare();await s.inspect('cancel');assert.equal(f.calls.at(-1).name,'reporting_authoring_preparation_control_v1');assert.deepEqual(f.calls.at(-1).args,{new_block:'private-chart',preparation:'preparation-a',action:'cancel'});assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);});
test('unsupported compiler disposition is explicit, uncached and permits local correction',async()=>{const f=datasetFixture(),s=await opened(async(name,args)=>name.includes('prepare_chart')?{status:'unsupported',code:'chart_shape_unsupported',schema:[],validation:'not_performed'}:f.invoke(name,args));await s.prepare();assert.equal(s.custody,null);assert.equal(s.preparation.code,'chart_shape_unsupported');assert.equal(s.canCreate(),false);s.edit(d=>{d.kind='table';});});
test('close and superseded metadata responses cannot repopulate staged state',async()=>{let resolve;const s=new DatasetSession(()=>new Promise(r=>{resolve=r;}));const read=s.loadTopics();s.close();resolve([datasetTopic]);await read;assert.deepEqual(s.topics,[]);const f=datasetFixture();let preparedResolve;const current=await opened(async(name,args)=>name.includes('prepare_chart')?new Promise(r=>{preparedResolve=()=>r(datasetPrepared(args));}):f.invoke(name,args));const pending=current.prepare();current.close();preparedResolve();await pending;assert.equal(current.preparation,null);assert.equal(current.view,null);});

test('native empty-slice and object key ordering normalization preserves exact logical mapping',async()=>{const f=datasetFixture(),s=await opened(async(name,args)=>{const value=await f.invoke(name,args);if(name.includes('prepare_chart')){value.mapping.order=null;value.mapping.bindings={columns:null,series:'',value:'m_revenue',category:'d_region'};}return value;});await s.prepare();assert.equal(s.canCreate(),true);await s.create();assert.equal(s.created.block.state.id,'private-chart');});

test('line and area intent pins native ascending temporal order before preparation',()=>{for(const kind of ['line','area']){const intent=datasetIntent(datasetView,{kind,dimensions:['day','region'],measure:'revenue',title:'Revenue trend'});assert.deepEqual(intent.mapping.order,[{column:'d_day',direction:'asc'}]);assert.deepEqual(intent.mapping.bindings,{category:'d_day',value:'m_revenue',series:'d_region'});}});

test('manual Prepare includes required native descriptive metadata without introducing model work',async()=>{const f=datasetFixture(),s=await opened(f.invoke);await s.prepare();const request=f.calls.find(c=>c.name==='reporting_authoring_prepare_chart_v1').args;assert.deepEqual(request.metadata,[{locale:'en-US',title:'Revenue by region',question:'Revenue by region',aliases:[],description:''}]);assert.deepEqual(f.calls.map(c=>c.name),['list_topics','describe_topic','reporting_authoring_dataset_v1','reporting_authoring_prepare_chart_v1']);});

test('frontend Prepare payload equals the shared native API schema conformance request',async()=>{const fixture=JSON.parse(await readFile(new URL('./testdata/dataset-prepare-contract.json',import.meta.url),'utf8'));let payload;const session=new DatasetSession(async(name,args)=>{assert.equal(name,'reporting_authoring_prepare_chart_v1');payload=args;throw appError('forbidden');},{locale:fixture.request.metadata[0].locale,operation:()=>fixture.request.operation});checkDatasetView(fixture.dataset,fixture.request.intent.topic,fixture.request.intent.dataset);session.view=datasetClone(fixture.dataset);session.newBlock=fixture.request.new_block;session.draft={kind:'kpi',dimensions:[],measure:fixture.request.intent.measure,title:fixture.request.metadata[0].title,pageSize:20};await assert.rejects(session.prepare(),/forbidden/);assert.deepEqual(payload,fixture.request);});

test('unchanged real PostgreSQL public DTOs satisfy the actual frontend preparation and private validation path', async t => {
 const f=JSON.parse(await readFile(new URL('./testdata/dataset-native-public.json',import.meta.url),'utf8'));
 t.mock.method(Date,'now',()=>Date.parse(f.preview.created_at));
 const {validatePrivateMapping,hasFreshMappingEvidence}=await import('./mapping.js');
 const {RetainedReport}=await import('./retained.js');
 const calls=[],invoke=async(name,args)=>{
  calls.push({name,args});
  if(name==='list_topics')return datasetClone(f.list_topics);
  if(name==='describe_topic')return datasetClone(f.describe_topic);
  if(name==='reporting_authoring_dataset_v1')return datasetClone(f.dataset);
  if(name==='reporting_authoring_prepare_chart_v1'){
   assert.deepEqual(args,f.request);return datasetClone(f.preparation);
  }
  if(name==='reporting_authoring_create_prepared_v1'){
   assert.deepEqual(args,{new_block:f.request.new_block,preparation:f.preparation.preparation,digest:f.preparation.digest});
   return datasetClone(f.created);
  }
  if(name==='reporting_authoring_block_validate_v1')return datasetClone(f.validation);
  throw Error(name);
 };
 const s=new DatasetSession(invoke,{locale:f.request.metadata[0].locale,operation:()=>f.request.operation});
 await s.loadTopics();await s.selectTopic(s.topics.find(v=>v.topic===f.request.intent.topic.topic));await s.selectDataset(f.dataset.dataset);
 s.newBlock=f.request.new_block;
 s.edit(d=>Object.assign(d,{title:f.request.metadata[0].title,kind:'kpi',dimensions:[],measure:f.request.intent.measure}));
 await s.prepare();await s.create();
 const block={block:f.created.block.state.id,revision:1,digest:f.created.block.digest,outputs:['chart'],policy:'private_preview',narrative:false};
 const validated=await validatePrivateMapping(invoke,s.created,{block},{filters:[],defaults:[]},{at:f.preview.created_at,timezone:'UTC'});
 assert(hasFreshMappingEvidence(validated));assert(!('attempt' in validated.block.validation));
 const retained=new RetainedReport(async()=>{throw Error('Unexpected retained fanout for root-selected single output');});
 t.after(()=>{retained.close();s.close();});await retained.load(datasetClone(f.view_root));
 // Resolve only the captured exact selection, then prove its saved topology and
 // private block pins agree with both the created and validated native DTOs.
 const selection=f.view_root.selection;
 const page=f.final_read.definition.report_pages.find(p=>p.id===selection.page);
 assert(page,'Captured selected page must exist in the saved report');
 const widget=page.widgets.find(w=>w.id===selection.widget);
 assert(widget,'Captured selected widget must exist on that exact saved page');
 assert.deepEqual(selection,f.view_output.selection);
 assert.equal(f.view_root.summary.target.revision,f.final_read.revision);
 assert.equal(f.view_root.summary.target.id,f.final_read.state.id);
 assert.deepEqual(f.final_read.definition,f.save_request.definition);
 assert.equal(widget.block.block,s.created.block.state.id);
 assert.equal(widget.block.digest,s.created.block.digest);
 assert.equal(widget.block.revision,s.created.block.revision);
 assert.equal(widget.block.policy,'private_preview');assert.equal(widget.block.narrative,false);
 assert.equal(widget.block.block,validated.block.state.id);
 assert.equal(widget.block.revision,validated.block.revision);
 assert.equal(widget.block.digest,validated.block.digest);
 assert.deepEqual(widget.block.outputs,[selection.output]);
 assert.deepEqual(widget.block.outputs,s.created.block.outputs.map(o=>o.id));
 assert.equal(retained.get(selection.page,selection.widget,selection.output).output.chart.points[0].value.exact,'9007199254740998.625');
 assert.deepEqual(calls.map(c=>c.name),['list_topics','describe_topic','reporting_authoring_dataset_v1','reporting_authoring_prepare_chart_v1','reporting_authoring_create_prepared_v1','reporting_authoring_block_validate_v1']);
});
