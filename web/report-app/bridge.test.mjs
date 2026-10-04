import test from 'node:test';
import assert from 'node:assert/strict';
import {MCPReportAdapter,EmbeddedReportAdapter} from './bridge.js';
function harness() { const sent=[],listeners=new Map(),parent={postMessage:(message,origin)=>sent.push({message,origin})},win={parent,addEventListener:(name,fn)=>listeners.set(name,fn),removeEventListener:(name,fn)=>{if(listeners.get(name)===fn)listeners.delete(name);}};return {sent,listeners,parent,win,receive:(message,origin='https://host.example',source=parent)=>listeners.get('message')?.({data:message,origin,source})}; }
async function connected() {const h=harness(),bridge=new MCPReportAdapter(h.win),pending=bridge.connect();h.receive({jsonrpc:'2.0',id:h.sent[0].message.id,result:{protocolVersion:'2026-01-26',hostCapabilities:{serverTools:{}},hostContext:{}}});await pending;return {...h,bridge};}

test('MCP initialization pins only the exact parent and valid origin',async()=>{const h=harness(),b=new MCPReportAdapter(h.win),p=b.connect();const response={jsonrpc:'2.0',id:1,result:{protocolVersion:'2026-01-26',hostCapabilities:{serverTools:{}}}};h.receive(response,'https://host.example',{});assert.equal(b.ready,false);h.receive(response,'null');assert.equal(b.ready,false);h.receive(response);await p;assert.equal(b.origin,'https://host.example');assert.equal(h.sent[1].origin,'https://host.example');b.close();});

test('calls use tools/call only and foreign origin/frame replies cannot settle them',async()=>{const h=await connected(),p=h.bridge.call('reporting_search',{kind:'report'});const m=h.sent.at(-1).message;assert.equal(m.method,'tools/call');assert.equal(m.params.name,'reporting_search');h.receive({jsonrpc:'2.0',id:m.id,result:{value:'bad'}},'https://wrong.example');assert.equal(h.bridge.pending.size,1);h.receive({jsonrpc:'2.0',id:m.id,result:{value:'bad'}},'https://host.example',{});assert.equal(h.bridge.pending.size,1);h.receive({jsonrpc:'2.0',id:m.id,result:{value:'good'}});assert.deepEqual(await p,{value:'good'});await assert.rejects(h.bridge.call('arbitrary_execute',{}),/forbidden/);h.bridge.close();});

test('pending requests are capped and teardown fences every late reply',async()=>{const h=await connected(),promises=Array.from({length:16},()=>h.bridge.call('reporting_view',{}));const handled=promises.map(p=>p.catch(e=>e.code));await assert.rejects(h.bridge.call('reporting_view',{}),/busy/);h.receive({jsonrpc:'2.0',id:99,method:'ui/resource-teardown'});assert.equal(h.bridge.closed,true);assert.equal(h.bridge.pending.size,0);assert((await Promise.all(handled)).every(c=>c==='unavailable'));h.receive({jsonrpc:'2.0',id:2,result:{}});assert.equal(h.bridge.ready,false);});

test('pagehide closes the transport and unknown methods never invoke tools',async()=>{const h=await connected();h.receive({jsonrpc:'2.0',id:100,method:'unknown'});assert.equal(h.sent.at(-1).message.error.code,-32601);h.listeners.get('pagehide')();assert.equal(h.bridge.closed,true);assert.equal(h.listeners.size,0);});

test('embedded boundary requires exact public registration and one-use challenge',async()=>{const h=harness();assert.throws(()=>new EmbeddedReportAdapter({win:h.win,origin:'*',frame:'f',generation:1,challenge:'a'.repeat(16)}),/invalid_request/);const b=new EmbeddedReportAdapter({win:h.win,origin:'https://host.example',frame:'frame-a',generation:7,challenge:'a'.repeat(16)}),p=b.connect();const m=h.sent[0].message;assert.equal(h.sent[0].origin,'https://host.example');const reply={protocol:'chartworks-report-app-v1',frame:'frame-a',generation:7,id:m.id,result:{challenge:'a'.repeat(16),tools:true}};h.receive({...reply,frame:'frame-b'});h.receive({...reply,generation:6});assert.equal(b.pending.size,1);h.receive(reply);await p;assert.equal(b.challenge,null);assert.equal(b.ready,true);const call=b.call('reporting_runs',{});const handled=call.catch(e=>e.code);assert.equal(h.sent.at(-1).message.generation,7);h.receive({protocol:'chartworks-report-app-v1',frame:'frame-a',generation:7,method:'close'});assert.equal(await handled,'unavailable');assert.equal(b.closed,true);});

test('wrong embedded challenge closes rather than accepting authority hints',async()=>{const h=harness(),b=new EmbeddedReportAdapter({win:h.win,origin:'https://host.example',frame:'frame-a',generation:1,challenge:'a'.repeat(16)}),p=b.connect();h.receive({protocol:'chartworks-report-app-v1',frame:'frame-a',generation:1,id:1,result:{challenge:'b'.repeat(16),tools:true}});await assert.rejects(p,/forbidden/);assert.equal(b.closed,true);});

test('manual chart tools preserve explicit source and target without opening generic authoring',async()=>{
 const h=await connected();
 for(const action of ['block_read','block_mapping','block_copy','block_validate','dataset','prepare_chart','preparation','create_prepared','preparation_control']) {
  const name=`reporting_authoring_${action}_v1`,args={block:'source',revision:3,...(action==='block_copy'?{new_block:'private-copy'}:{})};
  const pending=h.bridge.call(name,args),message=h.sent.at(-1).message;
  assert.deepEqual(message.params,{name,arguments:args});
  h.receive({jsonrpc:'2.0',id:message.id,result:{unchanged:true}});assert.deepEqual(await pending,{unchanged:true});
 }
 const catalog=h.bridge.call('chart_catalog',{}),message=h.sent.at(-1).message;
 assert.deepEqual(message.params,{name:'chart_catalog',arguments:{}});
 h.receive({jsonrpc:'2.0',id:message.id,result:{kinds:[]}});await catalog;
 for(const name of ['reporting_authoring_block_sql_v1','reporting_authoring_block_publish_v1','execute_sql'])await assert.rejects(h.bridge.call(name,{}),/forbidden/);
 h.bridge.close();
});

const allocationVersion='report-app-allocation-v1';
const sourceReference={block:'selected-chart',revision:3,expected_version:7,digest:'a'.repeat(64),output:'amount'};
const allocationInput=(intent='create_report')=>({version:allocationVersion,kind:intent==='create_report'?'report':'block',intent,title:'Quarterly amounts',idempotency_key:'allocation-operation-1',...(intent==='copy_chart'?{source:{...sourceReference}}:{})});
const allocationOutput=(input,id='host-owned-target')=>({version:input.version,kind:input.kind,intent:input.intent,idempotency_key:input.idempotency_key,id});
async function allocationConnected(kind='mcp',initialize={}) {
 const h=harness(),embedded=kind==='embedded',bridge=embedded?new EmbeddedReportAdapter({win:h.win,origin:'https://host.example',frame:'frame-allocation',generation:4,challenge:'challenge-allocation'}):new MCPReportAdapter(h.win),pending=bridge.connect();
 const envelope=message=>embedded?{protocol:'chartworks-report-app-v1',frame:'frame-allocation',generation:4,...message}:{jsonrpc:'2.0',...message};
 assert.equal(bridge.supportsTargetAllocation(),false);
 const result=embedded?{challenge:'challenge-allocation',tools:true,capabilities:{target_allocation:{version:allocationVersion}},...initialize}:{protocolVersion:'2026-01-26',hostCapabilities:{serverTools:{}},hostContext:{'chartworks/target-allocation':{version:allocationVersion}},...initialize};
 h.receive(envelope({id:h.sent[0].message.id,result}));await pending;return {...h,bridge,envelope};
}

test('allocation requires the pinned explicit initialization hint, never serverTools or experimental metadata',async()=>{
 const unsupported=[{},null,true,{version:'unknown'},{version:allocationVersion,authority:true}];
 for(const hint of unsupported)for(const kind of ['mcp','embedded']) {
  const initialize=kind==='mcp'?{hostContext:{'chartworks/target-allocation':hint},hostCapabilities:{serverTools:{},experimental:{'chartworks/target-allocation':{version:allocationVersion}}}}:{capabilities:{target_allocation:hint},context:{'chartworks/target-allocation':{version:allocationVersion}}};
  const h=await allocationConnected(kind,initialize),count=h.sent.length;
  assert.equal(h.bridge.supportsTargetAllocation(),false);
  await assert.rejects(h.bridge.allocateTarget(allocationInput()),e=>e.code==='forbidden'&&!e.unknown);
  assert.equal(h.sent.length,count);h.bridge.close();
 }
 const h=await connected();h.receive({jsonrpc:'2.0',method:'ui/notifications/host-context-changed',params:{'chartworks/target-allocation':{version:allocationVersion}}});
 assert.equal(h.bridge.supportsTargetAllocation(),false);h.bridge.close();
});

test('allocation support never implies provider tool authority',async()=>{
 for(const kind of ['mcp','embedded']) {
  const h=await allocationConnected(kind,kind==='mcp'?{hostCapabilities:{}}:{tools:false});
  assert.equal(h.bridge.supportsTargetAllocation(),true);
  await assert.rejects(h.bridge.call('reporting_authoring_create_v1',{}),/forbidden/);
  const input=allocationInput(),pending=h.bridge.allocateTarget(input),message=h.sent.at(-1).message;
  assert.equal(message.method,'app/allocate-target');h.receive(h.envelope({id:message.id,result:allocationOutput(input)}));await pending;h.bridge.close();
 }
});

test('both transports send all supported intents through the exact host method with frozen copy references',async()=>{
 for(const kind of ['mcp','embedded']) {
  const h=await allocationConnected(kind);
  for(const intent of ['create_report','create_chart','copy_chart']) {
   const input=allocationInput(intent),snapshot=structuredClone(input),pending=h.bridge.allocateTarget(input),message=h.sent.at(-1).message;
   input.title='Changed after send';if(input.source)input.source.revision=4;
   assert.equal(message.method,'app/allocate-target');assert.deepEqual(message.params,snapshot);assert.equal(h.sent.at(-1).origin,'https://host.example');
   const result=allocationOutput(snapshot);h.receive(h.envelope({id:message.id,result}));const returned=await pending;assert.deepEqual(returned,result);
   result.id='Changed after receipt';assert.equal(returned.id,'host-owned-target');
  }
  for(const name of ['app/allocate-target','allocate_target','reporting_authoring_allocate_v1'])await assert.rejects(h.bridge.call(name,{}),/forbidden/);
  h.bridge.close();
 }
});

test('allocation requests reject injected authority, unbounded fields, and invalid intent or source shapes before send',async()=>{
 const h=await allocationConnected(),base=allocationInput(),copy=allocationInput('copy_chart');
 const invalid=[null,[],{}, {...base,version:'v2'},{...base,kind:'block'},{...base,intent:'copy_report'},{...base,intent:'__proto__',kind:{}},{...base,title:''},{...base,title:'  '},{...base,title:' padded '},{...base,title:'x'.repeat(257)},{...base,idempotency_key:''},{...base,idempotency_key:'x'.repeat(129)},{...base,idempotency_key:'https://target.example'},{...base,source:sourceReference},{...copy,source:null},{...copy,source:{...sourceReference,revision:0}},{...copy,source:{...sourceReference,revision:257}},{...copy,source:{...sourceReference,expected_version:0}},{...copy,source:{...sourceReference,expected_version:Number.MAX_SAFE_INTEGER+1}},{...copy,source:{...sourceReference,digest:'A'.repeat(64)}},{...copy,source:{...sourceReference,output:'a/b'}},{...copy,source:{...sourceReference,block:'https://target.example'}}];
 for(const field of ['principal','actor','tenant','scopes','urls','url','token','id','frame','generation']) {
  invalid.push({...base,[field]:'injected'},{...copy,source:{...sourceReference,[field]:'injected'}});
 }
 for(const field of ['version','kind','intent','title','idempotency_key']){const value={...base};delete value[field];invalid.push(value);}
 for(const field of Object.keys(sourceReference)){const source={...sourceReference};delete source[field];invalid.push({...copy,source});}
 const count=h.sent.length;
 for(const value of invalid)await assert.rejects(h.bridge.allocateTarget(value),e=>['invalid_request','limit_exceeded'].includes(e.code)&&!e.unknown);
 assert.equal(h.sent.length,count);assert.equal(h.bridge.pending.size,0);h.bridge.close();
});

test('allocation results enforce closed bounded payloads and exact request correlation',async()=>{
 const h=await allocationConnected(),input=allocationInput(),result=allocationOutput(input);
 const invalid=[null,[],{}, {...result,version:'v2'},{...result,kind:'block'},{...result,intent:'create_chart'},{...result,idempotency_key:'another-operation'},{...result,id:''},{...result,id:'x'.repeat(129)},{...result,id:'https://target.example'},{...result,id:9}];
 for(const field of ['principal','actor','tenant','scopes','url','token','title','source','capabilities'])invalid.push({...result,[field]:'injected'});
 for(const field of Object.keys(result)){const value={...result};delete value[field];invalid.push(value);}
 for(const value of invalid) {
  const pending=h.bridge.allocateTarget(input),message=h.sent.at(-1).message;
  h.receive(h.envelope({id:message.id,result:value}));await assert.rejects(pending,e=>e.code==='unavailable'&&e.unknown===true);assert.equal(h.bridge.pending.size,0);
 }
 const copy=allocationInput('copy_chart'),pending=h.bridge.allocateTarget(copy),message=h.sent.at(-1).message;
 h.receive(h.envelope({id:message.id,result:allocationOutput(copy,copy.source.block)}));await assert.rejects(pending,e=>e.code==='unavailable'&&e.unknown===true);
 h.bridge.close();
});

test('allocation replies retain origin, parent, frame, generation, and request ID fences',async()=>{
 for(const kind of ['mcp','embedded']) {
  const h=await allocationConnected(kind),input=allocationInput(),pending=h.bridge.allocateTarget(input),id=h.sent.at(-1).message.id,reply=h.envelope({id,result:allocationOutput(input)});
  h.receive(reply,'https://other.example');h.receive(reply,'https://host.example',{});h.receive({...reply,id:id+100});
  if(kind==='embedded'){h.receive({...reply,frame:'other-frame'});h.receive({...reply,generation:3});}
  assert.equal(h.bridge.pending.size,1);h.receive(reply);assert.deepEqual(await pending,allocationOutput(input));
  const next=h.bridge.allocateTarget({...input,idempotency_key:'next-operation'}),nextID=h.sent.at(-1).message.id;
  h.receive(reply);assert.equal(h.bridge.pending.size,1);
  h.receive(h.envelope({id:nextID,result:allocationOutput({...input,idempotency_key:'next-operation'},'next-target')}));assert.equal((await next).id,'next-target');h.bridge.close();
 }
});

test('allocation timeout remains unknown and only explicit same-key retry sends another message',async t=>{
 t.mock.timers.enable({apis:['setTimeout']});
 const h=await allocationConnected(),input=allocationInput('copy_chart'),pending=h.bridge.allocateTarget(input),first=h.sent.at(-1).message,count=h.sent.length;
 const rejection=assert.rejects(pending,e=>e.code==='cancelled_or_timed_out'&&e.unknown===true);
 t.mock.timers.tick(65001);await rejection;assert.equal(h.bridge.pending.size,0);assert.equal(h.sent.length,count);
 h.receive(h.envelope({id:first.id,result:allocationOutput(input)}));assert.equal(h.bridge.pending.size,0);assert.equal(h.sent.length,count);
 const retry=h.bridge.allocateTarget(input),second=h.sent.at(-1).message;
 assert.notEqual(second.id,first.id);assert.deepEqual(second.params,first.params);
 h.receive(h.envelope({id:first.id,result:allocationOutput(input,'late-target')}));assert.equal(h.bridge.pending.size,1);
 h.receive(h.envelope({id:second.id,result:allocationOutput(input)}));assert.equal((await retry).id,'host-owned-target');h.bridge.close();
});

test('allocation host errors and teardown leave unknown outcomes and clear availability',async()=>{
 for(const kind of ['mcp','embedded']) {
  const h=await allocationConnected(kind),input=allocationInput(),failed=h.bridge.allocateTarget(input),id=h.sent.at(-1).message.id;
  h.receive(h.envelope({id,error:{code:-32603,message:'opaque failure'}}));await assert.rejects(failed,e=>e.code==='unavailable'&&e.unknown===true);
  const pending=h.bridge.allocateTarget(input),lateID=h.sent.at(-1).message.id,count=h.sent.length;
  const rejected=assert.rejects(pending,e=>e.code==='unavailable'&&e.unknown===true);h.listeners.get('pagehide')();await rejected;
  assert.equal(h.bridge.supportsTargetAllocation(),false);assert.equal(h.bridge.targetAllocation,false);assert.equal(h.bridge.pending.size,0);
  h.receive(h.envelope({id:lateID,result:allocationOutput(input)}));await assert.rejects(h.bridge.allocateTarget(input),/forbidden/);assert.equal(h.sent.length,count);
 }
});

test('teardown between receiving and adopting allocation still fences the result',async()=>{
 const h=await allocationConnected(),input=allocationInput(),pending=h.bridge.allocateTarget(input),id=h.sent.at(-1).message.id;
 h.receive(h.envelope({id,result:allocationOutput(input)}));h.bridge.close();await assert.rejects(pending,e=>e.code==='unavailable'&&e.unknown===true);
});

test('an allocation transport exception stays unknown and never triggers automatic resend',async()=>{
 const h=await allocationConnected(),count=h.sent.length;
 h.parent.postMessage=()=>{throw new Error('transport exception');};
 await assert.rejects(h.bridge.allocateTarget(allocationInput()),e=>e.code==='unavailable'&&e.unknown===true);
 assert.equal(h.bridge.pending.size,0);assert.equal(h.sent.length,count);h.bridge.close();
});
