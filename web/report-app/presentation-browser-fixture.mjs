// Exact native PostgreSQL public DTO replay. This is a synthetic host, never a
// provider, source, model, validation, publication or authority implementation.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {checkFormattingResult} from './formatting.js';
import {viewRequest, previewRequest, installNativeReplayClock} from './filters-browser-fixture.mjs';

const clone=value=>structuredClone(value);
const same=(actual,expected,message)=>assert.deepEqual(actual,expected,message);
export const presentationStageNames=['source','copied_table','amended_table','reset_table','formatted_kpi'];
export const presentationBrowserTools=['reporting_search','reporting_describe','reporting_view',...['capabilities','drafts','read','save','block_read','block_copy','block_mapping','block_validate','preview','execute'].map(name=>'reporting_authoring_'+name+'_v1')];
export const presentationMutations=['reporting_authoring_block_copy_v1','reporting_authoring_block_mapping_v1'];
const blockKey=view=>JSON.stringify([view.block.state.id,view.block.revision]);

export function verifyPresentationCapture(data){
  assert(data&&data.stages,'Native presentation capture must contain recorded stages');
  assert.equal(data.capture.backend,'real PostgreSQL synthetic source');
  same(data.capture.normalizations,[],'No native DTO normalization or generated data');
  assert.deepEqual(Object.keys(data.stages).sort(),[...presentationStageNames].sort());
  assert.equal(data.initial_report.private,true);assert.equal(data.initial_report.definition.schema_version,3);
  const report=data.initial_report.state.id,source=data.source_block;
  assert.equal(source.block.private,false,'Formatting source must actually be published before copying');
  const notes=data.initial_report.definition.report_pages.find(page=>page.id==='notes');assert(notes,'Native untargeted Notes page');
  let prior=source;
  for(const name of presentationStageNames){
    const stage=data.stages[name];assert(stage.block?.block&&stage.report?.definition&&stage.view_root?.summary,'Full public DTOs for '+name);
    assert.equal(stage.report.state.id,report);assert.equal(stage.report.private,true);
    same(stage.report.definition.report_pages.find(page=>page.id==='notes'),notes,'No invented Notes changes');
    same(stage.block_read_request,{block:stage.block.block.state.id,revision:stage.block.block.revision});
    assert.equal(stage.preview.private,true);assert.equal(stage.view_root.summary.private,true);assert.equal(stage.view_root.summary.run,stage.preview.id);
    assert.equal(stage.view_root.summary.target.revision,stage.report.revision);
    assert.equal(stage.complete.id,stage.preview.id);
    for(const widget of ['table','kpi','notes'])assert(stage.views[widget]&&stage.view_requests[widget],'Native retained '+widget+' '+name);
    if(name!=='source'){
      const request=stage.mutation_request,before=name==='formatted_kpi'?source:prior;
      assert(request.presentation&&!Object.hasOwn(request,'mapping'),'Purpose-specific native request only');
      checkFormattingResult(before,stage.block,request.output,request.presentation);
      assert.equal(stage.block.block.private,true);assert(!stage.block.block.validation,'Presentation success never carries validation');
      assert.equal(stage.block.block.execution_digest,before.block.execution_digest);
      if(request.new_block)assert.notEqual(request.new_block,request.block);
    }
    for(const key of ['mutation','save_reopen','preview_admission','retained_read_redraw_reopen'])if(stage.counts[key])same(stage.counts[key],{source_reads:0,source_lookups:0,model_calls:0},name+' '+key+' native no-work evidence');
    assert.equal(stage.counts.explicit_preview_execution.model_calls,0);
    same(stage.report_after_preview,stage.report,'Preview leaves exact saved report unchanged');
    prior=stage.block;
  }
  const before=data.stages.source.views.kpi.output.chart.kpi_result;
  for(const name of presentationStageNames)same(data.stages[name].views.kpi.output.chart.kpi_result,before,'KPI exact/derived/percent evidence is unchanged in '+name);
  const negative=data.response_contract.errors.duplicate_copy;
  assert.equal(negative.status,409);assert.equal(negative.body.error,'conflict');
  assert.equal(negative.mcp.isError,true);assert.equal(negative.mcp.structuredContent.error.code,'conflict');
  same(negative.request,data.stages.copied_table.mutation_request,'Actual duplicate native copy conflict uses the identical request');
  return data;
}

export async function presentationBrowserFixture(path=new URL('./testdata/presentation-native.json',import.meta.url)){
  const bytes=await readFile(path);assert(bytes.length<=1<<20,'Bounded exact native DTO recording');
  const data=verifyPresentationCapture(JSON.parse(bytes)),report=data.initial_report.state.id;
  const calls=[],allocations=[],availableBlocks=new Map(),availableViews=[];
  let currentName,reportName,validated,previewed,executed,scenario,holdName,held=[],releaseWaiters=[];
  const counters={metadataWrites:0,validations:0,privateExecutions:0,nativeSourceReadsRepresented:0};
  const stage=()=>data.stages[currentName],reportStage=()=>data.stages[reportName];
  function reset(nextScenario='happy'){
    assert.equal(held.length,0,'Release every held transport reply before resetting');
    currentName=reportName='source';validated=new Set();previewed=new Set();executed=new Set();scenario=nextScenario;holdName='';
    availableBlocks.clear();availableViews.splice(0);availableBlocks.set(blockKey(data.source_block),clone(data.source_block));
    calls.splice(0);allocations.splice(0);for(const name of Object.keys(counters))counters[name]=0;
  }
  const record=(name,args)=>calls.push({name,args:clone(args)});
  const snapshot=()=>({report:clone(reportStage().report),blocks:clone([...availableBlocks.values()]),stage:currentName});
  async function invoke(name,args){
    assert(presentationBrowserTools.includes(name),'Only advertised native metadata and explicit preview tools');
    let result;
    if(name==='reporting_authoring_capabilities_v1'){
      assert(Object.keys(args).length===1&&['',report].includes(args.report),'Exact report capability target');
      result=args.report?data.capabilities:data.global_capabilities;
    }else if(name==='reporting_search'){
      same(args,{kind:'report',query:'',after:'',limit:40,locale:'en-US'},'Exact bounded published catalog request');result=data.published_catalog;
    }else if(name==='reporting_authoring_drafts_v1'){
      same(args,{after:'',limit:40},'Exact bounded private catalog request');result=reportName==='source'?data.initial_drafts:reportStage().drafts;
    }else if(name==='reporting_authoring_read_v1'){
      assert([0,reportStage().report.revision].includes(args.revision),'Only current persisted native report');same(args,{report,revision:args.revision});result=reportStage().report;
    }else if(name==='reporting_authoring_block_read_v1'){
      same(args,{block:args.block,revision:args.revision},'Closed native block-read target');result=availableBlocks.get(JSON.stringify([args.block,args.revision]));assert(result,'Only recorded immutable block metadata can be read');
    }else if(presentationMutations.includes(name)){
      const next=presentationStageNames[presentationStageNames.indexOf(currentName)+1];assert(next,'No extra mutation');const target=data.stages[next];
      same(args,target.mutation_request,'Exact native presentation patch, target, revision, digest and CAS');
      assert.equal(name,args.new_block?'reporting_authoring_block_copy_v1':'reporting_authoring_block_mapping_v1');
      if(args.new_block){assert.equal(allocations.length,next==='formatted_kpi'?2:1,'A copy uses one exact host allocation');assert.equal(allocations.at(-1).id,args.new_block);}
      currentName=next;availableBlocks.set(blockKey(target.block),clone(target.block));counters.metadataWrites++;result=target.block;
    }else if(name==='reporting_authoring_save_v1'){
      assert.notEqual(reportName,currentName,'One report save after each native chart mutation');same(args,stage().report_save_request,'Exact native report CAS and selected-widget pin');reportName=currentName;counters.metadataWrites++;result=stage().report_state;
    }else if(name==='reporting_authoring_block_validate_v1'){
      assert(!validated.has(currentName),'Only one deliberate validation per changed chart');same(args,stage().validation_request,'Exact native validation and accepted resolution');validated.add(currentName);counters.validations++;counters.nativeSourceReadsRepresented+=stage().counts.explicit_validation.source_reads;
      availableBlocks.set(blockKey(stage().validated_block),clone(stage().validated_block));result=stage().validation;
    }else if(name==='reporting_authoring_preview_v1'){
      assert.equal(reportName,currentName,'Preview uses the saved native report');assert(!previewed.has(currentName),'One explicit preview of each saved stage');
      same(args,previewRequest(stage().preview_request),'Exact native private preview admission');previewed.add(currentName);result=stage().preview;
    }else if(name==='reporting_authoring_execute_v1'){
      assert(previewed.has(currentName)&&!executed.has(currentName),'Only one execution of the admitted preview');same(args,{resume:false,...stage().execute_request});executed.add(currentName);counters.privateExecutions++;counters.nativeSourceReadsRepresented+=stage().counts.explicit_preview_execution.source_reads;
      availableViews.push([viewRequest(stage().view_root_request),stage().view_root]);for(const key of ['table','kpi','notes'])availableViews.push([viewRequest(stage().view_requests[key]),stage().views[key]]);result=stage().complete;
    }else if(name==='reporting_view'){
      const found=availableViews.find(([request])=>{try{same(args,request);return true;}catch{return false;}});assert(found,'Only retained reads from an explicit completed native preview');result=found[1];
    }
    assert(result,'No invented provider response for '+name);record(name,args);return clone(result);
  }
  async function allocate(args){
    const next=presentationStageNames[presentationStageNames.indexOf(currentName)+1],request=data.stages[next]?.mutation_request;
    assert(request?.new_block,'Only the captured native copy needs a host allocation');
    same(Object.keys(args).sort(),['version','kind','intent','title','idempotency_key','source'].sort());
    same(args.source,Object.fromEntries(['block','revision','expected_version','digest','output'].map(key=>[key,request[key]])),'Allocation preserves exact native source coordinates');
    assert.equal(args.version,'report-app-allocation-v1');assert.equal(args.kind,'block');assert.equal(args.intent,'copy_chart');assert(typeof args.title==='string'&&args.title.length>0&&args.title.length<=256);
    assert(/^[A-Za-z0-9_.:-]{1,128}$/.test(args.idempotency_key));
    const prior=allocations.find(value=>value.request.idempotency_key===args.idempotency_key);
    if(prior){same(prior.request,args,'Stable allocation key cannot be retargeted');return clone(prior.reply);}
    assert(!allocations.some(value=>value.id===request.new_block),'No second key for the same copy target');
    const reply={version:args.version,kind:args.kind,intent:args.intent,idempotency_key:args.idempotency_key,id:request.new_block};
    allocations.push({request:clone(args),id:request.new_block,reply});return clone(reply);
  }
  async function reply(name,args){
    let response;
    if(presentationMutations.includes(name)&&scenario==='conflict'){
      same(args,data.response_contract.errors.duplicate_copy.request,'Native captured conflict is tied to identical copy input');
      record(name,args);response=clone(data.response_contract.errors.duplicate_copy.mcp);
    }else{
      const result=await invoke(name,args);
      // Intentionally synthetic transport uncertainty after a captured successful
      // native commit. This is not a fabricated native authority/provider error.
      response=presentationMutations.includes(name)&&scenario==='unknown'?{isError:true,structuredContent:{error:{code:'unavailable',outcome:'unknown'}}}:{structuredContent:{result}};
    }
    if(name===holdName){await new Promise(resolve=>held.push(resolve));for(const done of releaseWaiters.splice(0))done();}
    return response;
  }
  async function release(){holdName='';const pending=held.splice(0);if(!pending.length)return;const completed=new Promise(resolve=>releaseWaiters.push(resolve));for(const finish of pending)finish();await completed;}
  reset();
  return {data,report,calls,allocations,invoke,reply,allocate,snapshot,reset,counts:()=>clone(counters),hold:name=>{assert(!holdName&&!held.length);holdName=name;},held:()=>held.length,release,
    provenance:{kind:'native PostgreSQL public DTO-backed synthetic browser replay',sha256:createHash('sha256').update(bytes).digest('hex'),actualBrowserSourceCalls:0,actualBrowserModelCalls:0},
    clockScript:()=>`(${installNativeReplayClock.toString()})(${Date.parse(data.stages.source.preview_request.resolution.at)});`};
}
