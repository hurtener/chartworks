import test from 'node:test';
import assert from 'node:assert/strict';
import {ReportApp, awaitEmbeddedParent} from './app.js';
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
 else if(name==='reporting_authoring_read_v1')result={state,revision:state.draft_revision,private:true,definition:stored};
 else if(name==='reporting_authoring_create_v1'||name==='reporting_authoring_save_v1'){stored=args.definition;state={id:args.id||args.report,version:state.version+1,draft_revision:state.draft_revision+1};result=state;}
 else throw new Error(name);
 return {structuredContent:{result}};}};return {adapter,calls,stored:()=>stored};}

test('controller boots through server capability hints and exact creation target',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();assert(root.textContent.includes('Choose a published report'));assert.deepEqual(f.calls.map(c=>c.name),['reporting_authoring_capabilities_v1','reporting_search']);await app.changeMode('builder');app.newID='allowed-new';await app.createDraft();assert.equal(app.session.definition.widgets[0].kind,'text');await app.save();assert.equal(f.calls.find(c=>c.name==='reporting_authoring_create_v1').args.id,'allowed-new');assert(f.calls.some(c=>c.name==='reporting_authoring_capabilities_v1'&&c.args.report==='allowed-new'));app.close();});

test('unsaved navigation requires discard and cancellation keeps exact content',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();await app.openDraft('report-a');app.edit(d=>{d.widgets[0].text.text='Unsaved heading';});let navigated=false;app.navigate(async()=>{navigated=true;});assert.equal(navigated,false);assert(root.textContent.includes('Discard them'));root.querySelectorAll('button').find(b=>b.textContent==='Keep editing').emit('click');assert.equal(app.pendingNavigation,null);assert.equal(app.session.definition.widgets[0].text.text,'Unsaved heading');assert.equal(navigated,false);app.close();});

test('edit re-renders save enabled and closing clears draft, callbacks and retained buffers',async()=>{const root=installDOM(),f=fixture(),app=new ReportApp(root,f.adapter);await app.start();app.mode='builder';await app.openDraft('report-a');app.render();assert.equal(root.querySelectorAll('button').find(b=>b.textContent==='Save report').disabled,true);app.edit(d=>{d.metadata[0].title='Changed';});assert.equal(root.querySelectorAll('button').find(b=>b.textContent==='Save report').disabled,false);app.widgetParameters={sensitive:'fixture'};app.close();assert.equal(app.session.definition,null);assert.equal(app.widgetParameters,null);assert.equal(app.pendingNavigation,null);assert.equal(root.textContent,'This report app is closed. Reopen it through your authorized host.');});

test('embedded entry rejects unregistered parents and pagehide removes bootstrap listener',()=>{const root=installDOM(),listeners=new Map(),parent={},win={parent,addEventListener:(name,fn)=>listeners.set(name,fn),removeEventListener:name=>listeners.delete(name)};assert.throws(()=>awaitEmbeddedParent(root,['http://host.example'],win),/invalid_request/);const boundary=awaitEmbeddedParent(root,['https://host.example'],win);listeners.get('message')({source:{},origin:'https://host.example',data:{protocol:'chartworks-report-app-v1',method:'bootstrap'}});assert(root.textContent.includes('Waiting for'));listeners.get('message')({source:parent,origin:'https://wrong.example',data:{protocol:'chartworks-report-app-v1',method:'bootstrap'}});assert(root.textContent.includes('Waiting for'));listeners.get('pagehide')();assert.equal(listeners.size,0);assert(root.textContent.includes('closed'));boundary.close();});
