import test from 'node:test';
import assert from 'node:assert/strict';
import {RetainedReport,executionFingerprint,CANVAS_MAX_OUTPUTS} from './retained.js';
import {appError} from './model.js';

const clone=v=>JSON.parse(JSON.stringify(v));
function fixture(count=3){
 const widgets=[{id:'heading',kind:'text',grid:{row:0,column:0,width:12,height:1},presentation:{},outputs:[]}];
 for(let i=0;i<count;i++)widgets.push({id:'w'+i,kind:'block',state:'completed',grid:{row:i+1,column:0,width:12,height:1},presentation:{title:'Metric '+i},outputs:['out'+i]});
 const root={version:'reporting-view-v1',summary:{kind:'report',run:'run-a',target:{kind:'report',id:'report-a',revision:4},state:'completed',private:true,expires_at:'2099-01-01T00:00:00Z'},selection:{kind:'report',run:'run-a',page:'main',widget:'heading',output:'',offset:0,limit:100},pages:[{id:'main',report:'report-a',revision:4,widgets}],locale:'en-US',timezone:'UTC',outputs:[],filters:[],page_bounds:{offset:0,limit:100,total:0},text:{format:'plain',text:'Retained overview'},redacted:false};
 const view=request=>{const v=clone(root);v.selection={...request};delete v.text;v.outputs=[{id:request.output,enabled:true,selected:true}];v.output={id:request.output,state:'succeeded',kind:'table',retained_digest:'digest-'+request.output,table:{columns:[{id:'amount',name:'Amount',type:'decimal'}],rows:[[{null:false,value:request.offset===0?'9007199254740993.01':'5.5'}]],totals:[],warnings:[]}};v.page_bounds={offset:request.offset,limit:1,total:2,...(request.offset===0?{next:1}:{})};return v;};return {root,view};
}

test('retained fanout reuses the exact root payload, bounds concurrency and never executes',async()=>{
 const f=fixture(9),calls=[];let active=0,high=0;
 const retained=new RetainedReport(async(name,request)=>{calls.push({name,request});active++;high=Math.max(high,active);await new Promise(r=>setTimeout(r,2));active--;return f.view(request);});
 assert.equal(await retained.load(f.root),true);assert.equal(high,4);assert.equal(calls.length,9);assert(calls.every(c=>c.name==='reporting_view'));assert.equal(retained.entries.size,10);assert.equal(retained.get('main','heading').text.text,'Retained overview');assert.equal(retained.get('main','w8','out8').output.retained_digest,'digest-out8');retained.close();
});
test('table paging requests one exact output and replaces one page while keeping sibling identities',async()=>{
 const f=fixture(),calls=[],retained=new RetainedReport(async(name,r)=>{calls.push({name,r});return f.view(r);});await retained.load(f.root);const sibling=retained.get('main','w1','out1');
 await retained.page('main','w0','out0',1);assert.equal(calls.length,4);assert.deepEqual(calls.at(-1),{name:'reporting_view',r:{kind:'report',run:'run-a',page:'main',widget:'w0',output:'out0',offset:1,limit:1}});assert.equal(retained.get('main','w0','out0').output.table.rows[0][0].value,'5.5');assert.equal(retained.get('main','w1','out1'),sibling);assert.equal(retained.entries.size,4);retained.close();
});
test('concurrent repeated paging is fenced without another retained read',async()=>{
 const f=fixture(1);let resolve;const retained=new RetainedReport(async(_,r)=>f.view(r));await retained.load(f.root);retained.invoke=(_,r)=>new Promise(done=>{resolve=()=>done(f.view(r));});const first=retained.page('main','w0','out0',1);assert.equal(await retained.page('main','w0','out0',1),false);resolve();assert.equal(await first,true);retained.close();
});
test('all cached values clear on a denied page and no automatic retry occurs',async()=>{
 for(const code of ['forbidden','unauthenticated','not_found']){const f=fixture(),retained=new RetainedReport(async(_,r)=>f.view(r));await retained.load(f.root);let calls=0;retained.invoke=async()=>{calls++;throw appError(code);};await assert.rejects(retained.page('main','w0','out0',1),e=>e.code===code);assert.equal(retained.value,null);assert.equal(retained.entries.size,0);assert.equal(calls,1);retained.close();}
});
test('fanout denial fences already pending replies and erases root payload',async()=>{
 const f=fixture(6),waiting=[];let calls=0;const retained=new RetainedReport(async(_,r)=>{calls++;if(r.widget==='w1')throw appError('forbidden');return new Promise(resolve=>waiting.push(()=>resolve(f.view(r))));});const loading=retained.load(f.root);await assert.rejects(loading,/forbidden/);for(const finish of waiting)finish();await Promise.resolve();assert.equal(calls,4);assert.equal(retained.entries.size,0);assert.equal(retained.value,null);retained.close();
});
test('new generation and close cannot be repopulated by delayed fanout or page',async()=>{
 for(const op of ['load','page']){const f=fixture(1);let resolve;const retained=new RetainedReport(async(_,r)=>f.view(r));if(op==='page')await retained.load(f.root);retained.invoke=(_,r)=>new Promise(done=>{resolve=()=>done(f.view(r));});const work=op==='page'?retained.page('main','w0','out0',1):retained.load(f.root);retained.close();resolve();assert.equal(await work,false);assert.equal(retained.entries.size,0);assert.equal(retained.value,null);}
});
test('mismatched run, target, privacy, exact selection or authorized manifest fails closed',async()=>{
 const changes=[v=>v.summary.run='foreign',v=>v.summary.target.id='foreign',v=>v.summary.target.revision++,v=>v.summary.private=false,v=>v.selection.run='foreign',v=>v.selection.widget='w1',v=>v.selection.output='out1',v=>v.selection.offset++,v=>v.selection.limit++,v=>v.page_bounds.offset++,v=>v.pages[0].widgets.pop(),v=>v.pages[0].widgets[1].grid.width=6,v=>v.redacted=true,v=>v.output.id='out1',v=>v.output.retained_digest='foreign-digest',v=>v.outputs[0].selected=false];
 for(const change of changes){const f=fixture(),retained=new RetainedReport(async(_,r)=>f.view(r));await retained.load(f.root);retained.invoke=async(_,r)=>{const v=f.view(r);change(v);return v;};await assert.rejects(retained.page('main','w0','out0',1));assert.equal(retained.entries.size,0);assert.equal(retained.value,null);retained.close();}
});
test('expired root and in-flight expiry clear buffers without rerunning',async t=>{
 t.mock.timers.enable({apis:['Date','setTimeout'],now:new Date('2026-01-01T00:00:00Z')});
 const f=fixture(1);f.root.summary.expires_at='2000-01-01T00:00:00Z';let calls=0,expired=0;
 const retained=new RetainedReport(async(_,r)=>{calls++;return f.view(r);},()=>{expired++;});
 try{
  await assert.rejects(retained.load(f.root),/expired/);assert.equal(calls,0);assert.equal(expired,0);
  f.root.summary.expires_at=new Date(Date.now()+60).toISOString();assert.equal(await retained.load(f.root),true);
  assert.equal(retained.entries.size,2);assert(retained.value);assert.equal(calls,1);
  t.mock.timers.tick(59);assert.equal(retained.entries.size,2);assert(retained.value);assert.equal(expired,0);
  t.mock.timers.tick(1);assert.equal(retained.entries.size,0);assert.equal(retained.value,null);assert.equal(calls,1);assert.equal(expired,1);
  t.mock.timers.tick(1000);assert.equal(expired,1,'one expiry callback for this load');assert.equal(calls,1);

  let release;retained.invoke=(_,request)=>{calls++;return new Promise(resolve=>{release=()=>resolve(f.view(request));});};
  f.root.summary.expires_at=new Date(Date.now()+60).toISOString();const loading=retained.load(f.root);
  assert.equal(typeof release,'function');assert.equal(calls,2);assert.equal(retained.entries.size,1);
  t.mock.timers.tick(59);assert.equal(expired,1);assert.equal(retained.entries.size,1);assert(retained.value);
  t.mock.timers.tick(1);assert.equal(expired,2);assert.equal(retained.entries.size,0);assert.equal(retained.value,null);
  release();assert.equal(await loading,false);assert.equal(retained.entries.size,0);assert.equal(retained.value,null);assert.equal(calls,2);
  t.mock.timers.tick(1000);assert.equal(expired,2,'late completion cannot fire expiry again or restore buffers');assert.equal(calls,2);
 }finally{retained.close();}
});
test('pending roots show metadata only and do not fan out speculative payload reads',async()=>{
 const f=fixture();f.root.summary.state='running';delete f.root.text;let calls=0;const retained=new RetainedReport(async()=>{calls++;});assert.equal(await retained.load(f.root),true);assert.equal(calls,0);assert.equal(retained.entries.size,0);retained.close();
});
test('read fanout and aggregate cache budgets reject instead of silently dropping outputs',async()=>{
 let calls=0;const over=fixture(CANVAS_MAX_OUTPUTS-1),retained=new RetainedReport(async()=>{calls++;});over.root.pages[0].widgets[1].outputs.push('extra');await assert.rejects(retained.load(over.root),/limit_exceeded/);assert.equal(calls,0);
 const f=fixture(6);retained.invoke=async(_,r)=>({...f.view(r),boundedExtra:'x'.repeat(3<<20)});await assert.rejects(retained.load(f.root),/limit_exceeded/);assert.equal(retained.value,null);assert.equal(retained.entries.size,0);retained.close();
});
test('malformed/duplicate canonical grid and output identities are rejected before fanout',async()=>{
 for(const change of [v=>v.pages[0].widgets[1].grid.column=12,v=>v.pages[0].widgets[1].grid.row=-1,v=>v.pages[0].widgets[1].outputs=['a','a'],v=>v.pages[0].widgets[1].id='heading',v=>v.pages.push(clone(v.pages[0]))]){const f=fixture();change(f.root);const r=new RetainedReport(async()=>{assert.fail('must not read');});await assert.rejects(r.load(f.root),/invalid_request/);r.close();}
});
test('execution fingerprint ignores only safe presentation and ordering fields',()=>{
 const d={schema_version:2,metadata:[{locale:'en-US',title:'Report'}],locale:'en-US',timezone:'UTC',widgets:[{id:'heading',kind:'text',text:{format:'plain',text:'Overview'},presentation:{},grid:{row:0,column:0,width:12,height:1}},{id:'metric',kind:'block',block:{block:'approved',revision:7,outputs:['amount'],policy:'published',narrative:false},bindings:[{filter:'period',parameter:'period'}],presentation:{title:'Amount'},grid:{row:1,column:0,width:12,height:1}}],filters:[{label:'Period',parameter:{name:'period',type:'integer',default:{literal:'2'}}}]};
 const key=executionFingerprint(d),display=clone(d);display.metadata[0].title='Edited';display.widgets.reverse();display.widgets[0].grid.column=6;display.widgets[0].presentation.title='New label';display.widgets[1].text.text='New heading';assert.equal(executionFingerprint(display),key);
 for(const mutate of [v=>v.widgets[1].block.revision++,v=>v.widgets[1].block.outputs=['other'],v=>v.widgets[1].bindings=[],v=>v.filters[0].parameter.default.literal='3',v=>v.timezone='Europe/Madrid',v=>v.widgets[1].future_semantic_field='changed']){const next=clone(d);mutate(next);assert.notEqual(executionFingerprint(next),key);}assert.equal(d.metadata[0].title,'Report');
});

test('root-selected payload reuse has the same output identity/choice checks as fanout',async()=>{
 for(const change of [v=>v.output.id='wrong',v=>v.outputs[0].selected=false,v=>v.outputs[0].enabled=false,v=>v.selection.widget='hidden-widget',v=>v.selection.output='hidden-output',v=>v.selection.limit=1,v=>v.text={text:'Wrong union'}]){const f=fixture(),request={kind:'report',run:'run-a',page:'main',widget:'w0',output:'out0',offset:0,limit:100},root=f.view(request);change(root);let calls=0;const retained=new RetainedReport(async(_,r)=>{calls++;return f.view(r);});await assert.rejects(retained.load(root),/stale_validation/);assert.equal(retained.value,null);assert.equal(retained.entries.size,0);assert.equal(calls,0);retained.close();}
});

test('existing query widget uses only its fixed retained result, without enabling query creation or execution',async()=>{
 const f=fixture(1),widget=f.root.pages[0].widgets[1];widget.kind='query';widget.outputs=[];const calls=[],retained=new RetainedReport(async(name,r)=>{calls.push({name,r});return f.view(r);});await retained.load(f.root);assert.equal(calls.length,1);assert.equal(calls[0].name,'reporting_view');assert.equal(calls[0].r.output,'result');assert.equal(retained.get('main','w0','result').output.kind,'table');await retained.page('main','w0','result',1);assert.equal(calls[1].name,'reporting_view');assert.equal(calls[1].r.output,'result');assert.equal(retained.get('main','w0','result').output.table.rows[0][0].value,'5.5');retained.close();
});

function firstWidgetFailure(kind='block',state='failed'){
 const f=fixture(2),failed=f.root.pages[0].widgets[1];f.root.pages[0].widgets.shift();failed.kind=kind;failed.state=state;failed.code=state==='partial'?'output_failed':'dependency_unavailable';if(kind==='query')failed.outputs=[];
 f.root.summary.state='partial';f.root.selection.widget=failed.id;delete f.root.text;f.root.output={id:failed.id,kind,state,code:failed.code,retained_digest:''};f.root.outputs=kind==='query'?[]:[{id:'out0',enabled:true,selected:true}];
 const view=request=>{const v=f.view(request);if(request.widget===failed.id){v.output=clone(f.root.output);v.outputs=clone(f.root.outputs);v.page_bounds={offset:0,limit:request.limit,total:0};}return v;};return {root:f.root,view};
}
test('first failed or partial block/query status does not hide healthy siblings or masquerade as an output',async()=>{
 for(const kind of ['block','query'])for(const state of ['failed','partial']){const f=firstWidgetFailure(kind,state),calls=[],retained=new RetainedReport(async(name,r)=>{calls.push({name,r});return f.view(r);});assert.equal(await retained.load(f.root),true);assert.equal(retained.get('main','w0',''),undefined);assert.equal(retained.entries.size,2);const output=kind==='query'?'result':'out0',failed=retained.get('main','w0',output),healthy=retained.get('main','w1','out1');assert.equal(failed.output.id,'w0');assert.equal(failed.output.state,state);assert.equal(failed.output.table,undefined);assert.equal(healthy.output.table.rows[0][0].value,'9007199254740993.01');assert.deepEqual(calls.map(c=>[c.name,c.r.widget,c.r.output]),[['reporting_view','w0',output],['reporting_view','w1','out1']]);await retained.page('main','w1','out1',1);assert.equal(retained.get('main','w1','out1').output.table.rows[0][0].value,'5.5');assert.equal(retained.get('main','w0',output),failed);retained.close();}
});
test('unresolved widget failure cannot bypass authorized identity, state, union or canonical window checks',async()=>{
 const changes=[v=>v.selection.widget='hidden-widget',v=>v.selection.page='hidden-page',v=>v.output.id='other-widget',v=>v.output.kind='table',v=>v.output.state='succeeded',v=>v.output.code='different-code',v=>v.pages[0].widgets[0].state='completed',v=>v.output.table={rows:[]},v=>v.output.chart={kind:'kpi'},v=>v.text={text:'unexpected'},v=>v.output.retained_digest='unexpected-digest',v=>v.selection.offset=1,v=>v.selection.limit=1,v=>v.selection.output='hidden-output',v=>v.page_bounds.offset=1,v=>v.page_bounds.total=1,v=>v.page_bounds.next=1,v=>v.page_bounds.limit=1];
 for(const change of changes){const f=firstWidgetFailure();change(f.root);let calls=0;const retained=new RetainedReport(async(_,r)=>{calls++;return f.view(r);});await assert.rejects(retained.load(f.root),/stale_validation/);assert.equal(calls,0);assert.equal(retained.value,null);assert.equal(retained.entries.size,0);retained.close();}
});
test('exact-output fanout also verifies widget-level failure status before preserving healthy siblings',async()=>{const f=firstWidgetFailure('query');const retained=new RetainedReport(async(_,r)=>{const v=f.view(r);if(r.widget==='w0')v.output.id='foreign-widget';return v;});await assert.rejects(retained.load(f.root),/stale_validation/);assert.equal(retained.value,null);assert.equal(retained.entries.size,0);retained.close();});
function emptyFirstPage(){const f=fixture(1);f.root.pages[0].widgets.shift();f.root.pages.unshift({id:'empty',report:'report-a',revision:4,widgets:[]});f.root.selection={...f.root.selection,page:'empty',widget:''};delete f.root.text;return f;}
test('authorized empty first page is metadata only while healthy pages load retained outputs',async()=>{const f=emptyFirstPage(),calls=[],r=new RetainedReport(async(name,request)=>{calls.push({name,request});return f.view(request);});assert.equal(await r.load(f.root),true);assert.equal(r.entries.size,1);assert.equal(r.get('empty','',''),undefined);assert.equal(r.get('main','w0','out0').output.kind,'table');assert.deepEqual(calls.map(c=>[c.name,c.request.page,c.request.widget,c.request.output]),[['reporting_view','main','w0','out0']]);r.close();const empty=emptyFirstPage();empty.root.pages.pop();const all=new RetainedReport(async()=>assert.fail('empty report does not fan out'));assert.equal(await all.load(empty.root),true);assert.equal(all.entries.size,0);all.close();});
test('empty-page sentinel rejects hidden coordinates, payloads, nonempty pages and malformed bounds before fanout',async()=>{
 for(const change of [v=>v.version='wrong',v=>v.summary.target.id='foreign',v=>v.summary.target.revision++,v=>v.selection.run='foreign',v=>v.selection.kind='block',v=>v.selection.page='hidden',v=>v.pages.shift(),v=>v.pages[0].widgets.push(v.pages[1].widgets[0]),v=>v.selection.widget='hidden',v=>v.selection.output='hidden',v=>delete v.selection.widget,v=>delete v.selection.output,v=>v.selection.offset=1,v=>v.selection.limit=1,v=>v.page_bounds.offset=1,v=>v.page_bounds.limit=1,v=>v.page_bounds.total=1,v=>v.page_bounds.next=0,v=>v.text={text:'hidden'},v=>v.output={id:'hidden'},v=>v.chart={},v=>v.table={},v=>v.narrative={},v=>v.outputs=[{id:'hidden'}],v=>v.summary.expires_at='invalid']){const f=emptyFirstPage();change(f.root);let calls=0;const r=new RetainedReport(async()=>{calls++;});await assert.rejects(r.load(f.root));assert.equal(calls,0);assert.equal(r.value,null);assert.equal(r.entries.size,0);r.close();}
});
test('report pages cannot alter retained target coordinates or reuse global widget identity',async()=>{for(const change of [v=>v.pages[0].report='foreign',v=>v.pages[0].revision++,v=>v.pages.push({...clone(v.pages[0]),id:'second'})]){const f=fixture();change(f.root);const r=new RetainedReport(async()=>assert.fail('invalid page must not fan out'));await assert.rejects(r.load(f.root));assert.equal(r.value,null);assert.equal(r.entries.size,0);r.close();}});

function twoPageFixture(){const f=fixture(12),later=f.root.pages[0].widgets.splice(7);f.root.pages.push({id:'detail',report:'report-a',revision:4,widgets:later});return f;}
test('selected-page loading shows verified layout before data, bounds four reads, and defers siblings until selected',async()=>{
 const f=twoPageFixture(),calls=[],progress=[];let active=0,high=0;const r=new RetainedReport(async(name,request)=>{calls.push({name,request});active++;high=Math.max(high,active);await new Promise(resolve=>setTimeout(resolve,2));active--;return f.view(request);});
 assert.equal(await r.load(f.root,{page:'main',onProgress:()=>progress.push({entries:r.entries.size,calls:calls.length,page:r.loadingPage,layout:r.value.pages.length})}),true);assert.deepEqual(progress[0],{entries:1,calls:0,page:'main',layout:2});assert.equal(calls.length,6);assert(calls.every(c=>c.name==='reporting_view'&&c.request.page==='main'));assert.equal(high,4);assert(r.pageReady('main'));assert(!r.pageReady('detail'));assert.equal(r.get('detail','w6','out6'),undefined);
 assert.equal(await r.loadPage('detail'),true);assert.equal(calls.length,12);assert(calls.slice(6).every(c=>c.request.page==='detail'));assert(r.pageReady('detail'));await r.loadPage('main');await r.loadPage('detail');assert.equal(calls.length,12);assert.equal(r.loadingPage,'');r.close();
});
test('lazy sibling denial erases prior page values and fences late replies instead of retaining an authorized-looking partial cache',async()=>{
 const f=twoPageFixture();let reject=false,replies=[];const r=new RetainedReport(async(_,request)=>{if(!reject)return f.view(request);if(request.widget==='w6')throw appError('forbidden');return new Promise(resolve=>replies.push(()=>resolve(f.view(request))));});await r.load(f.root,{page:'main'});assert(r.entries.size>0);reject=true;await assert.rejects(r.loadPage('detail'),/forbidden/);for(const reply of replies)reply();await new Promise(resolve=>setImmediate(resolve));assert.equal(r.entries.size,0);assert.equal(r.value,null);assert.equal(r.loadingPage,'');r.close();
});
test('unknown page is rejected before reads and overlapping page loads cannot exceed the global fanout bound',async()=>{
 const f=twoPageFixture();let release,hold=false;const calls=[];const r=new RetainedReport(async(_,request)=>{calls.push(request);if(hold)await new Promise(resolve=>release=resolve);return f.view(request);});await assert.rejects(r.load(f.root,{page:'foreign'}),/invalid_request/);assert.equal(calls.length,0);await r.load(f.root,{page:'main'});await assert.rejects(r.loadPage('foreign'),/invalid_request/);hold=true;const releases=[];r.invoke=async(_,request)=>{calls.push(request);await new Promise(resolve=>releases.push(resolve));return f.view(request);};const pending=r.loadPage('detail');assert.equal(releases.length,4);await assert.rejects(r.loadPage('main'),/busy/);r.close();for(const done of releases)done();assert.equal(await pending,false);assert.equal(r.value,null);assert.equal(r.entries.size,0);assert.equal(calls.length,10);
});
