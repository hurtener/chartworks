// The production IIFE through both real host adapters over synthetic DTOs.
// Browser layout, native execution and authority require separate qualification.
import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {REPORT_APP_TOOLS} from './bridge.js';
import {formatView,formatSaved} from './formatting-fixture.mjs';
const script=await readFile(new URL('./generated/report-app.js',import.meta.url),'utf8');
const tick=()=>new Promise(resolve=>setImmediate(resolve)),origin='https://report-host.example';
class Element{
 constructor(tag,ownerDocument){this.ownerDocument=ownerDocument;this.tagName=tag.toUpperCase();this.children=[];this.attributes={};this.listeners={};this.dataset={};this.style={setProperty(k,v){this[k]=v;}};this._text='';this.value='';this.disabled=false;}
 set textContent(value){this.children=[];this._text=String(value);}get textContent(){return this._text+this.children.map(c=>c.textContent||'').join('');}
 set innerHTML(_){assert.fail('Formatting must never parse HTML');}
 append(...children){this.children.push(...children);}replaceChildren(...children){this._text='';this.children=children;}
 setAttribute(k,v){this.attributes[k]=String(v);}getAttribute(k){return this.attributes[k]??null;}addEventListener(name,fn){this.listeners[name]=fn;}
 querySelectorAll(tag){return this.children.flatMap(c=>[...(c.tagName===tag.toUpperCase()?[c]:[]),...(c.querySelectorAll?.(tag)||[])]);}
 focus(){if(this.ownerDocument)this.ownerDocument.activeElement=this;}
 getBoundingClientRect(){return {top:0,bottom:800,width:1000,height:800};}
}
async function harness(embedded,{missing=false,failure=''}={}){
 const root=new Element('main'),listeners=new Map(),sent=[],calls=[],timers=new Map(),delayed=[];let cursor=0,timer=0,random=0,hold='',view=formatView(),version=1;
 if(missing)delete view.block.outputs[0].presentation;
 let definition={schema_version:3,metadata:[{locale:'en-US',title:'Formatting report'}],locale:'en-US',timezone:'UTC',partial_failure:'fail_closed',report_pages:[{id:'main',title:'Summary',widgets:[{id:'revenue',kind:'block',grid:{row:0,column:0,width:12,height:4},presentation:{title:'Revenue detail'},block:{block:'source',revision:7,outputs:['amount'],policy:'published',narrative:false}}]}]};
 const parent={postMessage(message){sent.push(structuredClone(message));}},document={getElementById:()=>root,createElement:tag=>new Element(tag,document),createElementNS:(_,tag)=>new Element(tag,document),documentElement:new Element('html')};
 const context=vm.createContext({parent,URL,TextEncoder,console,document,crypto:{randomUUID(){return (++random).toString(16).padStart(32,'0');}},addEventListener(name,fn){if(!listeners.has(name))listeners.set(name,new Set());listeners.get(name).add(fn);},removeEventListener(name,fn){listeners.get(name)?.delete(fn);},setTimeout(fn,delay){const id=++timer;timers.set(id,{fn,delay});return id;},clearTimeout(id){timers.delete(id);}},{codeGeneration:{strings:false,wasm:false}});
 vm.runInContext('window=globalThis',context);
 const emit=data=>{const value=vm.runInContext(`JSON.parse(${JSON.stringify(JSON.stringify(data))})`,context);for(const fn of [...listeners.get('message')||[]])fn({data:value,source:parent,origin});};
 vm.runInContext((embedded?`const REPORT_APP_EMBEDDED_PARENTS=${JSON.stringify([origin])};\n`:'')+script,context);
 if(embedded)emit({protocol:'chartworks-report-app-v1',method:'bootstrap',frame:'formatting',generation:1,params:{challenge:'formatting-bundle-challenge'}});
 const tools={version:'chartworks-host-tools-v1',names:[...REPORT_APP_TOOLS]},allocation={version:'report-app-allocation-v1'},wrap=result=>({structuredContent:{result}});
 function dispatch(name,args){
  calls.push({name,args});
  if(name==='reporting_authoring_capabilities_v1')return wrap({version:'report-authoring-v1',builder:true,consumer:true,can_open:!!args.report,can_save:!!args.report,can_preview:!!args.report});
  if(name==='reporting_search')return wrap({version:'reporting-view-v1',items:[],next:''});
  if(name==='reporting_authoring_drafts_v1')return wrap({items:[{id:'report-a',metadata:definition.metadata,version,revision:version}]});
  if(name==='reporting_authoring_read_v1')return wrap({state:{id:'report-a',version,draft_revision:version},revision:version,private:true,definition});
  if(name==='reporting_authoring_save_v1'){assert.equal(args.expected_version,version);definition=structuredClone(args.definition);version++;return wrap({id:'report-a',version,draft_revision:version});}
  if(name==='reporting_authoring_block_read_v1')return wrap(structuredClone(view));
  if(['reporting_authoring_block_copy_v1','reporting_authoring_block_mapping_v1'].includes(name)){
   if(failure)return {isError:true,structuredContent:{error:{code:failure,outcome:failure==='unavailable'?'unknown':'not_started'}}};
   assert.equal(args.expected_version,view.block.state.version);assert.equal(args.revision,view.block.revision);assert.equal(args.digest,view.block.digest);assert(args.presentation);assert(!Object.hasOwn(args,'mapping'));view=formatSaved(view,args);return wrap(structuredClone(view));
  }
  assert.fail('Unexpected tool '+name);
 }
 const reply=(request,result)=>emit({...request,method:undefined,params:undefined,result});
 async function pump(){
  for(let rounds=0;rounds<30;rounds++){
   await tick();if(cursor===sent.length)return;
   while(cursor<sent.length){const request=sent[cursor++];let result;
    if(request.method==='ui/initialize')result={protocolVersion:'2026-01-26',hostCapabilities:{serverTools:{}},hostContext:{locale:'en-US','chartworks/supported-tools':tools,'chartworks/target-allocation':allocation}};
    else if(request.method==='initialize')result={challenge:'formatting-bundle-challenge',tools:true,capabilities:{supported_tools:tools,target_allocation:allocation},context:{locale:'en-US'}};
    else if(request.method==='app/allocate-target'){result={...request.params,id:'format-copy'};delete result.title;delete result.source;}
    else if(request.method==='tools/call'){result=dispatch(request.params.name,request.params.arguments);if(request.params.name===hold){delayed.push(()=>reply(request,result));continue;}}
    else continue;
    reply(request,result);
   }
  }
  assert.fail('Formatting bundle did not settle');
 }
 const select=()=>root.querySelectorAll('select').find(e=>e.getAttribute('aria-label')==='Format field'),button=label=>root.querySelectorAll('button').find(b=>b.textContent===label),input=label=>root.querySelectorAll('input').find(e=>e.getAttribute('aria-label')===label);
 async function click(label){const b=button(label);assert(b,'Missing '+label);assert.equal(b.disabled,false,label);b.listeners.click();await pump();}
 async function choose(id){const e=select();assert(e,'Format field');assert(e.children.some(o=>o.value===id),id);e.value=id;e.focus();e.listeners.change();await pump();assert.equal(document.activeElement,select(),'Selector focus survives redraw');}
 async function change(label,value){const e=input(label);assert(e,label);e.value=value;e.listeners.change();await pump();}
 const close=()=>{for(const fn of [...listeners.get('pagehide')||[]])fn();};
 await pump();await click('Build');await click('Formatting report');await click('Revenue detail');
 return {root,calls,sent,button,input,select,choose,active:()=>document.activeElement,click,change,close,pump,definition:()=>structuredClone(definition),view:()=>structuredClone(view),hold:name=>{hold=name;},async release(){hold='';for(const done of delayed.splice(0))done();await pump();}};
}
for(const embedded of [false,true]){
 const adapter=embedded?'embedded':'MCP';
 test(`${adapter} generated formatting selects one named field and stages zero/empty/reset across fields without queries`,async()=>{
  const f=await harness(embedded);try{
   await f.click('Field formatting');const count=f.calls.length,before=f.definition();assert(f.root.textContent.includes('Override'));
   assert.deepEqual(f.select().children.map(c=>[c.value,c.textContent]),[['category','Region'],['date','date'],['amount','Revenue'],['x','x']]);
   assert.equal(f.input('Fraction digits · Revenue'),undefined);assert.equal(f.root.querySelectorAll('input').filter(e=>e.getAttribute('aria-label')?.startsWith('Table header')).length,1);
   await f.choose('amount');assert.equal(f.input('Table header · Region'),undefined);assert(f.root.textContent.includes('Inherited from reviewed field'));
   await f.change('Fraction digits · Revenue','');assert.equal(f.button('Save formatting').disabled,true);await f.choose('category');assert.equal(f.button('Save formatting').disabled,true);await f.choose('amount');assert.equal(f.input('Fraction digits · Revenue').value,'');
   await f.change('Fraction digits · Revenue','0');await f.choose('category');await f.change('Table header · Region','');assert.equal(f.input('Table header · Region').value,'');await f.choose('amount');assert.equal(f.input('Fraction digits · Revenue').value,0);assert.equal(f.calls.length,count);assert.deepEqual(f.definition(),before);
   await f.click('Cancel formatting');assert.deepEqual(f.definition(),before);await f.click('Field formatting');assert.equal(f.input('Table header · Region').value,'Territory');await f.choose('amount');assert.equal(f.input('Fraction digits · Revenue').value,2);
   await f.change('Fraction digits · Revenue','0');await f.choose('category');await f.change('Table header · Region','');const save=f.button('Save formatting');save.listeners.click();save.listeners.click();await f.pump();
   const writes=f.calls.filter(c=>c.name==='reporting_authoring_block_copy_v1');assert.equal(writes.length,1);assert.deepEqual(writes[0].args.presentation,{version:1,edits:[{column:'category',set:{display_label:''}},{column:'amount',set:{fraction_digits:0}}]});assert.equal(f.sent.filter(c=>c.method==='app/allocate-target').length,1);assert.deepEqual(f.definition(),before);assert(f.root.textContent.includes('Private chart saved. It is unvalidated'));assert.equal(f.button('Private preview').disabled,true);assert.equal(f.button('Save report').disabled,false);await f.click('Save report');assert.equal(f.definition().report_pages[0].widgets[0].block.block,'format-copy');
   await f.click('Field formatting');await f.click('Reset Table header · Region');assert.equal(f.active(),f.input('Table header · Region'));assert.equal(f.input('Table header · Region').value,'Region');await f.choose('amount');await f.click('Reset Fraction digits · Revenue');assert.equal(f.active(),f.input('Fraction digits · Revenue'));assert.equal(f.input('Fraction digits · Revenue').value,3);await f.choose('category');assert.equal(f.input('Table header · Region').value,'Region');await f.click('Save formatting');assert.equal(f.calls.filter(c=>c.name==='reporting_authoring_block_mapping_v1').length,1);assert.equal(f.view().block.outputs[0].mapping.presentation,undefined);assert.equal(f.view().block.execution_digest,'b'.repeat(64));assert(!f.calls.some(c=>/validate|execute|reporting_run|chart_catalog/.test(c.name)));
  }finally{f.close();}
 });
 test(`${adapter} generated formatting keeps missing capabilities unavailable`,async()=>{const f=await harness(embedded,{missing:true});try{await f.click('Field formatting');assert(f.root.textContent.includes('Field formatting is unavailable'));assert.equal(f.button('Save formatting').disabled,true);assert.equal(f.input('Fraction digits · Revenue'),undefined);await f.click('Cancel formatting');assert(!f.calls.some(c=>c.name.includes('block_copy')));}finally{f.close();}});
 test(`${adapter} generated formatting keeps CAS and unknown-outcome fences without repeat mutation`,async()=>{
  for(const failure of ['conflict','unavailable']){const f=await harness(embedded,{failure});try{await f.click('Field formatting');await f.choose('amount');await f.change('Fraction digits · Revenue','0');await f.click('Save formatting');assert.equal(f.button('Save formatting').disabled,true);assert.equal(f.calls.filter(c=>c.name==='reporting_authoring_block_copy_v1').length,1);assert.equal(f.definition().report_pages[0].widgets[0].block.block,'source');if(failure==='unavailable'){assert(f.root.textContent.includes('outcome is unknown'));await f.click('Close chart editor');assert.equal(f.button('Field formatting').disabled,true);}else assert(f.root.textContent.includes('changed elsewhere'));}finally{f.close();}}
 });
 test(`${adapter} generated formatting close fences delayed reads and save replies`,async()=>{
  for(const phase of ['read','save']){const f=await harness(embedded);try{if(phase==='read'){f.hold('reporting_authoring_block_read_v1');await f.click('Field formatting');}else{await f.click('Field formatting');await f.choose('amount');await f.change('Fraction digits · Revenue','0');f.hold('reporting_authoring_block_copy_v1');await f.click('Save formatting');}f.close();await f.release();assert(f.root.textContent.includes('app is closed'));assert(!f.root.textContent.includes('Territory'));assert.equal(f.definition().report_pages[0].widgets[0].block.block,'source');}finally{f.close();}}
 });
}
