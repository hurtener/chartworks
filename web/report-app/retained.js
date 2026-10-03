import {boundedJSON, validateRetainedView} from '../report-viewer/app.js';
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
  for(const w of d.widgets||[]){delete w.grid;delete w.presentation;if(w.kind==='text')delete w.text;}
  d.widgets?.sort((a,b)=>a.id.localeCompare(b.id));
  return JSON.stringify(d);
}

function selections(view) {
  const result=[],pages=new Set();
  for(const page of view.pages||[]){
    if(!validID(page.id)||pages.has(page.id)||!Array.isArray(page.widgets)||page.widgets.length>100)throw appError('invalid_request');pages.add(page.id);
    const widgets=new Set();
    for(const widget of page.widgets){
      const g=widget.grid;
      if(!validID(widget.id)||widgets.has(widget.id)||!g||!['row','column','width','height'].every(k=>Number.isSafeInteger(g[k]))||g.row<0||g.row+g.height>10000||g.column<0||g.width<1||g.column+g.width>12||g.height<1||g.height>100)throw appError('invalid_request');widgets.add(widget.id);
      const outputs=retainedOutputs(widget);
      if(!Array.isArray(outputs)||outputs.length>64||new Set(outputs).size!==outputs.length||outputs.some(o=>widget.kind==='text'?o!=='':!validID(o)))throw appError('invalid_request');
      for(const output of outputs)result.push({kind:'report',run:view.summary.run,page:page.id,widget:widget.id,output,offset:0,limit:100});
      if(result.length>CANVAS_MAX_OUTPUTS)throw appError('limit_exceeded');
    }
  }
  return result;
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
    if(v.summary.kind!=='report'||v.summary.state==='expired'||expiry<=Date.now())throw appError(v.summary.state==='expired'||expiry<=Date.now()?'expired':'invalid_request');
    if(this.value&&(!sameTarget(v.summary.target,this.value.summary.target)||v.summary.run!==this.value.summary.run||v.summary.private!==this.value.summary.private||v.summary.expires_at!==this.value.summary.expires_at||v.redacted!==this.value.redacted||identity(v)!==identity(this.value)))throw appError('stale_validation');
    if(request&&['kind','run','page','widget','output','offset','limit'].some(key=>v.selection[key]!==request[key]))throw appError('stale_validation');
    if(request&&v.page_bounds.offset!==request.offset)throw appError('stale_validation');
    if(request&&(v.text&&request.output!==''||request.output===''&&v.output?.state==='succeeded'))throw appError('stale_validation');
    if(request?.output&&v.output?.state==='succeeded'&&(v.output.id!==request.output||!v.outputs?.some(o=>o.id===request.output&&o.enabled!==false&&o.selected!==false)))throw appError('stale_validation');
    return expiry;
  }
  keep(view){
    const key=retainedKey(view.selection.page,view.selection.widget,view.selection.output),bytes=new TextEncoder().encode(JSON.stringify(view)).length,next=this.bytes-(this.entries.get(key)?.bytes||0)+bytes;
    if(next>CANVAS_MAX_BYTES)throw appError('limit_exceeded');
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
      if((requests.length||value.text||value.output)&&!selected)throw appError('stale_validation');
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
    if(!entry.view.output?.table||!Number.isSafeInteger(offset)||offset<0||offset>=bounds.total)throw appError('invalid_request');
    const generation=this.generation,request={...entry.view.selection,offset,limit:bounds.limit};this.pending.add(key);
    try{const view=await this.invoke('reporting_view',request);if(this.closed||generation!==this.generation)return false;this.check(view,request);if(view.output?.retained_digest!==entry.view.output?.retained_digest)throw appError('stale_validation');this.keep(cloneRetained(view));return true;}
    catch(e){if(this.closed||generation!==this.generation)return false;this.clear();throw e;}
    finally{this.pending.delete(key);}
  }
}
