import {INVALID_REQUEST, UNAVAILABLE, BUSY} from './error-codes.js';
import {appError,copyData,validID} from './model.js';
export const TARGET_ALLOCATION_VERSION='report-app-allocation-v1';
export const TARGET_ALLOCATION_UNAVAILABLE='You cannot create reports here yet. You can still open reports and charts shared with you.';
const allocationKeys=(v,keys)=>v&&typeof v==='object'&&!Array.isArray(v)&&Object.keys(v).length===keys.length&&keys.every(k=>Object.hasOwn(v,k));
const allocationSource=v=>allocationKeys(v,['block','revision','expected_version','digest','output'])&&validID(v.block)&&validID(v.output)&&Number.isSafeInteger(v.revision)&&v.revision>0&&v.revision<=256&&Number.isSafeInteger(v.expected_version)&&v.expected_version>0&&typeof v.digest==='string'&&/^[a-f0-9]{64}$/.test(v.digest);
// The host display hint is narrower than a native chart title. Never rewrite
// the saved title, and never split a UTF-16 surrogate pair at the hint boundary.
export function allocationTitle(...values){return (values.find(v=>typeof v==='string'&&v.trim())||'Untitled chart').trim().slice(0,256).replace(/[\uD800-\uDBFF]$/u,'');}
export function allocationAvailable(adapter){return typeof adapter.allocateTarget==='function'&&adapter.supportsTargetAllocation?.()===true;}
export class TargetAllocation {
 constructor(adapter,{kind,intent,title,source},key=crypto.randomUUID()){
  if(!validID(key)||typeof title!=='string'||!title.trim()||title.length>256||!({create_report:'report',create_chart:'block',copy_chart:'block'}[intent]===kind)||(intent==='copy_chart'?!allocationSource(source):source!==undefined))throw appError(INVALID_REQUEST);
  this.adapter=adapter;this.request=Object.freeze({version:TARGET_ALLOCATION_VERSION,kind,intent,title:title.trim(),idempotency_key:key,...(source?{source:Object.freeze(copyData(source))}:{})});this.pending=false;this.unknown=false;this.closed=false;this.id='';
 }
 matches({kind,intent,source}){const a=this.request?.source;return !!this.request&&kind===this.request.kind&&intent===this.request.intent&&(!a&&!source||a&&source&&Object.keys(a).every(k=>a[k]===source[k])&&Object.keys(a).length===Object.keys(source).length);}
 async obtain(){
  if(this.closed||this.pending)throw appError(BUSY);if(!allocationAvailable(this.adapter)){const e=appError(UNAVAILABLE);e.allocationUnavailable=true;throw e;}if(this.id)return this.id;
  this.pending=true;const request=this.request;
  try{const value=await this.adapter.allocateTarget(copyData(request));if(this.closed)throw appError(UNAVAILABLE);if(!allocationKeys(value,['version','kind','intent','idempotency_key','id'])||['version','kind','intent','idempotency_key'].some(k=>value[k]!==request[k])||!validID(value.id)||value.id===request.source?.block)throw appError(UNAVAILABLE,true);this.id=value.id;this.unknown=false;return this.id;}
  catch(e){if(!this.closed){this.unknown=true;e.allocationUnknown=true;}throw e;}finally{this.pending=false;}
 }
 close(){this.closed=true;this.request=null;this.id='';this.unknown=false;}
}
