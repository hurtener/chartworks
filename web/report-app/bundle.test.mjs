// Executes the committed production bundle, not imports of the development
// modules. This small VM DOM proves compilation/bridge/renderer behavior only;
// the separate Chromium journeys remain required for actual browser evidence.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

const script = await readFile(new URL('./generated/report-app.js', import.meta.url), 'utf8');
const wrapped = result => ({structuredContent: {result}});
const flush = () => new Promise(resolve => setImmediate(resolve));
const hostOrigin = 'https://report-host.example';
const tools = ['reporting_authoring_capabilities_v1', 'reporting_search', 'reporting_describe', 'reporting_view', 'reporting_runs'];
const toolHint = {version: 'chartworks-host-tools-v1', names: tools};

class Element {
  constructor(tag) {
    this.tagName = tag.toUpperCase();
    this.children = [];
    this.attributes = {};
    this.listeners = new Map();
    this.dataset = {};
    this.style = {setProperty(name, value) { this[name] = value; }};
    this._text = '';
    this.disabled = false;
  }
  set textContent(value) { this.children = []; this._text = String(value); }
  get textContent() { return this._text + this.children.map(child => child.textContent || '').join(''); }
  set innerHTML(_) { assert.fail('The compiled renderer must never parse HTML'); }
  set outerHTML(_) { assert.fail('The compiled renderer must never parse HTML'); }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this._text = ''; this.children = children; }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  getAttribute(name) { return this.attributes[name] ?? null; }
  addEventListener(name, listener) { this.listeners.set(name, listener); }
  querySelectorAll(tag) {
    return this.children.flatMap(child => [...(child.tagName === tag.toUpperCase() ? [child] : []), ...(child.querySelectorAll?.(tag) || [])]);
  }
  getBoundingClientRect() { return {top: 0, bottom: 800, width: 1000, height: 800}; }
}

function harness({embedded = false, topLevel = false} = {}) {
  const root = new Element('main'), listeners = new Map(), sent = [], timers = new Map();
  const parent = {postMessage(message, origin) { sent.push({message: JSON.parse(JSON.stringify(message)), origin}); }};
  const win = {
    parent,
    addEventListener(name, listener) {
      if (!listeners.has(name)) listeners.set(name, new Set());
      listeners.get(name).add(listener);
    },
    removeEventListener(name, listener) { listeners.get(name)?.delete(listener); },
  };
  if (topLevel) win.parent = win;
  const document = {
    getElementById(id) { assert.equal(id, 'report-app'); return root; },
    createElement: tag => new Element(tag),
    createElementNS: (_, tag) => new Element(tag),
    documentElement: new Element('html'),
  };
  let timerID = 0;
  const context = vm.createContext({
    window: win, document, URL, TextEncoder, console,
    CSS: {supports: (property, value) => property === 'color' && /^#[0-9a-f]{6}$/.test(value)},
    setTimeout(callback, delay) { const id = ++timerID; timers.set(id, {callback, delay}); return id; },
    clearTimeout(id) { timers.delete(id); },
  }, {codeGeneration: {strings: false, wasm: false}});
  const prefix = embedded ? `const REPORT_APP_EMBEDDED_PARENTS=${JSON.stringify([hostOrigin])};\n` : '';
  new vm.Script(prefix + script, {filename: 'generated/report-app.js'}).runInContext(context, {timeout: 2000});
  // postMessage structured clone delivers objects in the receiving realm. Make
  // those objects there, preserving the real boundedJSON prototype checks.
  const cloneIntoFrame = value => vm.runInContext(`JSON.parse(${JSON.stringify(JSON.stringify(value))})`, context);
  const emit = (data, {source = parent, origin = hostOrigin} = {}) => {
    for (const listener of [...listeners.get('message') || []]) listener({source, origin, data: cloneIntoFrame(data)});
  };
  const close = () => { for (const listener of [...listeners.get('pagehide') || []]) listener(); };
  const reply = (request, result, options) => emit({...request.message, method: undefined, params: undefined, result}, options);
  return {root, document, sent, timers, listeners, emit, close, reply};
}

async function initializeMCP(frame, names = toolHint) {
  const request = frame.sent.find(item => item.message.method === 'ui/initialize');
  assert(request, 'The actual bundle must initialize MCP');
  frame.reply(request, {
    protocolVersion: '2026-01-26', hostCapabilities: {serverTools: {}},
    hostContext: {theme: 'dark', locale: 'en-US', 'chartworks/supported-tools': names},
  });
  await flush();
}

async function finishCatalog(frame) {
  const capabilities = frame.sent.find(item => item.message.params?.name === 'reporting_authoring_capabilities_v1');
  assert(capabilities, 'Capability read missing from compiled app');
  frame.reply(capabilities, wrapped({version: 'report-authoring-v1', consumer: true, builder: false}));
  await flush();
  const catalog = frame.sent.find(item => item.message.params?.name === 'reporting_search');
  assert(catalog, 'Catalog read missing from compiled app');
  frame.reply(catalog, wrapped({version: 'reporting-view-v1', items: [{target: {kind: 'report', id: 'report-a', revision: 4}, title: '<script>inert report title</script>'}], next: ''}));
  await flush();
  assert(frame.root.textContent.includes('Choose a published report'));
  assert(frame.root.textContent.includes('<script>inert report title</script>'));
  assert.equal(frame.root.querySelectorAll('script').length, 0);
}

test('production IIFE evaluates without module imports, globals, or runtime code generation', () => {
  const context = vm.createContext({}, {codeGeneration: {strings: false, wasm: false}});
  new vm.Script(script).runInContext(context, {timeout: 2000});
  assert.deepEqual(Object.keys(context), []);
  const frame = harness({topLevel: true});
  assert.equal(frame.root.textContent, 'Open this report app through an authorized host.');
  assert.equal(frame.sent.length, 0);
  assert.equal(frame.timers.size, 0);
});

test('compiled MCP initializes, narrows tools, and rejects spoofed responses', async () => {
  const frame = harness();
  try {
    const request = frame.sent[0];
    assert.equal(request.message.method, 'ui/initialize');
    assert.equal(request.origin, '*');
    frame.reply(request, {protocolVersion: '2026-01-26'}, {source: {}});
    await flush();
    assert.equal(frame.sent.length, 1);
    await initializeMCP(frame);
    assert.equal(frame.document.documentElement.dataset.theme, 'dark');
    assert(frame.sent.some(item => item.message.method === 'ui/notifications/initialized'));
    await finishCatalog(frame);
    assert(frame.sent.slice(1).every(item => item.origin === hostOrigin));
    assert.deepEqual(frame.sent.filter(item => item.message.method === 'tools/call').map(item => item.message.params.name), ['reporting_authoring_capabilities_v1', 'reporting_search']);
    assert(!frame.root.querySelectorAll('button').some(button => button.textContent === 'New report' || button.textContent === 'Build'));
  } finally { frame.close(); }
  await flush();
  assert.equal(frame.timers.size, 0);
  assert(frame.root.textContent.includes('closed'));
});

test('compiled source preserves fail-closed empty host inventory and delayed teardown fences', async () => {
  const denied = harness();
  await initializeMCP(denied, {version: 'chartworks-host-tools-v1', names: []});
  assert.equal(denied.sent.filter(item => item.message.method === 'tools/call').length, 0);
  assert(denied.root.textContent.includes('No report actions are available'));
  denied.close();

  const delayed = harness();
  await initializeMCP(delayed);
  const pending = delayed.sent.find(item => item.message.params?.name === 'reporting_authoring_capabilities_v1');
  delayed.close();
  delayed.reply(pending, wrapped({version: 'report-authoring-v1', consumer: true, builder: true}));
  await flush();
  assert(delayed.root.textContent.includes('closed'));
  assert.equal(delayed.sent.filter(item => item.message.method === 'tools/call').length, 1);
  assert.equal(delayed.timers.size, 0);
});

test('literal embedded prefix selects registered bootstrap with no MCP fallback', async () => {
  const frame = harness({embedded: true});
  const bootstrap = {protocol: 'chartworks-report-app-v1', method: 'bootstrap', frame: 'frame-a', generation: 2, params: {challenge: 'synthetic-challenge-001'}};
  try {
    assert(frame.root.textContent.includes('Waiting for the registered embedded host'));
    assert.equal(frame.sent.length, 0);
    frame.emit(bootstrap, {origin: 'https://wrong.example'});
    frame.emit(bootstrap, {source: {}});
    assert.equal(frame.sent.length, 0);
    frame.emit(bootstrap);
    assert.equal(frame.sent.length, 1);
    const request = frame.sent[0];
    assert.equal(request.message.method, 'initialize');
    assert.equal(request.message.protocol, 'chartworks-report-app-v1');
    assert.equal(request.message.frame, 'frame-a');
    assert.equal(request.message.generation, 2);
    assert.equal(request.origin, hostOrigin);
    frame.reply(request, {challenge: bootstrap.params.challenge, tools: true, capabilities: {supported_tools: toolHint}, context: {theme: 'dark'}});
    await flush();
    await finishCatalog(frame);
    assert(frame.sent.every(item => item.message.method !== 'ui/initialize' && item.message.jsonrpc === undefined));
  } finally { frame.close(); }
  await flush();
  assert.equal(frame.timers.size, 0);
  assert.equal([...frame.listeners.values()].reduce((sum, set) => sum + set.size, 0), 0);
});

for (const embedded of [false, true]) test(`compiled ${embedded ? 'embedded' : 'MCP'} context repaints without tool calls or content replacement`, async () => {
  const frame = harness({embedded});
  const bootstrap = {protocol: 'chartworks-report-app-v1', frame: 'theme-frame', generation: 1, method: 'bootstrap', params: {challenge: 'synthetic-theme-challenge'}};
  try {
    if (embedded) {
      frame.emit(bootstrap);
      frame.reply(frame.sent[0], {challenge: bootstrap.params.challenge, tools: true, capabilities: {supported_tools: toolHint}, context: {}});
      await flush();
    } else await initializeMCP(frame);
    await finishCatalog(frame);
    const content = frame.root.textContent, calls = frame.sent.filter(item => item.message.method === 'tools/call').length;
    const patch = {theme: 'dark', styles: {variables: {'--color-background-primary': '#123456'}}};
    const message = embedded ? {...bootstrap, method: 'context', params: patch} : {jsonrpc: '2.0', method: 'ui/notifications/host-context-changed', params: patch};
    frame.emit(message, {source: {}});
    assert.notEqual(frame.document.documentElement.style['--app-paper'], '#123456');
    frame.emit(message); await flush();
    assert.equal(frame.document.documentElement.style['--app-paper'], '#123456');
    assert.equal(frame.root.textContent, content);
    assert.equal(frame.sent.filter(item => item.message.method === 'tools/call').length, calls);
  } finally { frame.close(); }
});

test('compiled retained presenter displays exact values through the bounded bridge only', async () => {
  const frame = harness();
  try {
    await initializeMCP(frame);
    await finishCatalog(frame);
    frame.emit({jsonrpc: '2.0', method: 'ui/notifications/tool-result', params: wrapped({version: 'reporting-view-v1', summary: {kind: 'report', run: 'run-a', private: true}})});
    await flush();
    const read = frame.sent.find(item => item.message.params?.name === 'reporting_view');
    assert(read);
    const root = {
      version: 'reporting-view-v1',
      summary: {kind: 'report', run: 'run-a', target: {kind: 'report', id: 'report-a', revision: 4}, state: 'completed', private: true, expires_at: '2099-01-01T00:00:00Z'},
      selection: {kind: 'report', run: 'run-a', page: 'main', widget: 'amount', output: 'table', offset: 0, limit: 100},
      pages: [{id: 'main', report: 'report-a', revision: 4, widgets: [{id: 'amount', kind: 'block', state: 'completed', grid: {row: 0, column: 0, width: 12, height: 1}, presentation: {title: 'Exact amount'}, outputs: ['table']}]}],
      locale: 'en-US', timezone: 'UTC', outputs: [{id: 'table', enabled: true, selected: true}], filters: [], redacted: false,
      output: {id: 'table', kind: 'table', state: 'succeeded', retained_digest: 'synthetic-digest', table: {columns: [{id: 'amount', name: '<b>Amount</b>', type: 'decimal'}], rows: [[{null: false, value: '9007199254740993.01'}]], totals: [], warnings: []}},
      page_bounds: {offset: 0, limit: 100, total: 1},
    };
    frame.reply(read, wrapped(root));
    await flush();
    assert(frame.root.textContent.includes('9007199254740993.01'), frame.root.textContent);
    assert(frame.root.textContent.includes('<b>Amount</b>'));
    assert.equal(frame.root.querySelectorAll('b').length, 0);
    assert.deepEqual(frame.sent.filter(item => item.message.method === 'tools/call').map(item => item.message.params.name), ['reporting_authoring_capabilities_v1', 'reporting_search', 'reporting_view']);
  } finally { frame.close(); }
  await flush();
  assert.equal(frame.timers.size, 0);
});

for(const embedded of [false,true])test(`actual ${embedded?'embedded':'MCP'} filter controls stage civil dates and preserve block fallback defaults on Save`, async () => {
  const frame=harness({embedded}),names={version:'chartworks-host-tools-v1',names:[...tools,'reporting_authoring_drafts_v1','reporting_authoring_read_v1','reporting_authoring_save_v1','reporting_authoring_block_read_v1']};
  const capability={version:'report-authoring-v1',consumer:false,builder:true,can_open:true,can_save:true};
  const findCall=name=>frame.sent.filter(v=>v.message.params?.name===name).at(-1);
  const click=label=>{const b=frame.root.querySelectorAll('button').find(e=>e.textContent===label);assert(b,label);assert.equal(b.disabled,false,label);b.listeners.get('click')();};
  try {
    if(embedded){const bootstrap={protocol:'chartworks-report-app-v1',method:'bootstrap',frame:'filter-frame',generation:1,params:{challenge:'synthetic-filter-challenge'}};frame.emit(bootstrap);frame.reply(frame.sent[0],{challenge:bootstrap.params.challenge,tools:true,capabilities:{supported_tools:names},context:{locale:'en-US'}});await flush();}else await initializeMCP(frame,names);
    frame.reply(findCall('reporting_authoring_capabilities_v1'),wrapped(capability));await flush();
    frame.reply(findCall('reporting_authoring_drafts_v1'),wrapped({items:[{id:'report-a',revision:1,version:1,metadata:[{locale:'en-US',title:'Filter report'}]}],next:''}));await flush();click('Filter report');await flush();
    frame.reply(findCall('reporting_authoring_capabilities_v1'),wrapped(capability));
    const definition={schema_version:3,metadata:[{locale:'en-US',title:'Filter report'}],locale:'en-US',timezone:'UTC',partial_failure:'fail_closed',report_pages:[{id:'main',title:'Summary',widgets:[{id:'chart',kind:'block',grid:{column:0,row:0,width:12,height:3},presentation:{title:'Daily total'},block:{block:'block-a',revision:1,policy:'published',outputs:['chart'],narrative:false},bindings:[{filter:'days',parameter:'period'}]}],filters:[{label:'Days',parameter:{name:'days',type:'date_range',required:true,dimension:{topic:'topic',version:'v1',dimension:'day'},default:{date_range:{start:'2026-03-08',end_exclusive:'2026-03-09'}}}}],defaults:[{name:'period',value:{date_range:{start:'2026-01-01',end_exclusive:'2026-01-02'}}}]}]};
    frame.reply(findCall('reporting_authoring_read_v1'),wrapped({state:{id:'report-a',version:1,draft_revision:1},revision:1,digest:'a'.repeat(64),private:true,definition}));await flush();click('Change saved default');
    const end=frame.root.querySelectorAll('input').find(e=>e.getAttribute('aria-label')==='End date · inclusive');assert(end);const before=frame.sent.length;end.value='2026-03-10';end.listeners.get('input')();assert.equal(frame.sent.length,before);click('Done');assert.equal(frame.sent.length,before);click('Save report');await flush();
    const save=findCall('reporting_authoring_save_v1');assert(save);const page=save.message.params.arguments.definition.report_pages[0];assert.deepEqual(page.filters[0].parameter.default,{date_range:{start:'2026-03-08',end_exclusive:'2026-03-11'}});assert.deepEqual(page.defaults,definition.report_pages[0].defaults);assert.equal(save.message.params.arguments.expected_version,1);assert.equal(frame.sent.filter(v=>/options|preview|execute|reporting_run$/.test(v.message.params?.name||'')).length,0);
  } finally {frame.close();}
  await flush();assert.equal(frame.timers.size,0);
});

import {publicationFixture} from './publication-fixture.mjs';
for(const embedded of [false,true])test(`actual ${embedded?'embedded':'MCP'} bundle requires separate confirmations through chart publish, rebind, review and publication`,async()=>{
 const frame=harness({embedded}),native=publicationFixture(),names={version:'chartworks-host-tools-v1',names:[...tools,...['drafts','read','save','block_read','lifecycle','block_publish','rebind_published','report_transition'].map(n=>'reporting_authoring_'+n+'_v1')]},handled=new Set();
 const drain=async()=>{for(let i=0;i<60;i++){await flush();const pending=frame.sent.filter(v=>v.message.params?.name&&!handled.has(v));if(!pending.length){await flush();if(!frame.sent.some(v=>v.message.params?.name&&!handled.has(v)))return;continue;}for(const call of pending){handled.add(call);try{frame.reply(call,wrapped(await native.invoke(call.message.params.name,call.message.params.arguments)));}catch(e){frame.reply(call,{isError:true,structuredContent:{error:{code:e.code||'unavailable'}}});}}}throw Error('compiled lifecycle did not settle');};
 const button=title=>{const b=frame.root.querySelectorAll('button').find(n=>n.textContent===title);assert(b,title);return b;};
 const click=async title=>{const b=button(title);assert.equal(b.disabled,false,title);b.listeners.get('click')();await drain();};
 const confirm=label=>{const e=frame.root.querySelectorAll('input').find(n=>n.getAttribute('aria-label')===label);assert(e,label);assert.equal(e.disabled,false,label);e.checked=true;e.listeners.get('change')();};
 try{
  if(embedded){const bootstrap={protocol:'chartworks-report-app-v1',method:'bootstrap',frame:'publication-frame',generation:1,params:{challenge:'synthetic-publication-challenge'}};frame.emit(bootstrap);frame.reply(frame.sent[0],{challenge:bootstrap.params.challenge,tools:true,capabilities:{supported_tools:names},context:{locale:'en-US'}});}else await initializeMCP(frame,names);
  await drain();await click('Build');await click('Lifecycle report');
  const panel=frame.root.querySelectorAll('details').find(n=>n.getAttribute('aria-label')==='Publication and review');assert.equal(panel.open,false);panel.open=true;panel.listeners.get('toggle')();await click('Inspect publication status');
  assert.equal(button('Publish entire chart revision').disabled,true);assert(frame.root.textContent.includes('other · table'));
  confirm('I confirm publishing the entire chart revision and all listed outputs.');await click('Publish entire chart revision');assert.equal(native.blocks.get('chart-a:2').block.private,false);assert.equal(native.report().definition.report_pages[0].widgets[0].block.policy,'private_preview');
  confirm('Summary / first · first · chart-a revision 2');confirm('Summary / second · second · chart-a revision 2');assert.equal(button('Rebind selected widgets').disabled,true);confirm('I confirm rebinding only the selected widgets to their exact published chart revisions.');await click('Rebind selected widgets');assert(native.report().definition.report_pages[0].widgets.every(w=>w.block.policy==='published'));assert.equal(native.state().published_revision,0);
  assert.equal(button('Submit report for review').disabled,true);confirm('I confirm submitting this exact report revision for review.');await click('Submit report for review');assert.equal(native.state().draft_revision,0);assert.equal(native.state().review_revision,2);assert(frame.root.textContent.includes('Pending review'));
  assert.equal(button('Publish reviewed report').disabled,true);confirm('I confirm publishing this exact reviewed report revision.');await click('Publish reviewed report');assert.equal(native.state().published_revision,2);assert.equal(native.state().review_revision,0);assert.equal(native.report(2).private,false);
  await click('Browse');assert(frame.root.textContent.includes('Lifecycle report'));const mutations=native.calls.filter(c=>['reporting_authoring_block_publish_v1','reporting_authoring_rebind_published_v1','reporting_authoring_report_transition_v1'].includes(c.name));assert.equal(mutations.length,4);assert.deepEqual(mutations.map(c=>c.args.operation||c.name),['reporting_authoring_block_publish_v1','reporting_authoring_rebind_published_v1','review','publish']);assert(!native.calls.some(c=>/prepare|validate|execute|options|^reporting_run$/.test(c.name)));
 }finally{frame.close();}await flush();assert.equal(frame.timers.size,0);
});

import {mapView,mapSaved,mapCatalog,mapClone} from './mapping-fixture.mjs';
for(const embedded of [false,true])test(`compiled ${embedded?'embedded':'MCP'} manual catalog, page settings and native display options roundtrip`,async()=>{
 const frame=harness({embedded}),calls=[],handled=new Set();let state={id:'private-report',version:5,draft_revision:3},block=mapView('private-chart',7,true),definition={schema_version:3,metadata:[{locale:'en-US',title:'Editable report'}],locale:'en-US',timezone:'UTC',partial_failure:'fail_closed',report_pages:[{id:'main',title:'Summary',widgets:[{id:'metric',kind:'block',grid:{column:0,row:0,width:12,height:3},presentation:{title:'Revenue'},block:{block:'private-chart',revision:7,digest:'a'.repeat(64),outputs:['amount'],policy:'private_preview',narrative:false}}]},{id:'other',title:'Details',locale:'',timezone:'',widgets:[]}]};
 const names={version:'chartworks-host-tools-v1',names:[...tools,'chart_catalog',...['drafts','read','save','block_read','block_copy','block_mapping'].map(action=>'reporting_authoring_'+action+'_v1')]};
 const invoke=(name,args)=>{
  calls.push({name,args});if(name==='reporting_authoring_capabilities_v1')return {version:'report-authoring-v1',builder:true,consumer:true,can_open:true,can_save:true};
  if(name==='reporting_search')return {version:'reporting-view-v1',items:args.query==='Revenue'&&!args.after?[]:[{target:{kind:args.kind,id:args.kind==='report'?'published-report':'approved-block',revision:2},title:'Revenue overview',description:'Synthetic approved metadata preview'}],next:args.query==='Revenue'&&!args.after?'scan-next':''};
  if(name==='reporting_authoring_drafts_v1')return {items:[{id:state.id,metadata:definition.metadata,revision:state.draft_revision,version:state.version}],next:''};
  if(name==='reporting_authoring_read_v1')return {state,revision:state.draft_revision,private:true,definition};
  if(name==='reporting_authoring_save_v1'){assert.equal(args.expected_version,state.version);assert.equal(args.revision,state.draft_revision);definition=mapClone(args.definition);state={...state,version:state.version+1,draft_revision:state.draft_revision+1};return state;}
  if(name==='reporting_authoring_block_read_v1')return block;
  if(name==='chart_catalog')return mapClone(mapCatalog);
  if(name==='reporting_authoring_block_mapping_v1'){assert.equal(args.expected_version,block.block.state.version);assert.equal(args.revision,block.block.revision);assert.equal(args.digest,block.block.digest);block=mapSaved(args,false);return block;}
  throw Error('Unexpected compiled call '+name);
 };
 const drain=async()=>{for(let round=0;round<40;round++){await flush();const pending=frame.sent.filter(v=>v.message.params?.name&&!handled.has(v));if(!pending.length)return;for(const call of pending){handled.add(call);frame.reply(call,wrapped(invoke(call.message.params.name,call.message.params.arguments)));}}throw Error('Compiled authoring did not settle');};
 const elements=tag=>frame.root.querySelectorAll(tag),button=label=>{const e=elements('button').find(e=>e.textContent===label);assert(e,label);return e;},click=async label=>{const e=button(label);assert.equal(e.disabled,false,label);e.listeners.get('click')();await drain();},field=(tag,label)=>{const e=elements(tag).find(e=>e.getAttribute('aria-label')===label);assert(e,label);return e;},change=(tag,label,value,event='change')=>{const e=field(tag,label);assert.equal(e.disabled,false,label);e.value=value;e.listeners.get(event)();};
 try{
  if(embedded){frame.emit({protocol:'chartworks-report-app-v1',method:'bootstrap',frame:'authoring-frame',generation:1,params:{challenge:'synthetic-authoring-challenge'}});frame.reply(frame.sent[0],{challenge:'synthetic-authoring-challenge',tools:true,capabilities:{supported_tools:names},context:{locale:'en-US'}});}else await initializeMCP(frame,names);
  await drain();const before=calls.length;change('input','Find reports','Revenue','input');assert.equal(calls.length,before);await click('Search reports');assert.equal(calls.at(-1).args.query,'Revenue');assert(frame.root.textContent.includes('More reports may contain matches'));await click('More reports');assert.equal(calls.at(-1).args.after,'scan-next');assert(frame.root.textContent.includes('Synthetic approved metadata preview'));await click('Clear reports search');assert.equal(calls.at(-1).args.query,'');
  await click('Build');await click('Editable report');const sibling=mapClone(definition.report_pages[1]),beforeSettings=calls.length;change('select','Page locale','es-AR');change('select','Page timezone','America/Argentina/Buenos_Aires');assert.equal(calls.length,beforeSettings);await click('Save report');assert.equal(definition.report_pages[0].locale,'es-AR');assert.equal(definition.report_pages[0].timezone,'America/Argentina/Buenos_Aires');assert.deepEqual(definition.report_pages[1],sibling);await click('Reload latest');assert.equal(field('select','Page locale').children.find(o=>o.selected).value,'es-AR');change('select','Page locale','');await click('Save report');assert(!Object.hasOwn(definition.report_pages[0],'locale'));assert.equal(definition.locale,'en-US');
  change('input','Find blocks','Revenue','input');const beforeSearch=calls.length;assert.equal(calls.length,beforeSearch);await click('Search blocks');assert(frame.root.textContent.includes('More blocks may contain matches'));await click('More blocks');assert.equal(calls.at(-1).args.kind,'block');assert.equal(calls.at(-1).args.after,'scan-next');
  await click('Selected');await click('Edit chart');const beforeDisplay=calls.length;change('input','Chart title','Reviewed total');change('select','Legend position','right');change('input','Maximum displayed label characters','24');assert.equal(calls.length,beforeDisplay);assert(frame.root.textContent.includes('Use Field formatting separately'));await click('Save chart');const saved=calls.find(c=>c.name==='reporting_authoring_block_mapping_v1');assert.equal(saved.args.mapping.options.title,'Reviewed total');assert.equal(saved.args.mapping.options.legend.position,'right');assert.equal(saved.args.mapping.options.label_max_runes,24);assert(!Object.hasOwn(saved.args.mapping,'columns'));assert.equal(block.block.validation,undefined);await click('Save report');await click('Edit chart');assert.equal(field('input','Chart title').value,'Reviewed total');assert.equal(field('select','Legend position').children.find(o=>o.selected).value,'right');assert.equal(Number(field('input','Maximum displayed label characters').value),24);await click('Cancel chart edits');assert(!calls.some(c=>/validate|prepare|execute|^reporting_run$/.test(c.name)));
 }finally{frame.close();}await flush();assert.equal(frame.timers.size,0);assert(frame.root.textContent.includes('closed'));
});

test('production graph shares controls and error spellings while retaining one canonical presenter', async () => {
  const manifest = JSON.parse(await readFile(new URL('./generated/manifest.json', import.meta.url), 'utf8'));
  for (const path of ['web/report-app/dom.js', 'web/report-app/error-codes.js', 'web/report-viewer/presentation.js']) {
    const inputs = manifest.inputs.filter(input => input.path === path);
    assert.equal(inputs.length, 1, path);
    assert(inputs[0].bytesInOutput > 0, path);
  }
  for (const name of ['app', 'dataset-editor', 'mapping-editor', 'filter-controls', 'report-filters', 'publication-controls']) {
    const input = manifest.inputs.find(input => input.path === `web/report-app/${name}.js`);
    assert(input.imports.includes('web/report-app/dom.js'), name);
  }
  assert(!manifest.inputs.some(input => input.path === 'web/report-viewer/app.js'));
  assert.equal(Buffer.byteLength(script), manifest.output.bytes);
});
