import {appError,copyData,validID} from './model.js';
import {INVALID_REQUEST,STALE_VALIDATION} from './error-codes.js';
import {sourceDatasetPin} from './source-catalog.js';

export const FIELD_COMPILER='typed-dataset-postgres-v3';
const aggregations=['count','distinct_count','sum','average','minimum','maximum'];
const grains=['minute','hour','day','week','month','quarter','year'];
const unique=value=>new Set(value).size===value.length;
const invalid=()=>{throw appError(INVALID_REQUEST);};
export const fieldTemporal=column=>['date','timestamp','instant'].includes(column?.type);
export const fieldInstant=column=>column?.type==='instant';

export function checkFieldCatalog(view){
 const f=view.fields;if(f===undefined)return;
 if(f?.compiler!==FIELD_COMPILER||typeof f.supported!=='boolean'||f.supported&&(view.dialect!=='postgres'||f.reason)||!Number.isSafeInteger(f.max_columns)||f.max_columns<1||f.max_columns>256||!Array.isArray(f.columns)||f.columns.length>256||!Array.isArray(f.dimensions)||f.dimensions.length>512)throw appError(STALE_VALIDATION);
 const ids=new Set();
 for(const c of f.columns){
  if(!validID(c?.id)||ids.has(c.id)||typeof c.name!=='string'||typeof c.source_name!=='string'||typeof c.native_type!=='string'||typeof c.category!=='string'||typeof c.nullable!=='boolean'||typeof c.supported!=='boolean'||!Array.isArray(c.aggregations)||!unique(c.aggregations)||c.aggregations.some(a=>!aggregations.includes(a))||!Array.isArray(c.grains)||!unique(c.grains)||c.grains.some(g=>!grains.includes(g))||c.supported&&c.reason||!c.supported&&(c.aggregations.length||c.grains.length)||c.grains.length&&!fieldTemporal(c))throw appError(STALE_VALIDATION);
  if(c.supported&&!['number','text','boolean','date','timestamp','instant','identifier'].includes(c.type))throw appError(STALE_VALIDATION);
  ids.add(c.id);
 }
 const dimensions=new Set();
 for(const d of f.dimensions){
  if(!validID(d?.id)||dimensions.has(d.id)||typeof d.name!=='string'||!validID(d.column)||typeof d.supported!=='boolean'||d.supported&&(!ids.has(d.column)||d.reason)||d.temporal&&(!Array.isArray(d.temporal.grains)||d.temporal.grains.some(g=>!grains.includes(g))||typeof d.temporal.calendar!=='string'||d.temporal.timezone!==undefined&&typeof d.temporal.timezone!=='string'))throw appError(STALE_VALIDATION);
  dimensions.add(d.id);
 }
}

export function groupingMetadata(view,selection){
 const dimension=selection.kind==='dimension'?view.fields.dimensions.find(d=>d.id===selection.field):null;
 const column=view.fields.columns.find(c=>c.id===(dimension?.column||selection.field));
 return {column,dimension,label:dimension?.name||column?.name||selection.field};
}

export function fieldSelectionIssue(view,draft){
 const f=draft.fields;if(!f)return '';
 if(!view.fields?.supported)return 'This dataset is unavailable for field selection.';
 if(!f.dimensions.length&&!f.measures.length)return 'Add fields or a row count to define the result.';
 if(f.dimensions.length+f.measures.length>view.fields.max_columns)return `This source accepts up to ${view.fields.max_columns} result columns. Remove a field to continue.`;
 if(f.mode==='rows'&&(draft.kind!=='table'||f.measures.length))return 'Raw rows use a table with columns and no aggregates.';
 for(const m of f.measures)if(m.kind==='column'&&!m.aggregation)return `Choose an aggregation for ${view.fields.columns.find(c=>c.id===m.field)?.name||m.field}.`;
 for(const d of f.dimensions){const {column,dimension,label}=groupingMetadata(view,d);if(dimension?.temporal&&!d.grain)return `Choose the reviewed date grain for ${label}, or use its physical column for original values.`;if(d.grain&&fieldInstant(column)&&!d.timezone)return `Choose a timezone for ${label}.`;}
 if(draft.kind==='table')return '';
 if(draft.kind==='kpi')return f.dimensions.length||f.measures.length!==1?'A single-value card needs one measure and no grouping. Use a table or chart to show more fields.':'';
 if(!f.measures.length||!f.dimensions.length)return 'Choose a grouping field and at least one measure for this chart.';
 if(['pie','donut'].includes(draft.kind)&& (f.dimensions.length!==1||f.measures.length!==1))return 'This chart uses one category and one measure. A table or bar chart can show more fields.';
 if(f.dimensions.length>2)return 'This chart has category and series slots. Use a table to display all selected grouping fields.';
 if(['line','area'].includes(draft.kind)&&!fieldTemporal(groupingMetadata(view,f.dimensions[0]).column))return 'The first field of a line or area chart must have a date or timestamp type.';
 return '';
}

export function typedFieldIntent(view,draft){
 checkFieldCatalog(view);
 const f=draft.fields;
 if(!f||!['aggregate','rows'].includes(f.mode)||!Array.isArray(f.dimensions)||!Array.isArray(f.measures)||!unique(f.dimensions.map(d=>JSON.stringify(d)))||!unique(f.measures.map(m=>JSON.stringify(m)))||fieldSelectionIssue(view,draft)||typeof draft.title!=='string'||!draft.title.trim()||draft.title.length>256||!view.chart_kinds.includes(draft.kind))invalid();
 for(const d of f.dimensions){
  if(!['column','dimension'].includes(d.kind)||!validID(d.field)||Object.keys(d).some(k=>!['kind','field','grain','calendar','timezone'].includes(k)))invalid();
  const {column,dimension}=groupingMetadata(view,d);
  if(!column?.supported||d.kind==='dimension'&&!dimension?.supported)invalid();
  if(d.grain){
   if(f.mode==='rows'||d.calendar!=='gregorian'||!column.grains.includes(d.grain)||fieldInstant(column)&&(!d.timezone||d.timezone==='Local')||!fieldInstant(column)&&d.timezone)invalid();
   if(d.timezone){try{new Intl.DateTimeFormat('en',{timeZone:d.timezone});}catch{invalid();}}
   if(dimension?.temporal&&(d.calendar!==dimension.temporal.calendar||d.timezone!==(dimension.temporal.timezone||undefined)||!dimension.temporal.grains.includes(d.grain)))invalid();
  }else if(d.calendar||d.timezone||dimension?.temporal)invalid();
 }
 for(const m of f.measures){
  if(Object.keys(m).some(k=>!['kind','field','aggregation'].includes(k)))invalid();
  if(m.kind==='count'){if(m.field||m.aggregation)invalid();}
  else if(m.kind==='measure'){if(m.aggregation||!view.measures.some(v=>v.id===m.field&&v.supported))invalid();}
  else if(m.kind==='column'){if(!view.fields.columns.some(c=>c.id===m.field&&c.supported&&c.aggregations.includes(m.aggregation)))invalid();}
  else invalid();
 }
 const dimensions=f.dimensions.map((_,i)=>`group_${i+1}`),measures=f.measures.map((_,i)=>`value_${i+1}`),kind=draft.kind;
 const bindings=kind==='table'?{columns:[...dimensions,...measures]}:kind==='kpi'?{value:measures[0]}:{category:dimensions[0],...(dimensions[1]?{series:dimensions[1]}:{}),...(measures.length===1?{value:measures[0]}:{values:measures})};
 const mapping={kind,bindings,order:['line','area'].includes(kind)?[{column:dimensions[0],direction:'asc'}]:[],options:{title:draft.title.trim(),legend:{visible:true,position:'bottom'},label_max_runes:80}};
 if(kind==='table'){
  if(!Number.isSafeInteger(draft.pageSize)||draft.pageSize<1||draft.pageSize>1000)invalid();
  mapping.table={columns:bindings.columns.map(column=>({column,visible:true})),page_size:draft.pageSize,show_totals:false};
 }
 return {...(view.source_dataset?{source_dataset:sourceDatasetPin(view.source_dataset)}:{topic:copyData(view.topic)}),dataset:view.dataset,dimensions:[],measure:'',fields:copyData(f),mapping};
}
