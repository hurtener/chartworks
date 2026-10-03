import test from 'node:test';
import assert from 'node:assert/strict';
import {ReportApp, awaitEmbeddedParent} from './app.js';
import {appError} from './model.js';
class Element {
 constructor(tag){this.tagName=tag.toUpperCase();this.children=[];this.attributes={};this.listeners={};this.dataset={};this._text='';this.disabled=false;this.value='';}
 set textContent(value){this.children=[];this._text=String(value);}get textContent(){return this._text+this.children.map(c=>c.textContent||'').join('');}
 append(...children){this.children.push(...children);}replaceChildren(...children){this._text='';this.children=children;}
 setAttribute(name,value){this.attributes[name]=String(value);}getAttribute(name){return this.attributes[name]??null;}
 addEventListener(name,fn){this.listeners[name]=fn;}emit(name){this.listeners[name]?.({target:this});}
 querySelectorAll(tag){return this.children.flatMap(c=>[...(c.tagName===tag.toUpperCase()?[c]:[]),...(c.querySelectorAll?.(tag)||[])]);}
 getBoundingClientRect(){return {width:1000,height:800};}
}
function installDOM(){globalThis.document={createElement:tag=>new Element(tag),createElementNS:(_,tag)=>new Element(tag),documentElement:new Element('html')};return new Element('main');}
const caps=report=>({version:'report-authoring-v1',builder:true,consumer:true,can_create:report==='allowed-new',can_open:!!report,can_save:!!report,can_preview:!!report,can_execute:!!report});
const original=()=>({schema_version:2,metadata:[{locale:'en-US',title:'Operations'}],locale:'en-US',timezone:'UTC',widgets:[{id:'heading',kind:'text',grid:{row:0,column:0,width:12,height:1},text:{format:'plain',text:'Overview'},presentation:{}}],filters:[]});
function fixture(){const calls=[];let stored=original(),state={id:'report-a',version:1,draft_revision:1};const adapter={resize(){},async connect(){},async call(name,args){calls.push({name,args});let result;
 if(name==='reporting_authoring_capabilities_v1')result=caps(args.report);
 else if(name==='reporting_search')result={version:'reporting-view-v1',items:[{target:{kind:args.kind,id:args.kind==='report'?'report-a':'block-a',revision:2},title:'Operations'}],next:''};
 else if(name==='reporting_authoring_drafts_v1')result={items:[{id:state.id,metadata:stored.metadata,version:state.version,revision:state.draft_revision}],next:''};
 else if(name==='reporting_authoring_read_v1')result={state:{...state,id:args.report},revision:state.draft_revision,private:true,definition:stored};
 else if(name==='reporting_authoring_create_v1'||name==='reporting_authoring_save_v1'){stored=args.definition;state={id:args.id||args.report,version:state.version+1,draft_revision:state.draft_revision+1};result=state;}
 else throw new Error(name);
 return {structuredContent:{result}};}};return {adapter,calls,stored:()=>stored};}

test('controller boots through server capability hints and exact creation target',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();assert(root.textContent.includes('Choose a published report'));assert.deepEqual(f.calls.map(c=>c.name),['reporting_authoring_capabilities_v1','reporting_search']);await app.changeMode('builder');app.newID='allowed-new';await app.createDraft();assert.equal(app.session.definition.widgets[0].kind,'text');await app.save();assert.equal(f.calls.find(c=>c.name==='reporting_authoring_create_v1').args.id,'allowed-new');assert(f.calls.some(c=>c.name==='reporting_authoring_capabilities_v1'&&c.args.report==='allowed-new'));app.close();});

test('unsaved navigation requires discard and cancellation keeps exact content',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();await app.openDraft('report-a');app.edit(d=>{d.widgets[0].text.text='Unsaved heading';});let navigated=false;app.navigate(async()=>{navigated=true;});assert.equal(navigated,false);assert(root.textContent.includes('Discard them'));root.querySelectorAll('button').find(b=>b.textContent==='Keep editing').emit('click');assert.equal(app.pendingNavigation,null);assert.equal(app.session.definition.widgets[0].text.text,'Unsaved heading');assert.equal(navigated,false);app.close();});

test('edit re-renders save enabled and closing clears draft, callbacks and retained buffers',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();app.mode='builder';await app.openDraft('report-a');app.render();assert.equal(root.querySelectorAll('button').find(b=>b.textContent==='Save report').disabled,true);app.edit(d=>{d.metadata[0].title='Changed';});assert.equal(root.querySelectorAll('button').find(b=>b.textContent==='Save report').disabled,false);app.widgetParameters={sensitive:'fixture'};app.close();assert.equal(app.session.definition,null);assert.equal(app.widgetParameters,null);assert.equal(app.pendingNavigation,null);assert.equal(root.textContent,'This report app is closed. Reopen it through your authorized host.');});

test('embedded entry rejects unregistered parents and pagehide removes bootstrap listener',()=>{const root=installDOM(),listeners=new Map(),parent={},win={parent,addEventListener:(name,fn)=>listeners.set(name,fn),removeEventListener:name=>listeners.delete(name)};assert.throws(()=>awaitEmbeddedParent(root,['http://host.example'],win),/invalid_request/);const boundary=awaitEmbeddedParent(root,['https://host.example'],win);listeners.get('message')({source:{},origin:'https://host.example',data:{protocol:'chartworks-report-app-v1',method:'bootstrap'}});assert(root.textContent.includes('Waiting for'));listeners.get('message')({source:parent,origin:'https://wrong.example',data:{protocol:'chartworks-report-app-v1',method:'bootstrap'}});assert(root.textContent.includes('Waiting for'));listeners.get('pagehide')();assert.equal(listeners.size,0);assert(root.textContent.includes('closed'));boundary.close();});

test('fulfilled bridge response arriving after close cannot repopulate catalogs',async()=>{const root=installDOM();let resolve;const adapter={resize(){},call:()=>new Promise(r=>{resolve=r;})},app=new ReportApp(root,adapter);const read=app.loadBlocks();app.close();resolve({structuredContent:{result:{version:'reporting-view-v1',items:[{target:{kind:'block',id:'private-block',revision:1},title:'Private name'}],next:''}}});await assert.rejects(read,/unavailable/);assert.deepEqual(app.blockCatalog,[]);assert(!root.textContent.includes('Private name'));});

test('known private preview not_found requests host refresh and retry performs retained read only',async()=>{const root=installDOM(),calls=[];let denied=true;const adapter={resize(){},async call(name,args){calls.push({name,args});return denied?{isError:true,structuredContent:{error:{code:'not_found',outcome:'not_started'}}}:{structuredContent:{result:retainedStatus('known-private-preview')}};}};const app=new ReportApp(root,adapter);app.lastMutationRun='known-private-preview';app.lastMutationPrivate=true;app.recoveryOwner='report-a';app.uncertainRun=true;
 await assert.rejects(app.openRun('known-private-preview',true),e=>e.code==='not_found'&&e.needsRunAuthority===true);assert.equal(app.uncertainRun,true);denied=false;await app.openRun('known-private-preview',true);assert.deepEqual(calls.map(c=>c.name),['reporting_view','reporting_view']);assert(calls.every(c=>c.args.run==='known-private-preview'));assert.equal(app.uncertainRun,false);app.close();});

test('not_found for arbitrary or public run IDs preserves nondisclosing generic failure',async()=>{const root=installDOM(),adapter={resize(){},async call(){return {isError:true,structuredContent:{error:{code:'not_found'}}};}},app=new ReportApp(root,adapter);app.lastMutationRun='known-private-preview';app.lastMutationPrivate=true;await assert.rejects(app.openRun('arbitrary-run',true),e=>e.code==='not_found'&&e.needsRunAuthority!==true);app.lastMutationRun='public-run';app.lastMutationPrivate=false;await assert.rejects(app.openRun('public-run',true),e=>e.code==='not_found'&&e.needsRunAuthority!==true);app.close();});

function uncertainPreview(app,report='report-a') {app.recoveryOwner=report;app.lastMutationRun='preview-a';app.lastMutationPrivate=true;app.uncertainRun=true;}
test('switching reports resets recovery but same-report reload preserves uncertainty',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();await app.openDraft('report-a');uncertainPreview(app);await app.openDraft('report-a');assert.equal(app.recoveryOwner,'report-a');assert.equal(app.lastMutationRun,'preview-a');assert.equal(app.uncertainRun,true);await assert.rejects(app.previewDraft(),/forbidden/);await app.openDraft('report-b');assert.equal(app.recoveryOwner,'report-b');assert.equal(app.lastMutationRun,null);assert.equal(app.lastMutationPrivate,false);assert.equal(app.uncertainRun,false);app.mode='builder';app.render();assert(!root.querySelectorAll('button').some(b=>b.textContent==='Inspect last retained run'));assert.equal(root.querySelectorAll('button').find(b=>b.textContent==='Private preview').disabled,false);app.close();});

test('new exact-authorized draft resets previous report recovery',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();await app.openDraft('report-a');uncertainPreview(app);app.newID='allowed-new';await app.createDraft();assert.equal(app.recoveryOwner,'allowed-new');assert.equal(app.lastMutationRun,null);assert.equal(app.uncertainRun,false);assert.equal(app.session.state,null);app.close();});

test('late failed preview cannot attach old-report recovery to a newer report',async()=>{const root=installDOM(),f=fixture();let rejectPreview;const base=f.adapter.call.bind(f.adapter);f.adapter.call=(name,args)=>name==='reporting_authoring_preview_v1'?new Promise((resolve,reject)=>{rejectPreview=reject;}):base(name,args);const app=new ReportApp(root,f.adapter);await app.start();await app.openDraft('report-a');const preview=app.previewDraft();const rejected=assert.rejects(preview,/unavailable/);assert.equal(app.uncertainRun,true);await app.openDraft('report-b');rejectPreview(appError('unavailable',true));await rejected;assert.equal(app.recoveryOwner,'report-b');assert.equal(app.lastMutationRun,null);assert.equal(app.uncertainRun,false);app.close();});

function retainedStatus(run,state='completed',report='report-a',isPrivate=true,code='') {return {version:'reporting-view-v1',summary:{kind:'report',run,target:{kind:'report',id:report,revision:1},private:isPrivate,state,code},selection:{kind:'report',run},pages:[]};}
test('terminal read after denied private preview enables future preview while pending and foreign reads cannot clear',async()=>{
 const root=installDOM(),f=fixture(),base=f.adapter.call.bind(f.adapter);let denied=true,status='completed',target='report-a';
 f.adapter.call=async(name,args)=>{
  if(name==='reporting_authoring_preview_v1')return {structuredContent:{result:{id:'preview-a',private:true}}};
  if(name==='reporting_authoring_execute_v1')return {structuredContent:{result:{id:'preview-a',private:true,state:'completed'}}};
  if(name==='reporting_view')return denied?{isError:true,structuredContent:{error:{code:'not_found'}}}:{structuredContent:{result:retainedStatus(args.run,status,target)}};
  return base(name,args);
 };
 const app=new ReportApp(root,f.adapter);await app.start();await app.openDraft('report-a');app.mode='builder';
 await assert.rejects(app.previewDraft(),e=>e.needsRunAuthority===true);assert.equal(app.uncertainRun,true);assert.equal(app.lastMutationRun,'preview-a');
 denied=false;status='running';await app.openRun('preview-a',true);assert.equal(app.uncertainRun,true);
 status='unknown';await app.openRun('preview-a',true);assert.equal(app.uncertainRun,true);
 status='completed';await app.openRun('foreign-run',true);assert.equal(app.uncertainRun,true);
 target='report-b';await app.openRun('preview-a',true);assert.equal(app.uncertainRun,true);
 target='report-a';await app.openRun('preview-a',true);assert.equal(app.uncertainRun,false);app.closePreview();app.render();
 assert.equal(root.querySelectorAll('button').find(b=>b.textContent==='Private preview').disabled,false);
 await app.openDraft('report-a');assert.equal(app.uncertainRun,false);app.close();
});

test('successful pending retained read from preview and published-run callers remains fenced',async()=>{for(const kind of ['preview','published']){const root=installDOM(),f=fixture(),base=f.adapter.call.bind(f.adapter);f.adapter.call=async(name,args)=>{let result;if(name==='reporting_authoring_preview_v1')result={id:'pending-run',private:true};else if(name==='reporting_authoring_execute_v1')result={id:'pending-run',private:true,state:'running'};else if(name==='reporting_run')result={run:'pending-run'};else if(name==='reporting_view')result=retainedStatus('pending-run','running','report-a',kind==='preview');else return base(name,args);return {structuredContent:{result}};};const app=new ReportApp(root,f.adapter);await app.start();await app.openDraft('report-a');if(kind==='preview')await app.previewDraft();else{app.description={resource:{target:{kind:'report',id:'report-a',revision:1}},filters:[],timezone:'UTC'};await app.runPublished();}assert.equal(app.uncertainRun,true,kind);assert.equal(app.lastMutationRun,'pending-run');app.close();}});

test('terminal failed status with indeterminate query code stays fenced',()=>{const root=installDOM(),app=new ReportApp(root,{resize(){}});uncertainPreview(app);app.observeTerminalRecovery(retainedStatus('preview-a','failed','report-a',true,'query_indeterminate'),'preview-a');assert.equal(app.uncertainRun,true);app.observeTerminalRecovery(retainedStatus('preview-a','failed','report-a',true,'query_failed'),'preview-a');assert.equal(app.uncertainRun,false);app.close();});
