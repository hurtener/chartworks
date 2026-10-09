import {node as datasetNode, button as datasetButton, selectField as datasetSelect, textField} from './dom.js';
import {TARGET_ALLOCATION_UNAVAILABLE} from './allocation.js';
import {DATASET_CHART_KINDS,datasetFilterCapability,datasetFilterFields} from './dataset.js';
import {displayFilterRange} from './filters.js';
import {renderFilterInput} from './filter-controls.js';
import {renderFieldPicker} from './field-picker.js';
import {chartLabel,reportDate} from '../report-viewer/presentation.js';
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
  if(!capability?.supported)return capability?.reason?.replaceAll('_',' ')||'This field cannot be used as a filter';
  if(!field.column&&capability.kinds.includes('select'))return !capability.option_lookup?'Search is not available for this field':!searchFilter?'Filter search is not available here':!canAllocate?'Save the chart before searching filter values':'';
  return '';
 };
 if(!fields.some(f=>datasetFilterCapability(view,f.id)))section.append(datasetNode('p','Filters are not available for this data. You can still create the chart.','metadata'));
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
  if(stage&&lookup?.unknown)row.append(datasetNode('p','Your selection is kept. Check the unfinished search below to continue.','metadata'));
  section.append(row);
 }
 if(session.filterStages.size)section.append(datasetNode('p','Finish choosing your filter values before previewing the chart.','metadata'));
 parent.append(section);
}
function renderDatasetLookupRecovery(parent,session,{busy,inspectFilter}){
 for(const [dimension,lookup]of session.filterLookups){
  if(!lookup.pending&&!lookup.unknown)continue;
  const section=datasetNode('section',undefined,'dataset-option-recovery'),field=session.view&&datasetFilterFields(session.view).find(d=>d.id===dimension);
  section.append(datasetNode('h3',`Find values${field?.name?': '+field.name:''}`),datasetNode('p',lookup.pending?'Finding values…':'The search has not finished. Check its status or cancel it before changing data.','notice'));
  if(inspectFilter)for(const [action,label]of [['','Check search'],['cancel','Cancel search'],['reconcile','Resolve search']])section.append(datasetButton(label,()=>inspectFilter(dimension,action),busy||session.pending||lookup.pending));
  else section.append(datasetNode('p','This search cannot be recovered here. Ask your administrator for help.','metadata'));
  section.append(datasetNode('p','Checking status will not restart the search. You can close this panel and resume chart setup later.','metadata'));parent.append(section);
 }
}
export function renderDatasetEditor(parent,session,{busy=false,allocation=null,canAllocate=false,resume,change,read,prepare,review,create,recover,inspect,cancel,searchFilter,inspectFilter}){
 const section=datasetNode('section',undefined,'dataset-editor');section.append(datasetNode('h2','Create chart'),datasetNode('p','Choose your data, pick fields, and preview your chart.','metadata'));
 const locked=busy||session.locked||!!allocation?.pending||!!allocation?.unknown,edit=fn=>{session.edit(fn);change();};
 if(!session.custody){
  if(session.tablesEnabled&&session.topicsEnabled){const tabs=datasetNode('div',undefined,'dataset-origins');tabs.setAttribute('aria-label','Data catalog');for(const [mode,label]of [['topics','Data collections'],['tables','Tables & uploads']]){const button=datasetButton(label,()=>read(()=>session.chooseCatalog(mode)),locked);button.setAttribute('aria-pressed',String(session.catalog===mode));tabs.append(button);}section.append(tabs);}
  if(session.catalog==='tables'){
   section.append(datasetButton('Refresh sources',()=>read(()=>session.loadSources()),locked));
   for(const source of session.sources){const button=datasetButton(source.name,()=>read(()=>session.selectSource(source)),locked);button.className='dataset-topic';button.setAttribute('aria-pressed',String(session.source?.id===source.id));section.append(button);}
   if(!session.sources.length)section.append(datasetNode('p',session.sourceNext?'No data sources on this page. Try the next page.':'No data sources have been shared with you yet.','metadata'));
   if(session.sourceHistory.length>1)section.append(datasetButton('Previous sources',()=>read(()=>session.loadSources(session.sourceHistory.at(-2))),locked));
   if(session.sourceNext)section.append(datasetButton('Next sources',()=>read(()=>session.loadSources(session.sourceNext)),locked));
   if(session.source){
    section.append(datasetSelect('Table or upload',[{value:'',label:'Choose a table'},...session.tables.map(item=>({value:item.relation.id,label:item.relation.name}))],session.view?.dataset||'',id=>{if(id)read(()=>session.selectTable(id));},locked));
    if(!session.tables.length)section.append(datasetNode('p',session.tableNext?'No tables on this page. Try the next page.':'No tables are available in this data source.','metadata'));
    if(session.tableHistory.length>1)section.append(datasetButton('Previous tables',()=>read(()=>session.loadTables(session.tableHistory.at(-2))),locked));
    if(session.tableNext)section.append(datasetButton('Next tables',()=>read(()=>session.loadTables(session.tableNext)),locked));
   }
  }else{
   section.append(datasetButton('Refresh collections',()=>read(()=>session.loadTopics()),locked));
   for(const topic of session.topics){const button=datasetButton(topic.name||topic.topic,()=>read(()=>session.selectTopic(topic)),locked);button.className='dataset-topic';button.setAttribute('aria-pressed',String(session.publication?.topic===topic.topic));section.append(button);}if(session.topicHistory.length>1)section.append(datasetButton('Previous collections',()=>read(()=>session.loadTopics(session.topicHistory.at(-2))),locked));if(session.next)section.append(datasetButton('Next collections',()=>read(()=>session.loadTopics(session.next)),locked));
   if(!session.topics.length)section.append(datasetNode('p','No data collections have been shared with you yet.','metadata'));
   if(session.publication)section.append(datasetSelect('Dataset',[{value:'',label:'Choose dataset'},...session.datasets.map(d=>({value:d.id,label:d.name||d.id}))],session.view?.dataset||'',id=>{if(id)read(()=>session.selectDataset(id));},locked));
  }
 }
 const view=session.view,draft=session.draft;section.append(datasetNode('p',session.custody?'3 · Preview your chart':view?'2 · Choose fields and style':'1 · Choose your data','eyebrow'));
 if(view){
  section.append(datasetNode('p',view.source_dataset?`${session.source?.name||view.source} · ${session.tables.find(d=>d.relation.id===view.dataset)?.relation.name||view.dataset}`:`${session.publication?.name||view.topic.topic} · ${session.datasets.find(d=>d.id===view.dataset)?.name||view.dataset}`,'metadata'));
  if(!(draft.fields?view.fields?.supported:view.supported))section.append(datasetNode('p','Charts cannot be created from this dataset yet. Try another dataset.','notice error'));
  section.append(datasetInput('Chart title',draft.title,value=>edit(d=>{d.title=value;}),{disabled:locked}),datasetSelect('Chart type',view.chart_kinds.filter(k=>DATASET_CHART_KINDS.includes(k)).map(k=>({value:k,label:chartLabel(k)})),draft.kind,value=>edit(d=>{d.kind=value;if(value==='kpi')d.dimensions=[];}),locked));
  if(draft.fields)renderFieldPicker(section,session,{locked,edit,change});
  else{
  const dimensions=datasetNode('fieldset');dimensions.disabled=locked;dimensions.append(datasetNode('legend',draft.kind==='kpi'?'No grouping needed':'Group by · choose up to two'));
  for(const field of view.dimensions){const label=datasetNode('label',undefined,'dataset-field'),input=datasetNode('input');input.type='checkbox';input.checked=draft.dimensions.includes(field.id);input.disabled=locked||!field.supported||!input.checked&&(draft.kind==='kpi'||draft.dimensions.length>=2);input.setAttribute('aria-label',`Group by: ${field.name}`);input.addEventListener('change',()=>edit(d=>{d.dimensions=input.checked?[...d.dimensions,field.id]:d.dimensions.filter(id=>id!==field.id);}));label.append(input,datasetNode('span',`${field.name}${field.supported?'':' · not available'}`));dimensions.append(label);}if(draft.kind!=='kpi')section.append(dimensions);
  section.append(datasetSelect('Saved calculation',[{value:'',label:'Choose a calculation'},...view.measures.map(m=>({value:m.id,label:`${m.name} · ${m.aggregation}${m.unit?' · '+m.unit:''}${m.supported?'':' · not available'}`,disabled:!m.supported}))],draft.measure,value=>edit(d=>{d.measure=value;}),locked));
  }
  renderDatasetFilters(section,session,{locked,busy,canAllocate,change,searchFilter,inspectFilter});
  if(draft.kind==='table')section.append(datasetInput('Rows per page',draft.pageSize,value=>edit(d=>{d.pageSize=value;}),{disabled:locked,number:true}));
  section.append(datasetNode('p',draft.fields?'Choose how to summarize each value. Saved calculations keep their formulas. Dates use your selected calendar and timezone.':'Saved calculations keep their formulas and units. Line charts use dates; pie and donut charts use one category.','metadata'));

  if(!session.custody)section.append(datasetNode('p',!canAllocate?TARGET_ALLOCATION_UNAVAILABLE:allocation?.unknown?'Resume to recover your existing chart setup.':'This chart stays in your draft until you publish it.','metadata'),datasetButton('Preview chart',prepare,locked||!canAllocate||!session.intentValid()),datasetNode('p','Preview loads data for your selected fields and filters. Editing the setup does not refresh the data.','metadata'));
 }
 if(allocation?.unknown)section.append(datasetNode('p','We could not confirm that the chart was saved. Resume to recover it before creating another.','notice'),datasetButton('Resume chart creation',resume,busy||allocation.pending||!canAllocate));
 renderDatasetLookupRecovery(section,session,{busy,inspectFilter});
 const result=session.preparation;
 if(result)section.append(datasetNode('p',({prepared:'Your chart data is ready.',accepted:'Loading your chart data…',uncertain:'We cannot confirm the data update yet. Check its progress before trying again.',failed:'The data could not be loaded. Review the setup before trying again.',expired:'This preview has expired. Review the setup to load the data again.',cancelled:'Data update cancelled.'})[result.status]||'Check the chart’s progress to continue.','notice'));
 if(session.custody){
  if(result?.expires_at)section.append(datasetNode('p',`Preview available until ${reportDate(result.expires_at)}.`,'metadata'));
  if(session.unknown)section.append(datasetNode('p',session.unknown==='create'?'We could not confirm that the chart was created. Check its progress before trying again.':'Your data may still be loading. Check its progress before starting again.','notice error'));
  if(result?.status==='prepared')section.append(datasetNode('p',`${result.schema.length} fields loaded. Create the chart to add it to your draft.`, 'metadata'),datasetButton('Add chart to report',create,!session.canCreate()||busy));
  if(session.canRecover())section.append(datasetButton('Recover chart',recover,busy));
  if(session.rejected)section.append(datasetNode('p',session.rejected==='preparation_operation_expired'?'The preview could not start because the request time was out of date. Check your device clock, then review the setup.':'This chart setup needs an update. Reopen the app, then review the setup.','notice error'),datasetButton('Review chart setup',()=>review(session),busy||session.pending),datasetNode('p','Reviewing the setup does not load data. Choose Preview chart when you are ready.','metadata'));
  else section.append(datasetButton('Check progress',()=>inspect(''),busy||session.pending));
  if(['accepted','uncertain'].includes(result?.status)||session.unknown==='prepare')section.append(datasetButton('Check data update',()=>inspect('inspect'),busy||session.pending),datasetButton('Cancel data update',()=>inspect('cancel'),busy||session.pending),datasetButton('Resolve data update',()=>inspect('reconcile'),busy||session.pending));
  section.append(datasetNode('p','You can close this panel and resume here. Checking progress does not start another data update.','metadata'));
 }
 section.append(datasetButton(session.custody||session.hasUnsettledFilterLookups?'Close chart setup':'Cancel chart setup',cancel,busy||session.pending||[...session.filterLookups.values()].some(lookup=>lookup.pending)));parent.append(section);
}
