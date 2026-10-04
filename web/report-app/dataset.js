import {appError,authoringTool,copyData,validID} from './model.js';
import {checkMappingView,mappingDraft} from './mapping.js';

export const DATASET_CHART_KINDS=['bar','column','line','area','pie','donut','kpi','table'];
const datasetHash=v=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const sameDatasetPin=(a,b)=>a?.topic===b?.topic&&a?.version===b?.version&&a?.digest===b?.digest;
const datasetDate=v=>typeof v==='string'&&Number.isFinite(Date.parse(v));
function datasetPin(v){if(!validID(v?.topic)||!validID(v.version)||!datasetHash(v.digest))throw appError('stale_validation');return {topic:v.topic,version:v.version,digest:v.digest};}
export function checkDatasetView(view,pin,dataset){
 if(view?.compiler!=='reviewed-dataset-postgres-v1'||!sameDatasetPin(view.topic,pin)||view.dataset!==dataset||!validID(view.source)||!validID(view.context)||!Number.isSafeInteger(view.source_revision)||view.source_revision<1||typeof view.supported!=='boolean'||!Array.isArray(view.dimensions)||!Array.isArray(view.measures)||view.dimensions.length+view.measures.length>512||!Array.isArray(view.chart_kinds)||new Set(view.chart_kinds).size!==view.chart_kinds.length||view.chart_kinds.some(k=>!DATASET_CHART_KINDS.includes(k))||!Array.isArray(view.limitations)||view.limitations.length>32)throw appError('stale_validation');
 const ids=new Set(),bindings=new Set();for(const field of [...view.dimensions,...view.measures]){if(!validID(field.id)||!validID(field.binding)||ids.has(field.id)||bindings.has(field.binding)||typeof field.name!=='string'||typeof field.supported!=='boolean')throw appError('stale_validation');ids.add(field.id);bindings.add(field.binding);}
 if(view.supported&&(view.dialect!=='postgres'||view.reason))throw appError('stale_validation');return view;
}
export function datasetIntent(view,draft){
 if(!view?.supported||!view.chart_kinds.includes(draft.kind)||!DATASET_CHART_KINDS.includes(draft.kind)||!Array.isArray(draft.dimensions)||draft.dimensions.length>2||new Set(draft.dimensions).size!==draft.dimensions.length||typeof draft.title!=='string'||!draft.title.trim()||draft.title.length>256)throw appError('invalid_request');
 const dimensions=draft.dimensions.map(id=>view.dimensions.find(d=>d.id===id&&d.supported)),measure=view.measures.find(m=>m.id===draft.measure&&m.supported);
 if(dimensions.some(d=>!d)||!measure||draft.kind==='kpi'&&dimensions.length||!['kpi','table'].includes(draft.kind)&&!dimensions.length||['pie','donut'].includes(draft.kind)&&dimensions.length!==1||['line','area'].includes(draft.kind)&&dimensions[0]?.role!=='temporal')throw appError('invalid_request');
 const bindings=draft.kind==='table'?{columns:[...dimensions,measure].map(f=>f.binding)}:draft.kind==='kpi'?{value:measure.binding}:{category:dimensions[0].binding,value:measure.binding,...(dimensions[1]?{series:dimensions[1].binding}:{})};
 const mapping={kind:draft.kind,bindings,order:['line','area'].includes(draft.kind)?[{column:dimensions[0].binding,direction:'asc'}]:[],options:{title:draft.title.trim(),legend:{visible:true,position:'bottom'},label_max_runes:80}};
 if(draft.kind==='table'){if(!Number.isSafeInteger(draft.pageSize)||draft.pageSize<1||draft.pageSize>1000)throw appError('invalid_request');mapping.table={columns:bindings.columns.map(column=>({column,visible:true})),page_size:draft.pageSize,show_totals:false};}
 return {topic:datasetPin(view.topic),dataset:view.dataset,dimensions:[...draft.dimensions],measure:draft.measure,mapping};
}
export function checkPreparation(view,custody,previous=null){
 // Unsupported compilation happens before durable reservation and intentionally has no identity.
 if(view?.status==='unsupported'&&!custody.preparation&&!previous){if(view.validation!=='not_performed'||typeof view.code!=='string'||!view.code||!Array.isArray(view.schema)||view.schema.length||view.preparation||view.new_block||view.operation||view.digest||view.mapping)throw appError('stale_validation');return view;}
 if(!validID(view?.preparation)||view.new_block!==custody.new_block||view.operation!==custody.operation||custody.preparation&&view.preparation!==custody.preparation||previous&&view.preparation!==previous.preparation||!['accepted','prepared','failed','uncertain','consumed'].includes(view.status)||view.validation!=='native_validation_required'||!datasetDate(view.expires_at)||!Array.isArray(view.schema)||view.schema.length>256||custody.digest&&view.digest!==custody.digest||previous?.digest&&view.digest!==previous.digest)throw appError('stale_validation');
 if(['prepared','consumed'].includes(view.status)&&(!datasetHash(view.digest)||!view.schema.length||!view.mapping||!DATASET_CHART_KINDS.includes(view.mapping.kind)))throw appError('stale_validation');
 if(previous?.status==='consumed'&&view.status!=='consumed')throw appError('stale_validation');return view;
}
function stableDatasetValue(value){if(Array.isArray(value))return value.map(stableDatasetValue);if(value&&typeof value==='object')return Object.fromEntries(Object.keys(value).sort().map(key=>[key,stableDatasetValue(value[key])]));return value;}
function datasetMappingIdentity(mapping){const value=mappingDraft(mapping);value.bindings=Object.fromEntries(Object.entries(value.bindings).filter(([,v])=>v!==null&&v!==''&&(!Array.isArray(v)||v.length)));return JSON.stringify(stableDatasetValue(value));}
function checkPreparedIntent(value,intent){if(['prepared','consumed'].includes(value.status)&&(!intent||datasetMappingIdentity(value.mapping)!==datasetMappingIdentity(intent.mapping)||value.schema.length!==intent.dimensions.length+1||value.mapping.columns?.length!==value.schema.length||value.schema.some((f,i)=>typeof f.name!=='string'||typeof f.type!=='string'||f.name!==value.mapping.columns[i].name)))throw appError('stale_validation');}
export class DatasetSession {
 constructor(invoke,{locale='en-US',operation=()=>crypto.randomUUID()}={}){this.invoke=invoke;this.locale=locale;this.operation=operation;this.generation=0;this.closed=false;this.pending=false;this.topics=[];this.next='';this.publication=null;this.datasets=[];this.view=null;this.draft={kind:'bar',dimensions:[],measure:'',title:'Untitled chart',pageSize:20};this.newBlock='';this.custody=null;this.preparation=null;this.unknown='';this.created=null;this.acceptedIntent=null;}
 get locked(){return this.pending||!!this.custody||this.closed;}
 valid(){try{datasetIntent(this.view,this.draft);return validID(this.newBlock);}catch{return false;}}
 edit(fn){if(this.locked)throw appError('busy');const next=copyData(this.draft);fn(next);this.draft=next;}
 async read(name,args,accept){if(this.closed||this.pending||this.custody)throw appError('busy');const generation=++this.generation;this.pending=true;try{const value=await this.invoke(name,args);if(this.closed||generation!==this.generation)return null;return accept(value);}finally{this.pending=false;}}
 async loadTopics(after=''){return this.read('list_topics',{after,limit:40},items=>{if(!Array.isArray(items)||items.length>40||items.some((item,i)=>{datasetPin(item);return typeof item.name!=='string'||i>0&&item.topic<=items[i-1].topic||after&&item.topic<=after;}))throw appError('stale_validation');this.topics=after?[...this.topics,...copyData(items)].slice(0,200):copyData(items);this.next=items.length===40&&this.topics.length<200?items.at(-1).topic:'';return items;});}
 async selectTopic(item){const pin=datasetPin(item);return this.read('describe_topic',{topic:pin.topic},value=>{const d=value?.definition,s=value?.state;if(!sameDatasetPin({topic:d?.topic,version:d?.version,digest:value?.digest},pin)||s?.topic!==pin.topic||s.version!==pin.version||s.active!==true||s.archived===true||!Array.isArray(d.datasets)||d.datasets.length>100||new Set(d.datasets.map(v=>v.id)).size!==d.datasets.length||d.datasets.some(v=>!validID(v.id)||typeof v.name!=='string'))throw appError('stale_validation');this.publication={...pin,name:item.name};this.datasets=d.datasets.map(v=>({id:v.id,name:v.name}));this.view=null;this.draft.dimensions=[];this.draft.measure='';return this.datasets;});}
 async selectDataset(id){if(!this.publication||!this.datasets.some(d=>d.id===id))throw appError('invalid_request');const pin=datasetPin(this.publication);return this.read(authoringTool('dataset'),{topic:pin,dataset:id},value=>{checkDatasetView(value,pin,id);this.view=copyData(value);this.draft.dimensions=[];this.draft.measure='';return this.view;});}
 async prepare(){
  if(this.locked||!this.valid())throw appError('busy');const intent=datasetIntent(this.view,this.draft),operation=this.operation();if(!validID(operation))throw appError('invalid_request');
  const args={new_block:this.newBlock,operation,intent,metadata:[{locale:this.locale,title:this.draft.title.trim(),question:this.draft.title.trim(),aliases:[],description:''}]},generation=this.generation;
  this.custody={new_block:this.newBlock,operation};this.acceptedIntent=copyData(intent);this.pending=true;
  try{const value=await this.invoke(authoringTool('prepare_chart'),copyData(args));if(this.closed||generation!==this.generation)return null;try{checkPreparation(value,this.custody);checkPreparedIntent(value,this.acceptedIntent);}catch{throw appError('unavailable',true);}this.preparation=copyData(value);if(value.status==='unsupported')this.custody=null;else{this.custody.preparation=value.preparation;if(value.digest)this.custody.digest=value.digest;}this.unknown=['accepted','uncertain'].includes(value.status)?'prepare':'';return this.preparation;}
  catch(e){if(!this.closed&&generation===this.generation){this.unknown='prepare';}throw e;}finally{this.pending=false;}
 }
 async inspect(action=''){
  if(this.closed||this.pending||!this.custody||action&&!['inspect','cancel','reconcile'].includes(action))throw appError('busy');const generation=this.generation,previous=this.preparation,status=this.unknown,args={new_block:this.custody.new_block,...(this.custody.preparation?{preparation:this.custody.preparation}:{operation:this.custody.operation}),...(action?{action}:{})};this.pending=true;
  try{const value=await this.invoke(authoringTool(action?'preparation_control':'preparation'),args);if(this.closed||generation!==this.generation)return null;checkPreparation(value,this.custody,previous);checkPreparedIntent(value,this.acceptedIntent);this.preparation=copyData(value);this.custody.preparation=value.preparation;if(value.digest)this.custody.digest=value.digest;if(status!=='create')this.unknown=['accepted','uncertain'].includes(value.status)?'prepare':'';return this.preparation;}finally{this.pending=false;}
 }
 canCreate(){return !this.closed&&!this.pending&&!this.unknown&&this.preparation?.status==='prepared'&&Date.parse(this.preparation.expires_at)>Date.now();}
 canRecover(){return !this.closed&&!this.pending&&this.unknown==='create'&&this.preparation?.status==='consumed'&&this.custody.digest===this.preparation.digest;}
 async create(recover=false){
  if(recover?!this.canRecover():!this.canCreate())throw appError('busy');const generation=this.generation,args={new_block:this.custody.new_block,preparation:this.custody.preparation,digest:this.custody.digest};this.pending=true;
  try{const value=await this.invoke(authoringTool('create_prepared'),args);if(this.closed||generation!==this.generation)return null;try{checkMappingView(value,args.new_block,1,'chart');if(!value.block.private||value.block.state.draft_revision!==1||value.block.validation&&!recover||value.block.outputs.length!==1||datasetMappingIdentity(value.block.outputs[0].mapping)!==datasetMappingIdentity(this.preparation.mapping)||JSON.stringify(stableDatasetValue(value.block.expected_schema))!==JSON.stringify(stableDatasetValue(this.preparation.schema)))throw appError('stale_validation');}catch{throw appError('unavailable',true);}this.created=copyData(value);this.unknown='';return this.created;}
  catch(e){if(!this.closed&&generation===this.generation)this.unknown='create';throw e;}finally{this.pending=false;}
 }
 suspend(){if(this.pending)throw appError('busy');this.topics=[];this.datasets=[];this.publication=null;this.view=null;this.preparation=null;this.created=null;}
 close(){this.closed=true;this.generation++;this.topics=[];this.datasets=[];this.publication=null;this.view=null;this.draft=null;this.preparation=null;this.created=null;this.acceptedIntent=null;}
}
