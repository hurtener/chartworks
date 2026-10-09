// Native DTO-backed synthetic host. This replay is not a live authority or
// warehouse implementation: exact native argument matching is its entire seam.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {checkDatasetView} from './dataset.js';

const clone = value => structuredClone(value);
export const filterBrowserTools = ['reporting_search', 'reporting_describe', 'reporting_view', 'reporting_run',
  ...['capabilities','drafts','read','save','block_read','preview','execute','report_options','option_status','option_control'].map(name => `reporting_authoring_${name}_v1`)];

// Go omitempty fields and explicit browser empty/default fields are equivalent
// only at these known request coordinates. No unknown key is discarded.
export function viewRequest(request) {return {page:'',widget:'',output:'',offset:0,limit:100,...clone(request)};}
export function runRequest(request) {return {arguments:[],pages:[],outputs:[],policy:'',narrative:false,dynamic:false,partial_failure:'',...clone(request)};}
export function previewRequest(request) {return {pages:[],...clone(request)};}
export function optionRequest(request) {return {cursor:'',...clone(request)};}

// Serialized solely into the synthetic iframe by CDP before production code.
// Recorded operation IDs must remain exact because input_digest includes them.
// Unarmed UI UUIDs remain genuine random values; no production module changes.
export function installNativeReplayClock(at) {
  if (window === window.parent) return;
  const NativeDate = Date, nativeUUID = crypto.randomUUID.bind(crypto);
  let clock = at, nextUUID = null, operationTime = null;
  class ReplayDate extends NativeDate {
    constructor(...args) {super(...(args.length ? args : [clock]));}
    static now() {return operationTime ?? clock;}
  }
  window.Date = ReplayDate;
  Object.defineProperty(crypto,'randomUUID',{configurable:true,value:()=>{
    if(nextUUID===null)return nativeUUID();
    const value=nextUUID;nextUUID=null;operationTime=null;return value;
  }});
  window.armNativeRequest=(request,kind)=>{
    if(nextUUID!==null)throw new Error('An exact native request is already armed');
    if(kind==='option'){
      const match=/^option:([1-9][0-9]*):([a-f0-9]{32})$/.exec(request.operation);
      if(!match)throw new Error('Invalid native option operation');
      operationTime=Number(match[1])*1000;
      nextUUID=match[2].replace(/^(.{8})(.{4})(.{4})(.{4})(.{12})$/,'$1-$2-$3-$4-$5');
    }else{
      nextUUID=request.key;
      if(request.resolution)clock=NativeDate.parse(request.resolution.at);
    }
  };
}

export async function governedFilterBrowserFixture(path=new URL('./testdata/governed-filter-native.json',import.meta.url)) {
  const bytes=await readFile(path);assert(bytes.length<=512<<10,'Bounded native DTO recording');
  const data=JSON.parse(bytes);
  checkDatasetView(data.dataset,data.prepare_request.intent.topic,data.prepare_request.intent.dataset);
  const report=data.initial_report.state.id, block=data.created.block;
  assert.equal(data.created.block.source,'filtered-source');
  assert.equal(data.created.block.context,'filtered-source:v1');
  assert.deepEqual(block.topics,[data.prepare_request.intent.topic]);
  assert.equal(data.initial_report.private,true);
  assert.equal(data.private_report.private,true);
  assert.equal(data.published_description.resource.target.id,report);
  assert.equal(data.published_description.definition_digest,data.published_report.digest);
  assert.deepEqual(data.private_after_runs.definition,data.private_report.definition);
  assert.deepEqual(data.published_after_runs.definition,data.published_report.definition);
  assert.deepEqual(data.private_report.definition.report_pages[1],data.initial_report.definition.report_pages[1]);
  for(const [key,policy,view] of [['initial','private_preview',data.initial_report],['private','private_preview',data.private_report],['published','published',data.published_report]]){
    const request=data[key+'_option_request'],response=data[key+'_option_response'];
    assert.deepEqual(request.target,{report:{policy,report,revision:view.revision,digest:view.digest,page:'analysis',filter:'region'}});
    assert.equal(request.operation,response.operation);assert.equal(request.limit,199);assert.equal(request.search,'East');
    assert.deepEqual(response.options,[{value:'East',label:'East'}]);assert.equal(response.complete,true);
  }
  assert.deepEqual(data.read_counts,{create_open_metadata:0,save_reopen_metadata:0,explicit_option_search:1,preview_admission:0,explicit_validation:1,retained_view:0,model_calls:0});
  for(const [kind,runs] of [['private',data.private_runs],['published',data.published_runs]])for(const [key,value] of Object.entries(runs)){
    assert.equal(value.source_reads,1);assert.equal(value.view_root.summary.private,kind==='private');
    const output=kind==='private'?value.view_output:Object.values(value.outputs)[0].response;
    assert.equal(output.output.chart.points[0].value.exact,key==='temporary'?'1.250':'3.250');
  }
  let saved=false,consumer=false,previewed=false,executed=false,publishedRun=false;
  const calls=[],usedOperations=new Set(),counts={optionSearches:0,privateExecutions:0,publishedRuns:0,nativeSourceReadsRepresented:0};
  const privateRun=data.private_runs.temporary,publicRun=data.published_runs.temporary;
  const snapshot=()=>({report:clone(saved?data.private_report:data.initial_report),published:clone(data.published_report),block:clone(data.created.block)});
  const availableViews=new Map();
  function addViews(run){availableViews.set(JSON.stringify(viewRequest(run.root_request)),run.view_root);availableViews.set(JSON.stringify(viewRequest(run.notes_request)),run.view_notes);if(run.outputs)for(const output of Object.values(run.outputs))availableViews.set(JSON.stringify(viewRequest(output.request)),output.response);else availableViews.set(JSON.stringify(viewRequest(run.output_request)),run.view_output);}
  const match=(actual,expected,message)=>assert.deepEqual(actual,expected,message);
  async function invoke(name,args){
    assert(filterBrowserTools.includes(name),'Unadvertised tool');
    let result;
    if(name==='reporting_authoring_capabilities_v1'){
      assert(Object.keys(args).length===1&&['',report].includes(args.report),'Exact capability target');
      result=consumer?data.consumer_capabilities:data.capabilities;
    }else if(name==='reporting_search'){
      match(args,{kind:'report',query:'',after:'',limit:40,locale:'en-US'},'Exact bounded catalog request');result=data.published_catalog;
    }else if(name==='reporting_authoring_drafts_v1'){
      match(args,{after:'',limit:40},'Exact bounded private catalog request');result=saved?data.saved_drafts:data.initial_drafts;
    }else if(name==='reporting_authoring_read_v1'){
      const current=saved?data.private_report:data.initial_report;
      assert(args.report===report&&[0,current.revision].includes(args.revision),'Exact current private report');
      match(args,{report,revision:args.revision},'Closed report read target');result=current;
    }else if(name==='reporting_authoring_save_v1'){
      assert(!saved,'Exactly one explicit save');match(args,data.save_request,'Exact native CAS, revision, definition and typed defaults');saved=true;result=data.saved_state;
    }else if(name==='reporting_authoring_report_options_v1'){
      const key=args.target?.report?.policy==='published'?'published':saved?'private':'initial';
      const expected=optionRequest(data[key+'_option_request']);
      match(args,expected,'Closed native policy, report, revision, digest, page/filter and operation/search bounds');
      assert(!usedOperations.has(args.operation),'Each explicit source operation is issued once');usedOperations.add(args.operation);
      counts.optionSearches++;counts.nativeSourceReadsRepresented+=data.read_counts.explicit_option_search;result=data[key+'_option_response'];
    }else if(name==='reporting_authoring_block_read_v1'){
      match(args,{block:block.state.id,revision:block.revision},'Exact native block/context/topic metadata');result=data.validated_block;
    }else if(name==='reporting_authoring_preview_v1'){
      assert(saved&&!previewed,'One explicitly requested preview of saved defaults plus temporary inputs');
      match(args,previewRequest(privateRun.preview_request),'Exact private temporary page filters and accepted resolution');previewed=true;result=privateRun.preview_response;
    }else if(name==='reporting_authoring_execute_v1'){
      assert(previewed&&!executed,'Only the admitted private run executes');match(args,{resume:false,...privateRun.execute_request},'Exact admitted private run');executed=true;
      counts.privateExecutions++;counts.nativeSourceReadsRepresented+=privateRun.source_reads;addViews(privateRun);result=privateRun.execute_response;
    }else if(name==='reporting_describe'){
      match(args,{target:data.published_description.resource.target,locale:'en-US',outputs:null},'Exact published report revision');consumer=true;result=data.published_description;
    }else if(name==='reporting_run'){
      assert(consumer&&!publishedRun,'One explicitly requested published run');match(args,runRequest(publicRun.run_request),'Exact published temporary filters and no hidden execution switches');publishedRun=true;
      counts.publishedRuns++;counts.nativeSourceReadsRepresented+=publicRun.source_reads;addViews(publicRun);result=publicRun.run_response;
    }else if(name==='reporting_view'){
      // Stable canonical key order lets comparison be structural, not insertion-order-dependent.
      for(const [key,value] of availableViews){try{match(args,JSON.parse(key));result=value;break;}catch{}}
      assert(result,'Only exact native retained root/output/page reads from an explicit completed run');
    }else throw new Error('Unexpected tool; no hidden source, model, publication or recovery operation is permitted: '+name);
    calls.push({name,args:clone(args)});return clone(result);
  }
  return {data,report,privateRun,publicRun,calls,invoke,snapshot,counts:()=>clone(counts),
    provenance:{kind:'native PostgreSQL DTO-backed synthetic browser replay',sha256:createHash('sha256').update(bytes).digest('hex'),actualBrowserSourceCalls:0,actualBrowserModelCalls:0},
    clockScript:()=>`(${installNativeReplayClock.toString()})(${Date.parse(privateRun.preview_request.resolution.at)});`};
}
