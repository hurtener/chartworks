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
test('expired root and in-flight expiry clear buffers without rerunning',async()=>{
 const f=fixture(1);f.root.summary.expires_at='2000-01-01T00:00:00Z';let calls=0;const retained=new RetainedReport(async(_,r)=>{calls++;return f.view(r);});await assert.rejects(retained.load(f.root),/expired/);assert.equal(calls,0);
 let expired;const observed=new Promise(r=>{expired=r;});retained.onExpired=expired;f.root.summary.expires_at=new Date(Date.now()+60).toISOString();await retained.load(f.root);await observed;assert.equal(retained.entries.size,0);assert.equal(retained.value,null);assert.equal(calls,1);retained.close();
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
