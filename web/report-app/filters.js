import {INVALID_REQUEST, STALE_VALIDATION, BUSY, LIMIT_EXCEEDED} from './error-codes.js';
import {appError,authoringTool,copyData} from './model.js';

const filterDayMS=86400000;
function filterCivilDay(value){
 if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}$/.test(value)||value<'0001-01-01'||value>'9999-12-31')throw appError(INVALID_REQUEST);
 const time=Date.parse(value+'T00:00:00.000Z');if(!Number.isFinite(time)||new Date(time).toISOString().slice(0,10)!==value)throw appError(INVALID_REQUEST);return time;
}
// Date controls show inclusive civil dates. Wire bounds are half-open and never
// pass through the browser's local time zone or daylight-saving arithmetic.
export function inclusiveFilterRange(start,end){
 const a=filterCivilDay(start),b=filterCivilDay(end),next=b+filterDayMS;
 if(a>b||next-a>36625*filterDayMS||end==='9999-12-31')throw appError(INVALID_REQUEST);
 return {date_range:{start,end_exclusive:new Date(next).toISOString().slice(0,10)}};
}
export function displayFilterRange(value){
 const r=value?.date_range,a=filterCivilDay(r?.start),b=filterCivilDay(r?.end_exclusive);
 if(a>=b||b-a>36625*filterDayMS)throw appError(INVALID_REQUEST);return {start:r.start,end:new Date(b-filterDayMS).toISOString().slice(0,10)};
}
export function selectionFilterValue(items,multiple){
 if(!Array.isArray(items)||items.length<1||items.length>(multiple?16:1)||new Set(items).size!==items.length||items.some(x=>typeof x!=='string'||x.includes('\0')||new TextEncoder().encode(x).length>4096))throw appError(INVALID_REQUEST);
 return multiple?{items:[...items]}:{literal:items[0]};
}
// Staged input never aliases a saved default or a temporary run selection.
export class FilterValueDraft{
 constructor(value){this.original=copyData(value??null);this.value=copyData(value??null);}
 select(items,multiple){this.value=selectionFilterValue(items,multiple);}
 dates(start,end){this.value=inclusiveFilterRange(start,end);}
 cancel(){this.value=copyData(this.original);return copyData(this.original);}
 commit(){this.original=copyData(this.value);return copyData(this.value);}
}
function checkOptionResult(result,operation,digest=''){
 if(new TextEncoder().encode(JSON.stringify(result)).length>512<<10)throw appError(LIMIT_EXCEEDED,true);
 if(result?.operation!==operation||typeof result.input_digest!=='string'||!/^[a-f0-9]{64}$/.test(result.input_digest)||digest&&result.input_digest!==digest||!['accepted','completed','failed','uncertain','unsupported'].includes(result.status)||typeof result.values_available!=='boolean'||typeof result.new_operation_allowed!=='boolean'||typeof result.complete!=='boolean'||!Array.isArray(result.options)||result.options.length>199||result.next!==undefined&&(typeof result.next!=='string'||result.next.length>16384))throw appError(STALE_VALIDATION,true);
 if(result.values_available&&(result.status!=='completed'||!result.new_operation_allowed)||!result.values_available&&(result.options.length||result.next||result.complete))throw appError(STALE_VALIDATION,true);
 const seen=new Set();for(const option of result.options){if(typeof option?.value!=='string'||typeof option.label!=='string'||seen.has(option.value)||option.value.includes('\0')||new TextEncoder().encode(option.value).length>4096||option.label.length>4096)throw appError(STALE_VALIDATION,true);seen.add(option.value);}
 if(result.values_available&&!result.complete&&!result.next)throw appError(STALE_VALIDATION,true);return copyData(result);
}
export function newOptionOperation(){return `option:${Math.floor(Date.now()/1000)}:${crypto.randomUUID().replaceAll('-','')}`;}
// One disposable lookup controller belongs to one exact target. Search is always
// explicit. A missing response must be inspected, never replayed as a new query.
export class FilterOptionLookup{
 constructor(invoke,target,{operation=newOptionOperation,locale='en-US',validateValue=null}={}){this.invoke=invoke;this.validateValue=validateValue;this.target=copyData(target);this.operation=operation;this.locale=locale;this.pending=false;this.closed=false;this.epoch=0;this.request=null;this.result=null;this.unknown=false;this.values=[];}
 canSearch(){return !this.closed&&!this.pending&&(!this.request||this.result?.new_operation_allowed===true);}
 async search(search='',cursor=''){
  if(!this.canSearch()||typeof search!=='string'||new TextEncoder().encode(search).length>256||search.includes('\0')||typeof cursor!=='string')throw appError(BUSY);
  if(cursor&&(!this.result?.values_available||cursor!==this.result.next||search!==this.request.search))throw appError(STALE_VALIDATION);
  const key=this.operation();if(!/^option:[1-9][0-9]*:[a-f0-9]{32}$/.test(key)||key===this.request?.operation)throw appError(INVALID_REQUEST);
  const request={target:copyData(this.target),operation:key,search,cursor,limit:199,locale:this.locale};this.request=copyData(request);this.result=null;this.values=[];this.unknown=true;this.pending=true;const epoch=++this.epoch;
  try{const result=checkOptionResult(await this.invoke(authoringTool(this.target.dataset?'dataset_options':'report_options'),request),key);if(this.closed||epoch!==this.epoch)return false;for(const option of result.options)this.validateValue?.(option.value);this.result=result;this.values=copyData(result.options);this.unknown=!result.values_available&&!result.new_operation_allowed&&result.status!=='unsupported';return true;}
  finally{if(!this.closed&&epoch===this.epoch)this.pending=false;}
 }
 async inspect(action=''){
  if(this.closed||this.pending||!this.request||!['','cancel','reconcile'].includes(action))throw appError(BUSY);
  const epoch=++this.epoch;this.pending=true;const ref={target:copyData(this.target),operation:this.request.operation};
  try{const result=checkOptionResult(await this.invoke(authoringTool(action?'option_control':'option_status'),action?{...ref,action}:ref),ref.operation,this.result?.input_digest||'');if(this.closed||epoch!==this.epoch)return false;if(result.values_available)throw appError(STALE_VALIDATION,true);this.result=result;this.values=[];this.unknown=!result.new_operation_allowed;return true;}
  finally{if(!this.closed&&epoch===this.epoch)this.pending=false;}
 }
 close(){this.closed=true;this.epoch++;this.pending=false;this.values=[];this.result=null;this.request=null;this.target=null;}
}
