import test from 'node:test';
import assert from 'node:assert/strict';
import {CatalogSearch,CATALOG_PAGE_SIZE,CATALOG_MAX_PAGES} from './catalog.js';
const item=(id,kind='report')=>({target:{id,kind,revision:2},title:'Report '+id,description:'Synthetic metadata preview',locale:'en-US'});
const page=(items=[],next='')=>({version:'reporting-view-v1',items,next});
test('search pins the explicit query/locale and follows sparse pages without hidden scans',async()=>{
 const calls=[],s=new CatalogSearch('report',async(name,args)=>{calls.push({name,args});return calls.length===1?page([],'cursor-a'):page([item('found')]);});s.input='revenue';assert.equal(calls.length,0);
 assert.deepEqual(await s.load({locale:'es-AR'}),{items:[],next:'cursor-a'});assert.equal(calls.length,1);assert.equal(s.pages,1);
 await assert.rejects(s.load({after:'cursor-a',query:'other',locale:'es-AR'}),/stale_validation/);assert.equal(calls.length,1);
 await s.load({after:s.next,locale:'es-AR'});assert.equal(s.items[0].target.id,'found');assert.equal(s.next,'');assert.deepEqual(calls[1],{name:'reporting_search',args:{kind:'report',query:'revenue',locale:'es-AR',after:'cursor-a',limit:40}});
});
test('each explicit catalog search is bounded even when every authorized page is empty',async()=>{
 let count=0;const s=new CatalogSearch('block',async()=>page([],`cursor-${++count}`));for(let i=0;i<CATALOG_MAX_PAGES;i++)await s.load({after:s.next,locale:'en-US'});
 assert.equal(count,5);assert.equal(s.next,'');assert.equal(s.limited,true);await assert.rejects(s.load({after:'cursor-5',locale:'en-US'}),/stale_validation/);assert.equal(count,5);
 await s.load({query:'narrower',locale:'en-US'});assert.equal(s.pages,1);assert.equal(s.limited,false);
});
test('two hundred returned resources is the same bounded five-page search',async()=>{
 let count=0;const s=new CatalogSearch('report',async()=>{const n=count++;return page(Array.from({length:CATALOG_PAGE_SIZE},(_,i)=>item(`r-${n*40+i}`)),`cursor-${count}`);});for(let i=0;i<5;i++)await s.load({after:s.next,locale:'en-US'});assert.equal(s.items.length,200);assert.equal(s.next,'');assert.equal(s.limited,true);
});
test('new search, context invalidation and close fence earlier successful responses',async()=>{
 const pending=[];const s=new CatalogSearch('report',()=>new Promise(resolve=>pending.push(resolve)));
 const first=s.load({query:'old',locale:'en-US'}),second=s.load({query:'new',locale:'en-US'});pending[1](page([item('new')]));await second;pending[0](page([item('old')]));assert.equal(await first,null);assert.equal(s.items[0].target.id,'new');
 const third=s.load({locale:'en-US'});s.invalidate();pending[2](page([item('stale')]));assert.equal(await third,null);assert.deepEqual(s.items,[]);
 const fourth=s.load({locale:'en-US'});s.close();pending[3](page([item('secret')]));assert.equal(await fourth,null);assert.equal(s.input,'');assert.deepEqual(s.items,[]);await assert.rejects(s.load({locale:'en-US'}),/unavailable/);
});
test('invalid query, response bounds, resource shapes and cycling cursors fail closed',async()=>{
 let calls=0;const s=new CatalogSearch('report',async()=>{calls++;return page();});for(const query of ['a'.repeat(257),'é'.repeat(129),'secret\0value',null])await assert.rejects(s.load({query,locale:'en-US'}),/invalid_request/);assert.equal(calls,0);
 for(const invalid of [{version:'other',items:[]},page(Array.from({length:41},(_,i)=>item(String(i)))),page([item('block','block')]),page([item('same'),item('same')]),page([{...item('bad'),target:{kind:'report',id:'bad',revision:0}}]),page([],{}),page([],'bad cursor')]){const bad=new CatalogSearch('report',async()=>invalid);await assert.rejects(bad.load({locale:'en-US'}),/unavailable/);assert.deepEqual(bad.items,[]);}
 let next='one';const cycle=new CatalogSearch('report',async()=>page([],next));await cycle.load({locale:'en-US'});await assert.rejects(cycle.load({after:'one',locale:'en-US'}),/unavailable/);next='two';await cycle.load({after:'one',locale:'en-US'});next='one';await assert.rejects(cycle.load({after:'two',locale:'en-US'}),/unavailable/);
});
test('late rejected metadata reads cannot replace a newer search or navigation with an old error',async()=>{
 const pending=[];const s=new CatalogSearch('report',()=>new Promise((resolve,reject)=>pending.push({resolve,reject})));const old=s.load({query:'old',locale:'en-US'}),latest=s.load({query:'latest',locale:'en-US'});pending[1].resolve(page([item('latest')]));await latest;pending[0].reject(new Error('old access changed'));assert.equal(await old,null);assert.equal(s.items[0].target.id,'latest');let current=true;const navigation=s.load({locale:'en-US',current:()=>current});current=false;pending[2].reject(new Error('old page unavailable'));assert.equal(await navigation,null);
});
