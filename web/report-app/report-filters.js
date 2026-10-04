import {STALE_VALIDATION, FORBIDDEN, LIMIT_EXCEEDED, CONFLICT} from './error-codes.js';
import {node as rfNode, button as rfButton} from './dom.js';
import {appError,authoringTool,copyData,validID} from './model.js';
import {reportPages} from './pages.js';
import {FilterOptionLookup} from './filters.js';
import {filterInputState,renderFilterInput} from './filter-controls.js';
const reportFilterKey=(page,name)=>JSON.stringify([page,name]);

function rfText(value){return value?.items?value.items.map(x=>x===''?'Empty text':x).join(', '):value?.date_range?`${value.date_range.start} until ${value.date_range.end_exclusive} (exclusive)`:value?.literal===''?'Empty text':value?.literal??'Use parameter default';}
export class ReportFilterControls{
 constructor(app){this.app=app;this.editor=null;this.lookups=new Map();this.temporary=new Map();this.accepted=null;this.defaultsOpen=false;this.defaultsElement=null;this.focusTarget=null;}
 snapshot(){return JSON.stringify([...this.temporary].sort(([a],[b])=>a.localeCompare(b)));}
 reset(){this.editor=null;this.defaultsOpen=false;this.defaultsElement=null;this.focusTarget=null;this.temporary.clear();this.accepted=null;for(const lookup of this.lookups.values()){lookup.values=[];if(lookup.result)lookup.result={...lookup.result,options:[],next:'',complete:false,values_available:false};}}
 close(){this.reset();for(const value of this.lookups.values())value.close();this.lookups.clear();}
 pageInputs(definition){return reportPages(definition).flatMap(p=>{const filters=(p.filters||[]).filter(f=>this.temporary.has(reportFilterKey(p.id,f.parameter.name))).map(f=>({name:f.parameter.name,value:copyData(this.temporary.get(reportFilterKey(p.id,f.parameter.name)))}));return filters.length?[{page:p.id,filters,overrides:[]}]:[];});}
 current(editor){const a=this.app;return this.editor===editor&&!a.closed&&editor.epoch===a.epoch&&editor.generation===a.pageGeneration;}
 openerKey(filter,page,mode){return JSON.stringify([page,filter.parameter.name,mode]);}
 active(){const e=this.editor;return !!e&&this.current(e)&&!e.suspended;}
 open(filter,page,mode){
  const a=this.app,key=this.openerKey(filter,page,mode),previous=this.editor;
  if(a.busy||a.closed||mode!=='run'&&page!==a.activePageID)return;
  if(previous&&this.current(previous)&&previous.suspended&&previous.key!==key)return;
  if(previous&&this.current(previous)&&previous.key===key){previous.suspended=false;}
  else{
   const value=mode==='default'?filter.parameter.default:this.temporary.get(reportFilterKey(page,filter.parameter.name))??filter.parameter.default;
   this.editor={filter:copyData(filter),page,mode,key,epoch:a.epoch,generation:a.pageGeneration,state:filterInputState(filter.parameter,value),lookup:null,suspended:false};
  }
  this.defaultsOpen=true;this.focusTarget={editor:this.editor};a.render();
 }
 back(editor){
  if(!this.current(editor)||editor.suspended||this.app.busy)return;
  editor.suspended=true;this.app.panelTab='components';this.focusTarget={key:editor.key};this.app.render();
 }
 cancel(editor){
  if(!this.current(editor)||editor.suspended||this.app.busy)return;
  this.editor=null;this.focusTarget={key:editor.key};this.app.render();
 }
 restoreFocus(){
  const target=this.focusTarget;if(!target)return;this.focusTarget=null;
  const element=target.editor?(this.current(target.editor)&&this.active()?target.editor.inputElement:null):Array.from(this.app.root.querySelectorAll('button')).find(b=>b.dataset.filterEdit===target.key);
  element?.focus?.();
 }
 renderInspector(parent){
  const e=this.editor;if(!this.active()||e.mode==='run')return false;
  this.defaultsElement=null;
  const section=rfNode('section');section.className='filter-inspector';section.setAttribute('aria-label','Selected business filter');
  section.append(rfButton('Back to Components',()=>this.back(e),this.app.busy),rfNode('h2',e.filter.label||e.filter.parameter.name),rfNode('p',e.mode==='default'?'Done stages this default for Save report.':'Done changes the next preview only.'));
  section.append(rfNode('p','Back keeps your edits. Cancel or Escape discards them. Editing never runs a query.'));
  section.addEventListener('keydown',event=>{if(event.key==='Escape'&&!event.isComposing&&!this.app.busy&&this.active()){event.preventDefault();event.stopPropagation();this.cancel(e);}});
  this.renderEditor(section);parent.append(section);return true;
 }
 async search(editor,search,cursor){const a=this.app;if(!this.current(editor)||editor.suspended||!a.authoring('report_options','option_status','option_control'))throw appError(FORBIDDEN);const published=editor.mode==='run';if(!published&&a.session.dirty)throw appError(CONFLICT);let report,revision,digest;
  if(published){report=a.description?.resource.target.id;revision=a.description?.resource.target.revision;digest=a.description?.definition_digest;}
  else{report=a.session.state?.id;revision=a.session.revision;const view=await a.invoke(authoringTool('read'),{report,revision});if(!this.current(editor))return;if(view.state?.id!==report||view.revision!==revision||view.state.draft_revision!==revision)throw appError(STALE_VALIDATION);digest=view.digest;}
  if(!validID(report)||!Number.isSafeInteger(revision)||! /^[a-f0-9]{64}$/.test(digest||''))throw appError(STALE_VALIDATION);
  const target={report:{policy:published?'published':'private_preview',report,revision,digest,page:editor.page,filter:editor.filter.parameter.name}},key=JSON.stringify(target);let lookup=this.lookups.get(key);if(!lookup){if(this.lookups.size>=32)throw appError(LIMIT_EXCEEDED);lookup=new FilterOptionLookup((name,args)=>a.invoke(name,args),target,{locale:a.locale});this.lookups.set(key,lookup);}editor.lookup=lookup;await lookup.search(search,cursor);
 }
 async inspect(editor,action){if(!this.current(editor)||editor.suspended||!editor.lookup)throw appError(STALE_VALIDATION);await editor.lookup.inspect(action);}
 done(editor,value){if(!this.current(editor)||editor.suspended||this.app.busy)return;const a=this.app,name=editor.filter.parameter.name;this.editor=null;this.focusTarget={key:editor.key};if(editor.mode==='default')a.editPage(d=>{if(!d.filters?.some(f=>f.parameter.name===name))throw appError(STALE_VALIDATION);d.filters.find(f=>f.parameter.name===name).parameter.default=copyData(value);},editor.page);else{this.temporary.set(reportFilterKey(editor.page,name),copyData(value));a.message='Temporary filters changed. Run or preview explicitly to update values.';a.render();}}
 renderEditor(parent){const e=this.editor,a=this.app;if(!this.active())return;const search=a.authoring('report_options','option_status','option_control')&&(e.mode==='run'?!!a.description?.definition_digest:!!a.session.state&&!a.session.dirty&&a.session.state.draft_revision===a.session.revision);const box=renderFilterInput(parent,e.state,{label:`${e.filter.label||e.filter.parameter.name} · ${e.mode==='default'?'saved default':'temporary selection'}`,lookup:e.lookup,disabled:a.busy,onSearch:search?(q,c)=>void a.perform(()=>this.search(e,q,c)):undefined,onInspect:e.lookup?action=>void a.perform(()=>this.inspect(e,action)):undefined,onDone:value=>this.done(e,value),onCancel:()=>this.cancel(e)});e.inputElement=Array.from(box.querySelectorAll('input'))[0];if(!search&&['dimension_value','dimension_set'].includes(e.state.type))parent.append(rfNode('p','Option search needs a saved current draft binding (or current published report) and a host with governed option-search support.'));}
 renderDefaults(parent,definition){const a=this.app,page=a.activePageID,section=rfNode('details'),epoch=a.epoch,generation=a.pageGeneration;section.className='filter-editor';section.open=this.defaultsOpen;this.defaultsElement=section;section.addEventListener('toggle',()=>{if(!a.closed&&a.epoch===epoch&&a.pageGeneration===generation&&this.defaultsElement===section)this.defaultsOpen=section.open;});section.append(rfNode('summary','Business filters'),rfNode('p','Saved defaults and temporary preview selections are separate. Editing never runs a query.'));
  for(const filter of definition.filters||[]){const name=filter.parameter.name,key=reportFilterKey(page,name),card=rfNode('section'),label=rfNode('label','Filter label'),input=rfNode('input');input.value=filter.label;input.maxLength=256;input.setAttribute('aria-label','Filter label');input.addEventListener('change',()=>a.editPage(d=>{d.filters.find(f=>f.parameter.name===name).label=input.value;},page));label.append(input);card.append(label,rfNode('p',`Saved: ${rfText(filter.parameter.default)}`),rfButton('Change saved default',()=>this.open(filter,page,'default'),a.busy),rfButton('Choose temporary preview value',()=>this.open(filter,page,'preview'),a.busy));
   if(this.temporary.has(key))card.append(rfNode('p',`Next preview: ${rfText(this.temporary.get(key))}`),rfButton('Use saved default for preview',()=>{this.temporary.delete(key);a.render();},a.busy));
   for(const widget of definition.widgets||[])for(const binding of widget.bindings||[])if(binding.filter===name)card.append(rfNode('p',`Applies to ${widget.presentation?.title||widget.id}`),rfButton('Remove from '+(widget.presentation?.title||widget.id),()=>a.editPage(d=>{const w=d.widgets.find(w=>w.id===widget.id);w.bindings=w.bindings.filter(b=>b.filter!==name);},page),a.busy));
   card.append(rfButton('Remove filter',()=>{if(this.editor?.page===page&&this.editor.filter.parameter.name===name)this.editor=null;this.temporary.delete(key);a.editPage(d=>{d.filters=d.filters.filter(f=>f.parameter.name!==name);for(const w of d.widgets)if(w.bindings)w.bindings=w.bindings.filter(b=>b.filter!==name);},page);},a.busy));section.append(card);
   for(const [index,mode] of ['default','preview'].entries()){
    const key=this.openerKey(filter,page,mode),button=Array.from(card.querySelectorAll('button'))[index];button.dataset.filterEdit=key;
    if(this.editor&&this.current(this.editor)&&this.editor.key!==key)button.disabled=true;
   }
  }parent.append(section);
 }
 renderRun(parent){const a=this.app,d=a.description,form=rfNode('fieldset');form.append(rfNode('legend','Run this report'),rfNode('p','A run may query approved sources and creates a new retained result.'));
  for(const f of d.filters||[]){const key=reportFilterKey(f.page,f.parameter.name),row=rfNode('div'),label=f.label||f.parameter.name;row.append(rfNode('p',`${label}: ${this.temporary.has(key)?rfText(this.temporary.get(key)):'Use published default'}`),rfButton('Choose '+label,()=>this.open(f,f.page,'run'),a.busy||!a.capabilities.can_execute));if(this.temporary.has(key))row.append(rfButton('Use published default',()=>{this.temporary.delete(key);a.render();},a.busy));form.append(row);}
  this.renderEditor(form);form.append(rfButton('Run with these filters',()=>void a.perform(()=>a.runPublished()),a.busy||!a.capabilities.can_execute||a.uncertainRun||d.dynamic));if(d.dynamic)form.append(rfNode('p','Dynamic widgets are unavailable in this manual app.'));if(!a.capabilities.can_execute)form.append(rfNode('p','Current access permits retained reads only.'));parent.append(form);
 }
 publishedInputs(){const pages=new Map();for(const f of this.app.description?.filters||[]){const value=this.temporary.get(reportFilterKey(f.page,f.parameter.name));if(value===undefined)continue;if(!pages.has(f.page))pages.set(f.page,[]);pages.get(f.page).push({name:f.parameter.name,value:copyData(value)});}return Array.from(pages,([page,filters])=>({page,filters,overrides:[]}));}
}
