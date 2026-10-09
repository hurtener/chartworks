import {INVALID_REQUEST, UNAVAILABLE} from './error-codes.js';
import {node as filterNode, button as filterAction} from './dom.js';
import {copyData,appError} from './model.js';
import {inclusiveFilterRange,displayFilterRange,selectionFilterValue} from './filters.js';
import {columnInputState,columnInputValue,columnScalar} from './column-filters.js';

export function filterInputState(parameter,value){
 if(parameter.type?.startsWith('column_'))return columnInputState(parameter,value);
 const state={type:parameter.type,listLength:parameter.list_length,items:[],start:'',end:'',query:'',literal:''};
 if(parameter.type==='dimension_set')state.items=[...(value?.items||[])];
 else if(parameter.type==='dimension_value'&&value&&Object.hasOwn(value,'literal'))state.items=[value.literal];
 else if(parameter.type==='date_range'&&value?.date_range)Object.assign(state,displayFilterRange(value));
 else state.literal=parameter.type==='relative_period'?JSON.stringify(value?.period??{}):parameter.type.endsWith('_list')?JSON.stringify(value?.items??[]):value?.literal??'';
 return state;
}
export function filterInputValue(state){
 if(state.type?.startsWith('column_'))return columnInputValue(state);
 if(state.type==='date_range')return inclusiveFilterRange(state.start,state.end);
 if(['dimension_value','dimension_set'].includes(state.type))return selectionFilterValue(state.items,state.type==='dimension_set');
 if(state.type==='relative_period'||state.type.endsWith('_list')){let v;try{v=JSON.parse(state.literal);}catch{throw appError(INVALID_REQUEST);}if(state.type==='relative_period'){if(!v||typeof v!=='object'||Array.isArray(v))throw appError(INVALID_REQUEST);return {period:copyData(v)};}if(!Array.isArray(v)||v.length!==state.listLength||v.some(x=>typeof x!=='string'))throw appError(INVALID_REQUEST);return {items:[...v]};}
 if(typeof state.literal!=='string')throw appError(INVALID_REQUEST);return {literal:state.literal};
}
// The owner retains this small stage across redraws. Only Done hands a cloned
// canonical value to its owner; this module cannot mutate a report or start a run.
export function renderFilterInput(parent,state,{label='Filter value',lookup=null,disabled=false,onSearch,onInspect,onDone,onCancel,editPolicy=false}={}){
 const box=filterNode('fieldset');box.className='filter-value-editor';box.disabled=disabled;box.append(filterNode('legend',label));let done;
 const changed=()=>{if(!done)return;try{filterInputValue(state);done.disabled=disabled;}catch{done.disabled=true;}};
 if(state.type?.startsWith('column_')){
  renderColumnInput(box,state,{disabled,editPolicy,changed,redraw:()=>render()});
 }else if(state.type==='date_range'){
  for(const [key,title]of [['start','Start date'],['end','End date · inclusive']]){const row=filterNode('label',title),input=filterNode('input');input.type='date';input.value=state[key];input.setAttribute('aria-label',title);input.min='0001-01-01';input.max=key==='end'?'9999-12-30':'9999-12-31';input.addEventListener('input',()=>{state[key]=input.value;changed();});row.append(input);box.append(row);}
  box.append(filterNode('p','Calendar dates, including both selected days. Saved as start inclusive / following day exclusive; no local time-zone conversion.'));
 }else if(['dimension_value','dimension_set'].includes(state.type)){
  const multiple=state.type==='dimension_set';
  const selected=filterNode('div');selected.className='filter-selected-values';
  for(const value of state.items)selected.append(filterAction(`${value===''?'Empty text':value} · remove`,()=>{state.items=state.items.filter(x=>x!==value);render();},disabled));
  const count=filterNode('p',multiple?`${state.items.length} of 16 selected · at least one required`:'Choose one value');box.append(selected,count);
  const search=filterNode('label','Find values'),input=filterNode('input');input.type='search';input.value=state.query;input.maxLength=256;input.setAttribute('aria-label','Find values');input.addEventListener('input',()=>{state.query=input.value;});search.append(input);box.append(search);
  box.append(filterAction('Search options',()=>onSearch?.(state.query,''),disabled||!onSearch||lookup&&!lookup.canSearch()));
  box.append(filterNode('p','Search reads approved source data and may incur cost. It searches the full field population without changing this selection or running the report. Other chart filters are not applied to option search.'));
  if(!lookup)box.append(filterNode('p',onSearch?'Search to load approved field values.':'Option search is unavailable until this filter has a saved, authorized data binding.'));
  if(lookup?.pending)box.append(filterNode('p','Reading options…'));
  if(lookup?.result?.values_available){
   const choices=filterNode('div'),group='filter-choice-'+crypto.randomUUID();choices.className='filter-option-list';
   if(!lookup.values.length)choices.append(filterNode('p','No matching values.'));
   for(const option of lookup.values){const row=filterNode('label'),choice=filterNode('input');choice.type=multiple?'checkbox':'radio';choice.name=group;choice.value=option.value;choice.checked=state.items.includes(option.value);choice.disabled=disabled||multiple&&!choice.checked&&state.items.length>=16;choice.addEventListener('change',()=>{state.items=multiple?(choice.checked?[...state.items,option.value]:state.items.filter(x=>x!==option.value)):[option.value];render(option.value);});row.append(choice,filterNode('span',option.value===''?'Empty text':option.label));choices.append(row);}box.append(choices);
   if(!lookup.result.complete)box.append(filterAction('Next options',()=>onSearch?.(lookup.request.search,lookup.result.next),disabled||!lookup.canSearch()));
  }else if(lookup?.request&&!lookup.pending){
   box.append(filterNode('p',lookup.result?.status==='unsupported'?'This approved binding does not support option search.':lookup.result?.status==='failed'&&lookup.result?.new_operation_allowed?`Option lookup failed (${lookup.result.code||UNAVAILABLE}). No values were applied. Check the field or narrow the search before another explicit read.`:lookup.result?.new_operation_allowed?'This lookup finished, but its values are not retained. Search again explicitly for a new page.':'The lookup outcome is unconfirmed. Inspect or reconcile it before starting another search.'));
   for(const [action,title]of [['','Inspect lookup'],['cancel','Cancel lookup'],['reconcile','Reconcile lookup']])if(onInspect)box.append(filterAction(title,()=>onInspect(action),disabled||lookup.pending));
  }
 }else{const input=filterNode('input');input.type=state.type==='date'?'date':'text';input.value=state.literal;input.maxLength=4096;input.setAttribute('aria-label',label);input.addEventListener('input',()=>{state.literal=input.value;changed();});box.append(input);}
 done=filterAction('Done',()=>onDone?.(copyData(filterInputValue(state))),disabled);done.className='primary';const actions=filterNode('div');actions.className='filter-editor-actions';actions.append(done,filterAction('Cancel',()=>onCancel?.(),disabled));box.append(actions);changed();
 const render=value=>{const holder=filterNode('div'),next=renderFilterInput(holder,state,{label,lookup,disabled,onSearch,onInspect,onDone,onCancel,editPolicy});box.replaceWith(next);const inputs=Array.from(next.querySelectorAll('input'));(inputs.find(input=>value!==undefined&&['checkbox','radio'].includes(input.type)&&input.value===value)||inputs[0])?.focus?.({preventScroll:true});};parent.append(box);return box;
}

function renderColumnInput(box,state,{disabled,editPolicy,changed,redraw}){
 const kind=state.column.type,temporal=['date','timestamp','instant'].includes(kind);
 const input=(title,key,type='text')=>{
  const row=filterNode('label',title),field=filterNode('input');field.type=type;field.value=state[key];field.maxLength=kind==='number'||kind==='integer'?256:4096;field.setAttribute('aria-label',title);
  if(['number','integer'].includes(kind))field.inputMode='decimal';
  if(type==='date'){field.min='0001-01-01';field.max=key==='end'?'9999-12-30':'9999-12-31';}
  if(type==='datetime-local')field.step='0.000001';
  field.addEventListener('input',()=>{state[key]=field.value;changed();});row.append(field);box.append(row);return field;
 };
 if(temporal){
  box.append(filterNode('p',kind==='instant'?'Gregorian calendar. Times use the selected timezone; ambiguous or nonexistent local times are rejected when preparing or running.':'Gregorian calendar. Civil dates and times have no timezone conversion.'));
  if(kind==='instant'){
   if(editPolicy){const row=filterNode('label','Filter timezone'),zone=filterNode('input');zone.value=state.column.timezone||'';zone.maxLength=128;zone.placeholder='Choose an IANA timezone';zone.setAttribute('aria-label','Filter timezone');zone.addEventListener('input',()=>{state.column.timezone=zone.value;changed();});row.append(zone);box.append(row);}
   else box.append(filterNode('p','Timezone: '+state.column.timezone));
  }
 }
 if(state.type==='column_range'){
  const day=kind==='date',time=['timestamp','instant'].includes(kind);
  input(day?'Start date':'Lower bound · inclusive','start',day?'date':time?'datetime-local':'text');
  input(day?'End date · inclusive':'Upper bound · exclusive','end',day?'date':time?'datetime-local':'text');
  box.append(filterNode('p',day?'Both selected days are included.':'The lower bound is included; the upper bound is excluded.'));
 }else if(kind==='boolean'){
  const multiple=state.type==='column_set',group='boolean-'+crypto.randomUUID();
  for(const value of ['true','false']){const row=filterNode('label'),choice=filterNode('input');choice.type=multiple?'checkbox':'radio';choice.name=group;choice.value=value;choice.checked=multiple?state.items.includes(value):state.literal===value;choice.setAttribute('aria-label',value==='true'?'True':'False');choice.addEventListener('change',()=>{if(multiple)state.items=choice.checked?[...state.items,value]:state.items.filter(v=>v!==value);else state.literal=value;changed();});row.append(choice,filterNode('span',value==='true'?'True':'False'));box.append(row);}
 }else if(state.type==='column_value'){
  input('Exact value','literal');if(kind==='text')box.append(filterNode('p','An empty value matches empty text. It does not match missing values.'));
 }else{
  const selected=filterNode('div');selected.className='filter-selected-values';
  for(const value of state.items)selected.append(filterAction(`${value===''?'Empty text':value} · remove`,()=>{state.items=state.items.filter(x=>x!==value);redraw();},disabled));
  box.append(selected,filterNode('p',`${state.items.length} of 16 selected · at least one required`));
  const field=input('Exact value to add','literal'),add=filterAction('Add value',()=>{try{columnScalar(kind,state.literal);if(state.items.length>=16||state.items.includes(state.literal))return;state.items.push(state.literal);state.literal='';redraw();}catch{}},disabled);
  const update=()=>{try{columnScalar(kind,state.literal);add.disabled=disabled||state.items.length>=16||state.items.includes(state.literal);}catch{add.disabled=true;}};field.addEventListener('input',update);update();box.append(add);
 }
 if(['integer','number'].includes(kind))box.append(filterNode('p','Numbers keep all entered digits. Missing values are excluded.'));
}
