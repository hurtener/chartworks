import {INVALID_REQUEST,STALE_VALIDATION} from './error-codes.js';
import {appError,copyData} from './model.js';
import {selectionFilterValue} from './filters.js';

const temporal=new Set(['date','timestamp','instant']);
const types=new Set(['text','identifier','integer','number','boolean',...temporal]);
const invalid=()=>{throw appError(INVALID_REQUEST);};
const keys=(v,expected)=>v&&typeof v==='object'&&!Array.isArray(v)&&Object.keys(v).length===expected.length&&expected.every(k=>Object.hasOwn(v,k));
export const columnFilterKey=id=>JSON.stringify(['column',id]);
export const datasetFilterKey=f=>f?.column?columnFilterKey(f.column):f?.dimension;
export function columnFromFilterKey(key){try{const value=JSON.parse(key);return Array.isArray(value)&&value.length===2&&value[0]==='column'&&typeof value[1]==='string'?value[1]:null;}catch{return null;}}

export function checkColumnFilterCapability(column){
 const c=column.filters;if(c===undefined)return;
 const expected=temporal.has(c?.type)?['range']:['integer','number'].includes(c?.type)?['select','multi_select','range']:['select','multi_select'];
 if(!c||typeof c!=='object'||Object.keys(c).some(k=>!['type','kinds','max_set_size','calendar_required','timezone_required','option_lookup'].includes(k))||!column.supported||!types.has(c.type)||(c.type==='integer'?'number':c.type)!==column.type||JSON.stringify(c.kinds)!==JSON.stringify(expected)||c.option_lookup!==false&&c.option_lookup!==true||temporal.has(c.type)&&(c.calendar_required!==true||c.max_set_size!==undefined||c.option_lookup)||!temporal.has(c.type)&&(c.max_set_size!==16||c.calendar_required!==undefined)||c.type==='instant'&&c.timezone_required!==true||c.type!=='instant'&&c.timezone_required!==undefined)throw appError(STALE_VALIDATION);
}

export function columnFilterPolicy(column){
 if(!column||!types.has(column.type))invalid();
 if(temporal.has(column.type)){
  if(column.calendar!=='gregorian')invalid();
  if(column.type==='instant'){
   if(typeof column.timezone!=='string'||!column.timezone||column.timezone.length>128||column.timezone==='Local'||column.timezone.includes('..')||column.timezone.startsWith('/')||/[\\\0\r\n]/.test(column.timezone))invalid();
   try{new Intl.DateTimeFormat('en',{timeZone:column.timezone});}catch{invalid();}
  }else if(column.timezone)invalid();
 }else if(column.calendar||column.timezone)invalid();
 return column;
}

function civilDate(value){
 if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}$/.test(value)||value<'0001-01-01'||value>'9999-12-31')invalid();
 const parsed=new Date(value+'T00:00:00Z');if(!Number.isFinite(parsed.getTime())||parsed.toISOString().slice(0,10)!==value)invalid();return value;
}
export function civilTimestamp(value){
 if(typeof value!=='string')invalid();const m=/^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,6}))?)?$/.exec(value);
 if(!m||Number(m[2])>23||Number(m[3])>59||Number(m[4]||0)>59)invalid();civilDate(m[1]);
 const fraction=(m[5]||'').replace(/0+$/,'');return `${m[1]}T${m[2]}:${m[3]}:${m[4]||'00'}${fraction?'.'+fraction:''}`;
}
function decimalParts(value){
 if(typeof value!=='string'||value.length>256||!Number.isFinite(Number(value)))invalid();
 const m=/^([+-]?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(value);if(!m)invalid();
 const exponent=Number(m[4]||0)-(m[3]||'').length;if(!Number.isSafeInteger(exponent)||Math.abs(exponent)>10000)invalid();
 return {n:BigInt((m[1]==='-'?'-':'')+m[2]+(m[3]||'')),e:exponent};
}
function decimalCompare(a,b){const x=decimalParts(a),y=decimalParts(b),e=Math.min(x.e,y.e);const n=x.n*10n**BigInt(x.e-e),m=y.n*10n**BigInt(y.e-e);return n<m?-1:n>m?1:0;}

export function columnScalar(type,value){
 selectionFilterValue([value],false);
 if(/[\u0000-\u0008\u000b-\u001f\u007f]/.test(value))invalid();
 if(type==='integer'){
  if(!/^-?(0|[1-9]\d*)$/.test(value)||value==='-0')invalid();const n=BigInt(value);if(n<-(1n<<63n)||n>(1n<<63n)-1n)invalid();
 }else if(type==='number')decimalParts(value);
 else if(type==='boolean'){if(!['true','false'].includes(value))invalid();}
 else if(type==='identifier'){if(!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value))invalid();}
 else if(type==='date')civilDate(value);
 else if(type==='timestamp'||type==='instant'){if(civilTimestamp(value)!==value)invalid();}
 else if(type!=='text')invalid();return value;
}

// Local input checks improve authoring feedback. Native typed binding, source
// identity and timezone/DST checks remain the execution/authority boundary.
export function columnFilterValue(parameter,value){
 const c=columnFilterPolicy(parameter.column);
 if(parameter.type==='column_value'){
  if(temporal.has(c.type)||!keys(value,['literal']))invalid();columnScalar(c.type,value.literal);return copyData(value);
 }
 if(parameter.type==='column_set'){
  if(temporal.has(c.type)||!keys(value,['items']))invalid();const out=selectionFilterValue(value.items,true);for(const v of out.items)columnScalar(c.type,v);return out;
 }
 if(parameter.type==='column_range'){
  if(!temporal.has(c.type)&&!['integer','number'].includes(c.type)||!keys(value,['range'])||!keys(value.range,['start','end_exclusive']))invalid();
  const {start,end_exclusive:end}=value.range;columnScalar(c.type,start);columnScalar(c.type,end);
  if(['integer','number'].includes(c.type)?decimalCompare(start,end)>=0:start>=end)invalid();return copyData(value);
 }
 invalid();
}

export function columnInputState(parameter,value){
 const state={type:parameter.type,column:copyData(parameter.column),items:[...(value?.items||[])],literal:value?.literal??'',start:'',end:'',query:''};
 if(value?.range){if(state.column.type==='date')Object.assign(state,displayColumnDateRange(value.range));else{state.start=value.range.start;state.end=value.range.end_exclusive;}}
 return state;
}
export function columnInputValue(state){
 let value;if(state.type==='column_value')value={literal:state.literal};
 else if(state.type==='column_set')value={items:[...state.items]};
 else if(state.type==='column_range'){
  if(state.column.type==='date')value={range:columnDateRange(state.start,state.end)};
  else value={range:{start:temporal.has(state.column.type)?civilTimestamp(state.start):state.start,end_exclusive:temporal.has(state.column.type)?civilTimestamp(state.end):state.end}};
 }else invalid();return columnFilterValue(state,value);
}

function columnDateRange(start,end){
 civilDate(start);civilDate(end);if(start>end||end==='9999-12-31')invalid();
 return {start,end_exclusive:new Date(Date.parse(end+'T00:00:00Z')+86400000).toISOString().slice(0,10)};
}
function displayColumnDateRange(range){
 civilDate(range.start);civilDate(range.end_exclusive);if(range.start>=range.end_exclusive)invalid();
 return {start:range.start,end:new Date(Date.parse(range.end_exclusive+'T00:00:00Z')-86400000).toISOString().slice(0,10)};
}
