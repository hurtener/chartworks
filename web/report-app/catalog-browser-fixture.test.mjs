// Exercise the hosted journey's closed synthetic metadata fixture in a VM.
// This is not browser rendering, source execution or production-host evidence.
import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {initializeSyntheticHost,capturedViews,browserMappingSamples,browserDatasetSamples} from './browser_fixture.mjs';
const clone=value=>JSON.parse(JSON.stringify(value));
for(const embedded of [false,true])test(`hosted ${embedded?'embedded':'MCP'} search fixture preserves native bounded metadata semantics`,()=>{
 const listeners=new Map(),replies=[],frame={style:{},addEventListener(){},contentWindow:{postMessage(value){replies.push(clone(value));}}};
 const context=vm.createContext({TextEncoder,location:{origin:'https://report-host.example'},document:{getElementById:()=>frame},addEventListener:(name,listener)=>listeners.set(name,listener),setTimeout:fn=>fn(),performance:{now:()=>0}});
 vm.runInContext('window=globalThis',context);vm.runInContext(`(${initializeSyntheticHost.toString()})(${JSON.stringify(capturedViews)},${embedded},${JSON.stringify(browserMappingSamples)},${JSON.stringify(browserDatasetSamples)})`,context);
 const original=clone(capturedViews),send=(kind,query,after='')=>{listeners.get('message')({source:frame.contentWindow,origin:'https://report-host.example',data:{...(embedded?{protocol:'chartworks-report-app-v1'}:{jsonrpc:'2.0'}),method:'tools/call',id:replies.length+1,params:{name:'reporting_search',arguments:{kind,query,after,locale:'en-US',limit:40}}}});return replies.at(-1).result;};
 for(const kind of ['report','block']){assert.equal(send(kind,'No matching title').structuredContent.result.items.length,0);context.catalogSparseSearch=true;const first=send(kind,'Synthetic').structuredContent.result;assert.deepEqual(first,{version:'reporting-view-v1',items:[],next:'synthetic-next'});const second=send(kind,'Synthetic',first.next).structuredContent.result;assert.equal(second.items.length,1);assert.equal(second.items[0].target.kind,kind);assert.equal(second.next,'');assert.match(second.items[0].description,/Synthetic/);assert.equal(send(kind,'').structuredContent.result.items.length,1);assert.equal(send(kind,'Synthetic','wrong-cursor').structuredContent.error.code,'invalid_request');context.catalogSparseSearch=false;}
 assert(context.calls.every(call=>call.name==='reporting_search'));assert.deepEqual(capturedViews,original);
});
