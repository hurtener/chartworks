import {TARGET_ALLOCATION_UNAVAILABLE} from './allocation.js';
import {DATASET_CHART_KINDS,datasetFilterCapability} from './dataset.js';
import {displayFilterRange} from './filters.js';
import {renderFilterInput} from './filter-controls.js';
function datasetNode(tag,text,cls){const element=document.createElement(tag);if(text!==undefined)element.textContent=String(text);if(cls)element.className=cls;return element;}
function datasetButton(label,callback,disabled=false){const b=datasetNode('button',label);b.type='button';b.disabled=disabled;b.addEventListener('click',callback);return b;}
function datasetInput(label,value,callback,{disabled=false,number=false}={}){const l=datasetNode('label',label),i=datasetNode('input');i.type=number?'number':'text';i.value=value;i.defaultValue=i.value;i.maxLength=number?4:256;i.disabled=disabled;i.setAttribute('aria-label',label);i.addEventListener('change',()=>callback(number?Number(i.value):i.value));l.append(i);return l;}
function datasetSelect(label,items,value,callback,disabled=false){const l=datasetNode('label',label),s=datasetNode('select');s.setAttribute('aria-label',label);s.disabled=disabled;for(const item of items){const o=datasetNode('option',item.label);o.value=item.value;o.selected=item.value===value;o.disabled=!!item.disabled;s.append(o);}s.addEventListener('change',()=>callback(s.value));l.append(s);return l;}
const filterKindLabel={select:'One value',multi_select:'Multiple values',date_range:'Date range'};
function datasetFilterSummary(filter){
 if(!filter.default)return 'Choose a required default';
 if(filter.kind==='date_range'){const range=displayFilterRange(filter.default);return `Default: ${range.start} to ${range.end}, inclusive`;}
 const values=filter.kind==='select'?[filter.default.literal]:filter.default.items;
 return 'Default: '+values.map(value=>value===''?'Empty text':value).join(', ');
}
function renderDatasetFilters(parent,session,{locked,busy,canAllocate,change,searchFilter,inspectFilter}){
 const view=session.view,filters=session.draft.filters||[],section=datasetNode('section',undefined,'dataset-filters');
 section.append(datasetNode('h3','Chart filters'),datasetNode('p','Choose up to four reviewed fields and a required default for each. These defaults are saved with the new chart; temporary report selections are separate.','metadata'));
 const availability=field=>{
  const capability=datasetFilterCapability(view,field.id);
  if(!capability?.supported)return capability?.reason?.replaceAll('_',' ')||'No reviewed filter capability is available';
  if(capability.kinds.includes('select'))return !capability.option_lookup?'Governed option search is unavailable for this field':!searchFilter?'This host has not enabled governed option search':!canAllocate?'This host cannot prepare a private chart target for option search':'';
  return '';
 };
 if(!view.filter_capabilities)section.append(datasetNode('p','Filter creation is unavailable for this reviewed dataset version. You can still prepare an unfiltered chart.','metadata'));
 else{
  const selection=session.filterSelection,selected=view.dimensions.find(d=>d.id===selection.dimension),capability=datasetFilterCapability(view,selection.dimension);
  section.append(datasetSelect('Filter field',[{value:'',label:'Choose a filter field'},...view.dimensions.map(field=>{const reason=availability(field),used=filters.some(f=>f.dimension===field.id);return {value:field.id,label:`${field.name}${used?' · already added':reason?' · unavailable: '+reason:''}`,disabled:used||!!reason};})],selection.dimension,value=>{if(locked)return;selection.dimension=value;selection.kind=datasetFilterCapability(view,value)?.kinds[0]||'';change();},locked||filters.length>=4));
  section.append(datasetSelect('Filter type',(capability?.kinds||[]).map(kind=>({value:kind,label:filterKindLabel[kind]})),selection.kind,value=>{if(locked)return;selection.kind=value;change();},locked||!selected||filters.length>=4),datasetButton('Add chart filter',()=>{session.addFilter(selection.dimension,selection.kind);change();},locked||filters.length>=4||!selected||!!availability(selected)||!capability?.kinds.includes(selection.kind)));
  section.append(datasetNode('p',`${filters.length} of 4 filters added`,'metadata'));
  const unavailable=view.dimensions.filter(field=>availability(field));
  if(unavailable.length){const details=datasetNode('details');details.append(datasetNode('summary','Unavailable filter fields'));for(const field of unavailable)details.append(datasetNode('p',`${field.name}: ${availability(field)}.`,'metadata'));section.append(details);}
 }
 for(const filter of filters){
  const field=view.dimensions.find(d=>d.id===filter.dimension),capability=datasetFilterCapability(view,filter.dimension),lookup=session.filterLookups.get(filter.dimension),stage=session.filterStages.get(filter.dimension),row=datasetNode('section',undefined,'dataset-filter');
  row.append(datasetNode('h4',field?.name||filter.dimension),datasetSelect(`Filter type: ${field?.name||filter.dimension}`,(capability?.kinds||[]).map(kind=>({value:kind,label:filterKindLabel[kind]})),filter.kind,value=>{session.changeFilterKind(filter.dimension,value);change();},locked),datasetNode('p',datasetFilterSummary(filter),'metadata'));
  row.append(datasetButton(`Remove filter: ${field?.name||filter.dimension}`,()=>{session.removeFilter(filter.dimension);change();},locked));
  if(stage&&!lookup?.unknown)renderFilterInput(row,stage,{label:`Default for ${field?.name||filter.dimension}`,lookup,disabled:locked,onSearch:capability?.option_lookup&&canAllocate&&searchFilter?(search,cursor)=>searchFilter(filter.dimension,search,cursor):undefined,onInspect:inspectFilter?action=>inspectFilter(filter.dimension,action):undefined,onDone:value=>{session.commitFilter(filter.dimension,value);change();},onCancel:()=>{session.cancelFilter(filter.dimension);change();}});
  else if(!stage)row.append(datasetButton(`Choose default: ${field?.name||filter.dimension}`,()=>{session.beginFilter(filter.dimension);change();},locked));
  if(stage&&lookup?.unknown)row.append(datasetNode('p','The default selection is kept locally. Settle this option lookup below before continuing.','metadata'));
  section.append(row);
 }
 if(session.filterStages.size)section.append(datasetNode('p','Choose Done or Cancel for each open default editor before preparing the chart.','metadata'));
 parent.append(section);
}
function renderDatasetLookupRecovery(parent,session,{busy,inspectFilter}){
 for(const [dimension,lookup]of session.filterLookups){
  if(!lookup.pending&&!lookup.unknown)continue;
  const section=datasetNode('section',undefined,'dataset-option-recovery'),field=session.view?.dimensions.find(d=>d.id===dimension);
  section.append(datasetNode('h3',`Option lookup: ${field?.name||dimension}`),datasetNode('p',lookup.pending?'Reading governed options…':'The option lookup outcome is unconfirmed. Inspect, cancel, or reconcile this original lookup before preparing a chart or changing datasets.','notice'));
  if(inspectFilter)for(const [action,label]of [['','Inspect lookup'],['cancel','Cancel lookup'],['reconcile','Reconcile lookup']])section.append(datasetButton(label,()=>inspectFilter(dimension,action),busy||session.pending||lookup.pending));
  else section.append(datasetNode('p','This host has not enabled option lookup recovery. Resume this setup in a host with the original-operation controls.','metadata'));
  section.append(datasetNode('p','Status checks never repeat the source query. Closing keeps the original lookup available from Resume chart setup.','metadata'));parent.append(section);
 }
}
export function renderDatasetEditor(parent,session,{busy=false,allocation=null,canAllocate=false,resume,change,read,prepare,create,recover,inspect,cancel,searchFilter,inspectFilter}){
 const section=datasetNode('section',undefined,'dataset-editor');section.append(datasetNode('h2','Create from dataset'),datasetNode('p','Choose reviewed fields, then deliberately prepare their actual data shape.','metadata'));
 const locked=busy||session.locked||!!allocation?.pending||!!allocation?.unknown,edit=fn=>{session.edit(fn);change();};
 if(!session.custody){
  section.append(datasetButton('Refresh topics',()=>read(()=>session.loadTopics()),locked));
  for(const topic of session.topics){const button=datasetButton(topic.name||topic.topic,()=>read(()=>session.selectTopic(topic)),locked);button.className='dataset-topic';button.setAttribute('aria-pressed',String(session.publication?.topic===topic.topic));section.append(button);}if(session.next)section.append(datasetButton('More topics',()=>read(()=>session.loadTopics(session.next)),locked));
  if(!session.topics.length)section.append(datasetNode('p','No reviewed topics are visible under current access.','metadata'));
  if(session.publication)section.append(datasetSelect('Reviewed dataset',[{value:'',label:'Choose dataset'},...session.datasets.map(d=>({value:d.id,label:d.name||d.id}))],session.view?.dataset||'',id=>{if(id)read(()=>session.selectDataset(id));},locked));
 }
 const view=session.view,draft=session.draft;
 if(view){
  section.append(datasetNode('p',`${session.publication?.name||view.topic.topic} · ${session.datasets.find(d=>d.id===view.dataset)?.name||view.dataset} · ${view.topic.version}`,'metadata'));
  if(!view.supported)section.append(datasetNode('p',`This dataset cannot be prepared here: ${view.reason||'unsupported'}.`,'notice error'));
  section.append(datasetInput('Chart title',draft.title,value=>edit(d=>{d.title=value;}),{disabled:locked}),datasetSelect('New chart type',view.chart_kinds.filter(k=>DATASET_CHART_KINDS.includes(k)).map(k=>({value:k,label:k.replaceAll('_',' ')})),draft.kind,value=>edit(d=>{d.kind=value;}),locked));
  const dimensions=datasetNode('fieldset');dimensions.disabled=locked;dimensions.append(datasetNode('legend',draft.kind==='kpi'?'Dimensions · KPI uses no grouping':'Dimensions · choose up to two'));
  for(const field of view.dimensions){const label=datasetNode('label',undefined,'dataset-field'),input=datasetNode('input');input.type='checkbox';input.checked=draft.dimensions.includes(field.id);input.disabled=locked||!field.supported||!input.checked&&(draft.kind==='kpi'||draft.dimensions.length>=2);input.setAttribute('aria-label',`Dimension: ${field.name}`);input.addEventListener('change',()=>edit(d=>{d.dimensions=input.checked?[...d.dimensions,field.id]:d.dimensions.filter(id=>id!==field.id);}));label.append(input,datasetNode('span',`${field.name}${field.supported?'':` · unavailable: ${field.reason}`}`));dimensions.append(label);}section.append(dimensions);
  section.append(datasetSelect('Reviewed measure',[{value:'',label:'Choose one measure'},...view.measures.map(m=>({value:m.id,label:`${m.name} · ${m.aggregation}${m.unit?' · '+m.unit:''}${m.supported?'':' · unavailable: '+m.reason}`,disabled:!m.supported}))],draft.measure,value=>edit(d=>{d.measure=value;}),locked));
  renderDatasetFilters(section,session,{locked,busy,canAllocate,change,searchFilter,inspectFilter});
  if(draft.kind==='table')section.append(datasetInput('New table page size',draft.pageSize,value=>edit(d=>{d.pageSize=value;}),{disabled:locked,number:true}));
  section.append(datasetNode('p','Aggregation and units come from the reviewed measure. Line and area charts need a temporal first dimension; pie and donut use one dimension.','metadata'));
  const limits=datasetNode('details');limits.append(datasetNode('summary','Supported preparation scope'));for(const limitation of view.limitations)limits.append(datasetNode('p',limitation,'metadata'));section.append(limits);
  if(!session.custody)section.append(datasetNode('p',!canAllocate?TARGET_ALLOCATION_UNAVAILABLE:allocation?.unknown?'Resume preparation asks your host for the same chart target before preparing data.':'Your host will prepare a private chart target when you prepare.','metadata'),datasetButton('Prepare chart',prepare,locked||!canAllocate||!session.intentValid()),datasetNode('p','Prepare runs one bounded source read. Option search is a separate explicit read; editing fields and defaults does not query data.','metadata'));
 }
 if(allocation?.unknown)section.append(datasetNode('p','Chart target creation is unconfirmed. Resume asks for the original target without reading source data.','notice'),datasetButton('Resume chart creation',resume,busy||allocation.pending||!canAllocate));
 renderDatasetLookupRecovery(section,session,{busy,inspectFilter});
 const result=session.preparation;
 if(result)section.append(datasetNode('p',`Preparation: ${result.status}${result.code?' · '+result.code:''}`,'notice'));
 if(session.custody){
  const details=datasetNode('details');details.append(datasetNode('summary','Preparation and recovery details'),datasetNode('p',`Target: ${session.custody.new_block}`,'metadata'),datasetNode('p',`Operation: ${session.custody.operation}`,'metadata'));if(session.custody.preparation)details.append(datasetNode('p',`Preparation: ${session.custody.preparation}`,'metadata'));if(result?.expires_at)details.append(datasetNode('p',`Expires: ${result.expires_at}`,'metadata'));if(result?.execution_status||result?.remote_state)details.append(datasetNode('p',`Source attempt: ${result.execution_status||'unknown'} · ${result.remote_state||'unknown'}`,'metadata'));section.append(details);
  if(session.unknown)section.append(datasetNode('p',session.unknown==='create'?'Chart creation outcome is unknown. Inspect exact preparation custody before recovery.':'Preparation outcome is unknown or pending. Inspect the original operation; it will not run again.','notice error'));
  if(result?.status==='prepared')section.append(datasetNode('p',`Actual schema checked: ${result.schema.length} fields. Native chart validation is still required.`, 'metadata'),datasetButton('Create private chart',create,!session.canCreate()||busy));
  if(session.canRecover())section.append(datasetButton('Recover created chart',recover,busy));
  section.append(datasetButton('Inspect preparation',()=>inspect(''),busy||session.pending));
  if(['accepted','uncertain'].includes(result?.status)||session.unknown==='prepare')section.append(datasetButton('Inspect source attempt',()=>inspect('inspect'),busy||session.pending),datasetButton('Cancel source attempt',()=>inspect('cancel'),busy||session.pending),datasetButton('Reconcile source attempt',()=>inspect('reconcile'),busy||session.pending));
  section.append(datasetNode('p','Closing keeps these recovery coordinates in this report window. Source control never restarts preparation.','metadata'));
 }
 section.append(datasetButton(session.custody||session.hasUnsettledFilterLookups?'Close chart setup':'Cancel chart setup',cancel,busy||session.pending||[...session.filterLookups.values()].some(lookup=>lookup.pending)));parent.append(section);
}
