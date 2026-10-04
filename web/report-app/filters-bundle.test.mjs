// Compiled production IIFE against exact native DTOs in a minimal VM DOM.
// This is deterministic interoperability coverage, not Chromium visual proof.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
import {randomUUID} from 'node:crypto';
import {governedFilterBrowserFixture,filterBrowserTools} from './filters-browser-fixture.mjs';
const script=await readFile(new URL('./generated/report-app.js',import.meta.url),'utf8');
const tick=()=>new Promise(resolve=>setImmediate(resolve)),origin='https://report-host.example';
class Element {
  constructor(tag){this.tagName=tag.toUpperCase();this.children=[];this.attributes={};this.listeners={};this.dataset={};this.style={setProperty(k,v){this[k]=v;}};this._text='';this.disabled=false;this.value='';}
  set disabled(value){this._disabled=Boolean(value);}get disabled(){return this._disabled;}
  set textContent(value){this.children=[];this._text=String(value);}get textContent(){return this._text+this.children.map(c=>c.textContent||'').join('');}
  set innerHTML(_){assert.fail('No HTML parser');}set outerHTML(_){assert.fail('No HTML parser');}
  append(...children){for(const child of children){child.parent=this;this.children.push(child);}}
  replaceChildren(...children){this._text='';this.children=[];this.append(...children);}
  replaceWith(node){node.parent=this.parent;this.parent.children[this.parent.children.indexOf(this)]=node;}
  get firstChild(){return this.children[0];}
  setAttribute(k,v){this.attributes[k]=String(v);}getAttribute(k){return this.attributes[k]??null;}
  addEventListener(name,fn){this.listeners[name]=fn;}
  querySelectorAll(tag){return this.children.flatMap(c=>[...(c.tagName===tag.toUpperCase()?[c]:[]),...(c.querySelectorAll?.(tag)||[])]);}
  getBoundingClientRect(){return {top:0,bottom:800,left:0,right:1000,width:1000,height:800};}
}
async function harness(embedded){
  const fixture=await governedFilterBrowserFixture(process.env.CHARTWORKS_FILTER_FIXTURE_PATH||new URL('./testdata/governed-filter-native.json',import.meta.url));
  const root=new Element('main'),listeners=new Map(),sent=[],timers=new Map();let timer=0,cursor=0;
  const parent={postMessage(message){sent.push(JSON.parse(JSON.stringify(message)));}};
  const context=vm.createContext({parent,URL,TextEncoder,console,crypto:{randomUUID},
    document:{getElementById:()=>root,createElement:t=>new Element(t),createElementNS:(_,t)=>new Element(t),documentElement:new Element('html')},
    addEventListener(name,fn){if(!listeners.has(name))listeners.set(name,new Set());listeners.get(name).add(fn);},removeEventListener(name,fn){listeners.get(name)?.delete(fn);},
    setTimeout(fn,delay){const id=++timer;timers.set(id,{fn,delay});return id;},clearTimeout(id){timers.delete(id);}
  },{codeGeneration:{strings:false,wasm:false}});
  vm.runInContext('window=globalThis',context);vm.runInContext(fixture.clockScript(),context);
  const emit=data=>{const value=vm.runInContext(`JSON.parse(${JSON.stringify(JSON.stringify(data))})`,context);for(const fn of [...listeners.get('message')||[]])fn({data:value,source:parent,origin});};
  vm.runInContext((embedded?`const REPORT_APP_EMBEDDED_PARENTS=${JSON.stringify([origin])};\n`:'')+script,context);
  if(embedded)emit({protocol:'chartworks-report-app-v1',method:'bootstrap',frame:'filters',generation:1,params:{challenge:'native-filter-test-challenge'}});
  const names={version:'chartworks-host-tools-v1',names:filterBrowserTools};
  async function pump(){for(let rounds=0;rounds<20;rounds++){await tick();if(cursor===sent.length)return;while(cursor<sent.length){const request=sent[cursor++];let result;
    if(request.method==='ui/initialize')result={protocolVersion:'2026-01-26',hostCapabilities:{serverTools:{}},hostContext:{locale:'en-US','chartworks/supported-tools':names}};
    else if(request.method==='initialize')result={challenge:'native-filter-test-challenge',tools:true,capabilities:{supported_tools:names},context:{locale:'en-US'}};
    else if(request.method==='tools/call')result={structuredContent:{result:await fixture.invoke(request.params.name,request.params.arguments)}};
    else continue;
    emit({...request,method:undefined,params:undefined,result});
  }}assert.fail('Compiled fixture did not settle');}
  const all=tag=>root.querySelectorAll(tag),buttons=label=>all('button').filter(e=>e.textContent===label);
  async function click(label,index=0){const b=buttons(label)[index];assert(b,'Missing '+label);assert.equal(b.disabled,false,label);b.listeners.click();await pump();}
  const input=(label,value)=>{const e=all('input').find(e=>e.getAttribute('aria-label')===label);assert(e,label);e.value=value;e.listeners.input();};
  const arm=(request,kind='option')=>context.armNativeRequest(request,kind);
  const close=()=>{for(const fn of [...listeners.get('pagehide')||[]])fn();};
  await pump();return {fixture,root,all,click,input,arm,pump,close};
}
for(const embedded of [false,true])test(`compiled ${embedded?'embedded':'MCP'} native filters save, preview, clear and run without hidden source work`,async()=>{
  const f=await harness(embedded),d=f.fixture.data,counts=()=>f.fixture.counts().nativeSourceReadsRepresented;
  try{
    await f.click('Build');await f.click(d.initial_report.definition.metadata[0].title);
    await f.click('Change saved default',1);f.input('Find values','East');assert.equal(counts(),0);f.arm(d.initial_option_request);await f.click('Search options');assert.equal(counts(),1);
    await f.click('South · remove');const east=f.all('label').find(e=>e.children.some(c=>c.tagName==='SPAN'&&c.textContent==='East')).children.find(e=>e.tagName==='INPUT');east.checked=true;east.listeners.change();await f.click('Done');
    await f.click('Change saved default',0);f.input('Start date','2026-01-02');f.input('End date · inclusive','2026-01-31');await f.click('Done');assert.equal(counts(),1);
    await f.click('Save report');assert.deepEqual(f.fixture.calls.find(c=>c.name==='reporting_authoring_save_v1').args,d.save_request);await f.click('Reload latest');
    await f.click('Change saved default',1);f.input('Find values','East');f.arm(d.private_option_request);await f.click('Search options');await f.click('Cancel');assert.equal(counts(),2);
    await f.click('Choose temporary preview value',1);await f.click('East · remove');await f.click('Done');await f.click('Choose temporary preview value',0);f.input('Start date','2026-01-01');f.input('End date · inclusive','2026-01-01');await f.click('Done');assert.equal(counts(),2);
    await f.click('Selected');await f.click('Check chart status');f.arm(f.fixture.privateRun.preview_request,'preview');await f.click('Private preview');assert.equal(counts(),3);assert(f.root.textContent.includes('Private preview'));assert(!f.root.textContent.includes('Retained output unavailable.'));
    await f.click('Use saved default for preview',0);await f.click('Use saved default for preview',0);assert.equal(counts(),3);assert(f.root.textContent.includes('Preview is stale.'));
    await f.click('Browse');await f.click(d.published_catalog.items[0].title);await f.click('Choose Region');await f.click('East · remove');f.input('Find values','East');f.arm(d.published_option_request);await f.click('Search options');await f.click('Done');assert.equal(counts(),4);
    await f.click('Choose Day');f.input('Start date','2026-01-01');f.input('End date · inclusive','2026-01-01');await f.click('Done');f.arm(f.fixture.publicRun.run_request,'run');await f.click('Run with these filters');assert.equal(counts(),5);assert(f.root.textContent.includes('Retained report'));assert(!f.root.textContent.includes('Retained output unavailable.'));
    const disclosure=()=>f.all('details').find(e=>e.className==='consumer-tools');
    // Simulate the browser's native summary toggle, then deliver the actual
    // toggle event. Clicking hidden descendant buttons would miss this defect.
    if(!disclosure().open){disclosure().open=true;disclosure().listeners.toggle?.({target:disclosure(),currentTarget:disclosure()});await f.pump();}
    assert.equal(disclosure().open,true,'retained controls explicitly opened');
    await f.click('Use published default');assert.equal(disclosure().open,true,'first Clear must not close its remaining controls');
    await f.click('Choose Region');assert.equal(disclosure().open,true,'post-run filter editor must remain visible');
    assert(f.all('legend').some(e=>e.textContent==='Region · temporary selection'));
    f.input('Find values','North');await f.click('Cancel');assert.equal(disclosure().open,true,'Cancel must preserve the user-opened disclosure');
    await f.click('Use published default');assert.equal(disclosure().open,true,'second Clear must remain visible without reopening');assert.equal(counts(),5);assert(f.root.textContent.includes('These retained values use earlier filter selections.'));
    assert.deepEqual(f.fixture.snapshot().report,d.private_report);assert.equal(f.fixture.calls.filter(c=>c.name==='reporting_authoring_save_v1').length,1);
  }finally{f.close();}
});
