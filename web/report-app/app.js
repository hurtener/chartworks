import {INVALID_REQUEST, STALE_VALIDATION, FORBIDDEN, UNAVAILABLE, BUSY, LIMIT_EXCEEDED, CONFLICT, CANCELLED_OR_TIMED_OUT} from './error-codes.js';
import {node, button, selectField, textField as inputField} from './dom.js';
import {boundedJSON, renderRetainedOutput} from '../report-viewer/presentation.js';
import {DraftSession, authoringTool, appError, copyData, editableDocument, layoutWidgets, layoutPreset, unpack, titleFor, validID} from './model.js';
import {reportPages, pageContent, upgradeReportPages, addReportPage, renameReportPage, moveReportPage, removeReportPage, unusedWidgetID} from './pages.js';
import {RetainedReport, executionFingerprint, retainedOutputs} from './retained.js';
import {canPlace, placeWidget, moveWidget, resizeWidget, duplicateWidget, firstFreeCell, arrangeWidgets} from './grid.js';
import {MappingSession, checkMappingView, validatePrivateMapping, hasFreshMappingEvidence} from './mapping.js';
import {CatalogSearch} from './catalog.js';
import {pageSettingChoices,setPageSetting} from './page-settings.js';
import {formattingPreview} from './formatting.js';
import {renderMappingEditor} from './mapping-editor.js';
import {DatasetSession} from './dataset.js';
import {renderDatasetEditor} from './dataset-editor.js';
import {TargetAllocation,allocationAvailable,allocationTitle,TARGET_ALLOCATION_UNAVAILABLE} from './allocation.js';
import {MCPReportAdapter, EmbeddedReportAdapter} from './bridge.js';
import {ReportFilterControls} from './report-filters.js';
import {PublicationSession, stageLabel} from './publication.js';
import {renderPublicationControls} from './publication-controls.js';
import {applyHostTheme} from './theme.js';
import {applyStarter,starterPlacement,renderStarterChoices,renderStarterGuide} from './starters.js';

function action(label,fn,{primary=false,disabled=false}={}) { const b=button(label,fn,disabled);if(primary)b.className='primary';return b; }
function optionsField(label,items,value,onchange,{disabled=false}={}) { return selectField(label,items,value,onchange,disabled); }
function notice(text,error=false) { const p=node('p',text,error?'notice error':'notice');p.setAttribute('role',error?'alert':'status');return p; }

export function intrinsicReportSize(root) {const box=root.getBoundingClientRect(),top=Number.isFinite(box.top)?box.top:0,bottoms=Array.from(root.children||[]).filter(child=>!child.hidden).map(child=>child.getBoundingClientRect?.().bottom).filter(Number.isFinite);const padding=typeof getComputedStyle==='function'?parseFloat(getComputedStyle(root).paddingBottom)||0:0;return {width:box.width,height:bottoms.length?Math.max(0,...bottoms)-top+padding:box.height};}

export class ReportApp {
  constructor(root,adapter) {
    this.root=root;this.adapter=adapter;this.closed=false;this.epoch=0;this.mode='consumer';this.busy=false;this.message='';this.error=false;this.capabilities={};this.catalog=[];this.catalogNext='';this.drafts=[];this.draftsNext='';this.selected=null;this.description=null;this.runs=[];this.runsNext='';this.blockCatalog=[];this.blockNext='';this.blockDescription=null;this.selectedWidget='';this.showLibrary=false;this.preview=null;this._uiPreviewGeneration=0;this._uiPreviewDefinition=null;this._uiPreviewOrigin=null;this._uiLastPreviewDefinition=null;this._uiLastPreviewRevision=0;this._uiPendingNavigation=null;this.lastSize='';this.locale='en-US';this.timezone='UTC';this.newID='';this.startingNew=false;this.filters=new ReportFilterControls(this);this._uiFilterValues=this.filters.temporary;this._uiRecoveryOwner='';this._uiLastMutationRun=null;this._uiLastMutationPrivate=false;this.uncertainRun=false;this._uiCatalogCollapsed=false;this.consumerToolsOpen=null;this._uiConsumerToolsElement=null;this.panelTab='components';this.inlineWidget='';this.drag=null;this.gridFeedback=null;this.activePageID='main';this.consumerPageID='';this._uiPageGeneration=0;this.mapping=null;this._uiMappingContext=null;this._uiPrivateCharts=new Map();this._uiMappingFailures=new Map();this.datasetSession=null;this._uiDatasetContext=null;this._uiDatasetCustody=new Map();this.allocations=new Map();this.newTitle='Untitled report';this.newStarter='blank';this.canvasExpanded=false;
    this.reportSearch=new CatalogSearch('report',(name,args)=>this.invoke(name,args));this.blockSearch=new CatalogSearch('block',(name,args)=>this.invoke(name,args));this._uiBlockDescriptionGeneration=0;this.pageSettingsOpen=false;this._uiPageSettingsElement=null;
    this.retained=new RetainedReport((name,args)=>this.invoke(name,args),()=>{this.preview=null;this._uiPreviewDefinition=null;this._uiPreviewOrigin=null;this.message='Retained values have expired. Use an explicit preview or run to create new values.';this.render();});
    this.session=new DraftSession((name,args)=>this.invoke(name,args),name=>this.tools(name));
    this.publication=new PublicationSession((name,args)=>this.invoke(name,args),name=>this.tools(name));
    adapter.oncontext=c=>this.context(c);adapter.onclose=()=>this.close();adapter.onfailure=e=>this.reportError(e);adapter.onresult=result=>this.hostResult(result);
    this.observer=typeof ResizeObserver==='function'?new ResizeObserver(()=>this.requestRootSize()):null;this.observer?.observe(root);
    this.render();
  }
  tools(...names) { return names.every(name=>typeof this.adapter.supportsTool!=='function'||this.adapter.supportsTool(name)); }
  authoring(...actions) { return this.tools(...actions.map(authoringTool)); }
  createTools() { return this.authoring('capabilities','create','read','save'); }
  // Block metadata distinguishes report-only hosts from block authoring surfaces.
  blockTools() { return this.authoring('block_read')&&this.tools('reporting_search','reporting_describe'); }
  mappingTools(purpose='mapping') { return (purpose==='presentation'||this.tools('chart_catalog'))&&this.authoring('block_read','block_mapping','block_copy'); }
  datasetTools() { return (this.tools('list_topics','describe_topic')||this.tools('list_source_page','list_datasets'))&&this.authoring('block_read','dataset','prepare_chart','preparation','create_prepared','preparation_control'); }
  requestRootSize() { if(this.closed)return;const box=intrinsicReportSize(this.root),key=Math.ceil(box.width)+':'+Math.ceil(box.height);if(key===this.lastSize)return;this.lastSize=key;this.adapter.resize(box.width,box.height); }
  retainedBridge() { return {call:(name,args)=>{if(!this.tools(name))throw appError(FORBIDDEN);if(name==='reporting_run'&&(!this.capabilities.can_execute||args.dynamic||args.narrative))throw appError(FORBIDDEN);return this.adapter.call(name,args);},resize:()=>this.requestRootSize()}; }
  resetRecovery(report='') { this._uiRecoveryOwner=report;this._uiLastMutationRun=null;this._uiLastMutationPrivate=false;this.uncertainRun=false;this._uiLastPreviewDefinition=null;this._uiLastPreviewRevision=0; }
  observeTerminalRecovery(view,run) {
    const summary=view?.summary;
    const uncertainCode=code=>['query_indeterminate','outcome_unknown','unknown'].includes(code);
    if(run!==this._uiLastMutationRun||!validID(this._uiRecoveryOwner)||summary?.run!==run||view?.selection?.run!==run||view.selection.kind!=='report'||summary.kind!=='report'||summary.target?.kind!=='report'||summary.target.id!==this._uiRecoveryOwner||summary.private!==this._uiLastMutationPrivate)return;
    if(!['completed','partial','failed','cancelled','expired'].includes(summary.state)||uncertainCode(summary.code)||uncertainCode(view.output?.code)||(view.pages||[]).some(page=>(page.widgets||[]).some(widget=>uncertainCode(widget.code))))return;
    this.uncertainRun=false;
  }
  async invoke(name,args) { if(this.closed)throw appError(UNAVAILABLE);if(!this.tools(name))throw appError(FORBIDDEN);const result=await this.adapter.call(name,args);if(this.closed)throw appError(UNAVAILABLE);return unpack(result); }
  context(context) { boundedJSON(context,65536);applyHostTheme(document.documentElement,context);if(typeof context?.locale==='string'){try{this.locale=Intl.getCanonicalLocales(context.locale)[0]||this.locale;}catch{}}document.documentElement.lang=this.locale;if(this.preview)this.render(); }
  async start() { try{await this.adapter.connect();if(this.closed)return;this.lastSize='';this.requestRootSize();await this.refreshCapabilities();if(this.capabilities.consumer)await this.loadCatalog();else if(this.capabilities.builder){this.mode='builder';if(this.authoring('drafts'))await this.loadDrafts();}else this.message=this.createTools()&&allocationAvailable(this.adapter)?'Create a report to start building.':'No report actions are available in this host with the current access.';}catch(e){this.reportError(e);}this.render(); }
  async refreshCapabilities(report='') { const c=this.authoring('capabilities')?await this.invoke(authoringTool('capabilities'),{report}):{version:'report-authoring-v1'};if(this.closed)return;this.session.setCapabilities(c);this.capabilities=this.session.capabilities; }
  reportError(e) { if(this.closed)return;this.error=true;this.message=e?.publicationMessage?e.publicationMessage:e?.allocationUnavailable?TARGET_ALLOCATION_UNAVAILABLE:e?.allocationUnknown?'Creation unconfirmed. Resume creation checks the same target without requesting a new one.':e?.needsRunAuthority?'Preview created. Refresh access to this exact run, then use Inspect last retained run. Inspection never executes it again.':e?.code==='authority_limit_exceeded'?'This report exceeds the current access limit. The requested action was not started. Reduce the number of independent charts or split it into separate reports.':e?.code===CONFLICT?'This draft changed elsewhere. Your edits are still here. Reload the latest draft before saving again.':e?.unknown||this.session.uncertain?'The outcome is unknown. Reload the draft or inspect retained runs before submitting another change.':`Report action unavailable: ${e?.code||UNAVAILABLE}.`;this.render(); }
  async perform(fn) { if(this.busy||this.closed)return;this.busy=true;this.error=false;this.message='';this.render();try{await fn();}catch(e){this.reportError(e);}finally{this.busy=false;if(!this.closed)this.render();} }
  navigate(fn) { if(this.busy)return;if(this.mapping||this.datasetSession){this.message='Save or close chart edits before leaving this editor.';this.render();return;}if(this.session.dirty){this._uiPendingNavigation=fn;this.render();}else void this.perform(fn); }
  async changeMode(mode) { this.reportSearch.invalidate();this.resetLibrary();this.pageSettingsOpen=false;this.publication.invalidate();this.filters.reset();this.epoch++;this.mode=mode;this.startingNew=false;this._uiPrivateCharts.clear();this.selected=null;this.description=null;this.closePreview();this.session.definition=null;this.session.state=null;this.session.dirty=false;this.selectedWidget='';await this.refreshCapabilities();if(mode==='builder'){if(this.authoring('drafts'))await this.loadDrafts();}else if(this.capabilities.consumer)await this.loadCatalog(); }
  async loadCatalog(after='',query=after?this.reportSearch.query:this.reportSearch.input) {
    if(!this.tools('reporting_search','reporting_describe'))throw appError(FORBIDDEN);
    const epoch=this.epoch,pending=this.reportSearch.load({after,query,locale:after?this.reportSearch.locale:this.locale,current:()=>!this.closed&&epoch===this.epoch});
    if(!after){this.catalog=[];this.catalogNext='';}const result=await pending;if(result){this.catalog=result.items;this.catalogNext=result.next;}
  }
  renderCatalogSearch(parent,search,load,label,enabled=true) {
    const form=node('form',undefined,'catalog-search'),current=()=>!this.closed&&search.form===form,wrap=node('label',`Find ${label}`),input=node('input');input.type='search';input.value=search.input;input.maxLength=256;input.disabled=this.busy||!enabled;input.setAttribute('aria-label',`Find ${label}`);input.addEventListener('input',()=>{if(current())search.input=input.value;});wrap.append(input);search.form=form;
    const run=()=>{if(current())void this.perform(()=>load('',search.input));};form.addEventListener('submit',event=>{event.preventDefault();if(!this.busy&&enabled)run();});
    form.append(wrap,action(`Search ${label}`,run,{disabled:this.busy||!enabled}),action(`Clear ${label} search`,()=>{if(!current())return;search.input='';void this.perform(()=>load('',''));},{disabled:this.busy||!enabled||!search.input&&!search.query}));parent.append(form);
    if(search.pages&&search.query)parent.append(node('p',`Results for “${search.query}”`,'metadata'));
    if(search.limited)parent.append(node('p','Catalog browsing limit reached. Refine your search to find more.','metadata'));
  }
  resetLibrary() {this.blockSearch.reset();this.blockCatalog=[];this.blockNext='';this.blockDescription=null;this._uiBlockDescriptionGeneration++;this.libraryElement=null;this.showLibrary=false;}

  async loadDrafts(after='') { const epoch=this.epoch,r=await this.invoke(authoringTool('drafts'),{after,limit:40});if(this.closed||epoch!==this.epoch)return;if(!Array.isArray(r.items))throw appError(UNAVAILABLE);this.drafts=after?[...this.drafts,...r.items].slice(0,200):r.items;this.draftsNext=this.drafts.length<200?r.next:''; }
  async selectPublished(item) {
    this.publication.invalidate();this.filters.reset();const epoch=++this.epoch;
    this.startingNew=false;this.closePreview();this._uiPrivateCharts.clear();this.selected=item;this._uiCatalogCollapsed=true;this.consumerToolsOpen=null;this.consumerPageID='';this.description=null;this.runs=[];this._uiFilterValues.clear();
    const [d,r,c]=await Promise.all([
      this.invoke('reporting_describe',{target:item.target,locale:this.locale,outputs:null}),
      this.tools('reporting_runs')?this.invoke('reporting_runs',{kind:'report',resource:item.target.id,after:'',limit:40}):{items:[],next:''},
      this.invoke(authoringTool('capabilities'),{report:item.target.id})
    ]);
    if(this.closed||epoch!==this.epoch)return;
    if(d.resource?.target?.id!==item.target.id||d.resource.target.revision!==item.target.revision)throw appError(STALE_VALIDATION);
    if(this._uiRecoveryOwner!==item.target.id)this.resetRecovery(item.target.id);
    this.description=d;this.runs=r.items||[];this.runsNext=r.next||'';this.session.setCapabilities(c);this.capabilities=this.session.capabilities;
    // Opening a publication reads its newest retained result, never runs it.
    // Private previews and previous publications remain explicit history choices.
    const latest=this.runs.filter(run=>run.kind==='report'&&validID(run.run)&&run.private===false&&run.target?.kind==='report'&&run.target.id===item.target.id&&run.target.revision===item.target.revision&&['completed','partial'].includes(run.state)&&Date.parse(run.expires_at)>Date.now()&&Number.isFinite(Date.parse(run.created_at))).sort((a,b)=>Date.parse(b.created_at)-Date.parse(a.created_at))[0];
    if(latest&&this.tools('reporting_view'))await this.openRun(latest.run,false,item.target);
  }
  async openDraft(id,stage='',revision=0) { this.resetLibrary();this.pageSettingsOpen=false;this.publication.invalidate();this.filters.reset();const previousPage=this.session.state?.id===id?this.activePageID:'';++this.epoch;this.closePreview();this._uiPrivateCharts.clear();if(await this.session.open(id,stage,revision)){if(this._uiRecoveryOwner!==id)this.resetRecovery(id);this.capabilities=this.session.capabilities;this.newID=id;this.activePageID=reportPages(this.session.definition).find(p=>p.id===previousPage)?.id||reportPages(this.session.definition)[0]?.id||'';this._uiPageGeneration++;this.selectedWidget=this.activePage()?.widgets[0]?.id||'';this.inlineWidget='';this.showLibrary=false;this.startingNew=false;this._uiCatalogCollapsed=true;this.panelTab='components';this.message='';return true;}return false; }
  async editPublished() {
    const target=this.selected?.target;
    if(!target||!this.capabilities.can_save||!this.authoring('capabilities','read','save'))throw appError(FORBIDDEN);
    if(!await this.openDraft(target.id,'',target.revision))return;
    // Continue an existing private working copy instead of replacing its pointer
    // with an amendment of the older publication. Each read rechecks authority.
    const state=this.session.state;
    if(state.draft_revision&&state.draft_revision!==this.session.revision){if(!await this.openDraft(target.id,'draft'))return;}
    else if(state.review_revision&&state.review_revision!==this.session.revision){if(!await this.openDraft(target.id,'review'))return;}
    this.mode='builder';
    this.message=this.session.stage==='published'?'Editing a published report. Save creates a private draft; readers keep the published version.':'';
  }
  beginNewReport() {this._uiCatalogCollapsed=true;this.canvasExpanded=false;this.resetLibrary();this.pageSettingsOpen=false;this.publication.invalidate();this.filters.reset();if(!this.createTools())throw appError(FORBIDDEN);this.closePreview();this.startingNew=true;this.session.definition=null;this.session.state=null;this.session.dirty=false;this.newID='';this.newTitle=this.allocations.get('new-report')?.request.title||'Untitled report';if(!this.allocations.has('new-report'))this.newStarter='blank';}
  allocation(key,details) {if(this.closed)throw appError(UNAVAILABLE);let value=this.allocations.get(key);if(value){if(!value.matches(details))throw appError(STALE_VALIDATION);return value;}if(!allocationAvailable(this.adapter)){const e=appError(UNAVAILABLE);e.allocationUnavailable=true;throw e;}if(this.allocations.size>=32)throw appError(LIMIT_EXCEEDED);value=new TargetAllocation(this.adapter,details);this.allocations.set(key,value);return value;}
  async resumeAllocation(key) {const epoch=this.epoch,allocation=this.allocations.get(key);if(!allocation?.unknown)throw appError(INVALID_REQUEST);await allocation.obtain();if(!this.closed&&epoch===this.epoch)this.message='Your host confirmed the private target. Continue editing, then save or prepare when ready.';}
  async createDraft() { if(!this.createTools())throw appError(FORBIDDEN);if(!this.startingNew||this.closed)throw appError(BUSY);const epoch=this.epoch,allocation=this.allocation('new-report',{kind:'report',intent:'create_report',title:this.newTitle}),id=await allocation.obtain();if(this.closed||epoch!==this.epoch||!this.startingNew)return;const capabilities=await this.invoke(authoringTool('capabilities'),{report:id});if(this.closed||epoch!==this.epoch||!this.startingNew)return;this.session.setCapabilities(capabilities);this.capabilities=this.session.capabilities;if(!this.capabilities.can_create)throw appError(FORBIDDEN);this.newID=id;this.closePreview();this._uiPrivateCharts.clear();this.session.new(allocation.request.title,this.locale,this.timezone);this.session.definition=applyStarter(this.session.definition,this.newStarter);this.mode='builder';this.allocations.delete('new-report');allocation.close();this.activePageID='main';this._uiPageGeneration++;this.startingNew=false;this.resetRecovery(this.newID);this._uiCatalogCollapsed=true;this.selectedWidget='';this.inlineWidget='';this.panelTab='components';this.message='This new draft is only in this window until you save.';this.render(); }
  edit(fn) { try{this.session.edit(fn);this.publication.invalidate();this.error=false;this.message='Unsaved changes';this.render();}catch(e){this.reportError(e);} }
  activePage() { try{return pageContent(this.session.definition,this.activePageID);}catch{return null;} }
  editPage(fn,id=this.activePageID) { if(id!==this.activePageID||this.closed)return;this.edit(d=>fn(pageContent(d,id))); }
  switchPage(id,consumer=false) {
    const pages=consumer?this.preview?.pages:reportPages(this.session.definition);
    if(this.closed||this.busy||this.mapping||this.datasetSession||!pages?.some(p=>p.id===id))return;
    this.filters.editor=null;this.blockSearch.invalidate();this.blockDescription=null;this._uiBlockDescriptionGeneration++;this.pageSettingsOpen=false;this.cancelDrag(false);this._uiPageGeneration++;this.widgetParameters=null;this.inlineWidget='';
    if(consumer)this.consumerPageID=id;else{this.activePageID=id;this.selectedWidget='';}
    if(this.preview&&(consumer||this.compatiblePreview())&&this.retained.value&&!this.retained.pageReady(id))return this.perform(async()=>{const epoch=this.epoch;try{await this.retained.loadPage(id,()=>{if(!this.closed&&epoch===this.epoch)this.render();});}catch(error){if(this.closed||epoch!==this.epoch)return;this.closePreview();throw error;}});
    this.render();
  }
  changePages(transform,nextPage=this.activePageID) {
    if(!this.canEdit())return;
    try{this.session.edit(d=>{const next=transform(d);for(const key of Object.keys(d))delete d[key];Object.assign(d,next);});this.publication.invalidate();this.cancelDrag(false);this.activePageID=nextPage;this._uiPageGeneration++;if(!this.activePage()?.widgets.some(w=>w.id===this.selectedWidget))this.selectedWidget='';this.widgetParameters=null;this.inlineWidget='';this.message='Unsaved page changes';this.error=false;this.render();}catch(e){this.reportError(e);}
  }
  enablePages() { this.changePages(upgradeReportPages,'main'); }
  addPage() { const used=new Set(reportPages(this.session.definition).map(p=>p.id));let index=1;while(used.has('page-'+index))index++;const id='page-'+index;this.changePages(d=>addReportPage(d,id,'Page '+(reportPages(d).length+1)),id); }
  removePage() {const pages=reportPages(this.session.definition),index=pages.findIndex(p=>p.id===this.activePageID);if(index<0||pages.length<2)return;const next=pages[index+1]?.id||pages[index-1].id;this.changePages(d=>removeReportPage(d,this.activePageID),next);}
  renderPageTabs(parent,pages,active,consumer=false) {
    const tabs=node('nav',undefined,'report-page-tabs');tabs.setAttribute('aria-label',consumer?'Retained report pages':'Report pages');
    for(const page of pages){const b=action(page.title||page.id,()=>this.switchPage(page.id,consumer),{disabled:this.busy||!!this.mapping||!!this.datasetSession});b.dataset.page=page.id;b.setAttribute('aria-current',page.id===active?'page':'false');tabs.append(b);}parent.append(tabs);return tabs;
  }
  renderPageControls(parent,definition,editable) {
    if(definition.schema_version===2){parent.append(action('Enable pages',()=>this.enablePages(),{disabled:!editable}));return;}
    const pages=reportPages(definition),tabs=this.renderPageTabs(parent,pages,this.activePageID);tabs.append(action('Add page',()=>this.addPage(),{disabled:!editable||pages.length>=100}));const page=this.activePage();if(!page)return;
    const controls=node('details',undefined,'page-controls'),epoch=this.epoch,current=()=>!this.closed&&this.session.definition===definition&&this.activePageID===page.id;controls.open=this.pageSettingsOpen;this._uiPageSettingsElement=controls;controls.addEventListener('toggle',()=>{if(!this.closed&&epoch===this.epoch&&page.id===this.activePageID&&this._uiPageSettingsElement===controls)this.pageSettingsOpen=controls.open;});controls.append(node('summary','Page settings'),inputField('Page title',page.title,value=>{if(current())this.changePages(d=>renameReportPage(d,page.id,value));},{disabled:!editable}));
    for(const [kind,label] of [['locale','Page locale'],['timezone','Page timezone']])controls.append(optionsField(label,pageSettingChoices(definition,page.id,kind),page[kind]||'',value=>{if(current())this.changePages(d=>setPageSetting(d,page.id,kind,value));},{disabled:!editable}));
    controls.append(node('p','Page settings override report defaults. Formatting supports English and Spanish; other locales may use the service default. Save and preview to refresh values.','metadata'));
    const index=pages.findIndex(p=>p.id===this.activePageID);controls.append(action('Move page left',()=>{if(current())this.changePages(d=>moveReportPage(d,page.id,-1));},{disabled:!editable||index===0}),action('Move page right',()=>{if(current())this.changePages(d=>moveReportPage(d,page.id,1));},{disabled:!editable||index===pages.length-1}),action('Remove page',()=>{if(current())this.removePage();},{disabled:!editable||pages.length===1}),node('p','Page changes are local until Save report. Removing a page removes its components from this draft.','metadata'));parent.append(controls);
  }
  addHeading() { const id=unusedWidgetID(this.session.definition,'heading');this.editPage(d=>{d.widgets.push({id,kind:'text',grid:firstFreeCell(d.widgets,{width:12,height:1}),presentation:{},text:{format:'plain',text:'Section heading'}});});this.selectedWidget=id;this.render(); }
  async loadBlocks(after='',query=after?this.blockSearch.query:this.blockSearch.input) {
    if(!this.blockTools())throw appError(FORBIDDEN);const epoch=this.epoch,page=this.activePageID,generation=this._uiPageGeneration;
    if(!after){this.blockCatalog=[];this.blockNext='';this.blockDescription=null;this._uiBlockDescriptionGeneration++;}
    const result=await this.blockSearch.load({after,query,locale:after?this.blockSearch.locale:this.locale,current:()=>!this.closed&&epoch===this.epoch&&page===this.activePageID&&generation===this._uiPageGeneration});
    if(result){this.blockCatalog=result.items;this.blockNext=result.next;this.showLibrary=true;this.panelTab='components';}
  }
  async chooseBlock(item) {
    if(!this.blockTools())throw appError(FORBIDDEN);const epoch=this.epoch,page=this.activePageID,generation=this._uiPageGeneration,selection=++this._uiBlockDescriptionGeneration;this.blockDescription=null;
    const d=await this.invoke('reporting_describe',{target:item.target,locale:this.locale,outputs:null});if(this.closed||epoch!==this.epoch||page!==this.activePageID||generation!==this._uiPageGeneration||selection!==this._uiBlockDescriptionGeneration)return;
    if(d.resource?.target?.kind!=='block'||d.resource.target.id!==item.target.id||d.resource.target.revision!==item.target.revision)throw appError(STALE_VALIDATION);this.blockDescription=d;
  }

  addOutput(output) { if(!this.blockTools())throw appError(FORBIDDEN);const description=this.blockDescription;if(!description||output.enabled===false||!['chart','kpi','table'].includes(output.kind))return;const id=unusedWidgetID(this.session.definition,'widget'),size=output.kind==='kpi'?{width:4,height:3}:output.kind==='chart'?{width:8,height:4}:{width:12,height:4};this.editPage(d=>{d.widgets.push({id,kind:'block',grid:starterPlacement(d,output.kind,size),presentation:{title:output.title||output.id},block:{block:description.resource.target.id,revision:description.resource.target.revision,outputs:[output.id],policy:'published',narrative:false}});});this.selectedWidget=id;this.showLibrary=true;this.panelTab='components';this.render(); }
  addFilter(widget,parameter) { if(!this.blockTools())throw appError(FORBIDDEN);let index=1;while(this.activePage()?.filters?.some(f=>f.parameter.name==='filter_'+index))index++;const name='filter_'+index;this.editPage(d=>{d.filters||=[];if(d.filters.some(f=>f.parameter.name===name))throw appError(CONFLICT);d.filters.push({parameter:{...copyData(parameter),name},label:parameter.column?.name||parameter.name});const w=d.widgets.find(w=>w.id===widget.id);w.bindings||=[];if(w.bindings.some(b=>b.parameter===parameter.name))throw appError(CONFLICT);w.bindings.push({filter:name,parameter:parameter.name});});this.render(); }
  matchingFilters(parameter) {const identity=p=>{const value=copyData(p);delete value.name;delete value.default;return JSON.stringify(value);};return (this.activePage()?.filters||[]).filter(f=>identity(f.parameter)===identity(parameter));}
  bindFilter(widget,parameter,name){this.editPage(d=>{const w=d.widgets.find(w=>w.id===widget.id);if(!w||!d.filters?.some(f=>f.parameter.name===name)||w.bindings?.some(b=>b.parameter===parameter.name))throw appError(STALE_VALIDATION);w.bindings||=[];w.bindings.push({filter:name,parameter:parameter.name});});}
  async loadWidgetParameters(widget) { if(!this.blockTools())throw appError(FORBIDDEN);const epoch=this.epoch,page=this.activePageID,generation=this._uiPageGeneration,reference=copyData(widget.block);let filters;
    if(reference.policy==='private_preview'){const d=await this.invoke(authoringTool('block_read'),{block:reference.block,revision:reference.revision});if(d.block?.state?.id!==reference.block||d.block.revision!==reference.revision||d.block.digest!==reference.digest||!d.block.private)throw appError(STALE_VALIDATION);filters=(d.block.parameters||[]).map(parameter=>({parameter,label:parameter.column?.name||parameter.name}));}
    else{const d=await this.invoke('reporting_describe',{target:{kind:'block',id:reference.block,revision:reference.revision},locale:this.locale,outputs:reference.outputs});if(d.resource?.target?.id!==reference.block||d.resource.target.revision!==reference.revision)throw appError(STALE_VALIDATION);filters=d.filters||[];}
    if(this.closed||epoch!==this.epoch||page!==this.activePageID||generation!==this._uiPageGeneration)return;this.widgetParameters={page,widget:widget.id,filters}; }

  canEdit() { return !this.mapping&&!this.datasetSession&&!!this.session.definition&&editableDocument(this.session.definition,name=>this.tools(name))&&(this.session.state?this.session.capabilities.can_save:this.session.capabilities.can_create)&&!this.busy&&!this.session.conflict&&!this.session.uncertain&&this.session.stage!=='private_revision'&&!this.publication.blocked(this.session.state?.id); }
  selectWidget(id) { if(this.mapping||this.datasetSession)return;this.canvasExpanded=false;this.selectedWidget=id;this.panelTab='selected';this.widgetParameters=null;this.inlineWidget='';this.render(); }
  geometryEdit(transform) {
    if(!this.canEdit())return;
    try{this.session.edit(d=>{const page=pageContent(d,this.activePageID);page.widgets=transform(page.widgets);});this.error=false;this.message='Unsaved layout changes';this.render();}
    catch(e){if(e.code===INVALID_REQUEST){this.error=true;this.message='That position overlaps another component or falls outside the grid. Your layout is unchanged.';this.render();}else this.reportError(e);}
  }
  setWidgetGrid(id,field,value) { const n=Number(value);this.geometryEdit(widgets=>{const w=widgets.find(w=>w.id===id);if(!w||!Number.isSafeInteger(n))throw appError(INVALID_REQUEST);return placeWidget(widgets,id,{...w.grid,[field]:['column','row'].includes(field)?n-1:n});}); }
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
  async save() { if(this.publication.blocked(this.session.state?.id))throw appError(BUSY);this.publication.invalidate();if(await this.session.save(this.newID)){this.message=`Saved private draft revision ${this.session.revision}.`;await this.refreshCapabilities(this.session.state.id);if(this.authoring('drafts'))await this.loadDrafts();} }
  async inspectPublication() {
    if(this.session.dirty||this.mapping||this.datasetSession||!this.session.state)throw appError(BUSY);
    const epoch=this.epoch;
    try{const view=await this.publication.open(this.session.state.id,this.session.revision);
      if(!view||this.closed||epoch!==this.epoch)return;
      // This read may refresh head pointers but never rewrites retained preview provenance.
      this.session.replace(view.report);this.message='Publication status inspected. Review all outputs and confirm each separate change.';
    }catch(error){if(this.closed||epoch!==this.epoch)return;this.clearChartReadFailure(error);throw error;}
  }
  async adoptPublication(op,epoch) {
    if(!op||this.closed||epoch!==this.epoch)return;
    const message=this.publication.message,revision=['rebind_published','reject'].includes(op.action)?op.state.draft_revision:op.args.block?this.session.revision:op.args.revision;
    let view;try{view=await this.publication.open(op.report,revision);}catch(error){error.publicationMessage=message+' The follow-up inspection is unavailable. Inspect this exact revision before continuing.';throw error;}if(!view||this.closed||epoch!==this.epoch)return;
    this.session.replace(view.report);this.message=message;this._uiPrivateCharts.clear();
    if(this.authoring('drafts'))await this.loadDrafts();
  }
  async publishAction(action,target) {
    if(!this.publication.current(this.session)||this.mapping||this.datasetSession)throw appError(STALE_VALIDATION);
    const epoch=this.epoch;this.publication.message='';
    try{await this.adoptPublication(await this.publication.mutate(action,target),epoch);}
    catch(error){if(this.closed||epoch!==this.epoch)return;this.clearChartReadFailure(error);error.publicationMessage=error.publicationMessage||this.publication.message||'The change could not be confirmed. Inspect the exact revision before continuing.';throw error;}
  }
  async publishChartBatch() {
    if(!this.publication.current(this.session)||this.mapping||this.datasetSession)throw appError(STALE_VALIDATION);
    const epoch=this.epoch;this.publication.message='';
    try{await this.adoptPublication(await this.publication.publishChartBatch(()=>{if(!this.closed&&epoch===this.epoch){this.message=this.publication.message;this.render();}}),epoch);}
    catch(error){if(this.closed||epoch!==this.epoch)return;this.clearChartReadFailure(error);error.publicationMessage=this.publication.message||'Chart publication stopped. Inspect current status before continuing.';throw error;}
  }
  async retryPublicationOperation(op) {
    if(this.session.dirty||this.mapping||this.datasetSession||op.report!==this.session.state?.id)throw appError(BUSY);const epoch=this.epoch;this.publication.message='';this.publication.message='';
    try{await this.adoptPublication(await this.publication.retry(op),epoch);}
    catch(error){if(this.closed||epoch!==this.epoch)return;this.clearChartReadFailure(error);error.publicationMessage=error.publicationMessage||this.publication.message;throw error;}
  }
  async inspectPublicationOperation(op) {
    const epoch=this.epoch;
    try{const view=await this.publication.inspectOperation(op);if(!view||this.closed||epoch!==this.epoch)return;
      if(view.report&&!this.session.dirty&&view.report.state.id===this.session.state?.id)this.session.replace(view.report);
      this.message=this.publication.message;
    }catch(error){if(this.closed||epoch!==this.epoch)return;this.clearChartReadFailure(error);throw error;}
  }
  async previewDraft() { if(this.session.dirty||!this.session.capabilities.can_preview||this.uncertainRun||!!this.mapping||!!this.datasetSession||!this.privatePreviewReady())throw appError(FORBIDDEN);const epoch=this.epoch,report=this.session.state.id,revision=this.session.revision;this.closePreview();this.resetRecovery(report);this._uiLastPreviewDefinition=copyData(this.session.definition);this._uiLastPreviewRevision=revision;this.filters.accepted=this.filters.snapshot();this.uncertainRun=true;let result;try{result=await this.invoke(authoringTool('preview'),{report,key:crypto.randomUUID(),revision,resolution:{at:new Date().toISOString(),timezone:this.session.definition.timezone},pages:this.filters.pageInputs(this.session.definition)});if(this.closed||epoch!==this.epoch)return;if(!validID(result.id)||result.private!==true)throw appError(UNAVAILABLE,true);this._uiLastMutationRun=result.id;this._uiLastMutationPrivate=true;result=await this.invoke(authoringTool('execute'),{run:result.id,resume:false});if(this.closed||epoch!==this.epoch)return;await this.openRun(result.id,true);}catch(e){if(!this.closed&&epoch===this.epoch&&this._uiRecoveryOwner===report)this.uncertainRun=!!this._uiLastMutationRun||e.unknown===true||[UNAVAILABLE,CANCELLED_OR_TIMED_OUT].includes(e.code);throw e;} }
  async runPublished() { if(!this.capabilities.can_execute||!this.description||this.uncertainRun)throw appError(FORBIDDEN);const d=this.description,epoch=this.epoch,pages=this.filters.publishedInputs();this.filters.accepted=this.filters.snapshot();this.closePreview();this.resetRecovery(d.resource.target.id);this.uncertainRun=true;try{const r=await this.invoke('reporting_run',{target:d.resource.target,key:crypto.randomUUID(),arguments:[],pages,outputs:[],policy:'',locale:this.locale,timezone:d.timezone,narrative:false,dynamic:false,partial_failure:''});if(this.closed||epoch!==this.epoch)return;this._uiLastMutationRun=r.run;this._uiLastMutationPrivate=false;await this.openRun(r.run);}catch(e){if(!this.closed&&epoch===this.epoch&&this._uiRecoveryOwner===d.resource.target.id)this.uncertainRun=!!this._uiLastMutationRun||e.unknown===true||[UNAVAILABLE,CANCELLED_OR_TIMED_OUT].includes(e.code);throw e;} }
  async openRun(run,privateExpected=false,publishedTarget=null) {
    const epoch=this.epoch;this.closePreview();const generation=this._uiPreviewGeneration;let v;
    try{
      v=await this.invoke('reporting_view',{kind:'report',run,page:'',widget:'',output:'',offset:0,limit:100});
      if(this.closed||epoch!==this.epoch||generation!==this._uiPreviewGeneration)return;
      if(v?.version!=='reporting-view-v1'||v.summary?.run!==run||v.selection?.run!==run||v.summary.kind!=='report'||v.selection.kind!=='report'||privateExpected&&!v.summary.private)throw appError(UNAVAILABLE);
      const expectedReport=privateExpected&&run===this._uiLastMutationRun?this._uiRecoveryOwner:(this.mode==='builder'?this.session.state?.id:this.selected?.target.id)||this.session.state?.id;
      if(expectedReport&&v.summary.target?.id!==expectedReport||privateExpected&&run===this._uiLastMutationRun&&this._uiLastPreviewRevision&&v.summary.target?.revision!==this._uiLastPreviewRevision)throw appError(STALE_VALIDATION);
      if(publishedTarget&&(v.summary.private!==false||v.summary.target?.kind!==publishedTarget.kind||v.summary.target?.id!==publishedTarget.id||v.summary.target?.revision!==publishedTarget.revision))throw appError(STALE_VALIDATION);
      const preferred=this.mode==='builder'?this.activePageID:this.consumerPageID,page=v.pages?.some(p=>p.id===preferred)?preferred:v.pages?.[0]?.id||'';
      const progress=()=>{
        if(this.closed||epoch!==this.epoch||generation!==this._uiPreviewGeneration)return;
        this.preview=v;this.consumerToolsOpen=false;this._uiConsumerToolsElement=null;this.consumerPageID=page;
        if(this.mode==='consumer'&&!this.selected)this.selected={target:copyData(v.summary.target),title:v.pages?.[0]?.title||v.summary.target.id};
        if(v.summary.private&&run===this._uiLastMutationRun&&this._uiLastPreviewDefinition&&v.summary.target.id===this._uiRecoveryOwner&&v.summary.target.revision===this._uiLastPreviewRevision){this._uiPreviewDefinition=copyData(this._uiLastPreviewDefinition);this._uiPreviewOrigin={report:this._uiRecoveryOwner,revision:this._uiLastPreviewRevision};}
        this.message='';this.render();
      };
      if(!await this.retained.load(v,{page,onProgress:progress})||this.closed||epoch!==this.epoch||generation!==this._uiPreviewGeneration)return;
      this.observeTerminalRecovery(v,run);progress();
    }catch(e){if(this.closed||epoch!==this.epoch||generation!==this._uiPreviewGeneration)return;this.closePreview();if(privateExpected&&run===this._uiLastMutationRun&&this._uiLastMutationPrivate&&[FORBIDDEN,'unauthenticated','not_found'].includes(e.code))e.needsRunAuthority=true;throw e;}
  }
  closePreview() { this.consumerToolsOpen=null;this._uiConsumerToolsElement=null;this._uiPreviewGeneration++;this.retained.clear();this.preview=null;this._uiPreviewDefinition=null;this._uiPreviewOrigin=null;if(!this.closed)this.render(); }
  hostResult(result) { if(this.closed||!this.tools('reporting_view'))return;try{const value=unpack(result);if(value.version==='reporting-view-v1'&&value.summary?.kind==='report'&&validID(value.summary.run))void this.perform(()=>this.openRun(value.summary.run,value.summary.private===true));}catch{/* Opening tools may also return capabilities or a saved draft state. */} }
  formattingPreviewReady() {const s=this.mapping;return s?.purpose==='presentation'&&s.dirty&&!s.pending&&!s.unknown&&!s.conflict&&s.valid();}
  compatiblePreview() { return (this.filters.accepted===null||this.filters.accepted===this.filters.snapshot())&&(!this.mapping?.dirty||this.formattingPreviewReady())&&!!this._uiPreviewDefinition&&this._uiPreviewOrigin?.report===this.session.state?.id&&executionFingerprint(this._uiPreviewDefinition)===executionFingerprint(this.session.definition); }
  async pageOutput(page,widget,output,offset) {
    const run=this.preview?.summary.run;
    try{await this.retained.page(page,widget,output,offset);}
    catch(e){this.preview=null;this._uiPreviewDefinition=null;this._uiPreviewOrigin=null;if(run===this._uiLastMutationRun&&this._uiLastMutationPrivate&&[FORBIDDEN,'unauthenticated','not_found'].includes(e.code))e.needsRunAuthority=true;throw e;}
  }
  renderPreviewStatus(parent,builder=false) {
    if(!this.preview)return;
    const v=this.preview,status=node('section',undefined,'preview-provenance'),compatible=!builder||this.compatiblePreview();
    status.append(node('span',v.summary.private?'Private preview':'Retained report','badge'),node('span',v.summary.state,'badge'),node('p',`Values from revision ${v.summary.target.revision}`,'metadata'));
    if(!builder&&this.filters.accepted!==null&&this.filters.accepted!==this.filters.snapshot())status.append(notice('These retained values use earlier filter selections. Run explicitly to update them.',true));
    if(!builder&&v.summary.private)status.append(node('p','This private retained preview is not the published version.','metadata'));
    if(builder&&compatible&&this.formattingPreviewReady())status.append(node('p','Unsaved formatting preview · retained values unchanged.','metadata'));
    if(builder&&!compatible)status.append(notice('Preview is stale. Save and preview to update retained values.',true));
    if(v.redacted)status.append(notice('Some content is not visible under the current authority.'));
    if(v.mixed_freshness)status.append(notice('This report contains results observed at different times.'));
    const provenance=node('details');provenance.append(node('summary','Result provenance'),node('p',`Run ${v.summary.run}. ${v.summary.private?'This retained result stays private. ':''}Opening and paging do not run queries.`,'metadata'),node('p',`Retained until ${v.summary.expires_at} · ${v.locale} · ${v.timezone}`,'metadata'));
    if(builder&&compatible)provenance.append(node('p','Retained values, with current presentation and layout. Editing never runs a query.','metadata'));if(v.observed_at)provenance.append(node('p',`Observed: ${v.observed_at}`,'metadata'));
    if(v.summary.code)provenance.append(node('p',v.summary.code,'metadata'));
    status.append(provenance,action('Close retained view',()=>{this.closePreview();this.render();},{disabled:this.busy}));parent.append(status);
  }
  renderCanvas(parent,widgets,{builder=false,page='main'}={}) {
    const retained=!!this.preview,compatible=retained&&(!builder||this.compatiblePreview()),canvas=node('section',undefined,`composition-canvas${retained?' retained-preview':''}${builder?' editing-canvas':' reading-canvas'}`);
    canvas.setAttribute('aria-label',builder?'Report composition':'Retained report layout');canvas.style.gridTemplateRows=`repeat(${Math.max(builder&&!widgets.length?6:1,...widgets.map(w=>w.grid.row+w.grid.height))},${builder?'80px':'minmax(80px,auto)'})`;
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
        for(const output of outputs){const content=node('section',undefined,'retained-output');content.dataset.output=output;content.setAttribute('aria-label',`Retained output ${output}`);let view=compatible?this.retained.get(page,widget.id,output):null;
          if(view){try{if(builder&&this.formattingPreviewReady()&&this._uiMappingContext?.page===page&&this._uiMappingContext.widget===widget.id&&this.mapping.output===output)view=formattingPreview(view,this.mapping.view.block.outputs.find(o=>o.id===output),this.mapping.draft);renderRetainedOutput(content,view,{locale:this.locale,contextTitle:label,onPage:offset=>void this.perform(()=>this.pageOutput(page,widget.id,output,offset))});if(view.trust?.health?.status&&view.trust.health.status!=='healthy')content.append(notice(`Source health: ${view.trust.health.status}`));if(view.observed_at||view.trust){const details=node('details',undefined,'output-provenance');details.append(node('summary','Source and freshness'));if(view.observed_at)details.append(node('p',`Observed: ${view.observed_at}`,'metadata'));if(view.trust)details.append(node('p',`Publication / certification / current health: ${view.trust.publication} / ${view.trust.certification} / ${view.trust.health?.status||'unknown'}`,'metadata'));content.append(details);}}catch{content.replaceChildren(notice('Retained output unavailable.',true));}}
          else{const placeholder=node('div',undefined,'output-placeholder');placeholder.append(componentGlyph(first?.output?.kind||'chart'),node('p',retained?(compatible?(this.retained.loadingPage?'Loading retained values…':`Retained output: ${widget.state||this.preview.summary.state}`):'Preview is stale. Save and preview to update retained values.'):widget.block?.policy==='private_preview'?(hasFreshMappingEvidence(this._uiPrivateCharts.get(this.mappingKey(widget.block)))?'Private chart · use Private preview to see retained values.':'Private chart · Unvalidated or validation status not checked.'):'Approved output · values appear after a private preview.'));content.append(placeholder);}
          body.append(content);
        }
        if(!outputs.length)body.append(node('p',widget.kind==='block'?`No retained outputs · ${widget.state||'not included'}`:'This retained widget type is not supported in this app.','notice'));
      }
      if(builder&&this.selectedWidget===widget.id&&this.canEdit())this.renderGridHandles(card,canvas,widget);
      canvas.append(card);
    }
    if(!widgets.length){const empty=node('div',undefined,'canvas-empty');empty.append(componentGlyph('grid'),node('h2',builder?'Start with a component':'This page is empty'),node('p',builder?(this.session.definition.schema_version===3?(this.blockTools()?'Add a heading or an approved output from Components. Empty pages can be saved.':'Add a heading from Components. Empty pages can be saved.'):'Add a heading from Components to enable Save report.'):'No components were retained on this page.'));canvas.append(empty);}
    parent.append(canvas);
  }
  renderRetainedReport(parent) {
    this.renderPreviewStatus(parent);
    const pages=this.preview?.pages||[];if(pages.length>1)this.renderPageTabs(parent,pages,this.consumerPageID,true);const page=pages.find(p=>p.id===this.consumerPageID);if(page)this.renderCanvas(parent,page.widgets,{page:page.id});
  }
  render() {
    if(this.closed)return;const focus=captureEditorFocus(this.root);this._uiConsumerToolsElement=null;this.cancelDrag(false);this.root.replaceChildren();this.root.dataset.mode=this.mode;this.root.dataset.document=this.mode==='builder'?(this.session.state?.id||this.newID):(this.selected?.target.id||'');this.root.dataset.selection=this.selectedWidget;this.root.dataset.page=this.mode==='builder'?this.activePageID:this.consumerPageID;
    const header=node('header',undefined,'app-header'),brand=node('div',undefined,'app-brand');brand.append(componentGlyph('grid'),node('h1','Reports'));
    const toggle=action(this._uiCatalogCollapsed?'Show reports':'Hide reports',()=>{this._uiCatalogCollapsed=!this._uiCatalogCollapsed;this.render();});toggle.className='catalog-toggle';toggle.setAttribute('aria-expanded',String(!this._uiCatalogCollapsed));toggle.hidden=!this.selected&&!this.session.definition&&!this.startingNew;
    const tabs=node('nav',undefined,'mode-tabs');tabs.setAttribute('aria-label','Report mode');for(const [mode,label,enabled] of [['consumer','Browse',this.capabilities.consumer],['builder','Build',this.capabilities.builder]]){if(mode==='consumer'?!this.tools('reporting_search','reporting_describe','reporting_authoring_capabilities_v1'):!this.authoring('capabilities','read','save'))continue;const b=action(label,()=>this.navigate(()=>this.changeMode(mode)),{disabled:!enabled||this.busy});b.setAttribute('aria-current',this.mode===mode?'page':'false');tabs.append(b);}header.append(brand,toggle,tabs);if(this.mode==='consumer'&&this.createTools()&&allocationAvailable(this.adapter))header.append(action('New report',()=>this.navigate(async()=>this.beginNewReport()),{primary:true,disabled:this.busy}));this.root.append(header);
    if(this.message)this.root.append(notice(this.message,this.error));if(this.busy)this.root.append(notice('Working…'));
    if(this._uiPendingNavigation){const prompt=node('section',undefined,'notice');prompt.append(node('p','You have unsaved edits. Discard them and continue?'),action('Keep editing',()=>{this._uiPendingNavigation=null;this.render();}),action('Discard and continue',()=>{const fn=this._uiPendingNavigation;this._uiPendingNavigation=null;this.session.dirty=false;void this.perform(fn);}));this.root.append(prompt);}
    const layout=node('div',undefined,`app-layout${this._uiCatalogCollapsed?' catalog-collapsed':''}`),sidebar=node('aside',undefined,'catalog'),workspace=node('section',undefined,'workspace');const library=!this.startingNew&&(this.mode==='builder'?!this.session.definition:!this.selected);layout.dataset.library=String(library);sidebar.hidden=!library&&this._uiCatalogCollapsed;layout.append(sidebar,workspace);this.root.append(layout);
    if(this.mode==='builder')this.renderBuilder(sidebar,workspace);else this.renderConsumer(sidebar,workspace);restoreEditorFocus(this.root,focus);this.filters.restoreFocus();

  }
  renderCreation(workspace) {
    const allocation=this.allocations.get('new-report'),available=this.createTools()&&allocationAvailable(this.adapter),locked=this.busy||!!allocation,section=node('section',undefined,'report-creation');
    section.append(node('p','Start with a purpose','eyebrow'),node('h2','Create a private report'),node('p','Choose a starting point. Connect your reviewed data, arrange the story, then preview and publish when ready.','muted'));
    renderStarterChoices(section,this.newStarter,locked,id=>{if(this.startingNew&&!this.busy&&!this.allocations.has('new-report')){this.newStarter=id;this.render();}});
    section.append(inputField('Report title',this.newTitle,value=>{if(this.startingNew&&!this.allocations.has('new-report'))this.newTitle=value;},{disabled:locked}),node('p',available?'Starters add editable pages and headings. Data, filters and publication are always your explicit choices. Save report keeps your work.':TARGET_ALLOCATION_UNAVAILABLE,'metadata'),action(allocation?.unknown?'Resume report creation':'Create report',()=>void this.perform(()=>this.createDraft()),{primary:true,disabled:this.busy||!available||!this.newTitle.trim()}));workspace.append(section);
  }
  renderConsumer(sidebar,workspace) {
    if(this.selected)sidebar.append(node('h2','Published reports'));if(this.tools('reporting_search','reporting_describe')){const toolbar=node('div',undefined,'catalog-toolbar');this.renderCatalogSearch(toolbar,this.reportSearch,(after,query)=>this.loadCatalog(after,query),'reports',this.capabilities.consumer===true);toolbar.append(action('Refresh catalog',()=>void this.perform(()=>this.loadCatalog('',this.reportSearch.query)),{disabled:this.busy||!this.capabilities.consumer}));sidebar.append(toolbar);}const list=node('div',undefined,'catalog-list');for(const item of this.catalog){const card=node('article',undefined,'catalog-card'),b=action(item.title||item.target.id,()=>void this.perform(()=>this.selectPublished(item)),{disabled:this.busy});b.className='catalog-item';b.setAttribute('aria-pressed',String(this.selected?.target.id===item.target.id));card.append(b);if(item.description)card.append(node('p',item.description,'metadata'));card.append(node('span',`Published · revision ${item.target.revision}`,'metadata'));list.append(card);}sidebar.append(list);if(this.catalogNext)sidebar.append(action('More reports',()=>void this.perform(()=>this.loadCatalog(this.catalogNext)),{disabled:this.busy}));if(!this.catalog.length)sidebar.append(node('p',this.catalogNext?'No matches in these catalog pages. More reports may contain matches.':this.reportSearch.query||this.reportSearch.limited?'No matches in the catalog pages checked.':'No published reports are visible.','empty'));
    if(this.startingNew){this.renderCreation(workspace);return;}
    if(!this.selected){workspace.append(node('h2','Choose a published report'),node('p',this.tools('reporting_run','reporting_view')?'Open retained results or explicitly run an approved report with business filters.':'Open available retained report results.','muted'));return;}
    const heading=node('header',undefined,'reader-header'),identity=node('div',undefined,'reader-identity');identity.append(node('p',`Current published revision ${this.selected.target.revision}`,'metadata'),node('h2',this.selected.title||'Untitled report'));if(this.selected.description)identity.append(node('p',this.selected.description,'muted'));heading.append(identity);if(this.capabilities.can_save&&this.authoring('capabilities','read','save'))heading.append(action('Edit report',()=>this.navigate(()=>this.editPublished()),{disabled:this.busy}));workspace.append(heading);
    if(this.preview)this.renderRetainedReport(workspace);
    const consumerTools=node('details',undefined,'consumer-tools'),consumerEpoch=this.epoch;consumerTools.open=this.consumerToolsOpen??!this.preview;this._uiConsumerToolsElement=consumerTools;consumerTools.addEventListener('toggle',()=>{if(!this.closed&&this.mode==='consumer'&&this.epoch===consumerEpoch&&this._uiConsumerToolsElement===consumerTools)this.consumerToolsOpen=consumerTools.open;});consumerTools.append(node('summary',this.capabilities.can_execute?'Run and retained history':'Retained history'));workspace.append(consumerTools);
    if(this.description&&this.tools('reporting_run','reporting_view'))this.filters.renderRun(consumerTools);
    if(!this.tools('reporting_runs'))return;const history=node('section',undefined,'run-history');history.append(node('h3','Retained runs'),action('Refresh runs',()=>void this.perform(async()=>{const r=await this.invoke('reporting_runs',{kind:'report',resource:this.selected.target.id,after:'',limit:40});this.runs=r.items;this.runsNext=r.next;this.message=this.uncertainRun?'Review existing runs before starting a new operation.':'';}),{disabled:this.busy}));for(const run of this.runs){const row=node('div',undefined,'run-row');row.append(node('div',`${run.state} · ${run.created_at}${run.private?' · Private preview':''}`));if(this.tools('reporting_view'))row.append(action('Open retained run',()=>void this.perform(()=>this.openRun(run.run)),{disabled:this.busy||run.state==='expired'}));history.append(row);}if(!this.runs.length)history.append(node('p','No retained runs are visible.','metadata'));if(this.runsNext)history.append(action('More runs',()=>void this.perform(async()=>{const r=await this.invoke('reporting_runs',{kind:'report',resource:this.selected.target.id,after:this.runsNext,limit:40});this.runs=[...this.runs,...r.items].slice(0,200);this.runsNext=this.runs.length<200?r.next:'';}),{disabled:this.busy}));consumerTools.append(history);
  }
  renderBuilder(sidebar,workspace) {
    sidebar.append(node('h2','Private drafts and reviews'));if(this.createTools()&&allocationAvailable(this.adapter))sidebar.append(action('New report',()=>this.navigate(async()=>this.beginNewReport()),{primary:true,disabled:this.busy}));if(this.authoring('drafts'))sidebar.append(action('Refresh drafts',()=>void this.perform(()=>this.loadDrafts()),{disabled:this.busy}));const list=node('div',undefined,'catalog-list');for(const draft of this.drafts){const card=node('article',undefined,'catalog-card'),b=action(`${titleFor(draft.metadata,this.locale)}${draft.stage==='review'?' · Pending review':''}`,()=>this.navigate(()=>this.openDraft(draft.id,draft.stage==='review'?'review':'')),{disabled:this.busy});b.className='catalog-item';b.setAttribute('aria-pressed',String(this.session.state?.id===draft.id));card.append(b,node('span',`${draft.stage==='review'?'Pending review':'Private draft'} · revision ${draft.revision}`,'metadata'));list.append(card);}sidebar.append(list);if(this.draftsNext)sidebar.append(action('More drafts',()=>void this.perform(()=>this.loadDrafts(this.draftsNext)),{disabled:this.busy}));if(!this.drafts.length)sidebar.append(node('p',this.draftsNext?'No drafts on this page. More drafts may contain matches.':'No private drafts or reviews are visible.','empty'));
    if(this.startingNew){this.renderCreation(workspace);return;}const d=this.session.definition;if(!d){workspace.append(node('p','Your workspace','eyebrow'),node('h2',this.blockTools()?'Build from approved outputs':'Build a private report'),node('p',this.blockTools()?'Start a private report, add headings and published KPIs, charts or tables, then save and preview.':'Start a private report, add headings and pages, then save your work.','muted'));return;}
    const editable=this.canEdit(),documentBar=node('div',undefined,'document-bar'),identity=node('div',undefined,'document-identity');
    identity.append(inputField('Report title',titleFor(d.metadata,d.locale),value=>{this.edit(next=>{if(!value.trim())throw appError(INVALID_REQUEST);next.metadata.find(m=>m.locale===next.locale).title=value;});},{disabled:!editable}),node('span',this.session.state?`${stageLabel(this.session.stage)} · revision ${this.session.revision}${this.session.dirty?' · Unsaved changes':''}`:'Not saved yet','metadata'));
    if(this.session.capabilities.can_preview)identity.append(node('span',this.session.dirty?'Next: save your changes':!this.privatePreviewReady()?'Next: check chart statuses; validate any that need it':!this.preview||!this.compatiblePreview()?'Next: preview your report':'Next: review the result, then publish','metadata'));
    const controls=node('div',undefined,'document-actions');if(this.authoring('capabilities','read'))controls.append(action(this.session.stage==='published'&&this.publication.available()?'Reload published revision':'Reload latest',()=>this.navigate(()=>this.session.stage==='published'&&this.publication.available()?this.inspectPublication():this.openDraft(this.session.state?.id||this.newID)),{disabled:this.busy||!this.session.state&&!((this.session.conflict||this.session.uncertain)&&validID(this.newID))}));if(editableDocument(d,name=>this.tools(name))&&this.authoring(this.session.state?'save':'create'))controls.append(action('Save report',()=>void this.perform(()=>this.save()),{primary:true,disabled:!editable||!this.session.dirty||(d.schema_version===2&&!d.widgets?.length)}));if(this.authoring('preview','execute')&&this.tools('reporting_view'))controls.append(action('Private preview',()=>void this.perform(()=>this.previewDraft()),{disabled:this.busy||this.session.dirty||!this.session.state||!this.session.capabilities.can_preview||this.uncertainRun||!!this.mapping||!!this.datasetSession||!this.privatePreviewReady()}));if(this.authoring('block_read')&&!this.privatePreviewReady())controls.append(action('Check all chart statuses',()=>void this.perform(()=>this.checkAllChartStatuses()),{disabled:this.busy}));documentBar.append(identity,controls);workspace.append(documentBar);
    if(this.session.stage==='private_revision')workspace.append(notice('This older private revision is available for inspection. Open the current draft before editing.'));
    if(this.session.stage==='review'||this.session.state?.review_revision)workspace.append(notice(`Editing and saving creates a new draft. Pending review revision ${this.session.state.review_revision} remains unchanged.`));
    renderPublicationControls(workspace,this);
    if(this.tools('reporting_view')&&this._uiLastMutationRun&&this._uiRecoveryOwner===this.session.state?.id)controls.append(action('Inspect last retained run',()=>void this.perform(()=>this.openRun(this._uiLastMutationRun,this._uiLastMutationPrivate)),{disabled:this.busy}));
    if(!editableDocument(d,name=>this.tools(name))){workspace.append(notice('This draft contains content that this host cannot edit. It is available for inspection.'));return;}
    const pageBar=node('div',undefined,'page-bar');this.renderPageControls(pageBar,d,editable);workspace.append(pageBar);const active=this.activePage();if(!active){workspace.append(notice('The selected page is unavailable.',true));return;}
    const composition=node('div',undefined,'composition-editor'),canvasPanel=node('div',undefined,'canvas-panel'),toolbar=node('div',undefined,'canvas-toolbar');
    toolbar.append(action(this.canvasExpanded?'Show components':'Expand canvas',()=>{this.canvasExpanded=!this.canvasExpanded;this.render();},{disabled:this.busy}),optionsField('Layout',[...(arrangementPreset(active.widgets)==='custom'?[{value:'custom',label:'Custom positions'}]:[]),{value:'1',label:'Arrange in one column'},{value:'2',label:'Arrange in two columns'},{value:'3',label:'Arrange in three columns'}],arrangementPreset(active.widgets),value=>{if(value!=='custom')this.geometryEdit(widgets=>arrangeWidgets(widgets,Number(value)));}));canvasPanel.append(toolbar);this.renderPreviewStatus(canvasPanel,true);
    this.gridFeedback=node('p','Drag handles to move or resize. Arrow keys move; Shift + arrows resize.','grid-feedback');this.gridFeedback.setAttribute('role','status');canvasPanel.append(this.gridFeedback);
    renderStarterGuide(canvasPanel,active);this.renderCanvas(canvasPanel,active.widgets,{builder:true,page:this.activePageID});composition.dataset.expanded=String(this.canvasExpanded);composition.append(canvasPanel);if(!this.canvasExpanded)this.renderRightPanel(composition,active,editable);workspace.append(composition);
  }
  renderRightPanel(parent,definition,editable) {
    const panel=node('aside',undefined,'component-panel'),tabs=node('nav',undefined,'panel-tabs');tabs.setAttribute('aria-label','Component panel');
    if(this.filters.renderInspector(panel)){parent.append(panel);return;}
    for(const [value,label] of [['components','Components'],['selected','Selected']]){const b=action(label,()=>{this.panelTab=value;this.render();},{disabled:!!this.datasetSession&&value!=='components'||!!this.mapping&&value!=='selected'});b.setAttribute('aria-pressed',String(this.panelTab===value));tabs.append(b);}panel.append(tabs);
    const components=node('section',undefined,'component-library');components.hidden=this.panelTab!=='components';components.append(node('h2','Add to canvas'),node('p',this.blockTools()?'Compose with headings and approved output mappings.':'Compose with headings and pages.','metadata'));
    const filterEditor=this.filters.editor;if(filterEditor&&this.filters.current(filterEditor)&&filterEditor.suspended){components.append(node('p','Filter edits are paused in this window.','metadata'),action('Resume filter edits',()=>this.filters.open(filterEditor.filter,filterEditor.page,filterEditor.mode),{disabled:this.busy||!!this.mapping||!!this.datasetSession}));}
    const heading=action('Add heading',()=>this.addHeading(),{disabled:!editable});heading.className='component-option';heading.append(componentGlyph('text'));components.append(heading);if(this.blockTools())components.append(action('Add published output',()=>void this.perform(()=>this.loadBlocks()),{disabled:!editable}));
    if(this.datasetSession){components.replaceChildren();renderDatasetEditor(components,this.datasetSession,{busy:this.busy,allocation:this.allocations.get('dataset:'+this._uiDatasetContext.key),canAllocate:allocationAvailable(this.adapter),resume:()=>void this.perform(()=>this.resumeAllocation('dataset:'+this._uiDatasetContext.key)),change:()=>this.render(),read:fn=>void this.perform(()=>this.datasetRead(fn)),prepare:()=>void this.perform(()=>this.prepareDataset()),review:session=>void this.perform(()=>this.reviewDatasetPreparation(session)),create:()=>void this.perform(()=>this.createDatasetChart()),recover:()=>void this.perform(()=>this.createDatasetChart(true)),inspect:action=>void this.perform(()=>this.inspectDataset(action)),cancel:()=>this.cancelDataset(),searchFilter:this.authoring('dataset_options','option_status','option_control')?(dimension,search,cursor)=>void this.perform(()=>this.searchDatasetFilter(dimension,search,cursor)):undefined,inspectFilter:this.authoring('option_status','option_control')?(dimension,action)=>void this.perform(()=>this.inspectDatasetFilter(dimension,action)):undefined});}else{if(this.datasetTools())components.append(action((this._uiDatasetCustody.has(this.datasetKey())||this.allocations.has('dataset:'+this.datasetKey()))?'Resume chart setup':'Create from dataset',()=>void this.perform(()=>this.openDataset()),{disabled:!editable||this.session.definition.schema_version!==3}));if(this.datasetTools()&&this.session.definition.schema_version!==3)components.append(node('p','Enable pages to create a private chart from your dataset fields.','metadata'));if(this.blockTools())this.renderLibrary(components,editable);}panel.append(components);
    const selected=node('section',undefined,'selected-panel');selected.hidden=this.panelTab!=='selected';const widget=definition.widgets.find(w=>w.id===this.selectedWidget);if(widget&&this.mapping){renderMappingEditor(selected,this.mapping,{busy:this.busy,allocation:this.allocations.get('copy:'+this._uiMappingContext?.key),canAllocate:allocationAvailable(this.adapter),resume:()=>void this.perform(()=>this.resumeAllocation('copy:'+this._uiMappingContext?.key)),change:()=>this.render(),save:()=>void this.perform(()=>this.saveMapping()),cancel:()=>this.cancelMapping(),inspect:()=>void this.perform(()=>this.inspectMapping())});}else if(widget)this.renderInspector(selected,widget,editable);else selected.append(node('h2','Select a component'),node('p','Choose a card on the canvas to edit its content, position and size.','metadata'));panel.append(selected);
    if(this.blockTools())this.renderFilters(panel,definition);parent.append(panel);
  }
  renderLibrary(parent,editable=true) {
    const library=node('section',undefined,'output-library'),current=()=>!this.closed&&this.libraryElement===library;this.libraryElement=library;library.append(node('h3','Approved outputs'));this.renderCatalogSearch(library,this.blockSearch,(after,query)=>this.loadBlocks(after,query),'blocks',editable);
    if(!this.showLibrary&&!this.blockCatalog.length)library.append(node('p','Browse the published catalog to choose a KPI, chart or table.','metadata'));
    for(const item of this.blockCatalog){const b=action(item.title||item.target.id,()=>{if(current())void this.perform(()=>this.chooseBlock(item));},{disabled:this.busy});b.className='block-choice';b.setAttribute('aria-pressed',String(this.blockDescription?.resource.target.id===item.target.id));library.append(b);if(item.description)library.append(node('p',item.description,'metadata catalog-preview'));}
    if(this.showLibrary&&!this.blockCatalog.length)library.append(node('p',this.blockNext?'No matches in these catalog pages. More blocks may contain matches.':'No matches in the catalog pages checked.','metadata'));
    if(this.blockNext)library.append(action('More blocks',()=>void this.perform(()=>this.loadBlocks(this.blockNext)),{disabled:this.busy}));
    if(this.blockDescription){library.append(node('h4',this.blockDescription.resource.title));for(const output of this.blockDescription.outputs||[])if(output.enabled!==false&&['kpi','chart','table'].includes(output.kind)){const b=action(`Add ${output.kind}: ${output.title||output.id}`,()=>{if(current())this.addOutput(output);},{disabled:!editable});b.className='component-option';b.append(componentGlyph(output.kind));library.append(b);}library.append(node('p','These are approved mappings. Adding a component does not query data.','metadata'));}parent.append(library);
  }
  renderInspector(parent,widget,editable=true) {
    const panel=node('fieldset',undefined,'inspector');panel.disabled=!editable;panel.append(node('legend','Selected widget'));if(widget.kind==='text')panel.append(inputField('Heading text',widget.text.text,value=>{this.editPage(d=>{d.widgets.find(w=>w.id===widget.id).text={format:'plain',text:value};});},{type:'textarea',maxLength:32768}));else panel.append(inputField('Widget title',widget.presentation?.title||'',value=>{this.editPage(d=>{d.widgets.find(w=>w.id===widget.id).presentation.title=value;});}));
    panel.append(inputField('Supporting text',widget.presentation?.subtitle||'',value=>this.editPage(d=>{d.widgets.find(w=>w.id===widget.id).presentation.subtitle=value;}),{type:'textarea',maxLength:1024}));
    const dimensions=node('div',undefined,'geometry-fields');for(const [field,label] of [['column','Column'],['row','Row'],['width','Width'],['height','Height']])dimensions.append(inputField(label,widget.grid[field]+(['column','row'].includes(field)?1:0),value=>this.setWidgetGrid(widget.id,field,value),{type:'number',maxLength:5}));panel.append(node('h3','Position and size'),dimensions,node('p','Columns and rows start at 1. Changes keep every other component in place.','metadata'));
    const move=direction=>this.geometryEdit(widgets=>moveWidget(widgets,widget.id,widget.grid.column,widget.grid.row+direction));panel.append(action('Move up',()=>move(-1),{disabled:widget.grid.row===0}),action('Move down',()=>move(1)),action('Duplicate widget',()=>this.duplicateSelected(widget.id)),action('Remove widget',()=>this.removeSelected(widget.id)));
    if(this.authoring('widget'))panel.append(action('Save selected content',()=>void this.perform(async()=>{if(await this.session.saveWidget(widget.id,this.activePageID)){this.message=`Saved selected widget in draft revision ${this.session.revision}.`;if(this.authoring('drafts'))await this.loadDrafts();}}),{disabled:!this.session.canSaveWidget(widget.id,this.activePageID)}),node('p','Selected-content save changes text and presentation only. Save report also saves layout and filters.','metadata'));
    if(widget.block){this.renderChartActions(panel,widget,editable);const origin=node('details');origin.append(node('summary',widget.block.policy==='private_preview'?'Private chart source':'Approved source'),node('p',`${widget.block.block} · revision ${widget.block.revision} · ${widget.block.outputs.join(', ')}`,'metadata'));panel.append(origin);if(this.blockTools())panel.append(action('Choose business filter',()=>void this.perform(()=>this.loadWidgetParameters(widget))));if(this.blockTools()&&this.widgetParameters?.widget===widget.id&&this.widgetParameters.page===this.activePageID)for(const f of this.widgetParameters.filters){const bound=widget.bindings?.some(b=>b.parameter===f.parameter.name);panel.append(action(`Add filter: ${f.label||f.parameter.name}`,()=>this.addFilter(widget,f.parameter),{disabled:bound}));for(const existing of this.matchingFilters(f.parameter))panel.append(action(`Use filter: ${existing.label||existing.parameter.name}`,()=>this.bindFilter(widget,f.parameter,existing.parameter.name),{disabled:bound}));}}parent.append(panel);
  }
  datasetKey() {return JSON.stringify([this.session.state?.id||this.newID,this.activePageID]);}
  async openDataset() {
    if(!this.datasetTools()||this.closed||this.mapping||this.datasetSession||reportPages(this.session.definition).reduce((n,p)=>n+p.widgets.length,0)>=100||this.session.definition?.schema_version!==3||this.session.conflict||this.session.uncertain||!(this.session.state?this.session.capabilities.can_save:this.session.capabilities.can_create))throw appError(FORBIDDEN);
    const key=this.datasetKey();if(!this._uiDatasetCustody.has(key)&&this._uiDatasetCustody.size>=16)throw appError(LIMIT_EXCEEDED);
    const session=this._uiDatasetCustody.get(key)||new DatasetSession((name,args)=>this.invoke(name,args),{locale:this.activePage().locale||this.session.definition.locale,tables:this.tools('list_source_page','list_datasets'),topics:this.tools('list_topics','describe_topic')});this.datasetSession=session;this._uiDatasetContext={key,epoch:this.epoch,page:this.activePageID,generation:this._uiPageGeneration};this.panelTab='components';
    if(!session.custody&&!session.hasUnsettledFilterLookups)await this.datasetRead(()=>session.loadCatalog());else this.message=session.rejected?'This preparation was rejected before admission. Review setup before preparing again.':'Inspect preparation to refresh this exact saved operation. No source query will restart.';
  }
  datasetCurrent(session=this.datasetSession,context=this._uiDatasetContext) {return !!session&&!!context&&!this.closed&&this.datasetSession===session&&context.epoch===this.epoch&&context.page===this.activePageID&&context.generation===this._uiPageGeneration;}
  async datasetRead(fn) {const session=this.datasetSession,context=this._uiDatasetContext;try{await fn();if(!this.datasetCurrent(session,context)){session.close();if(this.datasetSession===session){this.datasetSession=null;this._uiDatasetContext=null;}}}catch(e){if(this.datasetCurrent(session,context)){this.clearChartReadFailure(e);session.suspend();}throw e;}}
  async prepareDataset() {if(!this.datasetTools())throw appError(FORBIDDEN);const session=this.datasetSession,context=this._uiDatasetContext;if(!this.datasetCurrent())throw appError(STALE_VALIDATION);if(session.locked)throw appError(BUSY);try{if(!session.intentValid())throw appError(INVALID_REQUEST);const allocation=this.allocations.get('dataset:'+context.key)||this.allocation('dataset:'+context.key,{kind:'block',intent:'create_chart',title:session.draft.title});let id;session.pending=true;try{id=await allocation.obtain();}finally{session.pending=false;}if(!this.datasetCurrent(session,context))return;session.newBlock=id;await session.prepare();if(this.datasetCurrent(session,context))this.message=session.preparation?.status==='prepared'?'Preparation checked the actual schema. Create private chart saves an unvalidated native draft.':'Preparation status is shown in the chart setup panel.';}catch(e){if(this.datasetCurrent(session,context)){this.clearChartReadFailure(e);if(session.custody)session.suspend();}throw e;}finally{if(this.datasetCurrent(session,context)&&session.custody)this._uiDatasetCustody.set(context.key,session);}}
  async reviewDatasetPreparation(session=this.datasetSession) {const context=this._uiDatasetContext;if(!this.datasetCurrent(session,context))throw appError(STALE_VALIDATION);session.reviewPreparation();this._uiDatasetCustody.delete(context.key);await this.datasetRead(()=>session.loadCatalog());if(this.datasetCurrent(session,context))this.message='Review the dataset and fields. Prepare chart starts a new bounded source read only when you choose it.';}
  async searchDatasetFilter(dimension,search,cursor){if(!this.datasetTools()||!this.authoring('dataset_options','option_status','option_control'))throw appError(FORBIDDEN);const session=this.datasetSession,context=this._uiDatasetContext;if(!this.datasetCurrent()||session.locked)throw appError(BUSY);const allocation=this.allocations.get('dataset:'+context.key)||this.allocation('dataset:'+context.key,{kind:'block',intent:'create_chart',title:session.draft.title});let id;session.pending=true;try{id=await allocation.obtain();}finally{session.pending=false;}if(!this.datasetCurrent(session,context))return;try{await session.searchFilter(dimension,id,search,cursor);}finally{if(this.datasetCurrent(session,context)&&session.hasUnsettledFilterLookups)this._uiDatasetCustody.set(context.key,session);}}
  async inspectDatasetFilter(dimension,action=''){if(!this.datasetCurrent()||!this.authoring('option_status','option_control'))throw appError(FORBIDDEN);await this.datasetSession.inspectFilter(dimension,action);}
  async inspectDataset(action='') {const session=this.datasetSession,context=this._uiDatasetContext;if(!this.datasetCurrent())throw appError(STALE_VALIDATION);try{await session.inspect(action);if(this.datasetCurrent(session,context))this.message='Preparation status refreshed. This action does not restart its source query.';}catch(e){if(this.datasetCurrent(session,context)){this.clearChartReadFailure(e);session.suspend();}throw e;}}
  async createDatasetChart(recover=false) {
    const session=this.datasetSession,context=this._uiDatasetContext;if(!this.datasetCurrent())throw appError(STALE_VALIDATION);let view;try{view=await session.create(recover);}catch(e){if(this.datasetCurrent(session,context)){this.clearChartReadFailure(e);session.suspend();}throw e;}if(!view||!this.datasetCurrent(session,context))return;
    const output=view.block.outputs[0],block={block:view.block.state.id,revision:view.block.revision,digest:view.block.digest,outputs:[output.id],policy:'private_preview',narrative:false},id=unusedWidgetID(this.session.definition,'widget'),size=output.kind==='kpi'?{width:4,height:3}:output.kind==='table'?{width:12,height:4}:{width:8,height:4};
    this.session.edit(d=>{const page=pageContent(d,context.page);page.widgets.push({id,kind:'block',grid:starterPlacement(page,output.kind,size),presentation:{title:session.draft.title.trim()},block});});this.rememberPrivateChart(block,view);this._uiDatasetCustody.delete(context.key);this.allocations.get('dataset:'+context.key)?.close();this.allocations.delete('dataset:'+context.key);session.close();this.datasetSession=null;this._uiDatasetContext=null;this.selectedWidget=id;this.panelTab='selected';this.message='Private chart added. Save report keeps its unvalidated reference. Validate data separately reads the source.';this.error=false;
  }
  cancelDataset() {const session=this.datasetSession,context=this._uiDatasetContext;if(!session||session.pending||this.allocations.get('dataset:'+context.key)?.pending)return;if(session.custody||session.hasUnsettledFilterLookups){session.suspend();this._uiDatasetCustody.set(context.key,session);}else session.close();this.datasetSession=null;this._uiDatasetContext=null;this.message='Chart setup closed. Existing preparation custody is available from Resume chart setup.';this.render();}
  mappingKey(block) {return JSON.stringify([block.block,block.revision,block.digest||'']);}
  rememberPrivateChart(reference,view) {const b=view.block,e=b.validation,block={revision:b.revision,digest:b.digest,execution_digest:b.execution_digest};if(e)block.validation={revision:e.revision,definition_digest:e.definition_digest,execution_digest:e.execution_digest,expires_at:e.expires_at};this._uiPrivateCharts.set(this.mappingKey(reference),{block});while(this._uiPrivateCharts.size>100)this._uiPrivateCharts.delete(this._uiPrivateCharts.keys().next().value);}
  clearChartReadFailure(error) {if([FORBIDDEN,'unauthenticated','not_found',STALE_VALIDATION,INVALID_REQUEST].includes(error.code)){this._uiPrivateCharts.clear();this.closePreview();}}
  mappingEditKey(widget) {return JSON.stringify([this.session.state?.id||this.newID,this.activePageID,widget.id]);}
  privatePreviewReady() {return reportPages(this.session.definition).every(p=>p.widgets.every(w=>w.block?.policy!=='private_preview'||!this._uiMappingFailures.has(this.mappingKey(w.block))&&hasFreshMappingEvidence(this._uiPrivateCharts.get(this.mappingKey(w.block)))));}
  async openMapping(widget,output=widget.block.outputs[0],purpose='mapping') {
    if(!this.mappingTools(purpose)||this.mapping||this.datasetSession||this.closed||this.session.pending||this.session.conflict||this.session.uncertain||!this.session.capabilities.can_save||this.session.definition?.schema_version!==3)throw appError(FORBIDDEN);
    const current=this.activePage()?.widgets.find(w=>w.id===widget.id);if(this.selectedWidget!==widget.id||!current||JSON.stringify(current.block)!==JSON.stringify(widget.block))throw appError(STALE_VALIDATION);
    const key=this.mappingEditKey(widget);if(this._uiMappingFailures.has(key)||this._uiMappingFailures.has(this.mappingKey(widget.block))){this.message='A previous chart save has an unknown outcome. Reconcile it through your host before editing this component again.';return;}
    const epoch=this.epoch,page=this.activePageID,generation=this._uiPageGeneration,session=new MappingSession((name,args)=>this.invoke(name,args),purpose);this.mapping=session;this._uiMappingContext={key,epoch,page,generation,widget:widget.id,original:copyData(widget.block)};this.panelTab='selected';this.render();
    try{if(!await session.open(widget.block.block,widget.block.revision,output,widget.block.policy!=='private_preview',widget.block.digest||''))return;if(this.closed||epoch!==this.epoch||page!==this.activePageID||generation!==this._uiPageGeneration){session.close();if(this.mapping===session)this.mapping=null;return;}const allocation=this.allocations.get('copy:'+key),b=session.view.block;if(allocation&&!allocation.matches({kind:'block',intent:'copy_chart',source:{block:b.state.id,revision:b.revision,expected_version:b.state.version,digest:b.digest,output:session.output}}))throw appError(STALE_VALIDATION);if(widget.block.policy==='private_preview')this.rememberPrivateChart(widget.block,session.view);}
    catch(e){session.close();if(this.closed||epoch!==this.epoch||generation!==this._uiPageGeneration)return;if(this.mapping===session){this.mapping=null;this._uiMappingContext=null;}this.clearChartReadFailure(e);throw e;}
  }
  async saveMapping() {
    if(!this.mappingTools(this.mapping?.purpose))throw appError(FORBIDDEN);const session=this.mapping,context=this._uiMappingContext;if(!session||!context)throw appError(INVALID_REQUEST);if(session.pending)throw appError(BUSY);
    try{if(session.copy){if(!session.dirty||!session.valid()||session.unknown||session.conflict)throw appError(BUSY);const b=session.view.block,allocation=this.allocation('copy:'+context.key,{kind:'block',intent:'copy_chart',title:allocationTitle(session.draft.options.title,b.metadata?.find(m=>m.locale===this.locale)?.title,b.metadata?.[0]?.title),source:{block:b.state.id,revision:b.revision,expected_version:b.state.version,digest:b.digest,output:session.output}});let id;session.pending=true;try{id=await allocation.obtain();}finally{session.pending=false;}if(this.closed||context.epoch!==this.epoch||context.page!==this.activePageID||this.mapping!==session)return;session.newBlock=id;}const view=await session.save();if(!view||this.closed||context.epoch!==this.epoch||context.page!==this.activePageID||this.mapping!==session)return;
      const current=pageContent(this.session.definition,context.page).widgets.find(w=>w.id===context.widget);if(!current||JSON.stringify(current.block)!==JSON.stringify(context.original))throw appError(CONFLICT);
      const block={...copyData(current.block),block:view.block.state.id,revision:view.block.revision,digest:view.block.digest,policy:'private_preview'};
      this.session.edit(d=>{pageContent(d,context.page).widgets.find(w=>w.id===context.widget).block=block;});this.rememberPrivateChart(block,view);this.allocations.get('copy:'+context.key)?.close();this.allocations.delete('copy:'+context.key);session.close();this.mapping=null;this._uiMappingContext=null;this.message='Private chart saved. It is unvalidated. Save report to keep it; Validate data separately reads the source.';this.error=false;
    }catch(e){if(session.unknown)this._uiMappingFailures.set(context.key,{...session.operation,output:session.output});throw e;}
  }
  cancelMapping() {const session=this.mapping;if(!session||session.pending||this.allocations.get('copy:'+this._uiMappingContext?.key)?.pending)return;if(session.unknown)this._uiMappingFailures.set(this._uiMappingContext.key,{...session.operation,output:session.output});session.close();this.mapping=null;this._uiMappingContext=null;this.message='Chart editor closed. No saved chart revision was deleted.';this.render();}
  async inspectMapping() {const session=this.mapping;if(!session?.unknown)throw appError(INVALID_REQUEST);const value=await session.inspect();if(this.closed||this.mapping!==session)return;checkMappingView(value,session.operation.target,session.operation.revision,session.output);this.message='Chart metadata loaded. The save outcome is still unknown; reconcile it before retrying.';}
  async inspectMappingRecord(record) {const epoch=this.epoch,value=await this.invoke(authoringTool('block_read'),{block:record.target,revision:record.revision});if(this.closed||epoch!==this.epoch)return;checkMappingView(value,record.target,record.revision,record.output);this.message='Chart metadata is readable. The save outcome remains unknown; reconcile it through your host before retrying.';}
  async readPrivateChart(widget) {
    const epoch=this.epoch,generation=this._uiPageGeneration,reference=copyData(widget.block);
    try{const view=await this.invoke(authoringTool('block_read'),{block:reference.block,revision:reference.revision});if(this.closed||epoch!==this.epoch||generation!==this._uiPageGeneration)return null;checkMappingView(view,reference.block,reference.revision,reference.outputs[0],reference.digest);return view;}
    catch(e){if(this.closed||epoch!==this.epoch||generation!==this._uiPageGeneration)return null;this.clearChartReadFailure(e);throw e;}
  }
  async checkAllChartStatuses() {
    if(!this.authoring('block_read'))throw appError(FORBIDDEN);
    const epoch=this.epoch,refs=[...new Map(reportPages(this.session.definition).flatMap(p=>p.widgets).filter(w=>w.block?.policy==='private_preview').map(w=>[this.mappingKey(w.block),copyData(w.block)])).values()],views=[];let cursor=0;
    try{const read=async()=>{while(cursor<refs.length&&!this.closed&&epoch===this.epoch){const ref=refs[cursor++],view=await this.invoke(authoringTool('block_read'),{block:ref.block,revision:ref.revision});checkMappingView(view,ref.block,ref.revision,ref.outputs[0],ref.digest);views.push([ref,view]);}};await Promise.all(Array.from({length:Math.min(4,refs.length)},read));if(this.closed||epoch!==this.epoch)return;for(const [ref,view]of views)this.rememberPrivateChart(ref,view);this.message=this.privatePreviewReady()?'All chart statuses checked. Preview rechecks current access and dependencies.':'Some charts still need validation or recovery. Select them to continue.';}
    catch(e){if(this.closed||epoch!==this.epoch)return;this.clearChartReadFailure(e);throw e;}
  }
  async checkPrivateChart(widget) {const view=await this.readPrivateChart(widget);if(!view)return;this.rememberPrivateChart(widget.block,view);this.message=hasFreshMappingEvidence(view)?'Exact chart validation evidence is present. Private preview will recheck current access and dependencies.':'This private chart is unvalidated or its evidence has expired.';}
  async validateChart(widget) {
    if(!this.authoring('block_read','block_validate'))throw appError(FORBIDDEN);const epoch=this.epoch,pageID=this.activePageID,reference=copyData(widget.block),key=this.mappingKey(reference);if(this._uiMappingFailures.has(key))throw appError(BUSY);const view=await this.readPrivateChart(widget);if(!view||epoch!==this.epoch||pageID!==this.activePageID)return;const page=pageContent(this.session.definition,pageID),resolution={at:new Date().toISOString(),timezone:page.timezone||this.session.definition.timezone};
    try{const validated=await validatePrivateMapping((name,args)=>this.invoke(name,args),view,widget,page,resolution);if(this.closed||epoch!==this.epoch)return;this.rememberPrivateChart(reference,validated);this.message='Chart validated. Save report edits, then use Private preview to create retained values.';}
    catch(e){if(!this.closed&&epoch===this.epoch&&(e.unknown===true||[UNAVAILABLE,CANCELLED_OR_TIMED_OUT].includes(e.code)))this._uiMappingFailures.set(key,{kind:'validation',target:reference.block,revision:reference.revision});throw e;}
  }
  renderChartActions(parent,widget,editable) {
    if(!this.authoring('block_read'))return;const failed=this._uiMappingFailures.get(this.mappingEditKey(widget));if(failed)parent.append(notice('A previous chart save has an unknown outcome. Edits stay blocked until host reconciliation.',true),action('Inspect chart state',()=>void this.perform(()=>this.inspectMappingRecord(failed)),{disabled:this.busy}));
    if(widget.block.policy==='private_preview'){
      const key=this.mappingKey(widget.block),meta=this._uiPrivateCharts.get(key),unknown=this._uiMappingFailures.has(key);parent.append(node('p',unknown?'Validation outcome unknown. Metadata reads cannot authorize a retry.':hasFreshMappingEvidence(meta)?'Private chart · validation evidence available':'Private chart · Unvalidated or validation status not checked','notice'),action('Check chart status',()=>void this.perform(()=>this.checkPrivateChart(widget)),{disabled:this.busy}));if(this.authoring('block_validate'))parent.append(action('Validate data',()=>void this.perform(()=>this.validateChart(widget)),{disabled:!editable||unknown}),node('p','Validation explicitly reads approved source data. Saving the report does not.','metadata'));
    }
    if(!this.mappingTools('presentation'))return;if(this.session.definition.schema_version!==3){parent.append(node('p','Enable pages to use a report-local private chart revision.','metadata'));return;}
    for(const output of widget.block.outputs)for(const [purpose,label] of [['mapping','Edit chart'],['presentation','Field formatting']])if(this.mappingTools(purpose))parent.append(action(widget.block.outputs.length===1?label:`${label}: ${output}`,()=>void this.perform(()=>this.openMapping(widget,output,purpose)),{disabled:!editable||this._uiMappingFailures.has(this.mappingEditKey(widget))||this._uiMappingFailures.has(this.mappingKey(widget.block))}));
  }
  renderFilters(parent,definition) { this.filters.renderDefaults(parent,definition); }

  close() { if(this.closed)return;this.closed=true;this.reportSearch.close();this.blockSearch.close();this._uiBlockDescriptionGeneration++;this._uiPageSettingsElement=null;this.libraryElement=null;this.catalogNext='';this.blockNext='';this.cancelDrag(false);this.epoch++;this.closePreview();this.retained.close();this.publication.close();this.session.close();this.mapping?.close();this.mapping=null;this._uiMappingContext=null;this._uiPrivateCharts.clear();this._uiMappingFailures.clear();this.datasetSession?.close();this.datasetSession=null;this._uiDatasetContext=null;for(const session of this._uiDatasetCustody.values())session.close();this._uiDatasetCustody.clear();for(const allocation of this.allocations.values())allocation.close();this.allocations.clear();this.observer?.disconnect();this.catalog=[];this.drafts=[];this.description=null;this.runs=[];this.blockCatalog=[];this.blockDescription=null;this.widgetParameters=null;this._uiPendingNavigation=null;this.selected=null;this.resetRecovery();this.filters.close();this._uiFilterValues.clear();this.root.replaceChildren(notice('This report app is closed. Reopen it through your authorized host.')); }
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
  if(!Array.isArray(parents)||!parents.length||parents.length>16||parents.some(origin=>{try{const u=new URL(origin);return u.protocol!=='https:'||u.origin!==origin||u.username||u.password;}catch{return true;}}))throw appError(INVALID_REQUEST);
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
