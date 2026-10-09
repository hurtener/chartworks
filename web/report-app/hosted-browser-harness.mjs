// Test-only shared hosted Chrome driver extracted from publication.browser.mjs.
// Importing this module never starts Chrome; execution is explicitly hosted-only.
import assert from 'node:assert/strict';
import {readFile, writeFile, mkdtemp, rm} from 'node:fs/promises';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {once} from 'node:events';
import {cleanupBrowserFixture} from './browser_cleanup.mjs';
import {initializePublicationHost, publicationFrameFits} from './publication-browser-fixture.mjs';


export function hostedPointerReady(current, previous) {
  if(!current||!previous||!Array.isArray(current.geometry)||!Array.isArray(previous.geometry)||!current.frameFits||!current.frameHit||!current.targetHit||current.quietMs<200)return false;
  if(![current.x,current.y,current.viewportWidth,current.viewportHeight,current.resizeCount,current.quietMs,...current.geometry,...previous.geometry].every(Number.isFinite))return false;
  if(current.x<=0||current.y<=0||current.x>=current.viewportWidth||current.y>=current.viewportHeight)return false;
  return current.resizeCount===previous.resizeCount&&current.geometry.length===previous.geometry.length&&
    current.geometry.every((value,index)=>Math.abs(value-previous.geometry[index])<=0.5);
}

export async function runHostedBrowser({htmlPath,screenshotPath,mode,fixture,tools,label},run) {
assert.equal(process.env.GITHUB_ACTIONS, 'true', 'Chromium proof runs only in the hosted CI job');
assert(['mcp','embedded'].includes(mode),'Explicit adapter required; no fallback');
assert(htmlPath && screenshotPath?.endsWith('.png'),'Provide actual compiled HTML and a PNG proof path');
assert(typeof label==='string' && /^[A-Za-z0-9 .·-]+$/.test(label),'Static synthetic host label only');
const html = await readFile(htmlPath, 'utf8');
assert(html.startsWith('<!doctype html>') && html.includes('Content-Security-Policy'), 'Exact production resource HTML required');
assert.equal(html.includes('const REPORT_APP_EMBEDDED_PARENTS='), mode === 'embedded', 'HTML must match its explicit transport');
const names = {version: 'chartworks-host-tools-v1', names: tools};
const host = `<!doctype html><meta charset="utf-8"><title>${label}</title><style>body{margin:0}#fixture-label{padding:6px 16px;background:#20372d;color:white;font:12px system-ui,sans-serif}iframe{border:0;width:100%;height:1500px}</style><div id="fixture-label">${label}</div><iframe id="app" title="Chartworks report app" src="/resource"></iframe><script>(${initializePublicationHost.toString()})(${mode === 'embedded'},${JSON.stringify(names).replaceAll('<', '\\u003c')},${typeof fixture.allocate === 'function'});</script>`;
const server = createServer((req, res) => {
  const content = req.url === '/' ? host : req.url === '/resource' ? html : '';
  res.writeHead(content ? 200 : 404, {'Content-Type': 'text/html', 'Cache-Control': 'no-store'});res.end(content);
});
server.listen(0, '127.0.0.1');await once(server, 'listening');
const origin = mode === 'embedded' ? 'https://report-host.example' : `http://127.0.0.1:${server.address().port}`;
const directory = await mkdtemp(join(tmpdir(), 'chartworks-hosted-browser-'));
const browser = spawn(process.env.CHARTWORKS_CHROME_BIN || 'chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--disable-background-networking', '--disable-component-update', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', '--user-data-dir=' + directory, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
const closed = new Promise(resolve => browser.once('close', resolve)), pending = new Map(), contexts = new Map();
const errors = [], requests = [], assertions = [];
let socket, sequence = 0, browserError, browserStderr = '', frameID, stopping = false, runError, watchdog, lastPointer, activeBindings = 0;
browser.on('error', error => browserError = error);
browser.stderr.on('data', data => { if (browserStderr.length < 16384) browserStderr += data.toString(); });
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
function rpc(method, params = {}) {
  if (stopping && method !== 'Browser.close') return Promise.reject(new Error('Hosted browser fixture is closing'));
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
    if (stopping) throw new Error('Hosted browser fixture is closing');
    if (browserError) throw browserError;
    if (browser.exitCode !== null || browser.signalCode !== null) throw new Error('Chromium exited: ' + browserStderr);
    if (errors.length) throw new Error(errors.join('\n'));
    if (await fn()) return;
    await pause(25);
  }
  throw new Error(message);
}
const root = "document.getElementById('app')?.contentDocument?.getElementById('report-app')";
const button = label => `Array.from(${root}.querySelectorAll('button')).find(e=>e.textContent===${JSON.stringify(label)})`;
async function check(expression, message) {assert.equal(await evaluate(expression), true, message);assertions.push(message);}
function equal(actual, expected, message) {assert.deepEqual(actual, expected, message);assertions.push(message);}
async function ready() {await until(() => evaluate(`!!${root}&&!Array.from(${root}.querySelectorAll('[role=status]')).some(e=>e.textContent==='Working…')`), 'Hosted browser operation did not settle');}
async function textHas(text) {await until(() => evaluate(`${root}?.textContent.includes(${JSON.stringify(text)})`), 'Missing text: ' + text);await ready();}
async function clickElement(expression, {wait = true, repeat = 1} = {}) {
  assert([1,2].includes(repeat),'Only one deliberate click or a bounded repeated click');
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
    const current=await sample(),stable=hostedPointerReady(current,previous);lastPointer={expression,current,previous,stable};previous=current;
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
    const verified=await sample();if(!hostedPointerReady(verified,current)){previous=verified;return false;}
    point={x:verified.x,y:verified.y};return true;
  },'Control did not reach stable, unobstructed pointer geometry: '+expression);
  for (let count=0;count<repeat;count++) {
  await rpc('Input.dispatchMouseEvent', {type: 'mousePressed', ...point, button: 'left', buttons: 1, clickCount: 1});
  await rpc('Input.dispatchMouseEvent', {type: 'mouseReleased', ...point, button: 'left', buttons: 0, clickCount: 1});
  }
  if (wait) await ready();
}
const click = (label, options) => clickElement(button(label), options);
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
  assert(Number.isSafeInteger(call.id) && call.id > 0 && (tools.includes(call.name) || call.name === 'app/allocate-target' && typeof fixture.allocate === 'function'), 'Only advertised synthetic tool calls are accepted');
  let result;
  try {result = call.name === 'app/allocate-target' ? await fixture.allocate(call.args) : fixture.reply ? await fixture.reply(call.name, call.args) : {structuredContent: {result: await fixture.invoke(call.name, call.args)}};}
  catch (error) {errors.push(`Synthetic fixture invocation failed: ${call.name}: ${error.message}`);result = {isError: true, structuredContent: {error: {code: error.code || 'unavailable'}}};}
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
  else if (message.method === 'Runtime.bindingCalled') {assert.equal(message.params.name, 'publicationInvoke');activeBindings++;try{await binding(message.params);}finally{activeBindings--;}}
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
  if (fixture.clockScript) await rpc('Page.addScriptToEvaluateOnNewDocument', {source: fixture.clockScript()});
  await rpc('Fetch.enable', {patterns: [{urlPattern: '*'}]});
  await rpc('Page.navigate', {url: origin + '/'});
  await run({rpc,evaluate,until,root,button,check,equal,ready,textHas,clickElement,click,capture,assertions,requests,
    navigate:()=>rpc('Page.navigate',{url:origin+'/'}),close:()=>evaluate('closeApp()'),drain:()=>until(()=>activeBindings===0,'Synthetic binding replies did not finish')});
  equal(errors, [], 'No browser exceptions or unexpected requests');
  assert(requests.includes(origin+'/resource'),'Browser loaded the exact compiled resource');
}
try {
  await Promise.race([journey(), new Promise((_, reject) => {watchdog = setTimeout(() => {stopping = true;reject(new Error('Hosted browser watchdog expired after 120 seconds'));}, 120000);})]);
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
      closeTransport: () => {for (const request of pending.values()) {clearTimeout(request.timer);request.reject(new Error('Hosted browser fixture closed'));}pending.clear();socket?.close();},
      closeServer: () => new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())), removeProfile: rm});
  } catch (cleanupError) {if (runError) throw new AggregateError([runError, cleanupError], 'Hosted browser assertions and owned-process cleanup failed');throw cleanupError;}
}
return {mode,checks:assertions.length,assertions,calls:fixture.calls,requests,passed:true};
}
