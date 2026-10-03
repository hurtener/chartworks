import {boundedJSON, renderRetainedOutput} from '../report-viewer/app.js';
import {DraftSession, authoringTool, appError, copyData, manualDocument, layoutWidgets, layoutPreset, unpack, titleFor, validID} from './model.js';
import {reportPages, pageContent, upgradeReportPages, addReportPage, renameReportPage, moveReportPage, removeReportPage, unusedWidgetID} from './pages.js';
import {RetainedReport, executionFingerprint, retainedOutputs} from './retained.js';
import {canPlace, placeWidget, moveWidget, resizeWidget, duplicateWidget, firstFreeCell, arrangeWidgets} from './grid.js';
import {MappingSession, checkMappingView, validatePrivateMapping, hasFreshMappingEvidence} from './mapping.js';
import {renderMappingEditor} from './mapping-editor.js';
import {MCPReportAdapter, EmbeddedReportAdapter} from './bridge.js';

function node(tag,text,cls) { const e=document.createElement(tag);if(text!==undefined)e.textContent=String(text);if(cls)e.className=cls;return e; }
function action(label,fn,{primary=false,disabled=false}={}) { const b=node('button',label,primary?'primary':'');b.type='button';b.disabled=disabled;b.addEventListener('click',fn);return b; }
function inputField(label,value,onchange,{type='text',max=256,disabled=false}={}) { const l=node('label',label),i=node(type==='textarea'?'textarea':'input');if(type!=='textarea')i.type=type;i.value=value??'';i.defaultValue=i.value;i.maxLength=max;i.disabled=disabled;i.setAttribute('aria-label',label);i.addEventListener('change',()=>onchange(i.value));l.append(i);return l; }
function optionsField(label,items,value,onchange) { const l=node('label',label),select=node('select');select.setAttribute('aria-label',label);for(const item of items){const o=node('option',item.label);o.value=item.value;o.selected=item.value===value;select.append(o);}select.addEventListener('change',()=>onchange(select.value));l.append(select);return l; }
function notice(text,error=false) { const p=node('p',text,error?'notice error':'notice');p.setAttribute('role',error?'alert':'status');return p; }
function readValue(parameter,raw) { if(parameter.type==='relative_period'){let period;try{period=JSON.parse(raw);}catch{throw appError('invalid_request');}boundedJSON(period,4096);return {period};}if(parameter.type.endsWith('_list')){let items;try{items=JSON.parse(raw);}catch{throw appError('invalid_request');}if(!Array.isArray(items)||items.some(x=>typeof x!=='string')||items.length!==parameter.list_length)throw appError('invalid_request');return {items};}return {literal:raw}; }
function valueText(value) { return value?.period?JSON.stringify(value.period):value?.items?JSON.stringify(value.items):value?.literal??''; }

export class ReportApp {
  constructor(root,adapter) {
    this.root=root;this.adapter=adapter;this.closed=false;this.epoch=0;this.mode='consumer';this.busy=false;this.message='';this.error=false;this.capabilities={};this.catalog=[];this.catalogNext='';this.drafts=[];this.draftsNext='';this.selected=null;this.description=null;this.runs=[];this.runsNext='';this.blockCatalog=[];this.blockNext='';this.blockDescription=null;this.selectedWidget='';this.showLibrary=false;this.preview=null;this.previewGeneration=0;this.previewDefinition=null;this.previewOrigin=null;this.lastPreviewDefinition=null;this.lastPreviewRevision=0;this.pendingNavigation=null;this.lastSize='';this.locale='en-US';this.timezone='UTC';this.newID='';this.startingNew=false;this.filterValues=new Map();this.recoveryOwner='';this.lastMutationRun=null;this.lastMutationPrivate=false;this.uncertainRun=false;this.catalogCollapsed=false;this.panelTab='components';this.inlineWidget='';this.drag=null;this.gridFeedback=null;this.activePageID='main';this.consumerPageID='';this.pageGeneration=0;this.mapping=null;this.mappingContext=null;this.privateCharts=new Map();this.mappingFailures=new Map();
    this.retained=new RetainedReport((name,args)=>this.invoke(name,args),()=>{this.preview=null;this.previewDefinition=null;this.previewOrigin=null;this.message='Retained values have expired. Use an explicit preview or run to create new values.';this.render();});
    this.session=new DraftSession((name,args)=>this.invoke(name,args));
    adapter.oncontext=c=>this.context(c);adapter.onclose=()=>this.close();adapter.onfailure=e=>this.reportError(e);adapter.onresult=result=>this.hostResult(result);
    this.observer=typeof ResizeObserver==='function'?new ResizeObserver(()=>this.requestRootSize()):null;this.observer?.observe(root);
    this.render();
  }
  requestRootSize() { if(this.closed)return;const box=this.root.getBoundingClientRect(),key=Math.ceil(box.width)+':'+Math.ceil(box.height);if(key===this.lastSize)return;this.lastSize=key;this.adapter.resize(box.width,box.height); }
  retainedBridge() { return {call:(name,args)=>{if(name==='reporting_run'&&(!this.capabilities.can_execute||args.dynamic||args.narrative))throw appError('forbidden');return this.adapter.call(name,args);},resize:()=>this.requestRootSize()}; }
  resetRecovery(report='') { this.recoveryOwner=report;this.lastMutationRun=null;this.lastMutationPrivate=false;this.uncertainRun=false;this.lastPreviewDefinition=null;this.lastPreviewRevision=0; }
  observeTerminalRecovery(view,run) {
    const summary=view?.summary;
    const uncertainCode=code=>['query_indeterminate','outcome_unknown','unknown'].includes(code);
    if(run!==this.lastMutationRun||!validID(this.recoveryOwner)||summary?.run!==run||view?.selection?.run!==run||view.selection.kind!=='report'||summary.kind!=='report'||summary.target?.kind!=='report'||summary.target.id!==this.recoveryOwner||summary.private!==this.lastMutationPrivate)return;
    if(!['completed','partial','failed','cancelled','expired'].includes(summary.state)||uncertainCode(summary.code)||uncertainCode(view.output?.code)||(view.pages||[]).some(page=>(page.widgets||[]).some(widget=>uncertainCode(widget.code))))return;
    this.uncertainRun=false;
  }
  async invoke(name,args) { if(this.closed)throw appError('unavailable');const result=await this.adapter.call(name,args);if(this.closed)throw appError('unavailable');return unpack(result); }
  context(context) { boundedJSON(context,65536);document.documentElement.dataset.theme=context?.theme==='dark'?'dark':'light';if(typeof context?.locale==='string'){try{this.locale=Intl.getCanonicalLocales(context.locale)[0]||this.locale;}catch{}}document.documentElement.lang=this.locale;if(this.preview)this.render(); }
  async start() { try{await this.adapter.connect();if(this.closed)return;this.lastSize='';this.requestRootSize();await this.refreshCapabilities();if(this.capabilities.consumer)await this.loadCatalog();else if(this.capabilities.builder){this.mode='builder';await this.loadDrafts();}else this.message='No report access is available under the current authority.';}catch(e){this.reportError(e);}this.render(); }
  async refreshCapabilities(report='') { const c=await this.invoke(authoringTool('capabilities'),{report});if(this.closed)return;this.session.setCapabilities(c);this.capabilities=c; }
  reportError(e) { if(this.closed)return;this.error=true;this.message=e?.needsRunAuthority?'The preview was created. Your host must refresh access to this exact retained run before it can be viewed. Use Inspect last retained run after access is refreshed; it will not execute again.':e?.code==='conflict'?'This draft changed elsewhere. Your edits are still here. Reload the latest draft before saving again.':e?.unknown||this.session.uncertain?'The outcome is unknown. Reload the draft or inspect retained runs before submitting another change.':`Report action unavailable: ${e?.code||'unavailable'}.`;this.render(); }
  async perform(fn) { if(this.busy||this.closed)return;this.busy=true;this.error=false;this.message='';this.render();try{await fn();}catch(e){this.reportError(e);}finally{this.busy=false;if(!this.closed)this.render();} }
  navigate(fn) { if(this.busy)return;if(this.mapping){this.message='Save or cancel chart edits before leaving this editor.';this.render();return;}if(this.session.dirty){this.pendingNavigation=fn;this.render();}else void this.perform(fn); }
  async changeMode(mode) { this.epoch++;this.mode=mode;this.privateCharts.clear();this.selected=null;this.description=null;this.closePreview();this.session.definition=null;this.session.state=null;this.session.dirty=false;this.selectedWidget='';await this.refreshCapabilities();if(mode==='builder')await this.loadDrafts();else await this.loadCatalog(); }
  async loadCatalog(after='') { const epoch=this.epoch,r=await this.invoke('reporting_search',{kind:'report',query:'',locale:this.locale,after,limit:40});if(this.closed||epoch!==this.epoch)return;if(r.version!=='reporting-view-v1'||!Array.isArray(r.items))throw appError('unavailable');this.catalog=after?[...this.catalog,...r.items].slice(0,200):r.items;this.catalogNext=this.catalog.length<200?r.next:''; }
  async loadDrafts(after='') { const epoch=this.epoch,r=await this.invoke(authoringTool('drafts'),{after,limit:40});if(this.closed||epoch!==this.epoch)return;if(!Array.isArray(r.items))throw appError('unavailable');this.drafts=after?[...this.drafts,...r.items].slice(0,200):r.items;this.draftsNext=this.drafts.length<200?r.next:''; }
  async selectPublished(item) { const epoch=++this.epoch;this.closePreview();this.privateCharts.clear();this.selected=item;this.consumerPageID='';this.description=null;this.runs=[];this.filterValues.clear();const [d,r,c]=await Promise.all([this.invoke('reporting_describe',{target:item.target,locale:this.locale,outputs:null}),this.invoke('reporting_runs',{kind:'report',resource:item.target.id,after:'',limit:40}),this.invoke(authoringTool('capabilities'),{report:item.target.id})]);if(this.closed||epoch!==this.epoch)return;if(d.resource?.target?.id!==item.target.id||d.resource.target.revision!==item.target.revision)throw appError('stale_validation');if(this.recoveryOwner!==item.target.id)this.resetRecovery(item.target.id);this.description=d;this.runs=r.items||[];this.runsNext=r.next||'';this.capabilities=c; }
  async openDraft(id) { const previousPage=this.session.state?.id===id?this.activePageID:'';++this.epoch;this.closePreview();this.privateCharts.clear();if(await this.session.open(id)){if(this.recoveryOwner!==id)this.resetRecovery(id);this.capabilities=this.session.capabilities;this.newID=id;this.activePageID=reportPages(this.session.definition).find(p=>p.id===previousPage)?.id||reportPages(this.session.definition)[0]?.id||'';this.pageGeneration++;this.selectedWidget=this.activePage()?.widgets[0]?.id||'';this.inlineWidget='';this.showLibrary=false;this.startingNew=false;this.catalogCollapsed=true;this.panelTab='components';this.message=`Draft revision ${this.session.revision} loaded.`;} }
  async createDraft() { if(!validID(this.newID))throw appError('invalid_request');this.closePreview();this.privateCharts.clear();await this.refreshCapabilities(this.newID);this.session.new('Untitled report',this.locale,this.timezone);this.activePageID='main';this.pageGeneration++;this.startingNew=false;this.resetRecovery(this.newID);this.catalogCollapsed=true;this.selectedWidget='';this.inlineWidget='';this.panelTab='components';this.message='This new draft is only in this window until you save.';this.render(); }
  edit(fn) { try{this.session.edit(fn);this.error=false;this.message='Unsaved changes';this.render();}catch(e){this.reportError(e);} }
  activePage() { try{return pageContent(this.session.definition,this.activePageID);}catch{return null;} }
  editPage(fn,id=this.activePageID) { if(id!==this.activePageID||this.closed)return;this.edit(d=>fn(pageContent(d,id))); }
  switchPage(id,consumer=false) {
    const pages=consumer?this.preview?.pages:reportPages(this.session.definition);
    if(this.closed||this.busy||this.mapping||!pages?.some(p=>p.id===id))return;
    this.cancelDrag(false);this.pageGeneration++;this.widgetParameters=null;this.inlineWidget='';
    if(consumer)this.consumerPageID=id;else{this.activePageID=id;this.selectedWidget='';}
    this.render();
  }
  changePages(transform,nextPage=this.activePageID) {
    if(!this.canEdit())return;
    try{this.session.edit(d=>{const next=transform(d);for(const key of Object.keys(d))delete d[key];Object.assign(d,next);});this.cancelDrag(false);this.activePageID=nextPage;this.pageGeneration++;if(!this.activePage()?.widgets.some(w=>w.id===this.selectedWidget))this.selectedWidget='';this.widgetParameters=null;this.inlineWidget='';this.message='Unsaved page changes';this.error=false;this.render();}catch(e){this.reportError(e);}
  }
  enablePages() { this.changePages(upgradeReportPages,'main'); }
  addPage() { const used=new Set(reportPages(this.session.definition).map(p=>p.id));let index=1;while(used.has('page-'+index))index++;const id='page-'+index;this.changePages(d=>addReportPage(d,id,'Page '+(reportPages(d).length+1)),id); }
  removePage() {const pages=reportPages(this.session.definition),index=pages.findIndex(p=>p.id===this.activePageID);if(index<0||pages.length<2)return;const next=pages[index+1]?.id||pages[index-1].id;this.changePages(d=>removeReportPage(d,this.activePageID),next);}
  renderPageTabs(parent,pages,active,consumer=false) {
    const tabs=node('nav',undefined,'report-page-tabs');tabs.setAttribute('aria-label',consumer?'Retained report pages':'Report pages');
    for(const page of pages){const b=action(page.title||page.id,()=>this.switchPage(page.id,consumer),{disabled:this.busy||!!this.mapping});b.dataset.page=page.id;b.setAttribute('aria-current',page.id===active?'page':'false');tabs.append(b);}parent.append(tabs);return tabs;
  }
  renderPageControls(parent,definition,editable) {
    if(definition.schema_version===2){parent.append(action('Enable pages',()=>this.enablePages(),{disabled:!editable}));return;}
    const pages=reportPages(definition),tabs=this.renderPageTabs(parent,pages,this.activePageID);tabs.append(action('Add page',()=>this.addPage(),{disabled:!editable||pages.length>=100}));const page=this.activePage();if(!page)return;
    const controls=node('details',undefined,'page-controls');controls.append(node('summary','Page settings'),inputField('Page title',page.title,value=>{if(this.activePageID===page.id)this.changePages(d=>renameReportPage(d,page.id,value));},{disabled:!editable}));
    const index=pages.findIndex(p=>p.id===this.activePageID);controls.append(action('Move page left',()=>this.changePages(d=>moveReportPage(d,this.activePageID,-1)),{disabled:!editable||index===0}),action('Move page right',()=>this.changePages(d=>moveReportPage(d,this.activePageID,1)),{disabled:!editable||index===pages.length-1}),action('Remove page',()=>this.removePage(),{disabled:!editable||pages.length===1}),node('p','Page changes are local until Save report. Removing a page removes its components from this draft.','metadata'));parent.append(controls);
  }
  addHeading() { const id=unusedWidgetID(this.session.definition,'heading');this.editPage(d=>{d.widgets.push({id,kind:'text',grid:firstFreeCell(d.widgets,{width:12,height:1}),presentation:{},text:{format:'plain',text:'Section heading'}});});this.selectedWidget=id;this.render(); }
  async loadBlocks(after='') { const r=await this.invoke('reporting_search',{kind:'block',query:'',locale:this.locale,after,limit:40});this.blockCatalog=after?[...this.blockCatalog,...r.items].slice(0,200):r.items;this.blockNext=this.blockCatalog.length<200?r.next:'';this.showLibrary=true;this.panelTab='components'; }
  async chooseBlock(item) { const d=await this.invoke('reporting_describe',{target:item.target,locale:this.locale,outputs:null});if(d.resource?.target?.id!==item.target.id||d.resource.target.revision!==item.target.revision)throw appError('stale_validation');this.blockDescription=d; }
  addOutput(output) { const description=this.blockDescription;if(!description||output.enabled===false||!['chart','kpi','table'].includes(output.kind))return;const id=unusedWidgetID(this.session.definition,'widget'),size=output.kind==='kpi'?{width:4,height:3}:output.kind==='chart'?{width:8,height:4}:{width:12,height:4};this.editPage(d=>{d.widgets.push({id,kind:'block',grid:firstFreeCell(d.widgets,size),presentation:{title:output.title||output.id},block:{block:description.resource.target.id,revision:description.resource.target.revision,outputs:[output.id],policy:'published',narrative:false}});});this.selectedWidget=id;this.showLibrary=true;this.panelTab='components';this.render(); }
  addFilter(widget,parameter) { let index=1;while(this.activePage()?.filters?.some(f=>f.parameter.name==='filter_'+index))index++;const name='filter_'+index;this.editPage(d=>{d.filters||=[];if(d.filters.some(f=>f.parameter.name===name))throw appError('conflict');d.filters.push({parameter:{...copyData(parameter),name},label:parameter.name});const w=d.widgets.find(w=>w.id===widget.id);w.bindings||=[];if(w.bindings.some(b=>b.parameter===parameter.name))throw appError('conflict');w.bindings.push({filter:name,parameter:parameter.name});});this.render(); }
  async loadWidgetParameters(widget) { const epoch=this.epoch,page=this.activePageID,generation=this.pageGeneration;const d=await this.invoke('reporting_describe',{target:{kind:'block',id:widget.block.block,revision:widget.block.revision},locale:this.locale,outputs:widget.block.outputs});if(this.closed||epoch!==this.epoch||page!==this.activePageID||generation!==this.pageGeneration)return;if(d.resource?.target?.id!==widget.block.block||d.resource.target.revision!==widget.block.revision)throw appError('stale_validation');this.widgetParameters={page,widget:widget.id,filters:d.filters||[]}; }
  canEdit() { return !this.mapping&&!!this.session.definition&&manualDocument(this.session.definition)&&(this.session.state?this.session.capabilities.can_save:this.session.capabilities.can_create)&&!this.busy&&!this.session.conflict&&!this.session.uncertain; }
  selectWidget(id) { if(this.mapping)return;this.selectedWidget=id;this.panelTab='selected';this.widgetParameters=null;this.inlineWidget='';this.render(); }
  geometryEdit(transform) {
    if(!this.canEdit())return;
    try{this.session.edit(d=>{const page=pageContent(d,this.activePageID);page.widgets=transform(page.widgets);});this.error=false;this.message='Unsaved layout changes';this.render();}
    catch(e){if(e.code==='invalid_request'){this.error=true;this.message='That position overlaps another component or falls outside the grid. Your layout is unchanged.';this.render();}else this.reportError(e);}
  }
  setWidgetGrid(id,field,value) { const n=Number(value);this.geometryEdit(widgets=>{const w=widgets.find(w=>w.id===id);if(!w||!Number.isSafeInteger(n))throw appError('invalid_request');return placeWidget(widgets,id,{...w.grid,[field]:['column','row'].includes(field)?n-1:n});}); }
  duplicateSelected(id) { const nextID=unusedWidgetID(this.session.definition,'widget');this.geometryEdit(widgets=>duplicateWidget(widgets,id,nextID));if(this.activePage()?.widgets.some(w=>w.id===nextID)){this.selectedWidget=nextID;this.panelTab='selected';this.render();} }
  removeSelected(id) { if(!this.canEdit())return;this.editPage(d=>{d.widgets=d.widgets.filter(w=>w.id!==id);});if(this.selectedWidget===id)this.selectedWidget='';this.inlineWidget='';this.render(); }
  keyboardGrid(event,widget) {
    if(event.target!==event.currentTarget&&!String(event.target?.className||'').includes('grid-handle'))return;
    if(event.key==='Escape'){event.preventDefault();event.stopPropagation?.();this.cancelDrag();return;}
    if(!this.canEdit())return;
    if(event.key==='Delete'||event.key==='Backspace'){event.preventDefault();event.stopPropagation?.();this.removeSelected(widget.id);return;}
    const delta={ArrowLeft:[-1,0],ArrowRight:[1,0],ArrowUp:[0,-1],ArrowDown:[0,1]}[event.key];if(!delta)return;
    event.preventDefault();event.stopPropagation?.();this.selectedWidget=widget.id;this.panelTab='selected';this.geometryEdit(widgets=>event.shiftKey?resizeWidget(widgets,widget.id,widget.grid.width+delta[0],widget.grid.height+delta[1]):moveWidget(widgets,widget.id,widget.grid.column+delta[0],widget.grid.row+delta[1]));queueMicrotask(()=>Array.from(this.root.querySelectorAll('article')).find(card=>card.dataset.widget===widget.id)?.focus?.());
  }
  paintGrid(card,grid) { card.style.gridColumn=`${Math.max(0,grid.column)+1} / span ${Math.max(1,grid.width)}`;card.style.gridRow=`${Math.max(0,grid.row)+1} / span ${Math.max(1,grid.height)}`; }
  cancelDrag(redraw=true) {
    const drag=this.drag;if(!drag)return;this.drag=null;this.paintGrid(drag.card,drag.original);drag.card.dataset.dragging='false';drag.card.dataset.invalid='false';
    try{if(drag.handle.hasPointerCapture?.(drag.pointerID))drag.handle.releasePointerCapture(drag.pointerID);}catch{/* Detached handles cannot retain a gesture. */}
    if(redraw&&!this.closed)this.render();
  }
  bindGridPointer(handle,card,canvas,widget,edge='move') {
    handle.addEventListener('pointerdown',event=>{
      if(!this.canEdit()||this.drag||event.button!==0)return;event.preventDefault();event.stopPropagation();
      const box=canvas.getBoundingClientRect(),style=typeof getComputedStyle==='function'?getComputedStyle(canvas):{},gap=parseFloat(style.columnGap)||12,padding=(parseFloat(style.paddingLeft)||0)+(parseFloat(style.paddingRight)||0);
      this.drag={handle,card,canvas,edge,pointerID:event.pointerId,id:widget.id,original:{...widget.grid},candidate:{...widget.grid},valid:true,x:event.clientX,y:event.clientY,scrollX:canvas.scrollLeft||0,scrollY:canvas.scrollTop||0,pitchX:(box.width-padding+gap)/12,pitchY:92,definition:this.session.definition,epoch:this.epoch};
      card.dataset.dragging='true';handle.setPointerCapture?.(event.pointerId);
    });
    handle.addEventListener('pointermove',event=>{
      const d=this.drag;if(!d||d.handle!==handle||d.pointerID!==event.pointerId)return;event.preventDefault();
      const dx=Math.round((event.clientX-d.x+(canvas.scrollLeft||0)-d.scrollX)/d.pitchX),dy=Math.round((event.clientY-d.y+(canvas.scrollTop||0)-d.scrollY)/d.pitchY),g={...d.original};
      if(d.edge==='move'){g.column+=dx;g.row+=dy;}else{if(d.edge.includes('e'))g.width+=dx;if(d.edge.includes('s'))g.height+=dy;if(d.edge.includes('w')){g.column+=dx;g.width-=dx;}if(d.edge.includes('n')){g.row+=dy;g.height-=dy;}}
      d.candidate=g;d.valid=canPlace(this.activePage().widgets,d.id,g);this.paintGrid(card,g);card.dataset.invalid=String(!d.valid);
      const announcement=JSON.stringify([g,d.valid]);if(this.gridFeedback&&d.announcement!==announcement){d.announcement=announcement;this.gridFeedback.textContent=d.valid?`Column ${g.column+1}, row ${g.row+1}, ${g.width} by ${g.height}. Release to apply; Escape cancels.`:'Overlaps a component or leaves the grid. Release to keep the original position.';}
    });
    handle.addEventListener('pointerup',event=>{
      const d=this.drag;if(!d||d.handle!==handle||d.pointerID!==event.pointerId)return;event.preventDefault();const valid=d.valid&&d.definition===this.session.definition&&d.epoch===this.epoch,candidate=d.candidate;this.cancelDrag(false);
      if(!valid){this.error=true;this.message='That position overlaps another component or falls outside the grid. Your layout is unchanged.';this.render();return;}
      if(JSON.stringify(candidate)===JSON.stringify(d.original)){this.selectWidget(widget.id);return;}
      this.geometryEdit(widgets=>placeWidget(widgets,d.id,candidate));
    });
    handle.addEventListener('pointercancel',()=>this.cancelDrag());handle.addEventListener('lostpointercapture',()=>this.cancelDrag());
    handle.addEventListener('keydown',event=>this.keyboardGrid(event,widget));
  }
  renderGridHandles(card,canvas,widget) {
    const toolbar=node('div',undefined,'widget-tools'),move=action('Move widget',()=>{});move.className='grid-handle move-handle';move.dataset.action='move';move.setAttribute('aria-label','Move widget. Drag or use arrow keys.');this.bindGridPointer(move,card,canvas,widget);
    toolbar.append(move,action('Duplicate widget',()=>this.duplicateSelected(widget.id)),action('Remove widget',()=>this.removeSelected(widget.id)),action('Edit title',()=>{this.inlineWidget=widget.id;this.render();}));card.append(toolbar);
    for(const edge of ['nw','n','ne','e','se','s','sw','w']){const handle=action('',()=>{});handle.className=`grid-handle resize-handle resize-${edge}`;handle.dataset.edge=edge;handle.setAttribute('aria-label',`Resize widget ${edge}. Drag or use Shift and arrow keys.`);this.bindGridPointer(handle,card,canvas,widget,edge);card.append(handle);}
  }
  renderInlineTitle(card,widget) {
    const input=node(widget.kind==='text'?'textarea':'input');input.value=widget.kind==='text'?widget.text.text:widget.presentation?.title||'';input.maxLength=widget.kind==='text'?32768:256;input.className='inline-title';input.setAttribute('aria-label',widget.kind==='text'?'Edit heading':'Edit component title');
    const commit=()=>{if(this.inlineWidget!==widget.id)return;this.inlineWidget='';this.editPage(d=>{const w=d.widgets.find(w=>w.id===widget.id);if(w.kind==='text')w.text={format:'plain',text:input.value};else w.presentation.title=input.value;});};
    input.addEventListener('change',commit);input.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();this.inlineWidget='';this.render();}else if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();commit();}});card.append(input);queueMicrotask(()=>input.focus?.());
  }
  async save() { if(await this.session.save(this.newID)){this.message=`Saved private draft revision ${this.session.revision}.`;await this.refreshCapabilities(this.session.state.id);await this.loadDrafts();} }
  async previewDraft() { if(this.session.dirty||!this.session.capabilities.can_preview||this.uncertainRun||!!this.mapping||!this.privatePreviewReady())throw appError('forbidden');const epoch=this.epoch,report=this.session.state.id,revision=this.session.revision;this.closePreview();this.resetRecovery(report);this.lastPreviewDefinition=copyData(this.session.definition);this.lastPreviewRevision=revision;this.uncertainRun=true;let result;try{result=await this.invoke(authoringTool('preview'),{report,key:crypto.randomUUID(),revision,resolution:{at:new Date().toISOString(),timezone:this.session.definition.timezone},pages:[]});if(this.closed||epoch!==this.epoch)return;if(!validID(result.id)||result.private!==true)throw appError('unavailable',true);this.lastMutationRun=result.id;this.lastMutationPrivate=true;result=await this.invoke(authoringTool('execute'),{run:result.id,resume:false});if(this.closed||epoch!==this.epoch)return;await this.openRun(result.id,true);}catch(e){if(!this.closed&&epoch===this.epoch&&this.recoveryOwner===report)this.uncertainRun=!!this.lastMutationRun||e.unknown===true||['unavailable','cancelled_or_timed_out'].includes(e.code);throw e;} }
  async runPublished() { if(!this.capabilities.can_execute||!this.description||this.uncertainRun)throw appError('forbidden');const d=this.description,epoch=this.epoch,pages=new Map();for(const f of d.filters||[]){const raw=this.filterValues.get(`${f.page}:${f.parameter.name}`);if(raw===undefined)continue;if(!pages.has(f.page))pages.set(f.page,[]);pages.get(f.page).push({name:f.parameter.name,value:readValue(f.parameter,raw)});}this.closePreview();this.resetRecovery(d.resource.target.id);this.uncertainRun=true;try{const r=await this.invoke('reporting_run',{target:d.resource.target,key:crypto.randomUUID(),arguments:[],pages:Array.from(pages,([page,filters])=>({page,filters,overrides:[]})),outputs:[],policy:'',locale:this.locale,timezone:d.timezone,narrative:false,dynamic:false,partial_failure:''});if(this.closed||epoch!==this.epoch)return;this.lastMutationRun=r.run;this.lastMutationPrivate=false;await this.openRun(r.run);}catch(e){if(!this.closed&&epoch===this.epoch&&this.recoveryOwner===d.resource.target.id)this.uncertainRun=!!this.lastMutationRun||e.unknown===true||['unavailable','cancelled_or_timed_out'].includes(e.code);throw e;} }
  async openRun(run,privateExpected=false) {
    const epoch=this.epoch;this.closePreview();const generation=this.previewGeneration;let v;
    try{
      v=await this.invoke('reporting_view',{kind:'report',run,page:'',widget:'',output:'',offset:0,limit:100});
      if(this.closed||epoch!==this.epoch||generation!==this.previewGeneration)return;
      if(v?.version!=='reporting-view-v1'||v.summary?.run!==run||v.selection?.run!==run||v.summary.kind!=='report'||v.selection.kind!=='report'||privateExpected&&!v.summary.private)throw appError('unavailable');
      const expectedReport=privateExpected&&run===this.lastMutationRun?this.recoveryOwner:(this.mode==='builder'?this.session.state?.id:this.selected?.target.id)||this.session.state?.id;
      if(expectedReport&&v.summary.target?.id!==expectedReport||privateExpected&&run===this.lastMutationRun&&this.lastPreviewRevision&&v.summary.target?.revision!==this.lastPreviewRevision)throw appError('stale_validation');
      if(!await this.retained.load(v)||this.closed||epoch!==this.epoch||generation!==this.previewGeneration)return;
      this.observeTerminalRecovery(v,run);
      this.preview=v;if(!v.pages?.some(p=>p.id===this.consumerPageID))this.consumerPageID=v.pages?.[0]?.id||'';
      if(this.mode==='consumer'&&!this.selected)this.selected={target:copyData(v.summary.target),title:v.pages?.[0]?.title||v.summary.target.id};
      if(v.summary.private&&run===this.lastMutationRun&&this.lastPreviewDefinition&&v.summary.target.id===this.recoveryOwner&&v.summary.target.revision===this.lastPreviewRevision){this.previewDefinition=copyData(this.lastPreviewDefinition);this.previewOrigin={report:this.recoveryOwner,revision:this.lastPreviewRevision};}
      this.message=v.summary.private?'Private preview. This retained result stays private.':'Viewing retained results. Opening and paging do not run queries.';
    }catch(e){if(this.closed||epoch!==this.epoch||generation!==this.previewGeneration)return;if(privateExpected&&run===this.lastMutationRun&&this.lastMutationPrivate&&['forbidden','unauthenticated','not_found'].includes(e.code))e.needsRunAuthority=true;throw e;}
  }
  closePreview() { this.previewGeneration++;this.retained.clear();this.preview=null;this.previewDefinition=null;this.previewOrigin=null;if(!this.closed)this.render(); }
  hostResult(result) { if(this.closed)return;try{const value=unpack(result);if(value.version==='reporting-view-v1'&&value.summary?.kind==='report'&&validID(value.summary.run))void this.perform(()=>this.openRun(value.summary.run,value.summary.private===true));}catch{/* Opening tools may also return capabilities or a saved draft state. */} }
  compatiblePreview() { return !this.mapping?.dirty&&!!this.previewDefinition&&this.previewOrigin?.report===this.session.state?.id&&executionFingerprint(this.previewDefinition)===executionFingerprint(this.session.definition); }
  async pageOutput(page,widget,output,offset) {
    const run=this.preview?.summary.run;
    try{await this.retained.page(page,widget,output,offset);}
    catch(e){this.preview=null;this.previewDefinition=null;this.previewOrigin=null;if(run===this.lastMutationRun&&this.lastMutationPrivate&&['forbidden','unauthenticated','not_found'].includes(e.code))e.needsRunAuthority=true;throw e;}
  }
  renderPreviewStatus(parent,builder=false) {
    if(!this.preview)return;
    const v=this.preview,status=node('section',undefined,'preview-provenance'),compatible=!builder||this.compatiblePreview();
    status.append(node('span',v.summary.private?'Private preview':'Retained report','badge'),node('span',v.summary.state,'badge'),node('p',`Values from revision ${v.summary.target.revision} · run ${v.summary.run}`,'metadata'));
    if(builder)status.append(notice(compatible?'Retained values, with current presentation and layout. Editing never runs a query.':'Preview is stale. Save and preview to update retained values.',!compatible));
    if(v.redacted)status.append(notice('Some content is not visible under the current authority.'));
    if(v.mixed_freshness)status.append(notice('This report contains results observed at different times.'));
    const provenance=node('details');provenance.append(node('summary','Result provenance'),node('p',`Retained until ${v.summary.expires_at} · ${v.locale} · ${v.timezone}`,'metadata'));
    if(v.observed_at)provenance.append(node('p',`Observed: ${v.observed_at}`,'metadata'));
    if(v.summary.code)provenance.append(node('p',v.summary.code,'metadata'));
    status.append(provenance,action('Close retained view',()=>{this.closePreview();this.render();},{disabled:this.busy}));parent.append(status);
  }
  renderCanvas(parent,widgets,{builder=false,page='main'}={}) {
    const retained=!!this.preview,compatible=retained&&(!builder||this.compatiblePreview()),canvas=node('section',undefined,`composition-canvas${retained?' retained-preview':''}${builder?' editing-canvas':' reading-canvas'}`);
    canvas.setAttribute('aria-label',builder?'Report composition':'Retained report layout');canvas.style.gridTemplateRows=`repeat(${Math.max(builder?8:1,...widgets.map(w=>w.grid.row+w.grid.height))},80px)`;
    for(const widget of [...widgets].sort((a,b)=>a.grid.row-b.grid.row||a.grid.column-b.grid.column)){
      const card=node('article',undefined,`widget-card${widget.kind==='text'?' heading-card':''}`),grid=widget.grid;
      card.dataset.widget=widget.id;card.dataset.row=String(grid.row);card.dataset.column=String(grid.column);card.dataset.width=String(grid.width);card.dataset.height=String(grid.height);this.paintGrid(card,grid);
      card.setAttribute('aria-label',widget.kind==='text'?'Heading widget':'Report output widget');
      if(builder){card.dataset.selected=String(this.selectedWidget===widget.id);card.tabIndex=0;card.addEventListener('keydown',event=>this.keyboardGrid(event,widget));card.addEventListener('click',event=>{if(!event.target.closest?.('button,input,textarea,select,summary,details'))this.selectWidget(widget.id);});}
      const heading=node('div',undefined,'widget-heading'),body=node('div',undefined,'widget-body');card.append(heading,body);
      const firstOutput=retainedOutputs(widget)[0]||'',first=compatible?this.retained.get(page,widget.id,firstOutput):null;
      const label=widget.kind==='text'?(widget.text?.text||this.retained.get(page,widget.id,'')?.text?.text||this.retained.get(page,widget.id,'')?.text?.content||'Heading'):(widget.presentation?.title||first?.outputs?.find(o=>o.id===firstOutput)?.title||widget.id);
      if(builder&&this.inlineWidget===widget.id&&this.canEdit())this.renderInlineTitle(heading,widget);
      else if(builder){const select=action(label,()=>this.selectWidget(widget.id),{disabled:this.busy});select.className='widget-select';select.setAttribute('aria-pressed',String(this.selectedWidget===widget.id));select.setAttribute('title','Select component; double-click to edit its title');select.addEventListener('dblclick',()=>{if(this.canEdit()){this.inlineWidget=widget.id;this.render();}});heading.append(select);}
      else heading.append(node(widget.kind==='text'?'h2':'h3',label,'widget-title'));
      if(widget.presentation?.subtitle)body.append(node('p',widget.presentation.subtitle,'muted'));
      if(widget.kind!=='text'){
        const outputs=retainedOutputs(widget);
        for(const output of outputs){const content=node('section',undefined,'retained-output');content.dataset.output=output;content.setAttribute('aria-label',`Retained output ${output}`);const view=compatible?this.retained.get(page,widget.id,output):null;
          if(view){try{renderRetainedOutput(content,view,{locale:this.locale,onPage:offset=>void this.perform(()=>this.pageOutput(page,widget.id,output,offset))});if(view.observed_at)content.append(node('p',`Observed: ${view.observed_at}`,'metadata'));if(view.trust)content.append(node('p',`Publication / certification / current health: ${view.trust.publication} / ${view.trust.certification} / ${view.trust.health?.status||'unknown'}`,'metadata'));}catch{content.replaceChildren(notice('Retained output unavailable.',true));}}
          else{const placeholder=node('div',undefined,'output-placeholder');placeholder.append(componentGlyph(first?.output?.kind||'chart'),node('p',retained?(compatible?`Retained output: ${widget.state||this.preview.summary.state}`:'Preview is stale. Save and preview to update retained values.'):widget.block?.policy==='private_preview'?(hasFreshMappingEvidence(this.privateCharts.get(this.mappingKey(widget.block)))?'Private chart · use Private preview to see retained values.':'Private chart · Unvalidated or validation status not checked.'):'Approved output · values appear after a private preview.'));content.append(placeholder);}
          body.append(content);
        }
        if(!outputs.length)body.append(node('p',widget.kind==='block'?`No retained outputs · ${widget.state||'not included'}`:'This retained widget type is not supported in this app.','notice'));
      }
      if(builder&&this.selectedWidget===widget.id&&this.canEdit())this.renderGridHandles(card,canvas,widget);
      canvas.append(card);
    }
    if(!widgets.length){const empty=node('div',undefined,'canvas-empty');empty.append(componentGlyph('grid'),node('h2',builder?'Start with a component':'This page is empty'),node('p',builder?(this.session.definition.schema_version===3?'Add a heading or an approved output from Components. Empty pages can be saved.':'Add a heading or an approved output from Components to enable Save report.'):'No components were retained on this page.'));canvas.append(empty);}
    parent.append(canvas);
  }
  renderRetainedReport(parent) {
    this.renderPreviewStatus(parent);
    const pages=this.preview?.pages||[];if(pages.length>1)this.renderPageTabs(parent,pages,this.consumerPageID,true);const page=pages.find(p=>p.id===this.consumerPageID);if(page)this.renderCanvas(parent,page.widgets,{page:page.id});
  }
  render() {
    if(this.closed)return;const focus=captureEditorFocus(this.root);this.cancelDrag(false);this.root.replaceChildren();this.root.dataset.mode=this.mode;this.root.dataset.document=this.mode==='builder'?(this.session.state?.id||this.newID):(this.selected?.target.id||'');this.root.dataset.selection=this.selectedWidget;this.root.dataset.page=this.mode==='builder'?this.activePageID:this.consumerPageID;
    const header=node('header',undefined,'app-header'),brand=node('div',undefined,'app-brand');brand.append(node('span','C','brand-mark'),node('h1','Reports'));
    const toggle=action(this.catalogCollapsed?'Show reports':'Hide reports',()=>{this.catalogCollapsed=!this.catalogCollapsed;this.render();});toggle.className='catalog-toggle';toggle.setAttribute('aria-expanded',String(!this.catalogCollapsed));
    const tabs=node('nav',undefined,'mode-tabs');tabs.setAttribute('aria-label','Report mode');for(const [mode,label,enabled] of [['consumer','Browse',this.capabilities.consumer],['builder','Build',this.capabilities.builder]]){const b=action(label,()=>this.navigate(()=>this.changeMode(mode)),{disabled:!enabled||this.busy});b.setAttribute('aria-current',this.mode===mode?'page':'false');tabs.append(b);}header.append(brand,toggle,tabs);this.root.append(header);
    if(this.message)this.root.append(notice(this.message,this.error));if(this.busy)this.root.append(notice('Working…'));
    if(this.pendingNavigation){const prompt=node('section',undefined,'notice');prompt.append(node('p','You have unsaved edits. Discard them and continue?'),action('Keep editing',()=>{this.pendingNavigation=null;this.render();}),action('Discard and continue',()=>{const fn=this.pendingNavigation;this.pendingNavigation=null;this.session.dirty=false;void this.perform(fn);}));this.root.append(prompt);}
    const layout=node('div',undefined,`app-layout${this.catalogCollapsed?' catalog-collapsed':''}`),sidebar=node('aside',undefined,'catalog'),workspace=node('section',undefined,'workspace');sidebar.hidden=this.catalogCollapsed;layout.append(sidebar,workspace);this.root.append(layout);
    if(this.mode==='builder')this.renderBuilder(sidebar,workspace);else this.renderConsumer(sidebar,workspace);restoreEditorFocus(this.root,focus);

  }
  renderConsumer(sidebar,workspace) {
    sidebar.append(node('h2','Published reports'),node('p','Available under your current access.','metadata'),action('Refresh catalog',()=>void this.perform(()=>this.loadCatalog()),{disabled:this.busy||!this.capabilities.consumer}));for(const item of this.catalog){const b=action(item.title||item.target.id,()=>void this.perform(()=>this.selectPublished(item)),{disabled:this.busy});b.className='catalog-item';b.setAttribute('aria-pressed',String(this.selected?.target.id===item.target.id));sidebar.append(b);}if(this.catalogNext)sidebar.append(action('More reports',()=>void this.perform(()=>this.loadCatalog(this.catalogNext)),{disabled:this.busy}));if(!this.catalog.length)sidebar.append(node('p','No published reports are visible.','empty'));
    if(!this.selected){workspace.append(node('p','Your governed reports, in one place','eyebrow'),node('h2','Choose a published report'),node('p','Open retained results or explicitly run an approved report with business filters.','muted'));return;}
    workspace.append(node('h2',this.selected.title||this.selected.target.id),node('p',this.selected.description||'','muted'),node('p',`Published revision ${this.selected.target.revision}`,'badge'));
    if(this.preview)this.renderRetainedReport(workspace);
    const consumerTools=node('details',undefined,'consumer-tools');consumerTools.open=!this.preview;consumerTools.append(node('summary',this.capabilities.can_execute?'Run and retained history':'Retained history'));workspace.append(consumerTools);
    if(this.description){const form=node('fieldset');form.append(node('legend','Run this report'),node('p','A run may query approved sources and creates a new retained result.','metadata'));for(const f of this.description.filters||[]){const key=`${f.page}:${f.parameter.name}`,pageLabel=this.description.pages?.find(p=>p.id===f.page)?.title||f.page,field=inputField(this.description.pages?.length>1?`${pageLabel} · ${f.label||f.parameter.name}`:f.label||f.parameter.name,this.filterValues.get(key)??'',v=>{if(v==='')this.filterValues.delete(key);else this.filterValues.set(key,v);},{type:f.parameter.type==='date'?'date':'text',max:4096,disabled:!this.capabilities.can_execute||this.busy});field.append(node('span',`Blank uses the published default · ${f.parameter.type}`,'metadata'));form.append(field);}form.append(action('Run with these filters',()=>void this.perform(()=>this.runPublished()),{primary:true,disabled:this.busy||!this.capabilities.can_execute||this.uncertainRun||this.description.dynamic}));if(this.description.dynamic)form.append(node('p','This report contains dynamic widgets outside this manual app slice.','metadata'));if(!this.capabilities.can_execute)form.append(node('p','Current access permits retained reads only.','metadata'));consumerTools.append(form);}
    const history=node('section',undefined,'run-history');history.append(node('h3','Retained runs'),action('Refresh runs',()=>void this.perform(async()=>{const r=await this.invoke('reporting_runs',{kind:'report',resource:this.selected.target.id,after:'',limit:40});this.runs=r.items;this.runsNext=r.next;this.message=this.uncertainRun?'Review existing runs before starting a new operation.':'';}),{disabled:this.busy}));for(const run of this.runs){const row=node('div',undefined,'run-row');row.append(node('div',`${run.state} · ${run.created_at}${run.private?' · Private preview':''}`),action('Open retained run',()=>void this.perform(()=>this.openRun(run.run)),{disabled:this.busy||run.state==='expired'}));history.append(row);}if(!this.runs.length)history.append(node('p','No retained runs are visible.','metadata'));if(this.runsNext)history.append(action('More runs',()=>void this.perform(async()=>{const r=await this.invoke('reporting_runs',{kind:'report',resource:this.selected.target.id,after:this.runsNext,limit:40});this.runs=[...this.runs,...r.items].slice(0,200);this.runsNext=this.runs.length<200?r.next:'';}),{disabled:this.busy}));consumerTools.append(history);
  }
  renderBuilder(sidebar,workspace) {
    sidebar.append(node('h2','Private drafts'),action('New report',()=>this.navigate(async()=>{this.closePreview();this.startingNew=true;this.session.definition=null;this.newID='';}),{primary:true,disabled:!this.capabilities.builder||this.busy}),action('Refresh drafts',()=>void this.perform(()=>this.loadDrafts()),{disabled:this.busy}));for(const draft of this.drafts){const b=action(titleFor(draft.metadata,this.locale),()=>this.navigate(()=>this.openDraft(draft.id)),{disabled:this.busy});b.className='catalog-item';b.setAttribute('aria-pressed',String(this.session.state?.id===draft.id));sidebar.append(b);}if(this.draftsNext)sidebar.append(action('More drafts',()=>void this.perform(()=>this.loadDrafts(this.draftsNext)),{disabled:this.busy}));if(!this.drafts.length)sidebar.append(node('p','No editable drafts are visible.','empty'));
    if(this.startingNew){workspace.append(node('h2','Create a private report'),node('p','Enter a report ID that your host has authorized for creation. Access is checked before the editor opens.','metadata'),inputField('Proposed report ID',this.newID,value=>{this.newID=value;},{max:128,disabled:this.busy}),action('Check access and start',()=>void this.perform(()=>this.createDraft()),{primary:true,disabled:this.busy}));return;}const d=this.session.definition;if(!d){workspace.append(node('p','MANUAL COMPOSITION','eyebrow'),node('h2','Build from approved outputs'),node('p','Start a private report, add headings and published KPIs, charts or tables, then save and preview.','muted'));return;}
    const editable=this.canEdit(),documentBar=node('div',undefined,'document-bar'),identity=node('div',undefined,'document-identity');
    identity.append(inputField('Report title',titleFor(d.metadata,d.locale),value=>{this.edit(next=>{if(!value.trim())throw appError('invalid_request');next.metadata.find(m=>m.locale===next.locale).title=value;});},{disabled:!editable}),node('span',this.session.state?`Private draft · revision ${this.session.revision}${this.session.dirty?' · Unsaved changes':''}`:'Not saved yet','metadata'));if(!this.session.state)identity.append(inputField('Report ID',this.newID,value=>{this.newID=value;},{max:128,disabled:!editable}));
    const controls=node('div',undefined,'document-actions');controls.append(action('Reload latest',()=>this.navigate(()=>this.openDraft(this.session.state?.id||this.newID)),{disabled:this.busy||!this.session.state&&!((this.session.conflict||this.session.uncertain)&&validID(this.newID))}),action('Save report',()=>void this.perform(()=>this.save()),{primary:true,disabled:!editable||!this.session.dirty||(d.schema_version===2&&!d.widgets?.length)}),action('Private preview',()=>void this.perform(()=>this.previewDraft()),{disabled:this.busy||this.session.dirty||!this.session.state||!this.session.capabilities.can_preview||this.uncertainRun||!!this.mapping||!this.privatePreviewReady()}));documentBar.append(identity,controls);workspace.append(documentBar);
    if(this.lastMutationRun&&this.recoveryOwner===this.session.state?.id)workspace.append(action('Inspect last retained run',()=>void this.perform(()=>this.openRun(this.lastMutationRun,this.lastMutationPrivate)),{disabled:this.busy}));
    if(!manualDocument(d)){workspace.append(notice('This draft uses widgets outside the manual editing slice. It is available for inspection; editing here is disabled.'));return;}
    this.renderPageControls(workspace,d,editable);const active=this.activePage();if(!active){workspace.append(notice('The selected page is unavailable.',true));return;}
    const composition=node('div',undefined,'composition-editor'),canvasPanel=node('div',undefined,'canvas-panel'),toolbar=node('div',undefined,'canvas-toolbar');
    toolbar.append(node('span','12-column grid','metadata'),optionsField('Layout',[...(arrangementPreset(active.widgets)==='custom'?[{value:'custom',label:'Custom positions'}]:[]),{value:'1',label:'Arrange in one column'},{value:'2',label:'Arrange in two columns'},{value:'3',label:'Arrange in three columns'}],arrangementPreset(active.widgets),value=>{if(value!=='custom')this.geometryEdit(widgets=>arrangeWidgets(widgets,Number(value)));}));canvasPanel.append(toolbar);this.renderPreviewStatus(canvasPanel,true);
    this.gridFeedback=node('p','Drag handles to move or resize. Arrow keys move; Shift + arrows resize.','grid-feedback');this.gridFeedback.setAttribute('role','status');canvasPanel.append(this.gridFeedback);
    this.renderCanvas(canvasPanel,active.widgets,{builder:true,page:this.activePageID});composition.append(canvasPanel);this.renderRightPanel(composition,active,editable);workspace.append(composition);
  }
  renderRightPanel(parent,definition,editable) {
    const panel=node('aside',undefined,'component-panel'),tabs=node('nav',undefined,'panel-tabs');tabs.setAttribute('aria-label','Component panel');
    for(const [value,label] of [['components','Components'],['selected','Selected']]){const b=action(label,()=>{this.panelTab=value;this.render();});b.setAttribute('aria-pressed',String(this.panelTab===value));tabs.append(b);}panel.append(tabs);
    const components=node('section',undefined,'component-library');components.hidden=this.panelTab!=='components';components.append(node('h2','Add to canvas'),node('p','Compose with headings and approved output mappings.','metadata'));
    const heading=action('Add heading',()=>this.addHeading(),{disabled:!editable});heading.className='component-option';heading.append(componentGlyph('text'));components.append(heading,action('Add published output',()=>void this.perform(()=>this.loadBlocks()),{disabled:!editable}));
    this.renderLibrary(components,editable);panel.append(components);
    const selected=node('section',undefined,'selected-panel');selected.hidden=this.panelTab!=='selected';const widget=definition.widgets.find(w=>w.id===this.selectedWidget);if(widget&&this.mapping){renderMappingEditor(selected,this.mapping,{busy:this.busy,change:()=>this.render(),save:()=>void this.perform(()=>this.saveMapping()),cancel:()=>this.cancelMapping(),inspect:()=>void this.perform(()=>this.inspectMapping())});}else if(widget)this.renderInspector(selected,widget,editable);else selected.append(node('h2','Select a component'),node('p','Choose a card on the canvas to edit its content, position and size.','metadata'));panel.append(selected);
    this.renderFilters(panel,definition);parent.append(panel);
  }
  renderLibrary(parent,editable=true) {
    const library=node('section',undefined,'output-library');library.append(node('h3','Approved outputs'));
    if(!this.showLibrary&&!this.blockCatalog.length)library.append(node('p','Browse the published catalog to choose a KPI, chart or table.','metadata'));
    for(const item of this.blockCatalog){const b=action(item.title||item.target.id,()=>void this.perform(()=>this.chooseBlock(item)),{disabled:this.busy});b.className='block-choice';b.setAttribute('aria-pressed',String(this.blockDescription?.resource.target.id===item.target.id));library.append(b);}
    if(this.blockNext)library.append(action('More blocks',()=>void this.perform(()=>this.loadBlocks(this.blockNext)),{disabled:this.busy}));
    if(this.blockDescription){library.append(node('h4',this.blockDescription.resource.title));for(const output of this.blockDescription.outputs||[])if(output.enabled!==false&&['kpi','chart','table'].includes(output.kind)){const b=action(`Add ${output.kind}: ${output.title||output.id}`,()=>this.addOutput(output),{disabled:!editable});b.className='component-option';b.append(componentGlyph(output.kind));library.append(b);}library.append(node('p','These are approved mappings. Adding a component does not query data.','metadata'));}parent.append(library);
  }
  renderInspector(parent,widget,editable=true) {
    const panel=node('fieldset',undefined,'inspector');panel.disabled=!editable;panel.append(node('legend','Selected widget'));if(widget.kind==='text')panel.append(inputField('Heading text',widget.text.text,value=>{this.editPage(d=>{d.widgets.find(w=>w.id===widget.id).text={format:'plain',text:value};});},{type:'textarea',max:32768}));else panel.append(inputField('Widget title',widget.presentation?.title||'',value=>{this.editPage(d=>{d.widgets.find(w=>w.id===widget.id).presentation.title=value;});}));
    const dimensions=node('div',undefined,'geometry-fields');for(const [field,label] of [['column','Column'],['row','Row'],['width','Width'],['height','Height']])dimensions.append(inputField(label,widget.grid[field]+(['column','row'].includes(field)?1:0),value=>this.setWidgetGrid(widget.id,field,value),{type:'number',max:5}));panel.append(node('h3','Position and size'),dimensions,node('p','Columns and rows start at 1. Changes keep every other component in place.','metadata'));
    const move=direction=>this.geometryEdit(widgets=>moveWidget(widgets,widget.id,widget.grid.column,widget.grid.row+direction));panel.append(action('Move up',()=>move(-1),{disabled:widget.grid.row===0}),action('Move down',()=>move(1)),action('Duplicate widget',()=>this.duplicateSelected(widget.id)),action('Remove widget',()=>this.removeSelected(widget.id)));
    panel.append(action('Save selected content',()=>void this.perform(async()=>{if(await this.session.saveWidget(widget.id,this.activePageID)){this.message=`Saved selected widget in draft revision ${this.session.revision}.`;await this.loadDrafts();}}),{disabled:!this.session.canSaveWidget(widget.id,this.activePageID)}),node('p','Selected-content save changes text and presentation only. Save report also saves layout and filters.','metadata'));
    if(widget.block){this.renderChartActions(panel,widget,editable);const origin=node('details');origin.append(node('summary',widget.block.policy==='private_preview'?'Private chart source':'Approved source'),node('p',`${widget.block.block} · revision ${widget.block.revision} · ${widget.block.outputs.join(', ')}`,'metadata'));panel.append(origin,action('Choose business filter',()=>void this.perform(()=>this.loadWidgetParameters(widget))));if(this.widgetParameters?.widget===widget.id&&this.widgetParameters.page===this.activePageID)for(const f of this.widgetParameters.filters)panel.append(action(`Add filter: ${f.label||f.parameter.name}`,()=>this.addFilter(widget,f.parameter),{disabled:widget.bindings?.some(b=>b.parameter===f.parameter.name)}));}parent.append(panel);
  }
  mappingKey(block) {return JSON.stringify([block.block,block.revision,block.digest||'']);}
  rememberPrivateChart(reference,view) {const b=view.block,e=b.validation,block={revision:b.revision,digest:b.digest,execution_digest:b.execution_digest};if(e)block.validation={revision:e.revision,definition_digest:e.definition_digest,execution_digest:e.execution_digest,expires_at:e.expires_at};this.privateCharts.set(this.mappingKey(reference),{block});while(this.privateCharts.size>100)this.privateCharts.delete(this.privateCharts.keys().next().value);}
  clearChartReadFailure(error) {if(['forbidden','unauthenticated','not_found','stale_validation','invalid_request'].includes(error.code)){this.privateCharts.clear();this.closePreview();}}
  mappingEditKey(widget) {return JSON.stringify([this.session.state?.id||this.newID,this.activePageID,widget.id]);}
  privatePreviewReady() {return reportPages(this.session.definition).every(p=>p.widgets.every(w=>w.block?.policy!=='private_preview'||!this.mappingFailures.has(this.mappingKey(w.block))&&hasFreshMappingEvidence(this.privateCharts.get(this.mappingKey(w.block)))));}
  async openMapping(widget,output=widget.block.outputs[0]) {
    if(this.mapping||this.closed||this.session.pending||this.session.conflict||this.session.uncertain||!this.session.capabilities.can_save||this.session.definition?.schema_version!==3)throw appError('forbidden');
    const current=this.activePage()?.widgets.find(w=>w.id===widget.id);if(this.selectedWidget!==widget.id||!current||JSON.stringify(current.block)!==JSON.stringify(widget.block))throw appError('stale_validation');
    const key=this.mappingEditKey(widget);if(this.mappingFailures.has(key)||this.mappingFailures.has(this.mappingKey(widget.block))){this.message='A previous chart save has an unknown outcome. Reconcile it through your host before editing this component again.';return;}
    const epoch=this.epoch,page=this.activePageID,generation=this.pageGeneration,session=new MappingSession((name,args)=>this.invoke(name,args));this.mapping=session;this.mappingContext={key,epoch,page,generation,widget:widget.id,original:copyData(widget.block)};this.panelTab='selected';this.render();
    try{if(!await session.open(widget.block.block,widget.block.revision,output,widget.block.policy!=='private_preview',widget.block.digest||''))return;if(this.closed||epoch!==this.epoch||page!==this.activePageID||generation!==this.pageGeneration){session.close();if(this.mapping===session)this.mapping=null;return;}if(widget.block.policy==='private_preview')this.rememberPrivateChart(widget.block,session.view);}
    catch(e){session.close();if(this.closed||epoch!==this.epoch||generation!==this.pageGeneration)return;if(this.mapping===session){this.mapping=null;this.mappingContext=null;}this.clearChartReadFailure(e);throw e;}
  }
  async saveMapping() {
    const session=this.mapping,context=this.mappingContext;if(!session||!context)throw appError('invalid_request');
    try{const view=await session.save();if(!view||this.closed||context.epoch!==this.epoch||context.page!==this.activePageID||this.mapping!==session)return;
      const current=pageContent(this.session.definition,context.page).widgets.find(w=>w.id===context.widget);if(!current||JSON.stringify(current.block)!==JSON.stringify(context.original))throw appError('conflict');
      const block={...copyData(current.block),block:view.block.state.id,revision:view.block.revision,digest:view.block.digest,policy:'private_preview'};
      this.session.edit(d=>{pageContent(d,context.page).widgets.find(w=>w.id===context.widget).block=block;});this.rememberPrivateChart(block,view);session.close();this.mapping=null;this.mappingContext=null;this.message='Private chart saved. It is unvalidated. Save report to keep this new reference; Validate data is a separate source read.';this.error=false;
    }catch(e){if(session.unknown)this.mappingFailures.set(context.key,{...session.operation,output:session.output});throw e;}
  }
  cancelMapping() {const session=this.mapping;if(!session||session.pending)return;if(session.unknown)this.mappingFailures.set(this.mappingContext.key,{...session.operation,output:session.output});session.close();this.mapping=null;this.mappingContext=null;this.message='Chart editor closed. No saved chart revision was deleted.';this.render();}
  async inspectMapping() {const session=this.mapping;if(!session?.unknown)throw appError('invalid_request');const value=await session.inspect();if(this.closed||this.mapping!==session)return;checkMappingView(value,session.operation.target,session.operation.revision,session.output);this.message='Chart metadata is readable at the inspected revision. This does not prove which request committed; the unknown-outcome fence remains.';}
  async inspectMappingRecord(record) {const epoch=this.epoch,value=await this.invoke(authoringTool('block_read'),{block:record.target,revision:record.revision});if(this.closed||epoch!==this.epoch)return;checkMappingView(value,record.target,record.revision,record.output);this.message='Chart metadata is readable. The save outcome remains unknown; reconcile it through your host before retrying.';}
  async readPrivateChart(widget) {
    const epoch=this.epoch,generation=this.pageGeneration,reference=copyData(widget.block);
    try{const view=await this.invoke(authoringTool('block_read'),{block:reference.block,revision:reference.revision});if(this.closed||epoch!==this.epoch||generation!==this.pageGeneration)return null;checkMappingView(view,reference.block,reference.revision,reference.outputs[0],reference.digest);return view;}
    catch(e){if(this.closed||epoch!==this.epoch||generation!==this.pageGeneration)return null;this.clearChartReadFailure(e);throw e;}
  }
  async checkPrivateChart(widget) {const view=await this.readPrivateChart(widget);if(!view)return;this.rememberPrivateChart(widget.block,view);this.message=hasFreshMappingEvidence(view)?'Exact chart validation evidence is present. Private preview will recheck current access and dependencies.':'This private chart is unvalidated or its evidence has expired.';}
  async validateChart(widget) {
    const epoch=this.epoch,pageID=this.activePageID,reference=copyData(widget.block),key=this.mappingKey(reference);if(this.mappingFailures.has(key))throw appError('busy');const view=await this.readPrivateChart(widget);if(!view||epoch!==this.epoch||pageID!==this.activePageID)return;const page=pageContent(this.session.definition,pageID),resolution={at:new Date().toISOString(),timezone:page.timezone||this.session.definition.timezone};
    try{const validated=await validatePrivateMapping((name,args)=>this.invoke(name,args),view,widget,page,resolution);if(this.closed||epoch!==this.epoch)return;this.rememberPrivateChart(reference,validated);this.message='Private chart validation completed. Save any report edits, then use Private preview to create retained values.';}
    catch(e){if(!this.closed&&epoch===this.epoch&&(e.unknown===true||['unavailable','cancelled_or_timed_out'].includes(e.code)))this.mappingFailures.set(key,{kind:'validation',target:reference.block,revision:reference.revision});throw e;}
  }
  renderChartActions(parent,widget,editable) {
    const failed=this.mappingFailures.get(this.mappingEditKey(widget));if(failed)parent.append(notice('A previous chart save has an unknown outcome. Edits stay blocked until host reconciliation.',true),action('Inspect chart state',()=>void this.perform(()=>this.inspectMappingRecord(failed)),{disabled:this.busy}));
    if(widget.block.policy==='private_preview'){
      const key=this.mappingKey(widget.block),meta=this.privateCharts.get(key),unknown=this.mappingFailures.has(key);parent.append(node('p',unknown?'Validation outcome unknown. Metadata reads cannot authorize a retry.':hasFreshMappingEvidence(meta)?'Private chart · validation evidence available':'Private chart · Unvalidated or validation status not checked','notice'),action('Check chart status',()=>void this.perform(()=>this.checkPrivateChart(widget)),{disabled:this.busy}),action('Validate data',()=>void this.perform(()=>this.validateChart(widget)),{disabled:!editable||unknown}),node('p','Validation explicitly reads approved source data. Saving the report does not.','metadata'));
    }
    if(this.session.definition.schema_version!==3){parent.append(node('p','Enable pages to use a report-local private chart revision.','metadata'));return;}
    for(const output of widget.block.outputs)parent.append(action(widget.block.outputs.length===1?'Edit chart':`Edit chart: ${output}`,()=>void this.perform(()=>this.openMapping(widget,output)),{disabled:!editable||this.mappingFailures.has(this.mappingEditKey(widget))||this.mappingFailures.has(this.mappingKey(widget.block))}));
  }
  renderFilters(parent,definition) {
    const pageID=this.activePageID,section=node('details',undefined,'filter-editor');section.append(node('summary','Business filters'),node('p','Filter declarations and defaults belong to this page. Filters do not grant data access.','metadata'));
    for(const filter of definition.filters||[]){const name=filter.parameter.name,card=node('div',undefined,'filter-card'),current=definition.defaults?.find(a=>a.name===name)?.value??filter.parameter.default;
      card.append(inputField('Filter label',filter.label,value=>{this.editPage(d=>{d.filters.find(f=>f.parameter.name===name).label=value;},pageID);}),node('p',`${name} · ${filter.parameter.type}`,'metadata'),inputField('Default value',valueText(current),value=>{this.editPage(d=>{const p=d.filters.find(f=>f.parameter.name===name).parameter,override=d.defaults?.find(a=>a.name===name);if(override){if(value==='')d.defaults=d.defaults.filter(a=>a.name!==name);else override.value=readValue(p,value);}else if(value==='')delete p.default;else p.default=readValue(p,value);},pageID);},{max:4096}),action('Remove filter',()=>{this.editPage(d=>{d.filters=d.filters.filter(f=>f.parameter.name!==name);d.defaults=d.defaults?.filter(a=>a.name!==name);for(const w of d.widgets)w.bindings=w.bindings?.filter(b=>b.filter!==name);},pageID);}));section.append(card);
    }parent.append(section);
  }

  close() { if(this.closed)return;this.closed=true;this.cancelDrag(false);this.epoch++;this.closePreview();this.retained.close();this.session.close();this.mapping?.close();this.mapping=null;this.mappingContext=null;this.privateCharts.clear();this.mappingFailures.clear();this.observer?.disconnect();this.catalog=[];this.drafts=[];this.description=null;this.runs=[];this.blockCatalog=[];this.blockDescription=null;this.widgetParameters=null;this.pendingNavigation=null;this.selected=null;this.resetRecovery();this.filterValues.clear();this.root.replaceChildren(notice('This report app is closed. Reopen it through your authorized host.')); }
}
function captureEditorFocus(root) {
  const active=document.activeElement,label=active?.getAttribute?.('aria-label');if(!label||!['INPUT','TEXTAREA','SELECT'].includes(active.tagName))return null;
  const matching=Array.from(root.querySelectorAll(active.tagName)).filter(e=>e.getAttribute('aria-label')===label),index=matching.indexOf(active);if(index<0)return null;
  return {mode:root.dataset.mode,document:root.dataset.document,selection:root.dataset.selection,page:root.dataset.page,tag:active.tagName,label,index,value:active.value,baseline:active.defaultValue,start:active.selectionStart,end:active.selectionEnd};
}
function restoreEditorFocus(root,saved) {
  if(!saved||saved.mode!==root.dataset.mode||saved.document!==root.dataset.document||saved.selection!==root.dataset.selection||saved.page!==root.dataset.page)return;const input=Array.from(root.querySelectorAll(saved.tag)).filter(e=>e.getAttribute('aria-label')===saved.label)[saved.index];if(!input||input.disabled)return;
  if(saved.tag!=='SELECT'&&input.value===saved.baseline)input.value=saved.value;
  input.focus?.({preventScroll:true});if(typeof saved.start==='number')try{input.setSelectionRange?.(saved.start,saved.end);}catch{/* Numeric inputs have no selection range. */}
}
function arrangementPreset(widgets) { for(const columns of [1,2,3]){try{if(JSON.stringify(arrangeWidgets(widgets,columns).map(w=>w.grid))===JSON.stringify(widgets.map(w=>w.grid)))return String(columns);}catch{}}return 'custom'; }
function componentGlyph(kind) {
  const icon=document.createElementNS('http://www.w3.org/2000/svg','svg');icon.setAttribute('viewBox','0 0 72 48');icon.setAttribute('aria-hidden','true');icon.setAttribute('class',`component-glyph glyph-${kind}`);
  const paths=kind==='text'?['M18 12H54 M36 12V38 M26 38H46']:kind==='table'?['M10 9H62V39H10Z M10 19H62 M10 29H62 M28 9V39 M46 9V39']:kind==='kpi'?['M12 14H36 M12 26H52 M12 36H42']:kind==='grid'?['M12 9H32V25H12Z M40 9H60V25H40Z M12 31H32V43H12Z M40 31H60V43H40Z']:['M10 8V39H64 M17 31L30 23L42 28L59 12'];
  for(const data of paths){const path=document.createElementNS('http://www.w3.org/2000/svg','path');path.setAttribute('d',data);icon.append(path);}return icon;
}
export function mountReportApp(root,adapter) { const app=new ReportApp(root,adapter);void app.start();return app; }
export function mountEmbeddedReportApp(root,registration) { return mountReportApp(root,new EmbeddedReportAdapter(registration)); }
// This entrypoint is selected by the separately registered embedded resource.
// The public origin registration is routing configuration, never a user grant.
export function awaitEmbeddedParent(root,parents,win=window) {
  if(!Array.isArray(parents)||!parents.length||parents.length>16||parents.some(origin=>{try{const u=new URL(origin);return u.protocol!=='https:'||u.origin!==origin||u.username||u.password;}catch{return true;}}))throw appError('invalid_request');
  const allowed=new Set(parents);let closed=false,timer;
  const cleanup=()=>{win.removeEventListener('message',receive);win.removeEventListener('pagehide',close);clearTimeout(timer);};
  const close=()=>{if(closed)return;closed=true;cleanup();root.replaceChildren(notice('This report app is closed. Reopen it through your authorized host.'));};
  const receive=event=>{if(closed||event.source!==win.parent||!allowed.has(event.origin))return;try{const m=boundedJSON(event.data,4096);if(m?.protocol!=='chartworks-report-app-v1'||m.method!=='bootstrap'||Object.keys(m).some(k=>!['protocol','method','frame','generation','params'].includes(k))||!m.params||Object.keys(m.params).some(k=>k!=='challenge'))return;const adapter=new EmbeddedReportAdapter({win,origin:event.origin,frame:m.frame,generation:m.generation,challenge:m.params.challenge});closed=true;cleanup();mountReportApp(root,adapter);}catch{/* Invalid bootstrap does not select a host or broaden authority. */}};
  root.replaceChildren(notice('Waiting for the registered embedded host…'));
  win.addEventListener('message',receive);win.addEventListener('pagehide',close);timer=setTimeout(close,65000);
  return {close};
}
const appRoot=typeof document==='undefined'?null:document.getElementById('report-app');
if(appRoot){if(window.parent===window)appRoot.replaceChildren(notice('Open this report app through an authorized host.'));else if(typeof REPORT_APP_EMBEDDED_PARENTS!=='undefined')awaitEmbeddedParent(appRoot,REPORT_APP_EMBEDDED_PARENTS);else mountReportApp(appRoot,new MCPReportAdapter());}
