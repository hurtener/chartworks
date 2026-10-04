import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {governedFilterBrowserFixture,installNativeReplayClock,optionRequest,previewRequest,runRequest,viewRequest} from './filters-browser-fixture.mjs';
import {FilterOptionLookup} from './filters.js';
import {filterInputState,filterInputValue} from './filter-controls.js';
import {ReportFilterControls} from './report-filters.js';

const load=()=>governedFilterBrowserFixture(process.env.CHARTWORKS_FILTER_FIXTURE_PATH||new URL('./testdata/governed-filter-native.json',import.meta.url));
const clean=value=>JSON.parse(JSON.stringify(value));

test('native DTO replay validates exact bindings and preserves immutable snapshots',async()=>{
  const f=await load(),d=f.data,before=f.snapshot();
  const lookup=new FilterOptionLookup((name,args)=>f.invoke(name,args),d.initial_option_request.target,{operation:()=>d.initial_option_request.operation});
  await lookup.search('East');assert.deepEqual(lookup.values,d.initial_option_response.options);assert.deepEqual(f.snapshot(),before);
  assert.deepEqual(f.counts(),{optionSearches:1,privateExecutions:0,publishedRuns:0,nativeSourceReadsRepresented:1});
  assert.deepEqual(await f.invoke('reporting_authoring_save_v1',d.save_request),d.saved_state);
  assert.deepEqual(f.snapshot().report,d.private_report);assert.deepEqual(f.snapshot().block,d.created.block);
  assert.deepEqual(f.snapshot().report.definition.report_pages[1],before.report.definition.report_pages[1]);
});

test('closed lookup coordinates and operation identities reject before any represented source read',async()=>{
  for(const change of [
    r=>r.target.report.policy='published',r=>r.target.report.report='other',r=>r.target.report.revision++,r=>r.target.report.digest='0'.repeat(64),
    r=>r.target.report.page='notes',r=>r.target.report.filter='day',r=>r.target.report.topic='other',r=>r.target.report.context='other',
    r=>r.target.dataset={},r=>r.operation+='x',r=>r.search='North',r=>r.cursor='unexpected',r=>r.limit=200,r=>r.locale='es-AR',r=>r.sql='synthetic forbidden input'
  ]){
    const f=await load(),request=optionRequest(f.data.initial_option_request);change(request);
    await assert.rejects(f.invoke('reporting_authoring_report_options_v1',request));assert.equal(f.counts().nativeSourceReadsRepresented,0);assert.equal(f.calls.length,0);
  }
  const f=await load(),request=optionRequest(f.data.initial_option_request);
  await f.invoke('reporting_authoring_report_options_v1',request);await assert.rejects(f.invoke('reporting_authoring_report_options_v1',request));assert.equal(f.counts().optionSearches,1);
});

test('native private preview and public run match exact temporary arguments and privacy without rewriting defaults',async()=>{
  const f=await load(),d=f.data,p=f.privateRun,r=f.publicRun;
  await f.invoke('reporting_authoring_save_v1',d.save_request);const saved=f.snapshot();
  await f.invoke('reporting_authoring_preview_v1',previewRequest(p.preview_request));assert.equal(f.counts().nativeSourceReadsRepresented,0);
  await f.invoke('reporting_authoring_execute_v1',{resume:false,...p.execute_request});
  assert.deepEqual(await f.invoke('reporting_view',viewRequest(p.root_request)),p.view_root);
  assert.deepEqual(await f.invoke('reporting_view',viewRequest(p.output_request)),p.view_output);
  assert.deepEqual(await f.invoke('reporting_view',viewRequest(p.notes_request)),p.view_notes);
  await f.invoke('reporting_describe',{target:d.published_description.resource.target,locale:'en-US',outputs:null});
  await f.invoke('reporting_run',runRequest(r.run_request));
  assert.deepEqual(await f.invoke('reporting_view',viewRequest(r.root_request)),r.view_root);
  for(const output of Object.values(r.outputs))assert.deepEqual(await f.invoke('reporting_view',viewRequest(output.request)),output.response);
  assert.deepEqual(await f.invoke('reporting_view',viewRequest(r.notes_request)),r.view_notes);
  assert.equal(f.counts().nativeSourceReadsRepresented,2);assert.deepEqual(f.snapshot(),saved);
  await assert.rejects(f.invoke('reporting_view',{...viewRequest(r.root_request),run:p.preview_response.id,widget:'foreign'}));
});

test('real native typed defaults survive staged cancel, page fences, temporary override and clear',async()=>{
  const f=await load(),definition=structuredClone(f.data.private_report.definition),page=definition.report_pages[0],day=page.filters.find(v=>v.parameter.name==='day'),region=page.filters.find(v=>v.parameter.name==='region');
  const app={closed:false,epoch:1,pageGeneration:1,activePageID:'analysis',session:{definition},render(){},editPage(change,id){change(definition.report_pages.find(v=>v.id===id));}};
  const controls=new ReportFilterControls(app),before=structuredClone(definition);
  controls.open(day,'analysis','default');const stale=controls.editor;stale.state.start='2026-01-01';app.pageGeneration++;controls.done(stale,filterInputValue(stale.state));assert.deepEqual(definition,before);
  controls.open(region,'analysis','preview');controls.done(controls.editor,{items:['North']});
  controls.open(day,'analysis','preview');controls.done(controls.editor,{date_range:{start:'2026-01-01',end_exclusive:'2026-01-02'}});
  assert.deepEqual(controls.pageInputs(definition),f.privateRun.preview_request.pages);
  assert.deepEqual(definition,before);controls.temporary.clear();assert.deepEqual(controls.pageInputs(definition),[]);assert.deepEqual(definition,before);
  const state=filterInputState(day.parameter,day.parameter.default);assert.equal(state.end,'2026-01-31');assert.deepEqual(filterInputValue(state),day.parameter.default);
  controls.open(region,'analysis','preview');assert.deepEqual(controls.editor.state.items,region.parameter.default.items);
  const exact=Array.from({length:16},(_,i)=>'Choice '+i);state.type='dimension_set';state.items=exact;assert.deepEqual(filterInputValue(state),{items:exact});state.items=[];assert.throws(()=>filterInputValue(state));state.items=[...exact,'Choice 16'];assert.throws(()=>filterInputValue(state));
  assert.equal(f.counts().nativeSourceReadsRepresented,0);
});

test('serialized synthetic clock arms exact native operation and resolution without changing DTOs',async()=>{
  const f=await load(),at=Date.parse(f.privateRun.preview_request.resolution.at),sandbox={Date,crypto:{randomUUID:()=> '12345678-1234-1234-1234-123456789abc'}};
  sandbox.window=sandbox;sandbox.parent={};vm.createContext(sandbox);vm.runInContext(`(${installNativeReplayClock.toString()})(${at})`,sandbox);
  sandbox.armNativeRequest(f.data.initial_option_request,'option');
  assert.equal(vm.runInContext("'option:'+Math.floor(Date.now()/1000)+':'+crypto.randomUUID().replaceAll('-','')",sandbox),f.data.initial_option_request.operation);
  assert.equal(vm.runInContext('Date.now()',sandbox),at);
  sandbox.armNativeRequest(f.privateRun.preview_request,'preview');
  assert.deepEqual(clean(vm.runInContext('({key:crypto.randomUUID(),at:new Date().toISOString()})',sandbox)),{key:f.privateRun.preview_request.key,at:f.privateRun.preview_request.resolution.at});
  assert.equal(vm.runInContext('crypto.randomUUID()',sandbox),'12345678-1234-1234-1234-123456789abc');
});


test('closing a pending native option response cannot repopulate choices or retarget a newer page',async()=>{
  const f=await load(),request=f.data.initial_option_request;let finish;
  const pending=new Promise(resolve=>{finish=resolve;});
  const lookup=new FilterOptionLookup(async(name,args)=>{assert.equal(name,'reporting_authoring_report_options_v1');assert.deepEqual(args,optionRequest(request));await pending;return f.data.initial_option_response;},request.target,{operation:()=>request.operation});
  const searched=lookup.search('East');assert.equal(lookup.pending,true);lookup.close();finish();assert.equal(await searched,false);assert.equal(lookup.closed,true);assert.deepEqual(lookup.values,[]);assert.equal(lookup.target,null);assert.equal(lookup.result,null);
});
