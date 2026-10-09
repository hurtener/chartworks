import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {presentationBrowserFixture,verifyPresentationCapture,presentationStageNames} from './presentation-browser-fixture.mjs';
import {MappingSession} from './mapping.js';
import {DraftSession,unpack} from './model.js';
import {RetainedReport} from './retained.js';
import {viewRequest,previewRequest} from './filters-browser-fixture.mjs';
const load=()=>presentationBrowserFixture(process.env.CHARTWORKS_PRESENTATION_FIXTURE_PATH||new URL('./testdata/presentation-native.json',import.meta.url));
const copy=value=>structuredClone(value);
function allocation(request,key='native-copy-key'){
 return {version:'report-app-allocation-v1',kind:'block',intent:'copy_chart',title:'Native presentation proof',idempotency_key:key,source:Object.fromEntries(['block','revision','expected_version','digest','output'].map(name=>[name,request[name]]))};
}
async function sessionFor(f,name){
 const stage=f.data.stages[name],request=stage.mutation_request,s=new MappingSession((tool,args)=>f.invoke(tool,args),'presentation');
 await s.open(request.block,request.revision,request.output,!!request.new_block,request.digest);
 s.edit(d=>{for(const edit of request.presentation.edits){const row=d.fields.find(value=>value.column===edit.column);assert(row);Object.assign(row,edit.set);for(const field of edit.reset||[])delete row[field];}});
 if(request.new_block)s.newBlock=(await f.allocate(allocation(request,'key-'+name))).id;
 return s;
}
async function execute(f,name){
 const s=f.data.stages[name];await f.invoke('reporting_authoring_preview_v1',previewRequest(s.preview_request));await f.invoke('reporting_authoring_execute_v1',{resume:false,...s.execute_request});
 const root=await f.invoke('reporting_view',viewRequest(s.view_root_request));assert.deepEqual(root,s.view_root);
 const retained=new RetainedReport((tool,args)=>f.invoke(tool,args)),before=f.counts();
 try{assert.equal(await retained.load(root),true);for(const key of ['table','kpi','notes']){const request=s.view_requests[key];assert.deepEqual(retained.get(request.page,request.widget,request.output||''),s.views[key]);}assert.deepEqual(f.counts(),before,'Production retained-page loading never runs another query');}
 finally{retained.close();}
}

test('native DTO capture keeps exact table values, modern KPI derivation, privacy and source measurement',async t=>{
 const f=await load(),d=f.data;t.mock.method(Date,'now',()=>Date.parse(d.stages.source.preview_request.resolution.at));assert.match(f.provenance.sha256,/^[a-f0-9]{64}$/);assert.equal(f.provenance.actualBrowserSourceCalls,0);
 assert.deepEqual(d.published_catalog.items,[],'There is no fabricated published Consumer report');
 const before=copy(d),draft=new DraftSession((name,args)=>f.invoke(name,args));await draft.open(f.report);await execute(f,'source');
 for(const name of presentationStageNames.slice(1)){
  const s=await sessionFor(f,name),stage=d.stages[name],reportBefore=f.snapshot().report,count=f.counts().nativeSourceReadsRepresented;
  assert.deepEqual(s.mutation(),stage.mutation_request.presentation);
  const saved=await s.save();assert.deepEqual(saved,stage.block);assert.deepEqual(f.snapshot().report,reportBefore,'No implicit report save');assert.equal(f.counts().nativeSourceReadsRepresented,count);
  draft.edit(definition=>{const widget=definition.report_pages[0].widgets.find(widget=>widget.id===(name==='formatted_kpi'?'kpi':'table'));widget.block={...widget.block,block:saved.block.state.id,revision:saved.block.revision,digest:saved.block.digest,policy:'private_preview'};});
  assert.equal(await draft.save(f.report),true);assert.deepEqual(draft.state,stage.report_state);await draft.open(f.report);assert.deepEqual(draft.definition,stage.report.definition);
  assert.equal(f.counts().nativeSourceReadsRepresented,count,'Metadata save and reopen have no source read');
  assert.deepEqual(await f.invoke('reporting_authoring_block_validate_v1',stage.validation_request),stage.validation);await execute(f,name);s.close();
 }
 draft.close();assert.deepEqual(d,before,'Replay never rewrites native evidence');assert.equal(f.allocations.length,2);assert.equal(f.counts().validations,4);assert.equal(f.counts().privateExecutions,5);
 assert.equal(f.counts().nativeSourceReadsRepresented,presentationStageNames.reduce((total,name)=>total+d.stages[name].counts.explicit_preview_execution.source_reads+(d.stages[name].counts.explicit_validation?.source_reads||0),0));
});

test('capture tampering fails instead of generating new provider results',async()=>{
 const f=await load();
 for(const mutate of [d=>d.source_block.block.private=true,d=>d.stages.copied_table.block.block.execution_digest='0'.repeat(64),d=>d.stages.formatted_kpi.views.kpi.output.chart.kpi_result.percent_delta.exact='999',d=>d.stages.reset_table.report.definition.report_pages.find(p=>p.id==='notes').title='changed',d=>d.response_contract.errors.duplicate_copy.status=200]){
  const d=copy(f.data);mutate(d);assert.throws(()=>verifyPresentationCapture(d));
 }
});

test('exact native mutation coordinates reject widened targets, revisions and unknown fields before any write',async()=>{
 for(const change of [r=>r.block='other',r=>r.new_block='unallocated',r=>r.revision++,r=>r.expected_version++,r=>r.digest='0'.repeat(64),r=>r.output='other',r=>r.mapping={},r=>r.presentation.edits[0].set.display_label='wrong']){
  const f=await load(),r=copy(f.data.stages.copied_table.mutation_request);await f.allocate(allocation(r));change(r);
  await assert.rejects(f.invoke('reporting_authoring_block_copy_v1',r));assert.equal(f.counts().metadataWrites,0);assert.equal(f.counts().nativeSourceReadsRepresented,0);
 }
});

test('synthetic host allocation uses exact native copy target and stable opaque key',async()=>{
 const f=await load(),request=f.data.stages.copied_table.mutation_request,args=allocation(request);
 const first=await f.allocate(args);assert.equal(first.id,request.new_block);assert.deepEqual(await f.allocate(copy(args)),first);assert.equal(f.allocations.length,1);
 await assert.rejects(f.allocate({...args,idempotency_key:'second-key'}));await assert.rejects(f.allocate({...args,source:{...args.source,digest:'0'.repeat(64)}}));
 assert.deepEqual(f.calls,[],'Allocation itself never invokes provider execution');
});

test('native captured CAS failure and synthetic unknown transport keep exact target and do not save report',async()=>{
 for(const failure of ['conflict','unknown']){
  const f=await load();f.reset(failure);const request=f.data.stages.copied_table.mutation_request;await f.allocate(allocation(request));const response=await f.reply('reporting_authoring_block_copy_v1',request);
  assert.equal(response.isError,true);assert.equal(response.structuredContent.error.code,failure==='conflict'?'conflict':'unavailable');
  assert.deepEqual(f.snapshot().report,f.data.initial_report);assert.equal(f.counts().nativeSourceReadsRepresented,0);
  if(failure==='unknown'){assert.equal(response.structuredContent.error.outcome,'unknown');assert.deepEqual(await f.invoke('reporting_authoring_block_read_v1',f.data.stages.copied_table.block_read_request),f.data.stages.copied_table.block);}
  else assert.deepEqual(response,f.data.response_contract.errors.duplicate_copy.mcp);
 }
});

test('native CAS error flows through production unpack without weakening its unknown outcome',async()=>{
 const f=await load();f.reset('conflict');const session=await sessionFor(f,'copied_table');
 session.invoke=async(name,args)=>unpack(await f.reply(name,args));
 await assert.rejects(session.save(),error=>error.code==='conflict');assert.equal(session.conflict,true);
 assert.equal(session.unknown,f.data.response_contract.errors.duplicate_copy.mcp.structuredContent.error.outcome==='unknown');
 await assert.rejects(session.save());assert.equal(f.calls.filter(call=>call.name==='reporting_authoring_block_copy_v1').length,1);
 assert.deepEqual(f.snapshot().report,f.data.initial_report);session.close();
});

test('bounded held native responses are released explicitly and cannot adopt into a closed mapping session',async()=>{
 const f=await load(),request=f.data.stages.copied_table.mutation_request,s=await sessionFor(f,'copied_table');
 const original=s.invoke;s.invoke=async(name,args)=>{const result=await f.reply(name,args);return result.structuredContent.result;};
 f.hold('reporting_authoring_block_copy_v1');const saving=s.save();await new Promise(resolve=>setImmediate(resolve));assert.equal(f.held(),1);
 await assert.rejects(s.save());s.close();await f.release();assert.equal(await saving,null);assert.deepEqual(f.snapshot().report,f.data.initial_report);assert.equal(f.calls.filter(c=>c.name==='reporting_authoring_block_copy_v1').length,1);s.invoke=original;
});

test('serialized clock pins native validation resolution without consuming a copy-allocation UUID',async()=>{
 const f=await load(),sandbox={Date,crypto:{randomUUID:()=> '11111111-1111-1111-1111-111111111111'}};sandbox.window=sandbox;sandbox.parent={};vm.createContext(sandbox);vm.runInContext(f.clockScript(),sandbox);
 const validation=f.data.stages.copied_table.validation_request;sandbox.armNativeRequest({...validation,key:null},'preview');assert.equal(vm.runInContext('new Date().toISOString()',sandbox),validation.resolution.at);assert.equal(vm.runInContext('crypto.randomUUID()',sandbox),'11111111-1111-1111-1111-111111111111');
 const preview=f.data.stages.copied_table.preview_request;sandbox.armNativeRequest(preview,'preview');assert.equal(vm.runInContext('crypto.randomUUID()',sandbox),preview.key);assert.equal(vm.runInContext('new Date().toISOString()',sandbox),preview.resolution.at);
});

test('hosted journey is source-only, captures both inspectors and retained layouts, and does not mislabel private Browse',async()=>{
 const source=await readFile(new URL('./presentation.browser.mjs',import.meta.url),'utf8');
 for(const requirement of ["captureAs('table-inspector')","captureAs('kpi-inspector')","'-saved-private'","'conflict','unknown','late-read','late-save'","repeat:2",".cell-precision",".retained-values > .raw-retained-value","await h.drain()"] )assert(source.includes(requirement),requirement);
 assert(!source.includes("click('Browse')"));assert(source.includes('private retained only; no live provider or published Consumer claim'));
 assert(!source.includes("spawn("));assert(!source.includes("from 'node:http'"));
});

test('late-reply journey expects the actual complete closed-app state',async()=>{
 const [app,journey]=await Promise.all(['app.js','presentation.browser.mjs'].map(file=>readFile(new URL('./'+file,import.meta.url),'utf8')));
 const message='This report is closed. Reopen it from your workspace.';
 assert(app.includes(message));assert(journey.includes(`textContent===${JSON.stringify(message).replaceAll('"',"'")}`));
 assert(journey.includes('&&!${inspector}'),'Late replies cannot restore inspector');
});
