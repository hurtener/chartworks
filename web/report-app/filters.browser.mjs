// Exact production HTML in hosted Chromium against native PostgreSQL DTO recordings.
// CLI: node filters.browser.mjs <mcp.html|embedded.html> <proof.png> <mcp|embedded>
// The browser replays synthetic native DTOs; it does not connect to PostgreSQL or Pengui.
import assert from 'node:assert/strict';
import {readFile, writeFile, mkdtemp, rm} from 'node:fs/promises';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {once} from 'node:events';
import {cleanupBrowserFixture} from './browser_cleanup.mjs';
import {initializePublicationHost, publicationFrameFits} from './publication-browser-fixture.mjs';
import {governedFilterBrowserFixture, filterBrowserTools} from './filters-browser-fixture.mjs';

assert.equal(process.env.GITHUB_ACTIONS, 'true', 'Governed-filter Chromium proof runs only in the hosted CI job');
const [htmlPath, screenshotPath, mode] = process.argv.slice(2);
assert(['mcp', 'embedded'].includes(mode), 'Explicit adapter required; no fallback');
assert(htmlPath && screenshotPath?.endsWith('.png'), 'Provide actual compiled HTML and a PNG proof path');
const html = await readFile(htmlPath, 'utf8'), fixture = await governedFilterBrowserFixture();
assert(html.startsWith('<!doctype html>') && html.includes('Content-Security-Policy'), 'Exact production resource HTML required');
assert.equal(html.includes('const REPORT_APP_EMBEDDED_PARENTS='), mode === 'embedded', 'HTML must match its explicit transport');
const names = {version: 'chartworks-host-tools-v1', names: filterBrowserTools};
const host = `<!doctype html><meta charset="utf-8"><title>Native DTO-backed filter proof</title><style>body{margin:0}#fixture-label{padding:6px 16px;background:#20372d;color:white;font:12px system-ui,sans-serif}iframe{border:0;width:100%;height:1500px}</style><div id="fixture-label">Synthetic host · Native PostgreSQL DTO recordings · No live provider connection</div><iframe id="app" title="Chartworks report app" src="/resource"></iframe><script>(${initializePublicationHost.toString()})(${mode === 'embedded'},${JSON.stringify(names).replaceAll('<', '\\u003c')});</script>`;
const server = createServer((req, res) => {
  const content = req.url === '/' ? host : req.url === '/resource' ? html : '';
  res.writeHead(content ? 200 : 404, {'Content-Type': 'text/html', 'Cache-Control': 'no-store'});res.end(content);
});
server.listen(0, '127.0.0.1');await once(server, 'listening');
const origin = mode === 'embedded' ? 'https://report-host.example' : `http://127.0.0.1:${server.address().port}`;
const directory = await mkdtemp(join(tmpdir(), 'chartworks-filter-browser-'));
const browser = spawn(process.env.CHARTWORKS_CHROME_BIN || 'chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--disable-background-networking', '--disable-component-update', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', '--user-data-dir=' + directory, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
const closed = new Promise(resolve => browser.once('close', resolve)), pending = new Map(), contexts = new Map();
const errors = [], requests = [], assertions = [], writes = () => fixture.calls.filter(call => ['reporting_authoring_save_v1'].includes(call.name));
const snapshot = () => fixture.snapshot();
const original = snapshot();

let socket, sequence = 0, browserError, browserStderr = '', frameID, stopping = false, runError, watchdog, lastPointer;
browser.on('error', error => browserError = error);
browser.stderr.on('data', data => { if (browserStderr.length < 16384) browserStderr += data.toString(); });
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
function rpc(method, params = {}) {
  if (stopping && method !== 'Browser.close') return Promise.reject(new Error('Governed filter fixture is closing'));
  const id = ++sequence;
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {pending.delete(id);reject(new Error('CDP timeout: ' + method));}, 10000);
    pending.set(id, {resolve, reject, timer});
    try { socket.send(JSON.stringify({id, method, params})); } catch (error) {clearTimeout(timer);pending.delete(id);reject(error);}
  });
}
async function evaluate(expression, contextId) {
  const result = await rpc('Runtime.evaluate', {expression, ...(contextId === undefined ? {} : {contextId}), awaitPromise: true, returnByValue: true});
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
  return result.result?.value;
}
async function until(fn, message) {
  const end = Date.now() + 15000;
  while (Date.now() < end) {
    if (stopping) throw new Error('Governed filter fixture is closing');
    if (browserError) throw browserError;
    if (browser.exitCode !== null || browser.signalCode !== null) throw new Error('Chromium exited: ' + browserStderr);
    if (errors.length) throw new Error(errors.join('\n'));
    if (await fn()) return;
    await pause(25);
  }
  throw new Error(message);
}
const root = "document.getElementById('app')?.contentDocument?.getElementById('report-app')";
const editor = `${root}.querySelector('.filter-value-editor')`;
const field = label => `Array.from(${root}.querySelectorAll('input')).find(e=>e.getAttribute('aria-label')===${JSON.stringify(label)})`;

const button = label => `Array.from(${root}.querySelectorAll('button')).find(e=>e.textContent===${JSON.stringify(label)})`;
async function check(expression, message) {assert.equal(await evaluate(expression), true, message);assertions.push(message);}
function equal(actual, expected, message) {assert.deepEqual(actual, expected, message);assertions.push(message);}
async function ready() {await until(() => evaluate(`!!${root}&&!Array.from(${root}.querySelectorAll('[role=status]')).some(e=>e.textContent==='Working…')`), 'Filter operation did not settle');}
async function textHas(text) {await until(() => evaluate(`${root}?.textContent.includes(${JSON.stringify(text)})`), 'Missing text: ' + text);await ready();}
// Pointer readiness contract. Tests execute this exact pure function without
// importing the hosted-only runner or starting a browser.
function filterPointerReady(current, previous) {
  if(!current||!previous||!Array.isArray(current.geometry)||!Array.isArray(previous.geometry)||!current.frameFits||!current.frameHit||!current.targetHit||current.quietMs<200)return false;
  if(![current.x,current.y,current.viewportWidth,current.viewportHeight,current.resizeCount,current.quietMs,...current.geometry,...previous.geometry].every(Number.isFinite))return false;
  if(current.x<=0||current.y<=0||current.x>=current.viewportWidth||current.y>=current.viewportHeight)return false;
  return current.resizeCount===previous.resizeCount&&current.geometry.length===previous.geometry.length&&
    current.geometry.every((value,index)=>Math.abs(value-previous.geometry[index])<=0.5);
}
// End pointer readiness contract.
async function clickElement(expression) {
  await until(()=>evaluate(`(()=>{const e=${expression};return !!e&&!e.disabled&&e.getClientRects().length>0;})()`),'Expected control did not become enabled and visible: '+expression);
  // Scrolling can resize the host iframe and change both document viewports.
  // Never reuse the rectangle measured in the same turn as scrollIntoView.
  await evaluate(`(()=>{const e=${expression};e.scrollIntoView({block:'center',inline:'nearest',behavior:'instant'});})()`);
  const sample=()=>evaluate(`(()=>{
    const e=${expression},f=document.getElementById('app'),d=f.contentDocument,w=f.contentWindow;
    if(!e||e.disabled||!e.getClientRects().length)return null;
    const r=e.getBoundingClientRect(),fr=f.getBoundingClientRect(),rr=${root}.getBoundingClientRect();
    const localX=r.left+r.width/2,localY=r.top+r.height/2,x=fr.left+f.clientLeft+localX,y=fr.top+f.clientTop+localY;
    const hit=d.elementFromPoint(localX,localY),frameHit=document.elementFromPoint(x,y)===f;
    return {x,y,viewportWidth:innerWidth,viewportHeight:innerHeight,resizeCount,quietMs:performance.now()-lastResizeAt,
      targetHit:hit===e||e.contains(hit),frameHit,
      frameFits:(${publicationFrameFits.toString()})({root:{top:0,bottom:rr.height,left:0,right:rr.width,height:rr.height},frameHeight:f.clientHeight,frameWidth:f.clientWidth,resizeCount,quietMs:performance.now()-lastResizeAt}),
      geometry:[r.left,r.top,r.width,r.height,fr.left,fr.top,fr.width,fr.height,rr.width,rr.height,scrollX,scrollY,w.scrollX,w.scrollY]};
  })()`);
  let previous=null,point=null;
  await until(async()=>{
    const current=await sample(),stable=filterPointerReady(current,previous);lastPointer={expression,current,previous,stable};previous=current;
    if(!stable){
      // A settled resize may leave the previously scrolled center clipped.
      // Re-scroll only; a pointer action is never replayed automatically.
      if(current&&current.quietMs>=200&&(!current.frameHit||!current.targetHit||current.x<=0||current.y<=0||current.x>=current.viewportWidth||current.y>=current.viewportHeight)){
        await evaluate(`(()=>{const e=${expression};if(e&&!e.disabled)e.scrollIntoView({block:'center',inline:'nearest',behavior:'instant'});})()`);previous=null;
      }
      return false;
    }
    // A second CDP sample immediately before dispatch also hit-tests the exact
    // element in both realms. A changed layout waits; it never retries a click.
    const verified=await sample();if(!filterPointerReady(verified,current)){previous=verified;return false;}
    point={x:verified.x,y:verified.y};return true;
  },'Control did not reach stable, unobstructed pointer geometry: '+expression);
  await rpc('Input.dispatchMouseEvent', {type: 'mousePressed', ...point, button: 'left', buttons: 1, clickCount: 1});
  await rpc('Input.dispatchMouseEvent', {type: 'mouseReleased', ...point, button: 'left', buttons: 0, clickCount: 1});
  await ready();
}
const click = label => clickElement(button(label));
async function capture(path, settle = true) {
  await evaluate(`window.scrollTo(0,0);document.getElementById('app').contentWindow.scrollTo(0,0);`);
  if (settle) await until(() => evaluate(`(()=>{const f=document.getElementById('app'),r=${root}.getBoundingClientRect();return (${publicationFrameFits.toString()})({root:r,frameHeight:f.clientHeight,frameWidth:f.clientWidth,resizeCount,quietMs:performance.now()-lastResizeAt});})()`), 'Screenshot must fit the current complete app without clipping');
  const metrics = await rpc('Page.getLayoutMetrics'), size = metrics.cssContentSize || metrics.contentSize;
  if (settle) assert(size.width <= 1600 && size.height <= 6000, 'Proof must not silently crop controls');
  const result = await rpc('Page.captureScreenshot', {format: 'png', captureBeyondViewport: true, fromSurface: true, clip: {x: 0, y: 0, width: Math.min(1600, Math.ceil(size.width)), height: Math.min(6000, Math.ceil(size.height)), scale: 1}});
  await writeFile(path, Buffer.from(result.data, 'base64'));
}
async function binding(params) {
  const context = contexts.get(params.executionContextId);
  assert.equal(context?.auxData?.frameId, frameID, 'Only the synthetic parent invokes the Node fixture');
  assert.equal(context?.auxData?.isDefault, true, 'Binding uses the normal parent realm');
  assert.equal(context?.origin, origin, 'Binding preserves the exact synthetic parent origin');
  const call = JSON.parse(params.payload);
  assert(Number.isSafeInteger(call.id) && call.id > 0 && filterBrowserTools.includes(call.name), 'Only advertised synthetic tool calls are accepted');
  let result;
  try {result = {structuredContent: {result: await fixture.invoke(call.name, call.args)}};}
  catch (error) {errors.push(`Native DTO-backed invocation failed: ${call.name}: ${error.message}`);result = {isError: true, structuredContent: {error: {code: error.code || 'unavailable'}}};}
  await evaluate(`window.completePublicationCall(${call.id},${JSON.stringify(result).replaceAll('<', '\\u003c')})`, params.executionContextId);
}
async function handleMessage(event) {
  const message = JSON.parse(event.data);
  if (message.id) {
    const request = pending.get(message.id);if (!request) return;
    clearTimeout(request.timer);pending.delete(message.id);
    message.error ? request.reject(new Error(message.error.message)) : request.resolve(message.result);return;
  }
  if (message.method === 'Runtime.executionContextCreated') contexts.set(message.params.context.id, message.params.context);
  else if (message.method === 'Runtime.executionContextDestroyed') contexts.delete(message.params.executionContextId);
  else if (message.method === 'Runtime.bindingCalled') {assert.equal(message.params.name, 'publicationInvoke');await binding(message.params);}
  else if (message.method === 'Runtime.exceptionThrown') errors.push(message.params.exceptionDetails.exception?.description || message.params.exceptionDetails.text);
  else if (message.method === 'Fetch.requestPaused') {
    const {request, requestId} = message.params, url = new URL(request.url);requests.push(request.url);
    if (url.origin !== origin || !['/', '/resource', '/favicon.ico'].includes(url.pathname) || url.search || request.method !== 'GET') {
      errors.push('Unexpected browser request: ' + request.url);await rpc('Fetch.failRequest', {requestId, errorReason: 'BlockedByClient'});return;
    }
    if (mode === 'mcp') {await rpc('Fetch.continueRequest', {requestId});return;}
    const content = url.pathname === '/' ? host : url.pathname === '/resource' ? html : '';
    await rpc('Fetch.fulfillRequest', {requestId, responseCode: content ? 200 : 404, responseHeaders: [{name: 'Content-Type', value: 'text/html'}], body: Buffer.from(content).toString('base64')});
  }
}
async function noSource(message, action) {
  const before = fixture.counts();
  await action();
  equal(fixture.counts(), before, message);
}
async function setInput(label, value) {
  await evaluate(`(()=>{const e=${field(label)};if(!e||e.disabled)throw new Error('Missing enabled input');e.value=${JSON.stringify(value)};e.dispatchEvent(new Event('input',{bubbles:true}));})()`);
  await ready();
}
const card = label => `Array.from(${root}.querySelectorAll('.filter-editor>section')).find(e=>e.querySelector('input[aria-label="Filter label"]')?.value===${JSON.stringify(label)})`;
const cardButton = (label, title) => `Array.from(${card(label)}.querySelectorAll('button')).find(e=>e.textContent===${JSON.stringify(title)})`;
async function openDefaults() {
  if (!await evaluate(`${root}.querySelector('.filter-editor')?.open===true`)) await clickElement(`${root}.querySelector('.filter-editor>summary')`);
  await until(()=>evaluate(`${root}.querySelector('.filter-editor')?.open===true`),'Business filters did not expand after its pointer click');
}
async function openFilter(label, mode='default') {
  await openDefaults();
  await clickElement(cardButton(label, mode==='default'?'Change saved default':'Choose temporary preview value'));
  const legend=label+' · '+(mode==='default'?'saved default':'temporary selection');
  await until(()=>evaluate(`(()=>{const e=${editor};return e?.querySelector('legend')?.textContent===${JSON.stringify(legend)}&&e.getClientRects().length>0&&Array.from(e.querySelectorAll('input')).some(i=>!i.disabled&&i.getClientRects().length>0);})()`),'Requested filter editor did not open: '+legend);
}
async function setDate(start, end) {await setInput('Start date',start);await setInput('End date · inclusive',end);}
async function arm(request, kind='option') {
  await evaluate(`document.getElementById('app').contentWindow.armNativeRequest(${JSON.stringify(request)},${JSON.stringify(kind)})`);
}
async function choose(value) {
  await clickElement(`Array.from(${editor}.querySelectorAll('.filter-option-list label')).find(e=>e.querySelector('span')?.textContent===${JSON.stringify(value)}).querySelector('input')`);
}
async function filterJourney() {
  const data=fixture.data, title=data.initial_report.definition.metadata[0].title;
  await until(()=>evaluate(`!!${root}&&${button('Build')}?.disabled===false`),'Native authoring capabilities did not arrive');
  await check(`hostMessages.filter(m=>m.method===${JSON.stringify(mode==='embedded'?'initialize':'ui/initialize')}).length===1&&!hostMessages.some(m=>m.method===${JSON.stringify(mode==='embedded'?'ui/initialize':'initialize')})`, 'Exact requested adapter handshakes once with no fallback');
  await click('Build');await click(title);await textHas(`Private draft revision ${data.initial_report.revision} loaded.`);
  equal(snapshot(),original,'Opening a native report is metadata-only');
  equal(fixture.counts().nativeSourceReadsRepresented,0,'Opening and rendering filters reads no source');

  await noSource('Date edits and Cancel do not query or run',async()=>{
    await openFilter('Day');await check(`${field('Start date')}.value==='2026-01-01'&&${field('End date · inclusive')}.value==='2026-01-31'`,'Native half-open date default displays an inclusive end');
    await setDate('2026-01-03','2026-01-20');await click('Cancel');await openFilter('Day');
    await check(`${field('Start date')}.value==='2026-01-01'&&${field('End date · inclusive')}.value==='2026-01-31'`,'Cancel discards the date stage');
    await setDate('2026-01-04','2026-01-20');await click('Notes');await check(`${editor}===null`,'Page navigation closes the old editor');
    await click('Analysis');await openFilter('Day');
    await check(`${field('Start date')}.value==='2026-01-01'&&${field('End date · inclusive')}.value==='2026-01-31'`,'A page-isolated stale stage cannot change Analysis');await click('Cancel');
  });
  await noSource('Search typing, selection removal and Cancel are local',async()=>{
    await openFilter('Region');await setInput('Find values','East');await click('South · remove');await click('Cancel');
    await openFilter('Region');await check(`${editor}.textContent.includes('North · remove')&&${editor}.textContent.includes('South · remove')&&${field('Find values')}.value===''`,'Cancelled multiselect changes never reach the saved default');
    await setInput('Find values','East');
  });
  await arm(data.initial_option_request);await click('Search options');
  equal(fixture.counts(),{optionSearches:1,privateExecutions:0,publishedRuns:0,nativeSourceReadsRepresented:1},'Only explicit Search replays one native bounded source read');
  await check(`JSON.stringify(Array.from(${editor}.querySelectorAll('.filter-option-list span'),e=>e.textContent))===JSON.stringify(['East'])&&${editor}.textContent.includes('2 of 16 selected')&&${editor}.textContent.includes('full field population')`,'Exact native multiselect choices, 16-value limit and full-population disclosure are visible');
  await capture(screenshotPath);
  await noSource('Selecting exact values and Done change the editor only',async()=>{
    await click('South · remove');await choose('East');await click('Done');
    await openDefaults();await check(`${card('Region')}.textContent.includes('Saved: North, East')&&${button('Save report')}.disabled===false`,'Done stages the distinct saved default for explicit Save');
    equal(snapshot(),original,'Done does not persist metadata');
    await openFilter('Day');await setDate('2026-01-02','2026-01-31');await capture(screenshotPath.replace(/\.png$/,'.dates.png'));await click('Done');
    await openDefaults();await check(`${card('Day')}.textContent.includes('2026-01-02 until 2026-02-01 (exclusive)')`,'Done converts inclusive January 31 to native February 1 exclusive');
  });
  await noSource('Explicit Save writes metadata without a source query',()=>click('Save report'));
  equal(writes().length,1,'One explicit report save');equal(writes()[0].args,data.save_request,'Exact captured native save request including CAS and typed defaults');
  equal(snapshot().report.definition.report_pages[1],original.report.definition.report_pages[1],'Saving Analysis defaults preserves native Notes byte-for-byte');
  await noSource('Reload reads the actual persisted native saved default',()=>click('Reload latest'));
  await openFilter('Region');await setInput('Find values','East');await arm(data.private_option_request);await click('Search options');
  equal(fixture.counts().optionSearches,2,'Saved-revision lookup uses its new exact digest and operation');
  await noSource('Cancel after Search applies no option result',()=>click('Cancel'));
  const saved=snapshot();
  await noSource('Temporary preview selections do not rewrite saved defaults or execute',async()=>{
    await openFilter('Region','preview');await click('East · remove');await click('Done');
    await openFilter('Day','preview');await setDate('2026-01-01','2026-01-01');await click('Done');
    await openDefaults();await check(`${card('Region')}.textContent.includes('Next preview: North')&&${card('Region')}.textContent.includes('Saved: North, East')`,'Temporary selection and persisted default remain separately labeled');
    await check(`${card('Day')}.textContent.includes('Next preview: 2026-01-01 until 2026-01-02 (exclusive)')`,'One inclusive temporary day becomes a half-open native day');
    equal(snapshot(),saved,'Temporary selections leave exact saved document untouched');
  });
  // Reopening the draft deliberately clears validation hints; this explicit
  // metadata check restores only the native captured fresh evidence.
  await click('Selected');await noSource('Checking native chart validation is metadata-only',()=>click('Check chart status'));
  await arm(fixture.privateRun.preview_request,'preview');await click('Private preview');
  await check(`${root}.querySelector('.preview-provenance')?.textContent.includes('Private')&&${root}.textContent.includes('1.250')&&!${root}.textContent.includes('Retained output unavailable.')`,'Explicit preview displays a native actor-private retained result');
  equal(fixture.counts(),{optionSearches:2,privateExecutions:1,publishedRuns:0,nativeSourceReadsRepresented:3},'Private preview executes one native-backed source attempt, retained reads execute none');
  equal(snapshot(),saved,'Preview preserves exact saved defaults and block defaults');
  await noSource('Clearing temporary preview selections restores saved defaults without a run',async()=>{
    await openDefaults();await clickElement(cardButton('Region','Use saved default for preview'));await openDefaults();await clickElement(cardButton('Day','Use saved default for preview'));await openDefaults();
    await check(`!${root}.textContent.includes('Next preview:')&&${card('Region')}.textContent.includes('Saved: North, East')&&${card('Day')}.textContent.includes('Saved: 2026-01-02 until 2026-02-01 (exclusive)')`,'Clear removes overrides and retains persisted values');
    equal(snapshot(),saved,'Clear never rewrites saved defaults');
  });
  await click('Browse');if(await evaluate(`${button('Show reports')}?.getClientRects().length>0`))await click('Show reports');await click(data.published_catalog.items.find(i=>i.target.id===fixture.report).title);
  await check(`${root}.dataset.mode==='consumer'&&${root}.textContent.includes('Use published default')`,'Consumer begins from native published defaults and no leaked temporary selection');
  await noSource('Consumer date and multiselect changes remain temporary',async()=>{
    await click('Choose Region');await click('East · remove');await setInput('Find values','East');
  });
  await arm(data.published_option_request);await click('Search options');
  equal(fixture.counts().optionSearches,3,'Consumer explicit Search uses the captured published policy, revision and digest');
  await noSource('Consumer Done and inclusive dates do not run',async()=>{
    await click('Done');await click('Choose Day');await setDate('2026-01-01','2026-01-01');await click('Done');
  });
  await arm(fixture.publicRun.run_request,'run');await click('Run with these filters');
  await check(`${root}.querySelector('.preview-provenance')?.textContent.includes('Retained report')&&${root}.textContent.includes('1.250')&&!${root}.textContent.includes('Retained output unavailable.')`,'Consumer displays the native public retained result');
  equal(fixture.counts(),{optionSearches:3,privateExecutions:1,publishedRuns:1,nativeSourceReadsRepresented:5},'Three explicit lookups and two deliberate executions account for all represented native source reads');
  await capture(screenshotPath.replace(/\.png$/,'.published.png'));
  await noSource('Consumer clear changes only future invocation inputs',async()=>{
    await click('Use published default');await click('Use published default');
    await check(`!${button('Use published default')}&&${root}.textContent.includes('These retained values use earlier filter selections. Run explicitly to update them.')`,'Clear keeps old retained values explicitly stale until another deliberate run');
    equal(snapshot(),saved,'Consumer temporary clear leaves all captured saved defaults intact');
  });
  equal(await evaluate('hostCalls.map(c=>({name:c.name,args:c.arguments}))'),fixture.calls,'Every adapter invocation is accounted for by the exact native-backed ledger');
  await check(`hostErrors.length===0&&document.getElementById('app').contentWindow.storageTouches===0`,'No protocol errors, browser storage or credential channel');
  await evaluate('closeApp()');await textHas('This report app is closed.');
  equal(errors,[],'No browser exceptions or unexpected requests');
  assert(requests.includes(origin+'/resource'),'Exact production resource was loaded');
}
async function journey() {
  let port;
  await until(async () => {try {port = Number((await readFile(join(directory, 'DevToolsActivePort'), 'utf8')).split('\n')[0]);return port > 0;} catch {return false;}}, 'Chromium did not start');
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, {method: 'PUT', signal: AbortSignal.timeout(10000)})).json();
  socket = new WebSocket(target.webSocketDebuggerUrl);
  let connectTimer;
  try {await Promise.race([once(socket, 'open'), new Promise((_, reject) => {connectTimer = setTimeout(() => reject(new Error('CDP connection did not open')), 10000);})]);}
  finally {clearTimeout(connectTimer);}
  socket.addEventListener('message', event => {void handleMessage(event).catch(error => {if (!stopping) errors.push(error.stack || String(error));});});
  await rpc('Runtime.enable');await rpc('Page.enable');frameID = (await rpc('Page.getFrameTree')).frameTree.frame.id;
  await rpc('Runtime.addBinding', {name: 'publicationInvoke'});
  await rpc('Emulation.setDeviceMetricsOverride', {width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false});
  await rpc('Page.addScriptToEvaluateOnNewDocument', {source: `window.storageTouches=0;for(const name of ['localStorage','sessionStorage','indexedDB'])Object.defineProperty(window,name,{get(){window.storageTouches++;throw new Error('Unexpected browser storage');}});`});
  await rpc('Page.addScriptToEvaluateOnNewDocument', {source: fixture.clockScript()});
  await rpc('Fetch.enable', {patterns: [{urlPattern: '*'}]});
  await rpc('Page.navigate', {url: origin + '/'});
  await filterJourney();
}
try {
  await Promise.race([journey(), new Promise((_, reject) => {watchdog = setTimeout(() => {stopping = true;reject(new Error('Filter browser watchdog expired after 120 seconds'));}, 120000);})]);
} catch (error) {
  runError = error;
  if (socket?.readyState === WebSocket.OPEN) {try {await capture(screenshotPath.replace(/\.png$/, '.failure.png'), false);} catch {}}
  let state;try {state = await evaluate(`({text:${root}?.textContent.slice(0,16000),messages:hostMessages.slice(-12),hostErrors})`);} catch {}
  console.error(JSON.stringify({mode, assertions, errors, browserStderr, lastPointer, calls: fixture.calls, state}, null, 2));
  throw error;
} finally {
  clearTimeout(watchdog);stopping = true;
  try {
    await cleanupBrowserFixture({browser, closed, directory,
      requestClose: () => socket?.readyState === WebSocket.OPEN ? rpc('Browser.close') : undefined,
      closeTransport: () => {for (const request of pending.values()) {clearTimeout(request.timer);request.reject(new Error('Governed filter fixture closed'));}pending.clear();socket?.close();},
      closeServer: () => new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())), removeProfile: rm});
  } catch (cleanupError) {if (runError) throw new AggregateError([runError, cleanupError], 'Filter assertions and owned-process cleanup failed');throw cleanupError;}
}
console.log(JSON.stringify({sourceReplay: fixture.counts(), nativeCapture: fixture.provenance, mode, checks: assertions.length, assertions, metadataWrites: writes(), calls: fixture.calls, requests, screenshots: [screenshotPath, screenshotPath.replace(/\.png$/, '.dates.png'), screenshotPath.replace(/\.png$/, '.published.png')], fixture: 'native PostgreSQL DTO-backed synthetic transport; no live PostgreSQL or Pengui connection', passed: true}));
