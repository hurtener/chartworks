import {node as datasetNode, button as datasetButton, selectField as datasetSelect, textField} from './dom.js';
import {TARGET_ALLOCATION_UNAVAILABLE} from './allocation.js';
import {DATASET_CHART_KINDS,datasetFilterCapability,datasetFilterFields} from './dataset.js';
import {displayFilterRange} from './filters.js';
import {renderFilterInput} from './filter-controls.js';
import {renderFieldPicker} from './field-picker.js';
import {datasetFilterKey} from './column-filters.js';

function datasetInput(label,value,callback,{disabled=false,number=false}={}){return textField(label,value,next=>callback(number?Number(next):next),{type:number?'number':'text',maxLength:number?4:256,disabled});}
const filterKindLabel={select:'One value',multi_select:'Multiple values',date_range:'Date range',range:'Range'};
function datasetFilterSummary(filter){
 if(!filter.default)return 'Choose a required default';
 if(filter.kind==='range')return `Default: ${filter.default.range.start} until ${filter.default.range.end_exclusive} (exclusive)${filter.timezone?' · '+filter.timezone:''}`;
 if(filter.kind==='date_range'){const range=displayFilterRange(filter.default);return `Default: ${range.start} to ${range.end}, inclusive`;}
 const values=filter.kind==='select'?[filter.default.literal]:filter.default.items;
 return 'Default: '+values.map(value=>value===''?'Empty text':value).join(', ');
}
function renderDatasetFilters(parent,session,{locked,busy,canAllocate,change,searchFilter,inspectFilter}){
 const view=session.view,filters=session.draft.filters||[],fields=datasetFilterFields(view,!!session.draft.fields),section=datasetNode('section',undefined,'dataset-filters');
 section.append(datasetNode('h3','Chart filters'),datasetNode('p','Choose up to four fields with required defaults. Defaults are saved with the chart, separately from temporary report selections.','metadata'));
 const availability=field=>{
  const capability=datasetFilterCapability(view,field.id);
  if(!capability?.supported)return capability?.reason?.replaceAll('_',' ')||'No filter capability is available';
  if(!field.column&&capability.kinds.includes('select'))return !capability.option_lookup?'Governed option search is unavailable for this field':!searchFilter?'This host has not enabled governed option search':!canAllocate?'This host cannot prepare a private chart target for option search':'';
  return '';
 };
 if(!fields.some(f=>datasetFilterCapability(view,f.id)))section.append(datasetNode('p','Filter creation is unavailable for this dataset version. You can still prepare an unfiltered chart.','metadata'));
 else{
  const selection=session.filterSelection,selected=fields.find(d=>d.id===selection.dimension),capability=datasetFilterCapability(view,selection.dimension);
  section.append(datasetSelect('Filter field',[{value:'',label:'Choose a filter field'},...fields.map(field=>{const reason=availability(field),used=filters.some(f=>datasetFilterKey(f)===field.id);return {value:field.id,label:`${field.name}${field.column?' · '+(field.filters?.type||field.type)+' column':''}${used?' · already added':reason?' · unavailable: '+reason:''}`,disabled:used||!!reason};})],selection.dimension,value=>{if(locked)return;selection.dimension=value;selection.kind=datasetFilterCapability(view,value)?.kinds[0]||'';change();},locked||filters.length>=4));
  section.append(datasetSelect('Filter type',(capability?.kinds||[]).map(kind=>({value:kind,label:filterKindLabel[kind]})),selection.kind,value=>{if(locked)return;selection.kind=value;change();},locked||!selected||filters.length>=4),datasetButton('Add chart filter',()=>{session.addFilter(selection.dimension,selection.kind);change();},locked||filters.length>=4||!selected||!!availability(selected)||!capability?.kinds.includes(selection.kind)));
  section.append(datasetNode('p',`${filters.length} of 4 filters added`,'metadata'));
  const unavailable=fields.filter(field=>availability(field));
  if(unavailable.length){const details=datasetNode('details');details.append(datasetNode('summary','Unavailable filter fields'));for(const field of unavailable)details.append(datasetNode('p',`${field.name}: ${availability(field)}.`,'metadata'));section.append(details);}
 }
 for(const filter of filters){
  const key=datasetFilterKey(filter),field=fields.find(d=>d.id===key),capability=datasetFilterCapability(view,key),lookup=session.filterLookups.get(key),stage=session.filterStages.get(key),row=datasetNode('section',undefined,'dataset-filter');
  row.append(datasetNode('h4',field?.name||key),datasetSelect(`Filter type: ${field?.name||key}`,(capability?.kinds||[]).map(kind=>({value:kind,label:filterKindLabel[kind]})),filter.kind,value=>{session.changeFilterKind(key,value);change();},locked),datasetNode('p',datasetFilterSummary(filter),'metadata'));
  row.append(datasetButton(`Remove filter: ${field?.name||key}`,()=>{session.removeFilter(key);change();},locked));
  if(stage&&!lookup?.unknown)renderFilterInput(row,stage,{label:`Default for ${field?.name||key}`,lookup,disabled:locked,editPolicy:!!filter.column,onSearch:capability?.option_lookup&&canAllocate&&searchFilter?(search,cursor)=>searchFilter(key,search,cursor):undefined,onInspect:inspectFilter?action=>inspectFilter(key,action):undefined,onDone:value=>{session.commitFilter(key,value);change();},onCancel:()=>{session.cancelFilter(key);change();}});
  else if(!stage)row.append(datasetButton(`Choose default: ${field?.name||key}`,()=>{session.beginFilter(key);change();},locked));
  if(stage&&lookup?.unknown)row.append(datasetNode('p','The default selection is kept locally. Settle this option lookup below before continuing.','metadata'));
  section.append(row);
 }
 if(session.filterStages.size)section.append(datasetNode('p','Choose Done or Cancel for each open default editor before preparing the chart.','metadata'));
 parent.append(section);
}
function renderDatasetLookupRecovery(parent,session,{busy,inspectFilter}){
 for(const [dimension,lookup]of session.filterLookups){
  if(!lookup.pending&&!lookup.unknown)continue;
  const section=datasetNode('section',undefined,'dataset-option-recovery'),field=session.view&&datasetFilterFields(session.view).find(d=>d.id===dimension);
  section.append(datasetNode('h3',`Option lookup: ${field?.name||dimension}`),datasetNode('p',lookup.pending?'Reading governed options…':'The option lookup outcome is unconfirmed. Inspect, cancel, or reconcile this original lookup before preparing a chart or changing datasets.','notice'));
  if(inspectFilter)for(const [action,label]of [['','Inspect lookup'],['cancel','Cancel lookup'],['reconcile','Reconcile lookup']])section.append(datasetButton(label,()=>inspectFilter(dimension,action),busy||session.pending||lookup.pending));
  else section.append(datasetNode('p','This host has not enabled option lookup recovery. Resume this setup in a host with the original-operation controls.','metadata'));
  section.append(datasetNode('p','Status checks never repeat the source query. Closing keeps the original lookup available from Resume chart setup.','metadata'));parent.append(section);
 }
}
export function renderDatasetEditor(parent,session,{busy=false,allocation=null,canAllocate=false,resume,change,read,prepare,review,create,recover,inspect,cancel,searchFilter,inspectFilter}){
 const section=datasetNode('section',undefined,'dataset-editor');section.append(datasetNode('h2','Create from dataset'),datasetNode('p','Choose your data and fields, then prepare the result.','metadata'));
 const locked=busy||session.locked||!!allocation?.pending||!!allocation?.unknown,edit=fn=>{session.edit(fn);change();};
 if(!session.custody){
  if(session.tablesEnabled&&session.topicsEnabled){const tabs=datasetNode('div',undefined,'dataset-origins');tabs.setAttribute('aria-label','Data catalog');for(const [mode,label]of [['topics','Reviewed topics'],['tables','Tables & uploads']]){const button=datasetButton(label,()=>read(()=>session.chooseCatalog(mode)),locked);button.setAttribute('aria-pressed',String(session.catalog===mode));tabs.append(button);}section.append(tabs);}
  if(session.catalog==='tables'){
   section.append(datasetButton('Refresh sources',()=>read(()=>session.loadSources()),locked));
   for(const source of session.sources){const button=datasetButton(source.name,()=>read(()=>session.selectSource(source)),locked);button.className='dataset-topic';button.setAttribute('aria-pressed',String(session.source?.id===source.id));section.append(button);}
   if(!session.sources.length)section.append(datasetNode('p',session.sourceNext?'No visible sources on this page. Continue to the next page.':'No registered sources are visible under current access.','metadata'));
   if(session.sourceHistory.length>1)section.append(datasetButton('Previous sources',()=>read(()=>session.loadSources(session.sourceHistory.at(-2))),locked));
   if(session.sourceNext)section.append(datasetButton('Next sources',()=>read(()=>session.loadSources(session.sourceNext)),locked));
   if(session.source){
    section.append(datasetSelect('Table or upload',[{value:'',label:'Choose a dataset'},...session.tables.map(item=>({value:item.relation.id,label:item.relation.name}))],session.view?.dataset||'',id=>{if(id)read(()=>session.selectTable(id));},locked));
    if(!session.tables.length)section.append(datasetNode('p',session.tableNext?'No visible datasets on this page. Continue to the next page.':'No registered datasets are visible in this source context.','metadata'));
    if(session.tableHistory.length>1)section.append(datasetButton('Previous datasets',()=>read(()=>session.loadTables(session.tableHistory.at(-2))),locked));
    if(session.tableNext)section.append(datasetButton('Next datasets',()=>read(()=>session.loadTables(session.tableNext)),locked));
   }
  }else{
   section.append(datasetButton('Refresh topics',()=>read(()=>session.loadTopics()),locked));
   for(const topic of session.topics){const button=datasetButton(topic.name||topic.topic,()=>read(()=>session.selectTopic(topic)),locked);button.className='dataset-topic';button.setAttribute('aria-pressed',String(session.publication?.topic===topic.topic));section.append(button);}if(session.topicHistory.length>1)section.append(datasetButton('Previous topics',()=>read(()=>session.loadTopics(session.topicHistory.at(-2))),locked));if(session.next)section.append(datasetButton('Next topics',()=>read(()=>session.loadTopics(session.next)),locked));
   if(!session.topics.length)section.append(datasetNode('p','No reviewed topics are visible under current access.','metadata'));
   if(session.publication)section.append(datasetSelect('Reviewed dataset',[{value:'',label:'Choose dataset'},...session.datasets.map(d=>({value:d.id,label:d.name||d.id}))],session.view?.dataset||'',id=>{if(id)read(()=>session.selectDataset(id));},locked));
  }
 }
 const view=session.view,draft=session.draft;section.append(datasetNode('p',session.custody?'3 · Review preparation':view?'2 · Define your chart':'1 · Choose your data','eyebrow'));
 if(view){
  section.append(datasetNode('p',view.source_dataset?`${session.source?.name||view.source} · ${session.tables.find(d=>d.relation.id===view.dataset)?.relation.name||view.dataset} · Source revision ${view.source_revision}`:`${session.publication?.name||view.topic.topic} · ${session.datasets.find(d=>d.id===view.dataset)?.name||view.dataset} · ${view.topic.version}`,'metadata'));
  if(!(draft.fields?view.fields?.supported:view.supported))section.append(datasetNode('p',`This dataset cannot be prepared here: ${draft.fields?view.fields?.reason:view.reason||'unsupported'}.`,'notice error'));
  section.append(datasetInput('Chart title',draft.title,value=>edit(d=>{d.title=value;}),{disabled:locked}),datasetSelect('New chart type',view.chart_kinds.filter(k=>DATASET_CHART_KINDS.includes(k)).map(k=>({value:k,label:k.replaceAll('_',' ')})),draft.kind,value=>edit(d=>{d.kind=value;if(value==='kpi')d.dimensions=[];}),locked));
  if(draft.fields)renderFieldPicker(section,session,{locked,edit,change});
  else{
  const dimensions=datasetNode('fieldset');dimensions.disabled=locked;dimensions.append(datasetNode('legend',draft.kind==='kpi'?'Dimensions · KPI uses no grouping':'Dimensions · choose up to two'));
  for(const field of view.dimensions){const label=datasetNode('label',undefined,'dataset-field'),input=datasetNode('input');input.type='checkbox';input.checked=draft.dimensions.includes(field.id);input.disabled=locked||!field.supported||!input.checked&&(draft.kind==='kpi'||draft.dimensions.length>=2);input.setAttribute('aria-label',`Dimension: ${field.name}`);input.addEventListener('change',()=>edit(d=>{d.dimensions=input.checked?[...d.dimensions,field.id]:d.dimensions.filter(id=>id!==field.id);}));label.append(input,datasetNode('span',`${field.name}${field.supported?'':` · unavailable: ${field.reason}`}`));dimensions.append(label);}if(draft.kind!=='kpi')section.append(dimensions);
  section.append(datasetSelect('Reviewed measure',[{value:'',label:'Choose one measure'},...view.measures.map(m=>({value:m.id,label:`${m.name} · ${m.aggregation}${m.unit?' · '+m.unit:''}${m.supported?'':' · unavailable: '+m.reason}`,disabled:!m.supported}))],draft.measure,value=>edit(d=>{d.measure=value;}),locked));
  }
  renderDatasetFilters(section,session,{locked,busy,canAllocate,change,searchFilter,inspectFilter});
  if(draft.kind==='table')section.append(datasetInput('New table page size',draft.pageSize,value=>edit(d=>{d.pageSize=value;}),{disabled:locked,number:true}));
  section.append(datasetNode('p',draft.fields?'Choose aggregates for physical columns. Reviewed measures keep their approved definitions. Date grouping uses the selected calendar and timezone.':'Aggregation and units come from the reviewed measure. Line and area charts need a temporal first dimension; pie and donut use one dimension.','metadata'));
  const limits=datasetNode('details');limits.append(datasetNode('summary','Supported preparation scope'));for(const limitation of view.limitations)limits.append(datasetNode('p',limitation,'metadata'));section.append(limits);
  if(!session.custody)section.append(datasetNode('p',!canAllocate?TARGET_ALLOCATION_UNAVAILABLE:allocation?.unknown?'Resume preparation asks your host for the same chart target before preparing data.':'Your host will prepare a private chart target when you prepare.','metadata'),datasetButton('Prepare chart',prepare,locked||!canAllocate||!session.intentValid()),datasetNode('p','Prepare runs one bounded source read. Option search is a separate explicit read; editing fields and defaults does not query data.','metadata'));
 }
 if(allocation?.unknown)section.append(datasetNode('p','Chart target creation is unconfirmed. Resume asks for the original target without reading source data.','notice'),datasetButton('Resume chart creation',resume,busy||allocation.pending||!canAllocate));
 renderDatasetLookupRecovery(section,session,{busy,inspectFilter});
 const result=session.preparation;
 if(result)section.append(datasetNode('p',`Preparation: ${result.status}${result.code?' · '+result.code:''}`,'notice'));
 if(session.custody){
  const details=datasetNode('details');details.append(datasetNode('summary','Preparation and recovery details'),datasetNode('p',`Target: ${session.custody.new_block}`,'metadata'),datasetNode('p',`Operation: ${session.custody.operation}`,'metadata'),datasetNode('p',`Contract: ${session.custody.operation_version||'legacy retained operation'}`,'metadata'));if(session.custody.preparation)details.append(datasetNode('p',`Preparation: ${session.custody.preparation}`,'metadata'));if(result?.expires_at)details.append(datasetNode('p',`Expires: ${result.expires_at}`,'metadata'));if(result?.execution_status||result?.remote_state)details.append(datasetNode('p',`Source attempt: ${result.execution_status||'unknown'} · ${result.remote_state||'unknown'}`,'metadata'));section.append(details);
  if(session.unknown)section.append(datasetNode('p',session.unknown==='create'?'Chart creation outcome is unknown. Inspect exact preparation custody before recovery.':'Preparation outcome is unknown or pending. Inspect the original operation; it will not run again.','notice error'));
  if(result?.status==='prepared')section.append(datasetNode('p',`Actual schema checked: ${result.schema.length} fields. Native chart validation is still required.`, 'metadata'),datasetButton('Create private chart',create,!session.canCreate()||busy));
  if(session.canRecover())section.append(datasetButton('Recover created chart',recover,busy));
  if(session.rejected)section.append(datasetNode('p',session.rejected==='preparation_operation_expired'?'The server rejected this operation before admission because its timestamp expired or is ahead. Check your device clock, then review setup.':'The server rejected this preparation contract before admission. Reopen the app if an update is needed, then review setup.','notice error'),datasetButton('Review setup for new preparation',()=>review(session),busy||session.pending),datasetNode('p','Reviewing only reloads metadata. A new source read needs another explicit Prepare chart.','metadata'));
  else section.append(datasetButton('Inspect preparation',()=>inspect(''),busy||session.pending));
  if(['accepted','uncertain'].includes(result?.status)||session.unknown==='prepare')section.append(datasetButton('Inspect source attempt',()=>inspect('inspect'),busy||session.pending),datasetButton('Cancel source attempt',()=>inspect('cancel'),busy||session.pending),datasetButton('Reconcile source attempt',()=>inspect('reconcile'),busy||session.pending));
  section.append(datasetNode('p','Closing keeps these recovery coordinates in this report window. Source control never restarts preparation.','metadata'));
 }
 section.append(datasetButton(session.custody||session.hasUnsettledFilterLookups?'Close chart setup':'Cancel chart setup',cancel,busy||session.pending||[...session.filterLookups.values()].some(lookup=>lookup.pending)));parent.append(section);
}
