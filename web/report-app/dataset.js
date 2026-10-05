import {INVALID_REQUEST, STALE_VALIDATION, UNAVAILABLE, BUSY} from './error-codes.js';
import {appError,authoringTool,copyData,validID} from './model.js';
import {checkMappingView,mappingDraft} from './mapping.js';
import {FilterOptionLookup,displayFilterRange,inclusiveFilterRange,selectionFilterValue} from './filters.js';
import {filterInputState} from './filter-controls.js';

export const DATASET_CHART_KINDS=['bar','column','line','area','pie','donut','kpi','table'];
export const DATASET_FILTER_KINDS=['select','multi_select','date_range'];
const datasetFilterCompiler='reviewed-dataset-postgres-v2';
const datasetFilterTypes={select:'dimension_value',multi_select:'dimension_set',date_range:'date_range'};
const datasetKeys=(value,keys)=>value&&typeof value==='object'&&!Array.isArray(value)&&Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
export function datasetFilterCapability(view,dimension){return view?.filter_compiler===datasetFilterCompiler?view.filter_capabilities?.find(c=>c.dimension===dimension):undefined;}
function checkDatasetFilterMetadata(view){
 if(view.filter_compiler===undefined&&view.filter_capabilities===undefined)return;
 // Go omitempty omits the empty capability list for measure-only datasets.
 if(view.filter_compiler===datasetFilterCompiler&&view.filter_capabilities===undefined&&!view.dimensions.length)return;
 if(view.filter_compiler!==datasetFilterCompiler||!Array.isArray(view.filter_capabilities)||view.filter_capabilities.length>view.dimensions.length)throw appError(STALE_VALIDATION);
 const seen=new Set();
 for(const capability of view.filter_capabilities){
  if(!validID(capability?.dimension)||!view.dimensions.some(d=>d.id===capability.dimension)||seen.has(capability.dimension)||typeof capability.supported!=='boolean'||capability.default_required!==true||typeof capability.option_lookup!=='boolean'||!Array.isArray(capability.kinds)||new Set(capability.kinds).size!==capability.kinds.length||capability.kinds.some(kind=>!DATASET_FILTER_KINDS.includes(kind)))throw appError(STALE_VALIDATION);
  seen.add(capability.dimension);
  if(capability.supported){
   const text=capability.kinds.length===2&&capability.kinds.includes('select')&&capability.kinds.includes('multi_select'),date=capability.kinds.length===1&&capability.kinds[0]==='date_range';
   if(!view.supported||!view.dimensions.find(d=>d.id===capability.dimension)?.supported||capability.reason||!text&&!date||text&&(capability.max_set_size!==16||capability.date_bounds!==undefined)||date&&(view.dimensions.find(d=>d.id===capability.dimension)?.role!=='temporal'||capability.option_lookup||capability.max_set_size!==undefined||capability.date_bounds!=='start_inclusive_end_exclusive_date_only'))throw appError(STALE_VALIDATION);
  }else if(capability.kinds.length||capability.option_lookup||typeof capability.reason!=='string'||!capability.reason||capability.max_set_size!==undefined||capability.date_bounds!==undefined)throw appError(STALE_VALIDATION);
 }
}
export function datasetFilterDefault(kind,value){
 if(kind==='select'){if(!datasetKeys(value,['literal']))throw appError(INVALID_REQUEST);return selectionFilterValue([value.literal],false);}
 if(kind==='multi_select'){if(!datasetKeys(value,['items']))throw appError(INVALID_REQUEST);return selectionFilterValue(value.items,true);}
 if(kind==='date_range'){if(!datasetKeys(value,['date_range'])||!datasetKeys(value.date_range,['start','end_exclusive']))throw appError(INVALID_REQUEST);const dates=displayFilterRange(value);return inclusiveFilterRange(dates.start,dates.end);}
 throw appError(INVALID_REQUEST);
}
function datasetFilters(view,filters){
 if(filters===undefined)return [];
 if(!Array.isArray(filters)||filters.length>4)throw appError(INVALID_REQUEST);
 const seen=new Set();return filters.map(filter=>{const capability=datasetFilterCapability(view,filter?.dimension);if(!datasetKeys(filter,['dimension','kind','default'])||!capability?.supported||!capability.kinds.includes(filter.kind)||seen.has(filter.dimension))throw appError(INVALID_REQUEST);seen.add(filter.dimension);return {dimension:filter.dimension,kind:filter.kind,default:datasetFilterDefault(filter.kind,filter.default)};});
}
const datasetHash=v=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const sameDatasetPin=(a,b)=>a?.topic===b?.topic&&a?.version===b?.version&&a?.digest===b?.digest;
const datasetDate=v=>typeof v==='string'&&Number.isFinite(Date.parse(v));
function datasetPin(v){if(!validID(v?.topic)||!validID(v.version)||!datasetHash(v.digest))throw appError(STALE_VALIDATION);return {topic:v.topic,version:v.version,digest:v.digest};}
export function checkDatasetView(view,pin,dataset){
 if(view?.compiler!=='reviewed-dataset-postgres-v1'||!sameDatasetPin(view.topic,pin)||view.dataset!==dataset||!validID(view.source)||!validID(view.context)||!Number.isSafeInteger(view.source_revision)||view.source_revision<1||typeof view.supported!=='boolean'||!Array.isArray(view.dimensions)||!Array.isArray(view.measures)||view.dimensions.length+view.measures.length>512||!Array.isArray(view.chart_kinds)||new Set(view.chart_kinds).size!==view.chart_kinds.length||view.chart_kinds.some(k=>!DATASET_CHART_KINDS.includes(k))||!Array.isArray(view.limitations)||view.limitations.length>32)throw appError(STALE_VALIDATION);
 const ids=new Set(),bindings=new Set();for(const field of [...view.dimensions,...view.measures]){if(!validID(field.id)||!validID(field.binding)||ids.has(field.id)||bindings.has(field.binding)||typeof field.name!=='string'||typeof field.supported!=='boolean')throw appError(STALE_VALIDATION);ids.add(field.id);bindings.add(field.binding);}
 if(view.supported&&(view.dialect!=='postgres'||view.reason))throw appError(STALE_VALIDATION);checkDatasetFilterMetadata(view);return view;
}
export function datasetIntent(view,draft){
 if(!view?.supported||!view.chart_kinds.includes(draft.kind)||!DATASET_CHART_KINDS.includes(draft.kind)||!Array.isArray(draft.dimensions)||draft.dimensions.length>2||new Set(draft.dimensions).size!==draft.dimensions.length||typeof draft.title!=='string'||!draft.title.trim()||draft.title.length>256)throw appError(INVALID_REQUEST);
 const dimensions=draft.dimensions.map(id=>view.dimensions.find(d=>d.id===id&&d.supported)),measure=view.measures.find(m=>m.id===draft.measure&&m.supported);
 if(dimensions.some(d=>!d)||!measure||draft.kind==='kpi'&&dimensions.length||!['kpi','table'].includes(draft.kind)&&!dimensions.length||['pie','donut'].includes(draft.kind)&&dimensions.length!==1||['line','area'].includes(draft.kind)&&dimensions[0]?.role!=='temporal')throw appError(INVALID_REQUEST);
 const bindings=draft.kind==='table'?{columns:[...dimensions,measure].map(f=>f.binding)}:draft.kind==='kpi'?{value:measure.binding}:{category:dimensions[0].binding,value:measure.binding,...(dimensions[1]?{series:dimensions[1].binding}:{})};
 const mapping={kind:draft.kind,bindings,order:['line','area'].includes(draft.kind)?[{column:dimensions[0].binding,direction:'asc'}]:[],options:{title:draft.title.trim(),legend:{visible:true,position:'bottom'},label_max_runes:80}};
 if(draft.kind==='table'){if(!Number.isSafeInteger(draft.pageSize)||draft.pageSize<1||draft.pageSize>1000)throw appError(INVALID_REQUEST);mapping.table={columns:bindings.columns.map(column=>({column,visible:true})),page_size:draft.pageSize,show_totals:false};}
 const filters=datasetFilters(view,draft.filters);return {topic:datasetPin(view.topic),dataset:view.dataset,dimensions:[...draft.dimensions],measure:draft.measure,mapping,...(filters.length?{filters}:{})};
}
export function checkPreparation(view,custody,previous=null){
 // Unsupported compilation happens before durable reservation and intentionally has no identity.
 if(view?.status==='unsupported'&&!custody.preparation&&!previous){if(view.validation!=='not_performed'||typeof view.code!=='string'||!view.code||!Array.isArray(view.schema)||view.schema.length||view.preparation||view.new_block||view.operation||view.digest||view.mapping)throw appError(STALE_VALIDATION);return view;}
 if(!validID(view?.preparation)||view.new_block!==custody.new_block||view.operation!==custody.operation||custody.preparation&&view.preparation!==custody.preparation||previous&&view.preparation!==previous.preparation||!['accepted','prepared','failed','uncertain','consumed'].includes(view.status)||view.validation!=='native_validation_required'||!datasetDate(view.expires_at)||!Array.isArray(view.schema)||view.schema.length>256||custody.digest&&view.digest!==custody.digest||previous?.digest&&view.digest!==previous.digest)throw appError(STALE_VALIDATION);
 if(['prepared','consumed'].includes(view.status)&&(!datasetHash(view.digest)||!view.schema.length||!view.mapping||!DATASET_CHART_KINDS.includes(view.mapping.kind)))throw appError(STALE_VALIDATION);
 if(previous?.status==='consumed'&&view.status!=='consumed')throw appError(STALE_VALIDATION);return view;
}
function stableDatasetValue(value){if(Array.isArray(value))return value.map(stableDatasetValue);if(value&&typeof value==='object')return Object.fromEntries(Object.keys(value).sort().map(key=>[key,stableDatasetValue(value[key])]));return value;}
function datasetMappingIdentity(mapping){const value=mappingDraft(mapping);value.bindings=Object.fromEntries(Object.entries(value.bindings).filter(([,v])=>v!==null&&v!==''&&(!Array.isArray(v)||v.length)));return JSON.stringify(stableDatasetValue(value));}
function checkPreparedIntent(value,intent){if(['prepared','consumed'].includes(value.status)&&(!intent||datasetMappingIdentity(value.mapping)!==datasetMappingIdentity(intent.mapping)||value.schema.length!==intent.dimensions.length+1||value.mapping.columns?.length!==value.schema.length||value.schema.some((f,i)=>typeof f.name!=='string'||typeof f.type!=='string'||f.name!==value.mapping.columns[i].name)))throw appError(STALE_VALIDATION);}
// Generate only for an explicit Prepare, after host target allocation. The server
// alone decides freshness and authority; an existing operation never gets rekeyed.
export function newPreparationOperation(){return `prepare:${Math.floor(Date.now()/1000)}:${crypto.randomUUID().replaceAll('-','')}`;}
function validPreparationOperation(value){return typeof value==='string'&&/^prepare:[1-9][0-9]*:[a-f0-9]{32}$/.test(value)&&Number.isSafeInteger(Number(value.split(':')[1]));}
const preparationRejections=new Set(['preparation_contract_required','preparation_operation_expired']);
export class DatasetSession {
 constructor(invoke,{locale='en-US',operation=newPreparationOperation,optionOperation}={}){this.invoke=invoke;this.locale=locale;this.operation=operation;this.generation=0;this.closed=false;this.pending=false;this.topics=[];this.next='';this.publication=null;this.datasets=[];this.view=null;this.draft={kind:'bar',dimensions:[],measure:'',title:'Untitled chart',pageSize:20};this.newBlock='';this.custody=null;this.preparation=null;this.unknown='';this.created=null;this.acceptedIntent=null;this.request=null;this.rejected='';this.optionOperation=optionOperation;this.filterLookups=new Map();this.filterStages=new Map();this.filterSelection={dimension:'',kind:''};}
 get hasUnsettledFilterLookups(){return [...this.filterLookups.values()].some(lookup=>lookup.pending||lookup.unknown);}
 get locked(){return this.pending||!!this.custody||this.closed||this.hasUnsettledFilterLookups;}
 resetFilters(){if(this.hasUnsettledFilterLookups)throw appError(BUSY);for(const lookup of this.filterLookups.values())lookup.close();this.filterLookups.clear();this.filterStages.clear();this.filterSelection={dimension:'',kind:''};delete this.draft.filters;}
 addFilter(dimension,kind){if(this.locked)throw appError(BUSY);const capability=datasetFilterCapability(this.view,dimension),filters=this.draft.filters||[];if(!capability?.supported||!capability.kinds.includes(kind)||filters.length>=4||filters.some(f=>f.dimension===dimension))throw appError(INVALID_REQUEST);this.draft.filters=[...filters,{dimension,kind,default:null}];this.filterSelection={dimension:'',kind:''};this.beginFilter(dimension);}
 removeFilter(dimension){if(this.locked)throw appError(BUSY);if(!this.draft.filters?.some(f=>f.dimension===dimension))throw appError(INVALID_REQUEST);this.draft.filters=this.draft.filters.filter(f=>f.dimension!==dimension);this.filterStages.delete(dimension);this.filterLookups.get(dimension)?.close();this.filterLookups.delete(dimension);}
 changeFilterKind(dimension,kind){if(this.locked)throw appError(BUSY);const filter=this.draft.filters?.find(f=>f.dimension===dimension),capability=datasetFilterCapability(this.view,dimension);if(!filter||!capability?.supported||!capability.kinds.includes(kind))throw appError(INVALID_REQUEST);if(filter.kind!==kind){filter.kind=kind;filter.default=null;}this.beginFilter(dimension);}
 beginFilter(dimension){if(this.locked)throw appError(BUSY);const filter=this.draft.filters?.find(f=>f.dimension===dimension);if(!filter)throw appError(INVALID_REQUEST);this.filterStages.set(dimension,filterInputState({type:datasetFilterTypes[filter.kind]},filter.default));return this.filterStages.get(dimension);}
 commitFilter(dimension,value){if(this.locked)throw appError(BUSY);const filter=this.draft.filters?.find(f=>f.dimension===dimension);if(!filter||!this.filterStages.has(dimension))throw appError(INVALID_REQUEST);filter.default=datasetFilterDefault(filter.kind,value);this.filterStages.delete(dimension);}
 cancelFilter(dimension){if(this.locked)throw appError(BUSY);this.filterStages.delete(dimension);}
 async searchFilter(dimension,newBlock,search='',cursor=''){
  if(this.locked)throw appError(BUSY);const capability=datasetFilterCapability(this.view,dimension),filter=this.draft.filters?.find(f=>f.dimension===dimension);
  if(!this.view?.supported||!capability?.supported||!capability.option_lookup||!filter||!['select','multi_select'].includes(filter.kind)||!validID(newBlock)||this.newBlock&&this.newBlock!==newBlock)throw appError(INVALID_REQUEST);
  this.newBlock=newBlock;const target={dataset:{topic:datasetPin(this.view.topic),dataset:this.view.dataset,dimension,new_block:newBlock}};let lookup=this.filterLookups.get(dimension);
  if(lookup&&JSON.stringify(stableDatasetValue(lookup.target))!==JSON.stringify(stableDatasetValue(target)))throw appError(STALE_VALIDATION);
  if(!lookup){lookup=new FilterOptionLookup(this.invoke,target,{locale:this.locale,...(this.optionOperation?{operation:this.optionOperation}:{})});this.filterLookups.set(dimension,lookup);}
  return lookup.search(search,cursor);
 }
 async inspectFilter(dimension,action=''){if(this.closed||this.pending||this.custody)throw appError(BUSY);const lookup=this.filterLookups.get(dimension);if(!lookup)throw appError(INVALID_REQUEST);return lookup.inspect(action);}
 intentValid(){if(this.filterStages.size)return false;try{datasetIntent(this.view,this.draft);return true;}catch{return false;}}
 valid(){return this.intentValid()&&validID(this.newBlock);}
 edit(fn){if(this.locked)throw appError(BUSY);const next=copyData(this.draft);fn(next);this.draft=next;}
 async read(name,args,accept){if(this.locked)throw appError(BUSY);const generation=++this.generation;this.pending=true;try{const value=await this.invoke(name,args);if(this.closed||generation!==this.generation)return null;return accept(value);}finally{this.pending=false;}}
 async loadTopics(after=''){return this.read('list_topics',{after,limit:40},page=>{
  const explicit=!Array.isArray(page),items=explicit?page?.items:page,next=explicit?page?.next||'':items?.length===40?items.at(-1)?.topic:'';
  if(!Array.isArray(items)||items.length>40||explicit&&(typeof page!=='object'||Object.keys(page).some(key=>!['items','next'].includes(key))||page.next!==undefined&&typeof page.next!=='string')||next&&(!validID(next)||next<=after||items.length&&next<items.at(-1).topic)||items.some((item,i)=>{datasetPin(item);return typeof item.name!=='string'||i>0&&item.topic<=items[i-1].topic||after&&item.topic<=after;}))throw appError(STALE_VALIDATION);
  this.topics=after?[...this.topics,...copyData(items)].slice(0,200):copyData(items);this.next=this.topics.length<200?next:'';return items;
 });}
 async selectTopic(item){const pin=datasetPin(item);return this.read('describe_topic',{topic:pin.topic},value=>{const d=value?.definition,s=value?.state;if(!sameDatasetPin({topic:d?.topic,version:d?.version,digest:value?.digest},pin)||s?.topic!==pin.topic||s.version!==pin.version||s.active!==true||s.archived===true||!Array.isArray(d.datasets)||d.datasets.length>100||new Set(d.datasets.map(v=>v.id)).size!==d.datasets.length||d.datasets.some(v=>!validID(v.id)||typeof v.name!=='string'))throw appError(STALE_VALIDATION);this.resetFilters();this.publication={...pin,name:item.name};this.datasets=d.datasets.map(v=>({id:v.id,name:v.name}));this.view=null;this.draft.dimensions=[];this.draft.measure='';return this.datasets;});}
 async selectDataset(id){if(!this.publication||!this.datasets.some(d=>d.id===id))throw appError(INVALID_REQUEST);const pin=datasetPin(this.publication);return this.read(authoringTool('dataset'),{topic:pin,dataset:id},value=>{checkDatasetView(value,pin,id);this.resetFilters();this.view=copyData(value);this.draft.dimensions=[];this.draft.measure='';return this.view;});}
 async prepare(){
  if(this.locked||!this.valid())throw appError(BUSY);const intent=datasetIntent(this.view,this.draft),operation=this.operation();if(!validPreparationOperation(operation)||operation===this.request?.operation)throw appError(INVALID_REQUEST);
  const args={new_block:this.newBlock,operation_version:'prepare-v1',operation,intent,metadata:[{locale:this.locale,title:this.draft.title.trim(),question:this.draft.title.trim(),aliases:[],description:''}]},generation=this.generation;
  this.preparation=null;this.request=copyData(args);this.custody={new_block:this.newBlock,operation_version:args.operation_version,operation};this.rejected='';this.acceptedIntent=copyData(intent);this.pending=true;
  try{const value=await this.invoke(authoringTool('prepare_chart'),copyData(args));if(this.closed||generation!==this.generation)return null;try{checkPreparation(value,this.custody);checkPreparedIntent(value,this.acceptedIntent);}catch{throw appError(UNAVAILABLE,true);}this.preparation=copyData(value);if(value.status==='unsupported')this.custody=null;else{this.custody.preparation=value.preparation;if(value.digest)this.custody.digest=value.digest;}this.unknown=['accepted','uncertain'].includes(value.status)?'prepare':'';return this.preparation;}
  catch(e){if(!this.closed&&generation===this.generation){this.rejected=!e.unknown&&preparationRejections.has(e.code)?e.code:'';this.unknown=this.rejected?'':'prepare';}throw e;}finally{this.pending=false;}
 }
 reviewPreparation(){
  if(this.closed||this.pending||!this.rejected||this.unknown)throw appError(BUSY);
  this.suspend();this.custody=null;this.acceptedIntent=null;this.rejected='';this.generation++;
 }
 async inspect(action=''){
  if(this.closed||this.pending||this.rejected||!this.custody||action&&!['inspect','cancel','reconcile'].includes(action))throw appError(BUSY);const generation=this.generation,previous=this.preparation,status=this.unknown,args={new_block:this.custody.new_block,...(this.custody.preparation?{preparation:this.custody.preparation}:{operation:this.custody.operation}),...(action?{action}:{})};this.pending=true;
  try{const value=await this.invoke(authoringTool(action?'preparation_control':'preparation'),args);if(this.closed||generation!==this.generation)return null;checkPreparation(value,this.custody,previous);checkPreparedIntent(value,this.acceptedIntent);this.preparation=copyData(value);this.custody.preparation=value.preparation;if(value.digest)this.custody.digest=value.digest;if(status!=='create')this.unknown=['accepted','uncertain'].includes(value.status)?'prepare':'';return this.preparation;}finally{this.pending=false;}
 }
 canCreate(){return !this.closed&&!this.pending&&!this.unknown&&this.preparation?.status==='prepared'&&Date.parse(this.preparation.expires_at)>Date.now();}
 canRecover(){return !this.closed&&!this.pending&&this.unknown==='create'&&this.preparation?.status==='consumed'&&this.custody.digest===this.preparation.digest;}
 async create(recover=false){
  if(recover?!this.canRecover():!this.canCreate())throw appError(BUSY);const generation=this.generation,args={new_block:this.custody.new_block,preparation:this.custody.preparation,digest:this.custody.digest};this.pending=true;
  try{const value=await this.invoke(authoringTool('create_prepared'),args);if(this.closed||generation!==this.generation)return null;try{checkMappingView(value,args.new_block,1,'chart');if(!value.block.private||value.block.state.draft_revision!==1||value.block.validation&&!recover||value.block.outputs.length!==1||datasetMappingIdentity(value.block.outputs[0].mapping)!==datasetMappingIdentity(this.preparation.mapping)||JSON.stringify(stableDatasetValue(value.block.expected_schema))!==JSON.stringify(stableDatasetValue(this.preparation.schema)))throw appError(STALE_VALIDATION);}catch{throw appError(UNAVAILABLE,true);}this.created=copyData(value);this.unknown='';return this.created;}
  catch(e){if(!this.closed&&generation===this.generation)this.unknown='create';throw e;}finally{this.pending=false;}
 }
 suspend(){if(this.pending||[...this.filterLookups.values()].some(lookup=>lookup.pending))throw appError(BUSY);for(const [dimension,lookup]of this.filterLookups)if(!lookup.unknown){lookup.close();this.filterLookups.delete(dimension);}this.topics=[];this.datasets=[];this.publication=null;this.view=null;this.preparation=null;this.created=null;}
 close(){for(const lookup of this.filterLookups.values())lookup.close();this.filterLookups.clear();this.filterStages.clear();this.closed=true;this.generation++;this.topics=[];this.datasets=[];this.publication=null;this.view=null;this.draft=null;this.preparation=null;this.created=null;this.acceptedIntent=null;this.request=null;this.rejected='';}
}
