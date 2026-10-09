import {INVALID_REQUEST, FORBIDDEN, UNAVAILABLE, BUSY, CANCELLED_OR_TIMED_OUT} from './error-codes.js';
import {boundedJSON} from '../report-viewer/presentation.js';
import {appError, APP_MAX_WIRE} from './model.js';

export const REPORT_APP_TOOLS = new Set(['chart_catalog','list_source_page','list_datasets','list_topics','describe_topic','reporting_search','reporting_describe','reporting_runs','reporting_view','reporting_run','reporting_filter_options',...['capabilities','drafts','read','create','save','preview','execute','widget','block_read','block_mapping','block_copy','block_validate','dataset','prepare_chart','preparation','create_prepared','preparation_control','dataset_options','report_options','option_status','option_control','lifecycle','block_publish','rebind_published','report_transition'].map(a=>`reporting_authoring_${a}_v1`)]);
// Initialization metadata is a bounded immutable narrowing hint, never authority.
export function hostToolHint(container,key) {
  if(!container||!Object.hasOwn(container,key))return null;
  try {
    const value=boundedJSON(container[key],16<<10);
    if(!value||Array.isArray(value)||Object.keys(value).length!==2||value.version!=='chartworks-host-tools-v1'||!Array.isArray(value.names)||value.names.length>96||value.names.some(name=>typeof name!=='string'||!/^[A-Za-z0-9_.:-]{1,128}$/.test(name))||new Set(value.names).size!==value.names.length)throw appError(INVALID_REQUEST);
    return Object.freeze([...value.names]);
  }catch{return Object.freeze([]);}
}
const wireLimit=APP_MAX_WIRE;
const bridgeAllocationVersion='report-app-allocation-v1';
const bridgeAllocationMethod='app/allocate-target';
const reportAccessVersion='report-access-v1';
function reportAccessSupport(value){try{boundedJSON(value,256);bridgeAllocationObject(value,['version']);return value.version===reportAccessVersion;}catch{return false;}}
const bridgeAllocationLimit=4096;
const bridgeAllocationID=value=>typeof value==='string'&&/^[A-Za-z0-9_.:-]{1,128}$/.test(value);
function bridgeAllocationObject(value,keys,required=keys) { if(!value||typeof value!=='object'||Array.isArray(value)||Object.keys(value).some(key=>!keys.includes(key))||required.some(key=>!Object.hasOwn(value,key)))throw appError(INVALID_REQUEST); }
function bridgeAllocationSupport(value) { try{boundedJSON(value,256);bridgeAllocationObject(value,['version']);return value.version===bridgeAllocationVersion;}catch{return false;} }
function bridgeAllocationRequest(value) {
  boundedJSON(value,bridgeAllocationLimit);bridgeAllocationObject(value,['version','kind','intent','title','idempotency_key','source'],['version','kind','intent','title','idempotency_key']);
  if(value.version!==bridgeAllocationVersion||!bridgeAllocationID(value.idempotency_key)||typeof value.title!=='string'||!value.title.trim()||value.title!==value.title.trim()||value.title.length>256||!['create_report','create_chart','copy_chart'].includes(value.intent)||!({create_report:'report',create_chart:'block',copy_chart:'block'}[value.intent]===value.kind))throw appError(INVALID_REQUEST);
  if(value.intent==='copy_chart') {
    const source=value.source;bridgeAllocationObject(source,['block','revision','expected_version','digest','output']);
    if(!bridgeAllocationID(source.block)||!bridgeAllocationID(source.output)||!Number.isSafeInteger(source.revision)||source.revision<1||source.revision>256||!Number.isSafeInteger(source.expected_version)||source.expected_version<1||typeof source.digest!=='string'||!/^[a-f0-9]{64}$/.test(source.digest))throw appError(INVALID_REQUEST);
  } else if(Object.hasOwn(value,'source'))throw appError(INVALID_REQUEST);
  return JSON.parse(JSON.stringify(value));
}
function bridgeAllocationResult(value,request) {
  try {
    boundedJSON(value,bridgeAllocationLimit);bridgeAllocationObject(value,['version','kind','intent','idempotency_key','id']);
    if(['version','kind','intent','idempotency_key'].some(key=>value[key]!==request[key])||!bridgeAllocationID(value.id)||value.id===request.source?.block)throw appError(INVALID_REQUEST);
    return JSON.parse(JSON.stringify(value));
  } catch { throw appError(UNAVAILABLE,true); }
}
function bridgeOrigin(origin,secure=false) { try { const u=new URL(origin); return u.origin===origin && (secure?u.protocol==='https:':['https:','http:'].includes(u.protocol)); } catch { return false; } }
class ParentTransport {
  constructor(win,expectedOrigin=null) { this.win=win;this.parent=win.parent;this.origin=expectedOrigin;this.ready=false;this.closed=false;this.sequence=0;this.pending=new Map();this.targetAllocation=false;this.reportAccess=false;this.oncontext=()=>{};this.onresult=()=>{};this.onclose=()=>{};this.onfailure=()=>{};this.listener=e=>this.receive(e);win.addEventListener('message',this.listener);this.unload=()=>this.close();win.addEventListener('pagehide',this.unload); }
  send(message) { if(this.closed)throw appError(UNAVAILABLE);boundedJSON(message,wireLimit);this.parent.postMessage(message,this.origin||'*'); }
  request(method,params) { if(this.closed||this.pending.size>=16)return Promise.reject(appError(BUSY));const id=++this.sequence;return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{this.pending.delete(id);reject(appError(CANCELLED_OR_TIMED_OUT,method==='tools/call'||method===bridgeAllocationMethod));},65000);this.pending.set(id,{resolve,reject,timer,method});try{this.send(this.envelope({id,method,params}));}catch(e){clearTimeout(timer);this.pending.delete(id);reject(method===bridgeAllocationMethod?appError(UNAVAILABLE,true):e);}}); }
  settle(message) { const p=this.pending.get(message.id);if(!p)return false;this.pending.delete(message.id);clearTimeout(p.timer);if(message.error)p.reject(appError(UNAVAILABLE,p.method==='tools/call'||p.method===bridgeAllocationMethod));else p.resolve(message.result);return true; }
  supportsTool(name) { return this.ready&&!this.closed&&this.canCall&&REPORT_APP_TOOLS.has(name)&&(this.toolHint===null||this.toolHint?.includes(name)===true); }
  async call(name,args) { if(!this.supportsTool(name))throw appError(FORBIDDEN);boundedJSON(args,1<<20);return this.request('tools/call',{name,arguments:args}); }
  // This host extension reserves an opaque target, never grants provider authority.
  // The caller owns the stable operation key and any explicit retry decision.
  supportsReportAccess(){return this.ready&&!this.closed&&this.reportAccess;}
  async manageReportAccess(value){if(!this.supportsReportAccess())throw appError(FORBIDDEN);boundedJSON(value,512);bridgeAllocationObject(value,['report','revision']);if(!bridgeAllocationID(value.report)||!Number.isSafeInteger(value.revision)||value.revision<1||value.revision>256)throw appError(INVALID_REQUEST);const result=await this.request('app/manage-report-access',{report:value.report,revision:value.revision});if(this.closed||!result||result.opened!==true||Object.keys(result).length!==1)throw appError(UNAVAILABLE);return result;}
  supportsTargetAllocation() { return this.ready&&!this.closed&&this.targetAllocation; }
  async allocateTarget(value) { if(!this.supportsTargetAllocation())throw appError(FORBIDDEN);const request=bridgeAllocationRequest(value),result=await this.request(bridgeAllocationMethod,request);if(this.closed)throw appError(UNAVAILABLE,true);return bridgeAllocationResult(result,request); }
  resize(width,height) { if(this.ready&&!this.closed&&Number.isFinite(width)&&Number.isFinite(height))this.send(this.envelope({method:'ui/notifications/size-changed',params:{width:Math.min(1600,Math.max(200,Math.ceil(width))),height:Math.min(2400,Math.max(100,Math.ceil(height)))}})); }
  close() { if(this.closed)return;this.closed=true;this.ready=false;this.targetAllocation=false;this.reportAccess=false;this.win.removeEventListener('message',this.listener);this.win.removeEventListener('pagehide',this.unload);for(const p of this.pending.values()){clearTimeout(p.timer);p.reject(appError(UNAVAILABLE,p.method===bridgeAllocationMethod));}this.pending.clear();this.onclose(); }
}
export class MCPReportAdapter extends ParentTransport {
  constructor(win=window) { super(win);this.canCall=false; }
  envelope(message) { return {jsonrpc:'2.0',...message}; }
  async connect() { if(this.closed||this.ready||this.pending.size)throw appError(FORBIDDEN);if(this.parent===this.win)throw appError(UNAVAILABLE);const r=await this.request('ui/initialize',{protocolVersion:'2026-01-26',appInfo:{name:'Chartworks report app',version:'1'},appCapabilities:{availableDisplayModes:['inline','fullscreen']}});if(this.closed||r?.protocolVersion!=='2026-01-26'){this.close();throw appError(UNAVAILABLE);}Object.defineProperty(this,'toolHint',{value:hostToolHint(r.hostContext,'chartworks/supported-tools')});this.canCall=!!r.hostCapabilities?.serverTools;this.targetAllocation=bridgeAllocationSupport(r.hostContext?.['chartworks/target-allocation']);this.reportAccess=reportAccessSupport(r.hostContext?.['chartworks/report-access']);this.ready=true;this.oncontext(r.hostContext||{});this.send(this.envelope({method:'ui/notifications/initialized'}));return r; }
  receive(event) { if(this.closed||event.source!==this.parent||this.origin!==null&&event.origin!==this.origin)return;try { const m=boundedJSON(event.data,wireLimit);if(m?.jsonrpc!=='2.0')return;
    if(m.id!==undefined&&(Object.hasOwn(m,'result')||Object.hasOwn(m,'error'))) { if(this.origin===null){if(this.pending.get(m.id)?.method!=='ui/initialize'||!bridgeOrigin(event.origin))return;this.origin=event.origin;}this.settle(m);return; }
    if(!this.ready)return;
    if(m.method==='ui/notifications/host-context-changed')this.oncontext(m.params);
    else if(m.method==='ui/notifications/tool-result')this.onresult(m.params);
    else if(m.method==='ui/resource-teardown'&&m.id!==undefined){this.send(this.envelope({id:m.id,result:{}}));this.close();}
    else if(m.method==='ping'&&m.id!==undefined)this.send(this.envelope({id:m.id,result:{}}));
    else if(m.id!==undefined)this.send(this.envelope({id:m.id,error:{code:-32601,message:'not_found'}}));
  } catch { this.onfailure(appError(INVALID_REQUEST)); } }
}
// Integration boundary, deliberately not a launched host lane. The trusted host
// registers the public origin/frame/generation and a one-use correlation challenge.
// None is an authority credential. Host admission and provider calls remain outside.
export class EmbeddedReportAdapter extends ParentTransport {
  constructor({win=window,origin,frame,generation,challenge}) { if(!bridgeOrigin(origin,true)||typeof frame!=='string'||!/^[-A-Za-z0-9_.:]{1,128}$/.test(frame)||!Number.isSafeInteger(generation)||generation<1||typeof challenge!=='string'||!/^[-A-Za-z0-9_.:]{16,128}$/.test(challenge))throw appError(INVALID_REQUEST);super(win,origin);this.frame=frame;this.generation=generation;this.challenge=challenge;this.canCall=false; }
  envelope(message) { return {protocol:'chartworks-report-app-v1',frame:this.frame,generation:this.generation,...message}; }
  async connect() { if(this.closed||this.ready||this.pending.size)throw appError(FORBIDDEN);if(this.parent===this.win)throw appError(UNAVAILABLE);const result=await this.request('initialize',{challenge:this.challenge});if(this.closed||result?.challenge!==this.challenge){this.close();throw appError(FORBIDDEN);}this.challenge=null;Object.defineProperty(this,'toolHint',{value:hostToolHint(result.capabilities,'supported_tools')});this.canCall=result.tools===true;this.targetAllocation=bridgeAllocationSupport(result.capabilities?.target_allocation);this.reportAccess=reportAccessSupport(result.capabilities?.report_access);this.ready=true;this.oncontext(result.context||{});return result; }
  receive(event) { if(this.closed||event.source!==this.parent||event.origin!==this.origin)return;try { const m=boundedJSON(event.data,wireLimit);if(m?.protocol!=='chartworks-report-app-v1'||m.frame!==this.frame||m.generation!==this.generation)return;
    if(m.id!==undefined&&(Object.hasOwn(m,'result')||Object.hasOwn(m,'error'))){this.settle(m);return;}
    if(!this.ready)return;
    if(m.method==='close'){this.close();return;}
    if(m.method==='context')this.oncontext(m.params);
    else if(m.id!==undefined)this.send(this.envelope({id:m.id,error:{code:-32601,message:'not_found'}}));
  } catch { this.onfailure(appError(INVALID_REQUEST)); } }
}
