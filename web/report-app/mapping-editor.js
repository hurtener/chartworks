import {node as mapNode, button as mapButton, selectField, textField as mapInput} from './dom.js';
import {renderFormattingFields} from './formatting.js';
import {TARGET_ALLOCATION_UNAVAILABLE} from './allocation.js';
import {bindingCandidates, mappingColumns, mappingVariants, mappingVariant} from './mapping.js';
function mapSelect(label,value,choices,change){return selectField(label,choices,value,change);}
function mapCheck(label,value,change){const wrap=mapNode('label',undefined,'mapping-check'),input=mapNode('input');input.type='checkbox';input.checked=!!value;input.setAttribute('aria-label',label);input.addEventListener('change',()=>change(input.checked));wrap.append(input,mapNode('span',label));return wrap;}
const mapTitle=kind=>kind.replaceAll('_',' ').replace(/^./,s=>s.toUpperCase());
const mapColumnLabel=c=>`${c.display_label||c.name||c.id} · ${c.type}${c.aggregation?' · '+c.aggregation:''}${c.format?.unit?' · '+c.format.unit:''}${c.format?.currency?' · '+c.format.currency:''}`;
function mapTable(d){if(!d.table)d.table={columns:d.bindings.columns.map(column=>({column,visible:true})),page_size:100,show_totals:false};return d.table;}
function mapSyncTable(d){if(d.table){const old=new Map(d.table.columns.map(c=>[c.column,c.visible]));d.table.columns=d.bindings.columns.map(column=>({column,visible:old.get(column)??true}));}}
function mapKPI(d){if(!d.kpi)d.kpi={value_row:'first',comparison_mode:'none',show_delta:false,show_percent_delta:false,show_target_difference:false,sparkline:false,thresholds:[]};return d.kpi;}

// Metadata controls only. No sample rows, canvas renderer or execution lives here.
export function renderMappingEditor(parent,session,{busy=false,allocation=null,canAllocate=false,resume,change,save,cancel,inspect}){
 const formatting=session.purpose==='presentation',editor=mapNode('section',undefined,'mapping-editor');editor.append(mapNode('h2',formatting?'Field formatting':'Edit chart'));
 if(!session.view){editor.append(mapNode('p','Loading exact chart metadata…','metadata'));parent.append(editor);return;}
 const b=session.view.block,draft=session.draft,columns=mappingColumns(session.view,session.output),edit=fn=>{session.edit(fn);change();};
 editor.append(mapNode('p',`${b.state.id} · revision ${b.revision} · ${session.output}`,'metadata'),mapNode('p','Save keeps all staged edits in an unvalidated private revision. Cancel discards them. Only explicit validation or preview runs source data.','metadata'));
 if(session.unknown)editor.append(mapNode('p','The save outcome is unknown. Inspect metadata and reconcile through your host before retrying.','notice error'));
 if(session.conflict)editor.append(mapNode('p','This chart changed elsewhere. Close these edits and reopen its current exact reference before trying again.','notice error'));
 const locked=busy||session.pending,blocked=locked||session.unknown||session.conflict||!!allocation?.unknown,actions=[mapButton(formatting?'Save formatting':'Save chart',save,blocked||!session.dirty||!session.valid()||session.copy&&!canAllocate),mapButton(session.unknown?'Close chart editor':formatting?'Cancel formatting':'Cancel chart edits',cancel,locked)];if(formatting)editor.append(...actions);
 const fields=mapNode('fieldset');fields.disabled=blocked;
 if(formatting)renderFormattingFields(fields,session,change);else{
 const display=mapNode('details',undefined,'mapping-display');display.open=session.displaySettingsOpen===true;session.displaySettingsElement=display;display.addEventListener('toggle',()=>{if(!session.closed&&session.displaySettingsElement===display)session.displaySettingsOpen=display.open;});display.append(mapNode('summary','Display settings'),mapInput('Chart title',draft.options.title,value=>edit(d=>{d.options.title=value;}),{maxLength:512}),mapCheck('Show legend',draft.options.legend.visible,value=>edit(d=>{d.options.legend.visible=value;})),mapSelect('Legend position',draft.options.legend.position,['top','bottom','left','right'].map(value=>({value,label:mapTitle(value)})),value=>edit(d=>{d.options.legend.position=value;})),mapInput('Maximum displayed label characters',draft.options.label_max_runes,value=>edit(d=>{d.options.label_max_runes=Number(value);}),{type:'number',min:1,max:1024}),mapNode('p','Shortened display labels keep their complete text and exact values in retained details.','metadata'));
 fields.append(display,mapNode('p','Use Field formatting separately for supported table headers and numeric precision. Saved reviewed units and aggregation are preserved.','metadata'));
 fields.append(mapSelect('Chart type',draft.kind,session.catalog.kinds.map(e=>({value:e.kind,label:mapTitle(e.kind)})),kind=>{session.variant=null;edit(d=>{d.kind=kind;d.bindings=kind==='table'?{columns:[]}:{};d.order=[];delete d.kpi;delete d.table;if(kind==='kpi')mapKPI(d);if(kind==='table')mapTable(d);});}));
 const variants=mappingVariants(session.catalog,draft.kind),selected=variants.some(v=>v.id===session.variant)?session.variant:mappingVariant(session.catalog,draft);
 if(variants.length>1)fields.append(mapSelect('Binding variant',selected,variants.map(v=>({value:v.id,label:mapTitle(v.id)})),id=>{session.variant=id;edit(d=>{const variant=variants.find(v=>v.id===id),allowed=[...variant.required_slots,...(variant.optional_slots||[])];for(const slot of Object.keys(d.bindings))if(!allowed.includes(slot))delete d.bindings[slot];d.order=d.order.filter(o=>Object.values(d.bindings).flat().includes(o.column));});}));
 const variant=variants.find(v=>v.id===selected),slots=[...variant.required_slots,...(variant.optional_slots||[])];
 if(draft.kind==='kpi'&&draft.kpi){if(draft.kpi.sparkline)slots.push('category');if(draft.kpi.comparison_mode==='comparison_column')slots.push('comparison');if(draft.kpi.show_target_difference)slots.push('target');}
 for(const slot of [...new Set(slots)]){
  const eligible=bindingCandidates(columns,draft.kind,slot),label={category:'Category / time',value:'Value',values:'Measures',series:'Series',x:'X field',y:'Y field',columns:'Table columns',hierarchy:'Hierarchy',parent:'Parent',size:'Bubble size',comparison:'Comparison value',target:'Target value'}[slot]||slot;
  if(['values','columns','hierarchy'].includes(slot)){
   const list=mapNode('div',undefined,'mapping-column-list');list.append(mapNode('h3',label));const selectedIDs=draft.bindings[slot]||[];
   selectedIDs.forEach((id,index)=>{const row=mapNode('div',undefined,'mapping-column-row'),c=columns.find(c=>c.id===id);row.append(mapNode('span',c?mapColumnLabel(c):id),mapButton(`Move ${label} ${index+1} up`,()=>edit(d=>{const ids=d.bindings[slot];[ids[index-1],ids[index]]=[ids[index],ids[index-1]];mapSyncTable(d);}),index===0),mapButton(`Move ${label} ${index+1} down`,()=>edit(d=>{const ids=d.bindings[slot];[ids[index+1],ids[index]]=[ids[index],ids[index+1]];mapSyncTable(d);}),index===selectedIDs.length-1),mapButton(`Remove ${label} ${index+1}`,()=>edit(d=>{d.bindings[slot].splice(index,1);d.order=d.order.filter(o=>o.column!==id);mapSyncTable(d);})));if(slot==='columns')row.append(mapCheck(`Show column ${c?.display_label||c?.name||id}`,draft.table?.columns.find(c=>c.column===id)?.visible??true,visible=>edit(d=>{mapTable(d).columns.find(c=>c.column===id).visible=visible;})));list.append(row);});
   list.append(mapSelect('Add '+label,'',[{value:'',label:'Choose field'},...eligible.filter(c=>!selectedIDs.includes(c.id)).map(c=>({value:c.id,label:mapColumnLabel(c)}))],id=>{if(id)edit(d=>{d.bindings[slot]||=[];d.bindings[slot].push(id);mapSyncTable(d);});}));fields.append(list);
  }else fields.append(mapSelect(label,draft.bindings[slot]||'',[{value:'',label:'Choose field'},...eligible.map(c=>({value:c.id,label:mapColumnLabel(c)}))],id=>edit(d=>{if(id)d.bindings[slot]=id;else delete d.bindings[slot];d.order=d.order.filter(o=>Object.values(d.bindings).flat().includes(o.column));})));
 }
 if(draft.kind==='kpi'){
  fields.append(mapNode('p','Aggregation and units come from the saved field metadata. The KPI selects a retained value; it does not invent an aggregate.','metadata'));
  fields.append(mapSelect('KPI value row',draft.kpi?.value_row||'first',[{value:'first',label:'First row'},{value:'last',label:'Last row'}],value=>edit(d=>{mapKPI(d).value_row=value;})),mapSelect('KPI comparison',draft.kpi?.comparison_mode||'none',[{value:'none',label:'None'},{value:'previous_row',label:'Previous ordered row'},{value:'comparison_column',label:'Comparison field'}],value=>edit(d=>{mapKPI(d).comparison_mode=value;if(value!=='comparison_column')delete d.bindings.comparison;})));
  for(const [key,label] of [['show_delta','Show delta'],['show_percent_delta','Show percent delta'],['show_target_difference','Show target difference'],['sparkline','Show sparkline']])fields.append(mapCheck(label,draft.kpi?.[key],value=>edit(d=>{mapKPI(d)[key]=value;if(!value&&key==='show_target_difference')delete d.bindings.target;if(!value&&key==='sparkline')delete d.bindings.category;})));
  if(draft.kpi?.thresholds?.length)fields.append(mapNode('p',`${draft.kpi.thresholds.length} saved thresholds are preserved.`, 'metadata'));
 }
 if(draft.kind==='table')fields.append(mapInput('Table page size',draft.table?.page_size||100,value=>edit(d=>{mapTable(d).page_size=Number(value);}),{type:'number',min:1,max:1000}),mapCheck('Show table totals',draft.table?.show_totals,value=>edit(d=>{mapTable(d).show_totals=value;})));
 const bound=new Set(Object.values(draft.bindings).flat()),sortColumns=columns.filter(c=>bound.has(c.id));
 fields.append(mapNode('h3','Sort order'));
 draft.order.forEach((order,index)=>{const row=mapNode('div',undefined,'mapping-sort');row.append(mapSelect(`Sort field ${index+1}`,order.column,sortColumns.map(c=>({value:c.id,label:mapColumnLabel(c)})),id=>edit(d=>{d.order[index].column=id;})),mapSelect(`Sort direction ${index+1}`,order.direction,[{value:'asc',label:'Ascending'},{value:'desc',label:'Descending'}],direction=>edit(d=>{d.order[index].direction=direction;})),mapButton(`Remove sort ${index+1}`,()=>edit(d=>{d.order.splice(index,1);})));fields.append(row);});
 const nextSort=sortColumns.find(c=>!draft.order.some(o=>o.column===c.id));fields.append(mapButton('Add sort',()=>edit(d=>{d.order.push({column:nextSort.id,direction:'asc'});}),!nextSort));
 }
 if(session.copy)editor.append(mapNode('p',!canAllocate?TARGET_ALLOCATION_UNAVAILABLE:allocation?.unknown?'Resume chart creation asks your host for the same private copy.':'Saving will create a private copy through your host.','metadata'));
 editor.append(fields);
 if(formatting&&session.dirty&&!session.valid())editor.append(mapNode('p','Use headers up to 256 UTF-8 bytes and whole fraction digits from 0 to 20. Reset inherits. Save requires a presentation change.','notice'));
 if(!formatting&&!session.valid())editor.append(mapNode('p','Choose compatible fields for every required slot; each field can fill one slot. Use a plain-text title, a supported legend position and label length 1–1024.','notice'));
 if(!formatting)editor.append(...actions);
 if(allocation?.unknown)editor.append(mapButton('Resume chart creation',resume,busy||allocation.pending||!canAllocate));
 if(session.unknown)editor.append(mapButton('Inspect chart state',inspect,locked));parent.append(editor);
}
