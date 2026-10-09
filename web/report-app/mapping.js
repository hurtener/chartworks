import {INVALID_REQUEST, STALE_VALIDATION, UNAVAILABLE, BUSY, CONFLICT, CANCELLED_OR_TIMED_OUT} from './error-codes.js';
import {appError, authoringTool, copyData, validID} from './model.js';
import {formattingDraft, formattingPatch, checkFormattingResult} from './formatting.js';
const mappingKinds=new Set(['area','bar','column','donut','grouped_bar','heatmap','kpi','line','pie','scatter','stacked_bar','stacked_column','table','treemap']);
const mappingHash=value=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const mappingInteger=(value,min=1,max=Number.MAX_SAFE_INTEGER)=>Number.isSafeInteger(value)&&value>=min&&value<=max;
const mappingKeys=(value,keys)=>value&&typeof value==='object'&&!Array.isArray(value)&&Object.keys(value).every(k=>keys.includes(k));
export function mappingColumns(view,output){const columns=view.output_columns?.find(item=>item.output===output)?.columns;if(!Array.isArray(columns)||!columns.length||columns.length>256||new Set(columns.map(c=>c.id)).size!==columns.length||columns.some(c=>!validID(c.id)))throw appError(INVALID_REQUEST);return columns;}
export function checkMappingView(view,block,revision,output,digest=''){
 const b=view?.block;
 if(view?.data_validation!=='not_performed'||b?.state?.id!==block||b.revision!==revision||!mappingInteger(revision,1,256)||!mappingInteger(b.state.version)||!mappingHash(b.digest)||!mappingHash(b.execution_digest)||digest&&b.digest!==digest||!Array.isArray(b.outputs)||b.outputs.length>64||new Set(b.outputs.map(o=>o.id)).size!==b.outputs.length||!Array.isArray(view.output_columns)||new Set(view.output_columns.map(o=>o.output)).size!==view.output_columns.length)throw appError(STALE_VALIDATION);
 const selected=b.outputs.find(o=>o.id===output);
 if(!selected?.editable||!selected.mapping||!mappingKinds.has(selected.mapping.kind))throw appError(INVALID_REQUEST);
 mappingColumns(view,output);return selected;
}
export function mappingDraft(mapping){const draft={kind:mapping.kind,bindings:copyData(mapping.bindings),order:copyData(mapping.order||[]),options:copyData(mapping.options)};for(const key of ['kpi','table'])if(mapping[key])draft[key]=copyData(mapping[key]);return draft;}
export function bindingCandidates(columns,kind,slot){
 const numeric=c=>['integer','decimal','number'].includes(c.type),category=c=>['text','temporal','boolean'].includes(c.type)||numeric(c)&&['dimension','identifier'].includes(c.role);
 return columns.filter(c=>slot==='columns'||['value','values','size','comparison','target'].includes(slot)&&numeric(c)||['x','y'].includes(slot)&&(kind==='scatter'?numeric(c):category(c))||['category','series','parent','hierarchy'].includes(slot)&&(slot==='category'&&['line','area'].includes(kind)?c.type==='temporal':category(c)));
}
export function mappingVariants(catalog,kind){const entry=catalog.kinds.find(e=>e.kind===kind);if(!entry)throw appError(INVALID_REQUEST);return entry.variants?.length?entry.variants:[{id:'scalar',required_slots:entry.required_slots||[],optional_slots:entry.optional_slots||[]}];}
export function mappingVariant(catalog,draft){const present=Object.keys(draft.bindings).filter(k=>Array.isArray(draft.bindings[k])?draft.bindings[k].length:!!draft.bindings[k]);return mappingVariants(catalog,draft.kind).find(v=>v.required_slots.every(k=>present.includes(k))&&present.every(k=>[...v.required_slots,...(v.optional_slots||[])].includes(k)))?.id||mappingVariants(catalog,draft.kind)[0].id;}
export function validateMappingDraft(draft,columns,catalog){
 if(!mappingKeys(draft,['kind','bindings','order','options','kpi','table'])||!mappingKinds.has(draft.kind)||!mappingKeys(draft.bindings,['category','value','series','x','y','parent','columns','values','hierarchy','size','comparison','target'])||!Array.isArray(draft.order)||!mappingKeys(draft.options,['title','legend','label_max_runes']))throw appError(INVALID_REQUEST);
 const options=draft.options,title=options.title,lower=typeof title==='string'?title.toLowerCase():'';
 if(typeof title!=='string'||new TextEncoder().encode(title).length>512||/[<>\x00-\x1f\x7f]/.test(title)||['javascript:','data:','://','file:','blob:'].some(value=>lower.includes(value))||lower.trimStart().startsWith('//')||!mappingKeys(options.legend,['visible','position'])||typeof options.legend.visible!=='boolean'||!['top','bottom','left','right'].includes(options.legend.position)||!mappingInteger(options.label_max_runes,1,1024))throw appError(INVALID_REQUEST);
 const b=draft.bindings,present=Object.keys(b).filter(k=>Array.isArray(b[k])?b[k].length:!!b[k]);
 if(!mappingVariants(catalog,draft.kind).some(v=>v.required_slots.every(k=>present.includes(k))&&present.every(k=>[...v.required_slots,...(v.optional_slots||[]),...(draft.kind==='kpi'&&draft.kpi?['category','comparison','target']:[])].includes(k))))throw appError(INVALID_REQUEST);
 const ids=[];for(const [slot,value] of Object.entries(b)){const repeated=['values','columns','hierarchy'].includes(slot);if(repeated&&!Array.isArray(value)||!repeated&&typeof value!=='string')throw appError(INVALID_REQUEST);const selected=repeated?value:value?[value]:[];const eligible=new Set(bindingCandidates(columns,draft.kind,slot).map(c=>c.id));if(selected.some(id=>!eligible.has(id)))throw appError(INVALID_REQUEST);ids.push(...selected);}
 if(!ids.length||ids.length>Math.min(256,catalog.limits?.max_columns||64)||new Set(ids).size!==ids.length||(b.values?.length||0)>(catalog.limits?.max_series||12)||(b.hierarchy?.length||0)>(catalog.max_hierarchy_depth||4))throw appError(INVALID_REQUEST);
 const ordered=new Set();for(const order of draft.order){if(!mappingKeys(order,['column','direction'])||!ids.includes(order.column)||ordered.has(order.column)||!['asc','desc'].includes(order.direction))throw appError(INVALID_REQUEST);ordered.add(order.column);}
 if(draft.kpi){const k=draft.kpi;if(draft.kind!=='kpi'||!mappingKeys(k,['value_row','comparison_mode','show_delta','show_percent_delta','show_target_difference','sparkline','thresholds'])||!['first','last'].includes(k.value_row)||!['none','previous_row','comparison_column'].includes(k.comparison_mode)||!Array.isArray(k.thresholds)||k.thresholds.length>16||(k.comparison_mode==='comparison_column')!==!!b.comparison||k.show_target_difference!==!!b.target||k.sparkline&&!b.category)throw appError(INVALID_REQUEST);}
 if(draft.table){const t=draft.table;if(draft.kind!=='table'||!mappingKeys(t,['columns','page_size','show_totals'])||!mappingInteger(t.page_size,1,1000)||!Array.isArray(t.columns)||t.columns.length!==b.columns.length||!t.columns.some(c=>c.visible)||t.columns.some((c,i)=>!mappingKeys(c,['column','visible'])||c.column!==b.columns[i]||typeof c.visible!=='boolean'))throw appError(INVALID_REQUEST);}
 if(draft.kind!=='kpi'&&(b.comparison||b.target)||draft.kind!=='kpi'&&draft.kpi||draft.kind!=='table'&&draft.table)throw appError(INVALID_REQUEST);
 return copyData(draft);
}
export function validationArguments(view,widget,page){
 const parameters=(view.block.parameters||[]).map(p=>p.name),declared=new Set(parameters),values=new Map();
 const apply=(arguments_,ignoreUnknown)=>{const seen=new Set();for(const a of arguments_||[]){if(seen.has(a.name)||!ignoreUnknown&&!declared.has(a.name))throw appError(INVALID_REQUEST);seen.add(a.name);if(declared.has(a.name))values.set(a.name,copyData(a.value));}};
 // Match native widgetArguments: report block-parameter fallback, widget
 // literals, then bound filter defaults. Filter names are not fallback keys.
 apply(page.defaults,true);apply(widget.literals,false);
 const filters=new Map();for(const filter of page.filters||[]){if(filters.has(filter.parameter.name))throw appError(INVALID_REQUEST);filters.set(filter.parameter.name,filter.parameter);}
 const bound=new Set();for(const binding of widget.bindings||[]){const filter=filters.get(binding.filter);if(!filter||!declared.has(binding.parameter)||bound.has(binding.parameter))throw appError(INVALID_REQUEST);bound.add(binding.parameter);if(filter.default!==undefined&&filter.default!==null)values.set(binding.parameter,copyData(filter.default));}
 return parameters.filter(name=>values.has(name)).map(name=>({name,value:values.get(name)}));
}
export function hasFreshMappingEvidence(view){const b=view?.block,e=b?.validation;return !!e&&e.revision===b.revision&&e.definition_digest===b.digest&&e.execution_digest===b.execution_digest&&Date.parse(e.expires_at)>Date.now();}
export class MappingSession {
 constructor(invoke,purpose='mapping'){if(!['mapping','presentation'].includes(purpose))throw appError(INVALID_REQUEST);this.purpose=purpose;this.invoke=invoke;this.generation=0;this.closed=false;this.pending=false;this.unknown=false;this.conflict=false;this.view=null;this.catalog=null;this.output='';this.draft=null;this.original=null;this.newBlock='';this.copy=true;this.operation=null;}
 async open(block,revision,output,copy=true,digest=''){
  const generation=++this.generation;this.pending=true;try{const [view,catalog]=await Promise.all([this.invoke(authoringTool('block_read'),{block,revision}),this.purpose==='mapping'?this.invoke('chart_catalog',{}):null]);if(this.closed||generation!==this.generation)return false;const selected=checkMappingView(view,block,revision,output,digest);if(this.purpose==='mapping'&&(catalog?.version!==1||!Array.isArray(catalog.kinds)||!catalog.kinds.length||catalog.kinds.length>14||catalog.kinds.some(e=>!mappingKinds.has(e.kind))))throw appError(INVALID_REQUEST);this.view=copyData(view);this.catalog=copyData(catalog);this.output=output;this.draft=this.makeDraft(selected);this.original=copyData(this.draft);if(this.purpose==='mapping')this.variant=mappingVariant(this.catalog,this.draft);this.copy=copy||!view.block.private||view.block.state.draft_revision!==revision||view.block.state.draft_state==='rejected';return true;}finally{if(generation===this.generation)this.pending=false;}
 }
 makeDraft(output){return this.purpose==='presentation'?formattingDraft(output):mappingDraft(output.mapping);}
 mutation(){return this.purpose==='presentation'?formattingPatch(this.draft,this.view.block.outputs.find(o=>o.id===this.output)):validateMappingDraft(this.draft,mappingColumns(this.view,this.output),this.catalog);}
 get dirty(){return JSON.stringify(this.draft)!==JSON.stringify(this.original);}
 edit(fn){if(this.closed||this.pending||this.unknown||this.conflict||!this.draft)throw appError(BUSY);const next=copyData(this.draft);fn(next);this.draft=copyData(next);}
 valid(){try{this.mutation();return true;}catch{return false;}}
 async save(){
  if(this.closed||this.pending||this.unknown||this.conflict||!this.dirty)throw appError(BUSY);const b=this.view.block,mutation=this.mutation();
  if(this.copy&&(!validID(this.newBlock)||this.newBlock===b.state.id))throw appError(INVALID_REQUEST);if(!this.copy&&(!b.private||b.state.draft_revision!==b.revision))throw appError(CONFLICT);
  const target=this.copy?this.newBlock:b.state.id,revision=this.copy?1:b.revision+1,args={block:b.state.id,expected_version:b.state.version,revision:b.revision,digest:b.digest,output:this.output,[this.purpose]:mutation,...(this.copy?{new_block:target}:{})},generation=this.generation;
  this.pending=true;this.operation={kind:this.copy?'copy':this.purpose,target,revision};
  try{const result=await this.invoke(authoringTool(this.copy?'block_copy':'block_mapping'),args);if(this.closed||generation!==this.generation)return null;try{checkMappingView(result,target,revision,this.output);if(this.purpose==='presentation')checkFormattingResult(this.view,result,this.output,mutation);if(!result.block.private||result.block.state.draft_revision!==revision||result.block.validation||!this.copy&&result.block.state.version<=b.state.version)throw appError(STALE_VALIDATION);}catch{throw appError(UNAVAILABLE,true);}this.view=copyData(result);this.draft=this.makeDraft(result.block.outputs.find(o=>o.id===this.output));this.original=copyData(this.draft);this.copy=false;return this.view;}
  catch(e){if(!this.closed&&generation===this.generation){this.conflict=e.code===CONFLICT;this.unknown=e.unknown===true||[UNAVAILABLE,CANCELLED_OR_TIMED_OUT].includes(e.code);}throw e;}finally{if(generation===this.generation)this.pending=false;}
 }
 async inspect(){if(!this.operation)throw appError(INVALID_REQUEST);return this.invoke(authoringTool('block_read'),{block:this.operation.target,revision:this.operation.revision});}
 close(){this.closed=true;this.generation++;this.displaySettingsElement=null;this.displaySettingsOpen=false;this.view=null;this.catalog=null;this.draft=null;this.original=null;}
}
export async function validatePrivateMapping(invoke,view,widget,page,resolution){
 const b=view.block;checkMappingView(view,widget.block.block,widget.block.revision,widget.block.outputs[0],widget.block.digest);
 if(!b.private||b.state.draft_revision!==b.revision)throw appError(CONFLICT);
 const result=await invoke(authoringTool('block_validate'),{block:b.state.id,expected_version:b.state.version,revision:b.revision,digest:b.digest,arguments:validationArguments(view,widget,page),resolution});
 const e=result?.evidence;if(result?.state?.id!==b.state.id||result.state.draft_revision!==b.revision||result.state.archived===true||!mappingInteger(result.state.version)||result.state.version<=b.state.version||e?.revision!==b.revision||e.definition_digest!==b.digest||e.execution_digest!==b.execution_digest||!validID(e.id)||!mappingHash(e.schema_digest)||!Array.isArray(e.schema)||!Number.isFinite(Date.parse(e.created_at))||(!Number.isFinite(Date.parse(e.expires_at))||Date.parse(e.expires_at)<=Date.now()))throw appError(UNAVAILABLE,true);
 const next=copyData(view);next.block.state=copyData(result.state);next.block.validation=copyData(e);return next;
}
