import {copyData,appError} from './model.js';
import {inclusiveFilterRange,displayFilterRange,selectionFilterValue} from './filters.js';
function filterNode(tag,text){const e=document.createElement(tag);if(text!==undefined)e.textContent=String(text);return e;}
function filterAction(text,callback,disabled=false){const b=filterNode('button',text);b.type='button';b.disabled=disabled;b.addEventListener('click',callback);return b;}
export function filterInputState(parameter,value){
 const state={type:parameter.type,listLength:parameter.list_length,items:[],start:'',end:'',query:'',literal:''};
 if(parameter.type==='dimension_set')state.items=[...(value?.items||[])];
 else if(parameter.type==='dimension_value'&&value&&Object.hasOwn(value,'literal'))state.items=[value.literal];
 else if(parameter.type==='date_range'&&value?.date_range)Object.assign(state,displayFilterRange(value));
 else state.literal=parameter.type==='relative_period'?JSON.stringify(value?.period??{}):parameter.type.endsWith('_list')?JSON.stringify(value?.items??[]):value?.literal??'';
 return state;
}
export function filterInputValue(state){
 if(state.type==='date_range')return inclusiveFilterRange(state.start,state.end);
 if(['dimension_value','dimension_set'].includes(state.type))return selectionFilterValue(state.items,state.type==='dimension_set');
 if(state.type==='relative_period'||state.type.endsWith('_list')){let v;try{v=JSON.parse(state.literal);}catch{throw appError('invalid_request');}if(state.type==='relative_period'){if(!v||typeof v!=='object'||Array.isArray(v))throw appError('invalid_request');return {period:copyData(v)};}if(!Array.isArray(v)||v.length!==state.listLength||v.some(x=>typeof x!=='string'))throw appError('invalid_request');return {items:[...v]};}
 if(typeof state.literal!=='string')throw appError('invalid_request');return {literal:state.literal};
}
// The owner retains this small stage across redraws. Only Done hands a cloned
// canonical value to its owner; this module cannot mutate a report or start a run.
export function renderFilterInput(parent,state,{label='Filter value',lookup=null,disabled=false,onSearch,onInspect,onDone,onCancel}={}){
 const box=filterNode('fieldset');box.className='filter-value-editor';box.disabled=disabled;box.append(filterNode('legend',label));let done;
 const changed=()=>{if(!done)return;try{filterInputValue(state);done.disabled=disabled;}catch{done.disabled=true;}};
 if(state.type==='date_range'){
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
   box.append(filterNode('p',lookup.result?.status==='unsupported'?'This approved binding does not support option search.':lookup.result?.status==='failed'&&lookup.result?.new_operation_allowed?`Option lookup failed (${lookup.result.code||'unavailable'}). No values were applied. Check the field or narrow the search before another explicit read.`:lookup.result?.new_operation_allowed?'This lookup finished, but its values are not retained. Search again explicitly for a new page.':'The lookup outcome is unconfirmed. Inspect or reconcile it before starting another search.'));
   for(const [action,title]of [['','Inspect lookup'],['cancel','Cancel lookup'],['reconcile','Reconcile lookup']])if(onInspect)box.append(filterAction(title,()=>onInspect(action),disabled||lookup.pending));
  }
 }else{const input=filterNode('input');input.type=state.type==='date'?'date':'text';input.value=state.literal;input.maxLength=4096;input.setAttribute('aria-label',label);input.addEventListener('input',()=>{state.literal=input.value;changed();});box.append(input);}
 done=filterAction('Done',()=>onDone?.(copyData(filterInputValue(state))),disabled);done.className='primary';const actions=filterNode('div');actions.className='filter-editor-actions';actions.append(done,filterAction('Cancel',()=>onCancel?.(),disabled));box.append(actions);changed();
 const render=value=>{const holder=filterNode('div'),next=renderFilterInput(holder,state,{label,lookup,disabled,onSearch,onInspect,onDone,onCancel});box.replaceWith(next);const inputs=Array.from(next.querySelectorAll('input'));(inputs.find(input=>value!==undefined&&['checkbox','radio'].includes(input.type)&&input.value===value)||inputs[0])?.focus?.({preventScroll:true});};parent.append(box);return box;
}
