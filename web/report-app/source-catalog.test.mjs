import test from 'node:test';
import assert from 'node:assert/strict';
import {DatasetSession,datasetIntent,checkDatasetView} from './dataset.js';
import {datasetClone,datasetView} from './dataset-fixture.mjs';
import {typedFieldCatalog} from './field-selection-fixture.mjs';
import {checkCatalogPage,checkSource} from './source-catalog.js';
const source={id:'registered',name:'Collected observations',context_id:'registered:v1',revision:1,dialect:'postgres',status:'registered'};
const table={source:source.id,context:source.context_id,source_revision:1,dialect:'postgres',schema_digest:'c'.repeat(64),relation:{id:'samples',name:'Samples',columns:[]}};
const pin={source:table.source,context:table.context,dataset:table.relation.id,source_revision:1,schema_digest:table.schema_digest};
function view(){const v=datasetClone(datasetView);return {...v,topic:{topic:'',version:'',digest:''},source_dataset:pin,dataset:pin.dataset,source:pin.source,context:pin.context,source_revision:1,dimensions:[],measures:[],fields:structuredClone(typedFieldCatalog)};}
test('tables without topics use public catalogs and the exact native schema pin',async()=>{
 const calls=[],session=new DatasetSession(async(name,args)=>{calls.push({name,args});if(name==='list_source_page')return {items:[source]};if(name==='list_datasets')return [table];if(name==='reporting_authoring_dataset_v1')return view();throw new Error(name);},{tables:true,topics:false});
 await session.loadCatalog();await session.selectSource(session.sources[0]);await session.selectTable('samples');
 session.edit(d=>{d.kind='table';d.fields={mode:'rows',dimensions:[{kind:'column',field:'specimen'},{kind:'column',field:'year'}],measures:[]};});
 const intent=datasetIntent(session.view,session.draft);assert.deepEqual(intent.source_dataset,pin);assert.equal(intent.topic,undefined);assert.equal(intent.fields.measures.length,0);assert.deepEqual(calls.at(-1).args,{source_dataset:pin,dataset:'samples'});
 assert.deepEqual(calls.map(c=>c.name),['list_source_page','list_datasets','reporting_authoring_dataset_v1']);assert.equal(session.intentValid(),true);
 await assert.rejects(session.chooseCatalog('topics'),/invalid_request/);
});
test('source pages are bounded in memory but have no artificial total catalog ceiling',async()=>{
 const s=new DatasetSession(async(name,args)=>({items:[{...source,id:'source-'+String(Number(args.after.slice(7)||0)+1).padStart(3,'0')}],next:'source-'+String(Number(args.after.slice(7)||0)+1).padStart(3,'0')}),{tables:true});
 for(let i=0;i<205;i++)await s.loadSources(s.sourceNext);
 assert.equal(s.sources.length,1);assert.equal(s.sources[0].id,'source-205');assert.equal(s.sourceNext,'source-205');
 await s.loadSources(s.sourceHistory.at(-2));assert.equal(s.sources[0].id,'source-204');assert.equal(s.sourceHistory.length,204);
});
test('sparse dataset pages retain next and support going back without issuing queries',async()=>{
 const s=new DatasetSession(async(name,args)=> name==='list_source_page'?{items:[source]}:args.after?{items:[table]}:{items:[],next:'earlier'}, {tables:true});
 await s.loadSources();await s.selectSource(s.sources[0]);assert.equal(s.tableNext,'earlier');assert.equal(s.tables.length,0);
 await s.loadTables(s.tableNext);assert.equal(s.tables.length,1);assert.equal(s.tableNext,'');
 await s.loadTables(s.tableHistory.at(-2));assert.equal(s.tables.length,0);assert.equal(s.tableNext,'earlier');
});
test('changed origin, unsafe cursors and malformed metadata cannot replace the prior selection',async()=>{
 for(const patch of [{source:'other'},{context:'other'},{source_revision:2},{schema_digest:'bad'}]){
  const s=new DatasetSession(async(name)=>name==='list_source_page'?{items:[source]}:{items:[{...table,...patch}]},{tables:true});await s.loadSources();await assert.rejects(s.selectSource(s.sources[0]),/stale_validation/);assert.equal(s.view,null);assert.deepEqual(s.tables,[]);
 }
 for(const changed of [{...pin,source_revision:2},{...pin,schema_digest:'d'.repeat(64)}])assert.throws(()=>checkDatasetView({...view(),source_dataset:changed},pin,pin.dataset),/stale_validation/);
 assert.throws(()=>checkDatasetView({...view(),topic:datasetView.topic},pin,pin.dataset),/stale_validation/);
 for(const page of [{items:[],next:'a'},{items:[],next:'../bad'},{items:[source,source]},{items:[],extra:true}])assert.throws(()=>checkCatalogPage(page,'a',32,item=>item.id,checkSource),/stale_validation/);
});
test('late source pages after closing are discarded and unresolved operations prevent switching data',async()=>{
 let resolve;const s=new DatasetSession(()=>new Promise(r=>resolve=r),{tables:true});const pending=s.loadSources();s.close();resolve({items:[source]});assert.equal(await pending,null);assert.deepEqual(s.sources,[]);
 const locked=new DatasetSession(async()=>({items:[]}),{tables:true});locked.custody={operation:'accepted'};await assert.rejects(locked.chooseCatalog('topics'),/busy/);await assert.rejects(locked.loadSources(),/busy/);
});

test('authority failure and close erase physical catalog names and source selections',async()=>{
 for(const action of ['suspend','close']){
  const s=new DatasetSession(async name=>name==='list_source_page'?{items:[source]}:[table],{tables:true});await s.loadSources();await s.selectSource(s.sources[0]);s[action]();
  assert.deepEqual(s.sources,[]);assert.deepEqual(s.tables,[]);assert.equal(s.source,null);assert.equal(s.sourceNext,'');assert.equal(s.tableNext,'');
 }
});
