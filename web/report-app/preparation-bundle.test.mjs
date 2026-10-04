// Execute the actual generated IIFE over both host adapters with synthetic DTOs.
// This proves transport/controller behavior, not a hosted-browser or native run.
import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {REPORT_APP_TOOLS} from './bridge.js';
import {datasetFixture, datasetClone} from './dataset-fixture.mjs';
const script = await readFile(new URL('./generated/report-app.js', import.meta.url), 'utf8');
const tick = () => new Promise(resolve => setImmediate(resolve));
const origin = 'https://report-host.example';
class Element {
  constructor(tag) { this.tagName=tag.toUpperCase();this.children=[];this.attributes={};this.listeners={};this.dataset={};this.style={setProperty(k,v){this[k]=v;}};this._text='';this.value='';this.disabled=false; }
  set textContent(value) { this.children=[];this._text=String(value); }
  get textContent() { return this._text+this.children.map(c=>c.textContent||'').join(''); }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this._text='';this.children=children; }
  setAttribute(k,v) { this.attributes[k]=String(v); }
  getAttribute(k) { return this.attributes[k]??null; }
  addEventListener(name,fn) { this.listeners[name]=fn; }
  querySelectorAll(tag) { return this.children.flatMap(c=>[...(c.tagName===tag.toUpperCase()?[c]:[]),...(c.querySelectorAll?.(tag)||[])]); }
  getBoundingClientRect() { return {top:0,bottom:800,width:1000,height:800}; }
}
async function harness(embedded) {
  const root=new Element('main'),listeners=new Map(),sent=[],calls=[],timers=new Map(),native=datasetFixture();
  let cursor=0, timer=0, random=0, now=1791095400123, failure='', createUnknown=false;
  const definition={schema_version:3,metadata:[{locale:'en-US',title:'Preparation report'}],locale:'en-US',timezone:'UTC',partial_failure:'fail_closed',report_pages:[{id:'main',title:'Summary',widgets:[]}]};
  const parent={postMessage(message){sent.push(datasetClone(message));}};
  const context=vm.createContext({parent,URL,TextEncoder,console,clock:()=>now,
    crypto:{randomUUID(){return (++random).toString(16).padStart(32,'0');}},
    document:{getElementById:()=>root,createElement:tag=>new Element(tag),createElementNS:(_,tag)=>new Element(tag),documentElement:new Element('html')},
    addEventListener(name,fn){if(!listeners.has(name))listeners.set(name,new Set());listeners.get(name).add(fn);},
    removeEventListener(name,fn){listeners.get(name)?.delete(fn);},
    setTimeout(fn,delay){const id=++timer;timers.set(id,{fn,delay});return id;},clearTimeout(id){timers.delete(id);}
  },{codeGeneration:{strings:false,wasm:false}});
  vm.runInContext('window=globalThis;Date.now=clock',context);
  const emit=data=>{const value=vm.runInContext(`JSON.parse(${JSON.stringify(JSON.stringify(data))})`,context);for(const fn of [...listeners.get('message')||[]])fn({data:value,source:parent,origin});};
  vm.runInContext((embedded?`const REPORT_APP_EMBEDDED_PARENTS=${JSON.stringify([origin])};\n`:'')+script,context);
  if(embedded)emit({protocol:'chartworks-report-app-v1',method:'bootstrap',frame:'preparation',generation:1,params:{challenge:'preparation-bundle-challenge'}});
  const tools={version:'chartworks-host-tools-v1',names:[...REPORT_APP_TOOLS]},allocation={version:'report-app-allocation-v1'};
  const wrap=result=>({structuredContent:{result}});
  async function dispatch(name,args) {
    calls.push({name,args});
    if(name==='reporting_authoring_capabilities_v1')return wrap({version:'report-authoring-v1',builder:true,consumer:true,can_open:!!args.report,can_save:!!args.report,can_preview:!!args.report});
    if(name==='reporting_search')return wrap({version:'reporting-view-v1',items:[],next:''});
    if(name==='reporting_authoring_drafts_v1')return wrap({items:[{id:'report-a',metadata:definition.metadata,version:1,revision:1}]});
    if(name==='reporting_authoring_read_v1')return wrap({state:{id:'report-a',version:1,draft_revision:1},revision:1,private:true,definition});
    if(name==='reporting_authoring_prepare_chart_v1'&&failure){
      if(failure==='unknown')await native.invoke(name,args);
      return {isError:true,structuredContent:{error:{code:failure==='unknown'?'unavailable':failure,outcome:failure==='unknown'?'unknown':'not_started'}}};
    }
    const result=await native.invoke(name,args);
    if(name==='reporting_authoring_create_prepared_v1'&&createUnknown){createUnknown=false;return {isError:true,structuredContent:{error:{code:'unavailable',outcome:'unknown'}}};}
    return wrap(result);
  }
  async function pump() {
    for(let rounds=0;rounds<30;rounds++){
      await tick();if(cursor===sent.length)return;
      while(cursor<sent.length){const request=sent[cursor++];let result;
        if(request.method==='ui/initialize')result={protocolVersion:'2026-01-26',hostCapabilities:{serverTools:{}},hostContext:{locale:'en-US','chartworks/supported-tools':tools,'chartworks/target-allocation':allocation}};
        else if(request.method==='initialize')result={challenge:'preparation-bundle-challenge',tools:true,capabilities:{supported_tools:tools,target_allocation:allocation},context:{locale:'en-US'}};
        else if(request.method==='app/allocate-target')result={...request.params,id:'private-chart'};
        else if(request.method==='tools/call')result=await dispatch(request.params.name,request.params.arguments);
        else continue;
        if(request.method==='app/allocate-target')delete result.title;
        emit({...request,method:undefined,params:undefined,result});
      }
    }
    assert.fail('Preparation bundle did not settle');
  }
  const buttons=label=>root.querySelectorAll('button').filter(b=>b.textContent===label);
  async function click(label) {const button=buttons(label)[0];assert(button,'Missing '+label);assert.equal(button.disabled,false,label);button.listeners.click();await pump();}
  async function choose(label,value) {const element=root.querySelectorAll('select').find(e=>e.getAttribute('aria-label')===label);assert(element,label);assert.equal(element.disabled,false);element.value=value;element.listeners.change();await pump();}
  async function fields(){await click('Reviewed sales');await choose('Reviewed dataset','orders');await choose('New chart type','kpi');await choose('Reviewed measure','revenue');}
  async function stage(){await click('Build');await click('Preparation report');await click('Create from dataset');await fields();}
  const close=()=>{for(const fn of [...listeners.get('pagehide')||[]])fn();};
  await pump();return {root,calls,sent,native,buttons,click,fields,stage,close,pump,advance(ms){now+=ms;},now:()=>now,random:()=>random,setFailure(code){failure=code;},loseCreate(){createUnknown=true;}};
}

for(const embedded of [false,true]){
  const lane=embedded?'embedded':'MCP';
  test(`compiled ${lane} keeps long staged editing source-free, pins fresh Prepare and survives unknown Create`,async()=>{
    const f=await harness(embedded);try{
      await f.stage();assert.equal(f.random(),0);f.advance(86400000);
      assert(!f.root.querySelectorAll('input').some(e=>/\bID\b/.test(e.getAttribute('aria-label')||'')));
      const before=f.calls.length;await f.click('Prepare chart');
      const request=f.calls[before];assert.equal(request.name,'reporting_authoring_prepare_chart_v1');assert.equal(request.args.operation_version,'prepare-v1');
      assert.match(request.args.operation,/^prepare:[1-9][0-9]*:[a-f0-9]{32}$/);assert.equal(Number(request.args.operation.split(':')[1]),Math.floor(f.now()/1000));
      const random=f.random();f.advance(86400000);await f.click('Close chart setup');await f.click('Resume chart setup');await f.click('Inspect preparation');
      assert.equal(f.random(),random);assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);
      f.loseCreate();await f.click('Create private chart');assert(f.root.textContent.includes('Chart creation outcome is unknown'));
      await f.click('Inspect preparation');assert.equal(f.random(),random);await f.click('Recover created chart');
      const creates=f.calls.filter(c=>c.name.includes('create_prepared'));assert.equal(creates.length,2);assert.deepEqual(creates[0].args,creates[1].args);
      assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);assert.equal(f.random(),random+1,'Only the new local widget ID is allocated after recovery');
      assert(!f.calls.some(c=>/block_validate|preview|execute/.test(c.name)));assert(f.root.textContent.includes('Unvalidated'));
    }finally{f.close();}
  });
  test(`compiled ${lane} preserves typed admission errors and requires separate review and Prepare`,async()=>{
    for(const code of ['preparation_contract_required','preparation_operation_expired']){
      const f=await harness(embedded);try{
        await f.stage();f.setFailure(code);await f.click('Prepare chart');const first=f.calls.find(c=>c.name.includes('prepare_chart')).args,random=f.random();
        assert(f.root.textContent.includes('rejected'));assert.equal(f.buttons('Inspect preparation').length,0);assert.equal(f.buttons('Prepare chart').length,0);
        await f.click('Close chart setup');await f.click('Resume chart setup');assert.equal(f.random(),random);assert(f.root.textContent.includes(first.operation));
        const before=f.calls.length;await f.click('Review setup for new preparation');assert.deepEqual(f.calls.slice(before).map(c=>c.name),['list_topics']);assert.equal(f.random(),random);
        await f.fields();assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);f.setFailure('');await f.click('Prepare chart');
        const prepares=f.calls.filter(c=>c.name.includes('prepare_chart'));assert.equal(prepares.length,2);assert.notEqual(prepares[1].args.operation,first.operation);
        assert.equal(prepares[1].args.operation_version,'prepare-v1');assert.equal(prepares[1].args.new_block,first.new_block);
      }finally{f.close();}
    }
  });
  test(`compiled ${lane} never auto-rekeys an unknown Prepare and only inspects original custody`,async()=>{
    const f=await harness(embedded);try{
      await f.stage();f.setFailure('unknown');await f.click('Prepare chart');const first=f.calls.find(c=>c.name.includes('prepare_chart')).args,random=f.random();
      assert(f.root.textContent.includes('Preparation outcome is unknown'));assert.equal(f.buttons('Review setup for new preparation').length,0);
      await f.click('Close chart setup');f.advance(86400000);await f.click('Resume chart setup');await f.click('Inspect source attempt');
      assert.deepEqual(f.calls.at(-1).args,{new_block:first.new_block,operation:first.operation,action:'inspect'});
      assert.equal(f.calls.filter(c=>c.name.includes('prepare_chart')).length,1);assert.equal(f.random(),random);
    }finally{f.close();}
  });
}
