import {boundedJSON} from '../report-viewer/app.js';
import {appError, APP_MAX_WIRE} from './model.js';

export const REPORT_APP_TOOLS = new Set(['reporting_search','reporting_describe','reporting_runs','reporting_view','reporting_run','reporting_filter_options',...['capabilities','drafts','read','create','save','preview','execute','widget'].map(a=>`reporting_authoring_${a}_v1`)]);
const wireLimit=APP_MAX_WIRE;
function bridgeOrigin(origin,secure=false) { try { const u=new URL(origin); return u.origin===origin && (secure?u.protocol==='https:':['https:','http:'].includes(u.protocol)); } catch { return false; } }
class ParentTransport {
  constructor(win,expectedOrigin=null) { this.win=win;this.parent=win.parent;this.origin=expectedOrigin;this.ready=false;this.closed=false;this.sequence=0;this.pending=new Map();this.oncontext=()=>{};this.onresult=()=>{};this.onclose=()=>{};this.onfailure=()=>{};this.listener=e=>this.receive(e);win.addEventListener('message',this.listener);this.unload=()=>this.close();win.addEventListener('pagehide',this.unload); }
  send(message) { if(this.closed)throw appError('unavailable');boundedJSON(message,wireLimit);this.parent.postMessage(message,this.origin||'*'); }
  request(method,params) { if(this.closed||this.pending.size>=16)return Promise.reject(appError('busy'));const id=++this.sequence;return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{this.pending.delete(id);reject(appError('cancelled_or_timed_out',method==='tools/call'));},65000);this.pending.set(id,{resolve,reject,timer,method});try{this.send(this.envelope({id,method,params}));}catch(e){clearTimeout(timer);this.pending.delete(id);reject(e);}}); }
  settle(message) { const p=this.pending.get(message.id);if(!p)return false;this.pending.delete(message.id);clearTimeout(p.timer);if(message.error)p.reject(appError('unavailable',p.method==='tools/call'));else p.resolve(message.result);return true; }
  async call(name,args) { if(!this.ready||this.closed||!REPORT_APP_TOOLS.has(name)||!this.canCall)throw appError('forbidden');boundedJSON(args,1<<20);return this.request('tools/call',{name,arguments:args}); }
  resize(width,height) { if(this.ready&&!this.closed&&Number.isFinite(width)&&Number.isFinite(height))this.send(this.envelope({method:'ui/notifications/size-changed',params:{width:Math.min(1600,Math.max(200,Math.ceil(width))),height:Math.min(2400,Math.max(100,Math.ceil(height)))}})); }
  close() { if(this.closed)return;this.closed=true;this.ready=false;this.win.removeEventListener('message',this.listener);this.win.removeEventListener('pagehide',this.unload);for(const p of this.pending.values()){clearTimeout(p.timer);p.reject(appError('unavailable'));}this.pending.clear();this.onclose(); }
}
export class MCPReportAdapter extends ParentTransport {
  constructor(win=window) { super(win);this.canCall=false; }
  envelope(message) { return {jsonrpc:'2.0',...message}; }
  async connect() { if(this.parent===this.win)throw appError('unavailable');const r=await this.request('ui/initialize',{protocolVersion:'2026-01-26',appInfo:{name:'Chartworks report app',version:'1'},appCapabilities:{availableDisplayModes:['inline','fullscreen']}});if(this.closed||r?.protocolVersion!=='2026-01-26'){this.close();throw appError('unavailable');}this.canCall=!!r.hostCapabilities?.serverTools;this.ready=true;this.oncontext(r.hostContext||{});this.send(this.envelope({method:'ui/notifications/initialized'}));return r; }
  receive(event) { if(this.closed||event.source!==this.parent||this.origin!==null&&event.origin!==this.origin)return;try { const m=boundedJSON(event.data,wireLimit);if(m?.jsonrpc!=='2.0')return;
    if(m.id!==undefined&&(Object.hasOwn(m,'result')||Object.hasOwn(m,'error'))) { if(this.origin===null){if(this.pending.get(m.id)?.method!=='ui/initialize'||!bridgeOrigin(event.origin))return;this.origin=event.origin;}this.settle(m);return; }
    if(!this.ready)return;
    if(m.method==='ui/notifications/host-context-changed')this.oncontext(m.params);
    else if(m.method==='ui/notifications/tool-result')this.onresult(m.params);
    else if(m.method==='ui/resource-teardown'&&m.id!==undefined){this.send(this.envelope({id:m.id,result:{}}));this.close();}
    else if(m.method==='ping'&&m.id!==undefined)this.send(this.envelope({id:m.id,result:{}}));
    else if(m.id!==undefined)this.send(this.envelope({id:m.id,error:{code:-32601,message:'not_found'}}));
  } catch { this.onfailure(appError('invalid_request')); } }
}
// Integration boundary, deliberately not a launched host lane. The trusted host
// registers the public origin/frame/generation and a one-use correlation challenge.
// None is an authority credential. Host admission and provider calls remain outside.
export class EmbeddedReportAdapter extends ParentTransport {
  constructor({win=window,origin,frame,generation,challenge}) { if(!bridgeOrigin(origin,true)||typeof frame!=='string'||!/^[-A-Za-z0-9_.:]{1,128}$/.test(frame)||!Number.isSafeInteger(generation)||generation<1||typeof challenge!=='string'||!/^[-A-Za-z0-9_.:]{16,128}$/.test(challenge))throw appError('invalid_request');super(win,origin);this.frame=frame;this.generation=generation;this.challenge=challenge;this.canCall=false; }
  envelope(message) { return {protocol:'chartworks-report-app-v1',frame:this.frame,generation:this.generation,...message}; }
  async connect() { if(this.parent===this.win)throw appError('unavailable');const result=await this.request('initialize',{challenge:this.challenge});if(this.closed||result?.challenge!==this.challenge){this.close();throw appError('forbidden');}this.challenge=null;this.canCall=result.tools===true;this.ready=true;this.oncontext(result.context||{});return result; }
  receive(event) { if(this.closed||event.source!==this.parent||event.origin!==this.origin)return;try { const m=boundedJSON(event.data,wireLimit);if(m?.protocol!=='chartworks-report-app-v1'||m.frame!==this.frame||m.generation!==this.generation)return;
    if(m.id!==undefined&&(Object.hasOwn(m,'result')||Object.hasOwn(m,'error'))){this.settle(m);return;}
    if(!this.ready)return;
    if(m.method==='close'){this.close();return;}
    if(m.method==='context')this.oncontext(m.params);
    else if(m.id!==undefined)this.send(this.envelope({id:m.id,error:{code:-32601,message:'not_found'}}));
  } catch { this.onfailure(appError('invalid_request')); } }
}
