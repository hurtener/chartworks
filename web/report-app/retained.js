import {INVALID_REQUEST, STALE_VALIDATION, LIMIT_EXCEEDED} from './error-codes.js';
import {boundedJSON, validateRetainedView} from '../report-viewer/presentation.js';
import {reportPages} from './pages.js';
import {appError, copyData, validID} from './model.js';

export const CANVAS_MAX_OUTPUTS=100;
const CANVAS_MAX_BYTES=16<<20;
const cloneRetained=value=>{boundedJSON(value,4<<20);return JSON.parse(JSON.stringify(value));};
// Query widgets are read-only here: the delivery contract exposes only result.
export const retainedOutputs=widget=>widget.kind==='text'?['']:widget.kind==='query'?['result']:widget.kind==='block'?(widget.outputs||widget.block?.outputs||[]):[];
export const retainedKey=(page,widget,output='')=>JSON.stringify([page,widget,output]);
const sameTarget=(a,b)=>a?.kind===b?.kind&&a?.id===b?.id&&a?.revision===b?.revision;
const identity=v=>JSON.stringify((v.pages||[]).map(p=>({id:p.id,report:p.report,revision:p.revision,widgets:(p.widgets||[]).map(w=>({id:w.id,kind:w.kind,grid:w.grid,presentation:w.presentation,outputs:w.outputs}))})));

// This conservative fingerprint removes only documented presentation fields.
// It is tied to an immutable preview admission snapshot, never a saved baseline.
export function executionFingerprint(definition) {
  const d=copyData(definition);delete d.metadata;
  for(const p of reportPages(d)){if(d.schema_version===3)delete p.title;for(const w of p.widgets){delete w.grid;delete w.presentation;if(w.kind==='text')delete w.text;}p.widgets.sort((a,b)=>a.id.localeCompare(b.id));}
  d.report_pages?.sort((a,b)=>a.id.localeCompare(b.id));
  return JSON.stringify(d);
}

function selections(view) {
  const result=[],pages=new Set(),widgets=new Set();
  for(const page of view.pages||[]){
    if(!validID(page.id)||pages.has(page.id)||page.report!==view.summary.target.id||page.revision!==view.summary.target.revision||!Array.isArray(page.widgets)||page.widgets.length>100)throw appError(INVALID_REQUEST);pages.add(page.id);
    for(const widget of page.widgets){
      const g=widget.grid;
      if(!validID(widget.id)||widgets.has(widget.id)||!g||!['row','column','width','height'].every(k=>Number.isSafeInteger(g[k]))||g.row<0||g.row+g.height>10000||g.column<0||g.width<1||g.column+g.width>12||g.height<1||g.height>100)throw appError(INVALID_REQUEST);widgets.add(widget.id);if(widgets.size>100)throw appError(LIMIT_EXCEEDED);
      const outputs=retainedOutputs(widget);
      if(!Array.isArray(outputs)||outputs.length>64||new Set(outputs).size!==outputs.length||outputs.some(o=>widget.kind==='text'?o!=='':!validID(o)))throw appError(INVALID_REQUEST);
      for(const output of outputs)result.push({kind:'report',run:view.summary.run,page:page.id,widget:widget.id,output,offset:0,limit:100});
      if(result.length>CANVAS_MAX_OUTPUTS)throw appError(LIMIT_EXCEEDED);
    }
  }
  return result;
}

// Delivery can return a widget-level failure before choosing a default output.
// This is verified status for an authorized widget, never an output payload.
function widgetFailure(view) {
  const selected=view.selection,output=view.output;
  const widget=view.pages?.find(p=>p.id===selected?.page)?.widgets?.find(w=>w.id===selected?.widget);
  return !!widget&&['block','query'].includes(widget.kind)&&['failed','partial'].includes(widget.state)&&
    output?.id===widget.id&&output.kind===widget.kind&&output.state===widget.state&&
    (output.code||'')===(widget.code||'')&&!view.text&&!output.retained_digest&&
    view.page_bounds.offset===0&&view.page_bounds.total===0&&view.page_bounds.limit===selected.limit&&!Object.hasOwn(view.page_bounds,'next')&&
    Object.keys(output).every(key=>['id','kind','state','code','retained_digest'].includes(key));
}

// A real inline page may be empty. This is authorized page metadata, never an
// output placeholder or permission to accept an unresolved nonempty widget.
function emptyPageRoot(view) {
  const s=view.selection,p=view.pages?.find(page=>page.id===s?.page),b=view.page_bounds;
  return !!p&&p.report===view.summary.target.id&&p.revision===view.summary.target.revision&&
    Array.isArray(p.widgets)&&p.widgets.length===0&&s.widget===''&&s.output===''&&s.offset===0&&s.limit===100&&
    b.offset===0&&b.limit===100&&b.total===0&&!Object.hasOwn(b,'next')&&
    !view.text&&!view.output&&!view.chart&&!view.table&&!view.narrative&&Array.isArray(view.outputs)&&view.outputs.length===0;
}

// Disposable, generation-bound retained values. Only reporting_view is used;
// neither layout redraw nor per-output paging can submit a new execution.
export class RetainedReport {
  constructor(invoke,onExpired=()=>{}){this.invoke=invoke;this.onExpired=onExpired;this.generation=0;this.value=null;this.entries=new Map();this.pending=new Set();this.timer=null;this.bytes=0;this.closed=false;}
  clear(){this.generation++;clearTimeout(this.timer);this.timer=null;this.value=null;this.entries.clear();this.pending.clear();this.bytes=0;}
  close(){this.clear();this.closed=true;}
  get(page,widget,output=''){return this.entries.get(retainedKey(page,widget,output))?.view;}
  check(v,request=null){
    const expiry=validateRetainedView(v);
    if(v.summary.kind!=='report'||v.summary.state==='expired'||expiry<=Date.now())throw appError(v.summary.state==='expired'||expiry<=Date.now()?'expired':INVALID_REQUEST);
    if(this.value&&(!sameTarget(v.summary.target,this.value.summary.target)||v.summary.run!==this.value.summary.run||v.summary.private!==this.value.summary.private||v.summary.expires_at!==this.value.summary.expires_at||v.redacted!==this.value.redacted||identity(v)!==identity(this.value)))throw appError(STALE_VALIDATION);
    if(request&&['kind','run','page','widget','output','offset','limit'].some(key=>v.selection[key]!==request[key]))throw appError(STALE_VALIDATION);
    if(request&&v.page_bounds.offset!==request.offset)throw appError(STALE_VALIDATION);
    if(request&&(v.text&&request.output!==''||request.output===''&&v.output?.state==='succeeded'))throw appError(STALE_VALIDATION);
    if(request&&['block','query'].includes(v.output?.kind)&&!widgetFailure(v))throw appError(STALE_VALIDATION);
    if(request?.output&&v.output?.state==='succeeded'&&(v.output.id!==request.output||!v.outputs?.some(o=>o.id===request.output&&o.enabled!==false&&o.selected!==false)))throw appError(STALE_VALIDATION);
    return expiry;
  }
  keep(view){
    const key=retainedKey(view.selection.page,view.selection.widget,view.selection.output),bytes=new TextEncoder().encode(JSON.stringify(view)).length,next=this.bytes-(this.entries.get(key)?.bytes||0)+bytes;
    if(next>CANVAS_MAX_BYTES)throw appError(LIMIT_EXCEEDED);
    this.entries.set(key,{view,bytes});this.bytes=next;
  }
  async load(value){
    this.clear();if(this.closed)return false;
    const generation=this.generation;
    try{
      const expiry=this.check(value),requests=selections(value);this.value=cloneRetained(value);
      const armExpiry=()=>{if(this.closed||generation!==this.generation)return;const remaining=expiry-Date.now();if(remaining>0){this.timer=setTimeout(armExpiry,Math.min(2147483647,remaining));this.timer.unref?.();return;}this.clear();this.onExpired();};armExpiry();
      if(!['completed','partial'].includes(value.summary.state))return true;
      const selected=requests.find(r=>['kind','run','page','widget','output','offset'].every(key=>r[key]===value.selection[key]));
      const initialFailure=!selected&&value.selection.output===''&&widgetFailure(value);
      const initialEmpty=!selected&&emptyPageRoot(value);
      if(initialFailure)this.check(value,{kind:'report',run:value.summary.run,page:value.selection.page,widget:value.selection.widget,output:'',offset:0,limit:100});
      if((value.pages?.length||value.text||value.output)&&!selected&&!initialFailure&&!initialEmpty)throw appError(STALE_VALIDATION);
      if(selected){this.check(value,selected);this.keep(this.value);requests.splice(requests.indexOf(selected),1);}
      let cursor=0;
      const read=async()=>{while(cursor<requests.length&&!this.closed&&generation===this.generation){const request=requests[cursor++],view=await this.invoke('reporting_view',request);if(this.closed||generation!==this.generation)return;this.check(view,request);this.keep(cloneRetained(view));}};
      await Promise.all(Array.from({length:Math.min(4,requests.length)},()=>read()));
      return !this.closed&&generation===this.generation;
    }catch(e){if(generation!==this.generation||this.closed)return false;this.clear();throw e;}
  }
  async page(page,widget,output,offset){
    const key=retainedKey(page,widget,output),entry=this.entries.get(key);
    if(this.closed||!this.value||!entry||this.pending.has(key))return false;
    const bounds=entry.view.page_bounds;
    if(!entry.view.output?.table||!Number.isSafeInteger(offset)||offset<0||offset>=bounds.total)throw appError(INVALID_REQUEST);
    const generation=this.generation,request={...entry.view.selection,offset,limit:bounds.limit};this.pending.add(key);
    try{const view=await this.invoke('reporting_view',request);if(this.closed||generation!==this.generation)return false;this.check(view,request);if(view.output?.retained_digest!==entry.view.output?.retained_digest)throw appError(STALE_VALIDATION);this.keep(cloneRetained(view));return true;}
    catch(e){if(this.closed||generation!==this.generation)return false;this.clear();throw e;}
    finally{this.pending.delete(key);}
  }
}
