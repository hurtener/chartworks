// Actual production HTML in hosted Chromium, with synthetic metadata only.
// CLI: node publication.browser.mjs <mcp.html|embedded.html> <proof.png> <mcp|embedded>
// Deliberately separate from the larger canvas fixture. No deployed-host claim.
import assert from 'node:assert/strict';
import {readFile, writeFile, mkdtemp, rm} from 'node:fs/promises';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {once} from 'node:events';
import {cleanupBrowserFixture} from './browser_cleanup.mjs';
import {publicationBrowserFixture, initializePublicationHost, publicationTools, publicationWrites, publicationFrameFits} from './publication-browser-fixture.mjs';

assert.equal(process.env.GITHUB_ACTIONS, 'true', 'Publication Chromium proof runs only in the hosted CI job');
const [htmlPath, screenshotPath, mode] = process.argv.slice(2);
assert(['mcp', 'embedded'].includes(mode), 'Explicit adapter required; no fallback');
assert(htmlPath && screenshotPath?.endsWith('.png'), 'Provide actual compiled HTML and a PNG proof path');
const html = await readFile(htmlPath, 'utf8'), fixture = publicationBrowserFixture();
assert(html.startsWith('<!doctype html>') && html.includes('Content-Security-Policy'), 'Exact production resource HTML required');
assert.equal(html.includes('const REPORT_APP_EMBEDDED_PARENTS='), mode === 'embedded', 'HTML must match its explicit transport');
const names = {version: 'chartworks-host-tools-v1', names: publicationTools};
const host = `<!doctype html><meta charset="utf-8"><title>Synthetic publication lifecycle proof</title><style>body{margin:0}#fixture-label{padding:6px 16px;background:#20372d;color:white;font:12px system-ui,sans-serif}iframe{border:0;width:100%;height:1500px}</style><div id="fixture-label">Synthetic host fixture · Publication metadata only · No production provider connection</div><iframe id="app" title="Chartworks report app" src="/resource"></iframe><script>(${initializePublicationHost.toString()})(${mode === 'embedded'},${JSON.stringify(names).replaceAll('<', '\\u003c')});</script>`;
const server = createServer((req, res) => {
  const content = req.url === '/' ? host : req.url === '/resource' ? html : '';
  res.writeHead(content ? 200 : 404, {'Content-Type': 'text/html', 'Cache-Control': 'no-store'});res.end(content);
});
server.listen(0, '127.0.0.1');await once(server, 'listening');
const origin = mode === 'embedded' ? 'https://report-host.example' : `http://127.0.0.1:${server.address().port}`;
const directory = await mkdtemp(join(tmpdir(), 'chartworks-publication-browser-'));
const browser = spawn(process.env.CHARTWORKS_CHROME_BIN || 'chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--disable-background-networking', '--disable-component-update', '--no-first-run', '--no-default-browser-check', '--remote-debugging-port=0', '--user-data-dir=' + directory, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
const closed = new Promise(resolve => browser.once('close', resolve)), pending = new Map(), contexts = new Map();
const errors = [], requests = [], assertions = [], writes = () => fixture.calls.filter(call => publicationWrites.includes(call.name));
const snapshot = () => structuredClone({state: fixture.state(), definitions: [...fixture.definitions], blocks: [...fixture.blocks], privatePreview: fixture.privatePreview});
const original = snapshot();
let socket, sequence = 0, browserError, browserStderr = '', frameID, stopping = false, runError, watchdog;
browser.on('error', error => browserError = error);
browser.stderr.on('data', data => { if (browserStderr.length < 16384) browserStderr += data.toString(); });
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
function rpc(method, params = {}) {
  if (stopping && method !== 'Browser.close') return Promise.reject(new Error('Publication fixture is closing'));
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
    if (stopping) throw new Error('Publication fixture is closing');
    if (browserError) throw browserError;
    if (browser.exitCode !== null || browser.signalCode !== null) throw new Error('Chromium exited: ' + browserStderr);
    if (errors.length) throw new Error(errors.join('\n'));
    if (await fn()) return;
    await pause(25);
  }
  throw new Error(message);
}
const root = "document.getElementById('app')?.contentDocument?.getElementById('report-app')";
const panel = `${root}.querySelector('details[aria-label="Publication and review"]')`;
const button = label => `Array.from(${root}.querySelectorAll('button')).find(e=>e.textContent===${JSON.stringify(label)})`;
const checkbox = label => `Array.from(${root}.querySelectorAll('input[type="checkbox"]')).find(e=>e.getAttribute('aria-label')===${JSON.stringify(label)})`;
async function check(expression, message) {assert.equal(await evaluate(expression), true, message);assertions.push(message);}
function equal(actual, expected, message) {assert.deepEqual(actual, expected, message);assertions.push(message);}
async function ready() {await until(() => evaluate(`!!${root}&&!Array.from(${root}.querySelectorAll('[role=status]')).some(e=>e.textContent==='Working…')`), 'Publication operation did not settle');}
async function textHas(text) {await until(() => evaluate(`${root}?.textContent.includes(${JSON.stringify(text)})`), 'Missing text: ' + text);await ready();}
async function clickElement(expression) {
  // start() can show the initial empty catalog before its awaited capability
  // and catalog reads finish. Wait for this exact actionable control; absence
  // of a Working status alone does not establish startup readiness.
  let point;
  await until(async () => {
    point = await evaluate(`(()=>{const e=${expression};if(!e||e.disabled||!e.getClientRects().length)return null;e.scrollIntoView({block:'center',inline:'nearest'});const r=e.getBoundingClientRect(),f=document.getElementById('app').getBoundingClientRect();return {x:f.left+r.left+r.width/2,y:f.top+r.top+r.height/2};})()`);
    return point !== null;
  }, 'Expected control did not become enabled and visible: ' + expression);
  assert(point.x > 0 && point.x < 1440 && point.y > 0 && point.y < 1000, 'Control must be reachable in the actual viewport');
  await rpc('Input.dispatchMouseEvent', {type: 'mousePressed', ...point, button: 'left', buttons: 1, clickCount: 1});
  await rpc('Input.dispatchMouseEvent', {type: 'mouseReleased', ...point, button: 'left', buttons: 0, clickCount: 1});
  await ready();
}
const click = label => clickElement(button(label));
async function unchanged(message, action) {const before = snapshot(), count = writes().length;await action();equal(snapshot(), before, message);equal(writes().length, count, message + ' makes no metadata write');}
async function confirm(label) {await unchanged('Confirmation checkbox is local until its action is clicked', async () => {await clickElement(checkbox(label));await check(`${checkbox(label)}?.checked===true`, label);});}
async function disabled(label) {
  await unchanged(label + ' requires its own explicit confirmation', async () => {
    await check(`${button(label)}?.disabled===true`, label + ' is disabled before confirmation');
    await evaluate(`${button(label)}.click()`);await ready();
  });
}
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
  assert(Number.isSafeInteger(call.id) && call.id > 0 && publicationTools.includes(call.name), 'Only advertised synthetic tool calls are accepted');
  let result;
  try {result = {structuredContent: {result: await fixture.invoke(call.name, call.args)}};}
  catch (error) {errors.push(`Synthetic metadata invocation failed: ${call.name}: ${error.message}`);result = {isError: true, structuredContent: {error: {code: error.code || 'unavailable'}}};}
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
  await rpc('Fetch.enable', {patterns: [{urlPattern: '*'}]});
  await rpc('Page.navigate', {url: origin + '/'});
  await textHas('No published reports are visible.');
  await check(`hostMessages.filter(m=>m.method===${JSON.stringify(mode === 'embedded' ? 'initialize' : 'ui/initialize')}).length===1&&!hostMessages.some(m=>m.method===${JSON.stringify(mode === 'embedded' ? 'ui/initialize' : 'initialize')})`, 'Exact requested adapter handshakes once without fallback');
  await click('Build');await textHas('Private drafts');await click('Lifecycle report');await textHas('Private draft revision 1 loaded.');
  equal(snapshot(), original, 'Opening the native draft catalog and selecting its report are read-only');
  await check(`${panel}?.open===false&&${panel}.querySelector('summary').textContent==='Publish and review'`, 'Publication is compact by default');
  await unchanged('Expanding publication controls is local', () => clickElement(`${panel}.querySelector('summary')`));
  await check(`${panel}.open===true`, 'Deliberate expansion exposes publication controls');
  await unchanged('Inspection is metadata-only', () => click('Inspect publication status'));
  await textHas('Publication status inspected.');
  await check(`${panel}.open&&${panel}.textContent.includes('ENTIRE immutable chart revision')&&${panel}.textContent.includes('every output listed above')&&${panel}.textContent.includes('audience names and count are not provided')&&JSON.stringify(Array.from(${panel}.querySelectorAll('.publication-chart li'),e=>e.textContent))===JSON.stringify(['amount · chart · Revenue','other · table'])`, 'Whole-chart disclosure includes the unselected table output and bounded audience effect');
  await disabled('Publish entire chart revision');
  await capture(screenshotPath);
  await confirm('I confirm publishing the entire chart revision and all listed outputs.');
  await click('Publish entire chart revision');await textHas('The entire chart revision is published.');
  equal(writes().length, 1, 'Only chart publication was written');
  equal(fixture.blocks.get('chart-a:2').block.private, false, 'Exact validated chart revision is published');
  equal(fixture.report(1).definition, original.definitions[0][1], 'Chart publication does not rewrite any report pin');
  equal(fixture.state().published_revision, 0, 'Chart publication does not publish the report');
  await confirm('Summary / first · first · chart-a revision 2');
  await confirm('Summary / second · second · chart-a revision 2');
  await disabled('Rebind selected widgets');
  await confirm('I confirm rebinding only the selected widgets to their exact published chart revisions.');
  await click('Rebind selected widgets');await textHas('Selected widgets now use their published chart revisions.');
  equal(writes().length, 2, 'Only one separate rebind is written');
  const expected = structuredClone(original.definitions[0][1]);
  for (const widget of expected.report_pages[0].widgets) {widget.block.policy = 'published';delete widget.block.digest;}
  equal(fixture.report(2).definition, expected, 'Rebind preserves all outputs, filters, literals, grids and the untargeted Notes page');
  equal(fixture.definitions.get(1), original.definitions[0][1], 'Original private report revision is immutable');
  equal(fixture.state().published_revision, 0, 'Rebound revision remains private');
  await disabled('Submit report for review');
  await confirm('I confirm submitting this exact report revision for review.');
  await click('Submit report for review');await textHas('Report submitted for review. It remains private.');
  equal(writes().length, 3, 'Only the separately confirmed review transition is written');
  equal([fixture.state().draft_revision, fixture.state().review_revision, fixture.state().published_revision], [0, 2, 0], 'Review has its independent private pointer');
  await check(`${panel}.textContent.includes('Pending review')&&${panel}.textContent.includes('Existing private preview results stay private. No public data run occurs here.')`, 'Review discloses retained privacy and no public run');
  await disabled('Publish reviewed report');
  await confirm('I confirm publishing this exact reviewed report revision.');
  await click('Publish reviewed report');await textHas('Reviewed report published. Existing private previews remain private.');
  equal([fixture.state().draft_revision, fixture.state().review_revision, fixture.state().published_revision], [0, 0, 2], 'Exactly the reviewed report revision is published');
  equal(fixture.report(2).private, false, 'Native publication is reflected in report metadata');
  equal(fixture.privatePreview, fixture.beforePrivatePreview, 'Earlier retained run stays private at its original revision');
  const beforeBrowse = fixture.calls.length;
  await unchanged('Browse reads the new publication without writes', () => click('Browse'));
  await textHas('Choose a published report');
  await unchanged('Reopening the collapsed catalog is local', () => click('Show reports'));
  await check(`${root}.dataset.mode==='consumer'&&${button('Lifecycle report')}?.className==='catalog-item'&&${button('Lifecycle report')}.getClientRects().length>0&&!${root}.querySelector('.retained-preview,.preview-provenance')`, 'Consumer catalog lists the actual publication without adopting private preview values');
  await unchanged('Selecting the publication reads metadata and private history only', () => click('Lifecycle report'));
  await textHas('Current published revision 2');
  await check(`${root}.querySelectorAll('.run-row').length===1&&${root}.querySelector('.run-row').textContent.includes('Private preview')&&!${root}.querySelector('.retained-preview,.preview-provenance')&&${button('Open retained run')}?.disabled===false`, 'Pre-existing private history stays explicitly private and requires a separate retained-open action');
  equal(fixture.calls.slice(beforeBrowse).map(call => call.name), ['reporting_authoring_capabilities_v1', 'reporting_search', 'reporting_describe', 'reporting_runs', 'reporting_authoring_capabilities_v1'], 'Browse and selection make only exact public catalog/metadata/history reads');
  await capture(screenshotPath.replace(/\.png$/, '.published.png'));
  equal(writes().map(call => ({name: call.name, args: call.args})), [
    {name: 'reporting_authoring_block_publish_v1', args: {block: 'chart-a', expected_version: 8, revision: 2, digest: 'a'.repeat(64), evidence: 'validation-a'}},
    {name: 'reporting_authoring_rebind_published_v1', args: {report: 'report-a', expected_version: 1, revision: 1, digest: '1'.repeat(64), widgets: ['first', 'second'].map(widget => ({widget, block: 'chart-a', revision: 2, digest: 'a'.repeat(64)}))}},
    {name: 'reporting_authoring_report_transition_v1', args: {report: 'report-a', expected_version: 2, revision: 2, operation: 'review', note: ''}},
    {name: 'reporting_authoring_report_transition_v1', args: {report: 'report-a', expected_version: 3, revision: 2, operation: 'publish', note: ''}}
  ], 'Exactly four metadata writes preserve exact validation, revision, version and selected-widget pins');
  assert(fixture.calls.every(call => ['reporting_authoring_capabilities_v1', 'reporting_search', 'reporting_describe', 'reporting_runs', 'reporting_authoring_drafts_v1', 'reporting_authoring_read_v1', 'reporting_authoring_lifecycle_v1', ...publicationWrites].includes(call.name)), 'No save, source/model, validation, preparation, execution, or retained-value invocation');
  assertions.push('No hidden save, source/model, validation, preparation, execution or retained-value invocation');
  equal(fixture.report(2).definition.report_pages[1], original.definitions[0][1].report_pages[1], 'Untargeted page remains byte-for-byte unchanged through all four writes');
  await check(`hostErrors.length===0&&document.getElementById('app').contentWindow.storageTouches===0`, 'No protocol error, storage access or credential channel');
  equal(await evaluate('hostCalls.map(c=>({name:c.name,args:c.arguments}))'), fixture.calls, 'All real adapter calls are accounted for by the synthetic metadata ledger');
  await evaluate('closeApp()');await textHas('This report app is closed.');
  equal(errors, [], 'No browser exceptions or unexpected requests');
  assert(requests.some(url => url === origin + '/resource'), 'Browser loaded the exact compiled resource');
}
try {
  await Promise.race([journey(), new Promise((_, reject) => {watchdog = setTimeout(() => {stopping = true;reject(new Error('Publication browser watchdog expired after 120 seconds'));}, 120000);})]);
} catch (error) {
  runError = error;
  if (socket?.readyState === WebSocket.OPEN) {try {await capture(screenshotPath.replace(/\.png$/, '.failure.png'), false);} catch {}}
  let state;try {state = await evaluate(`({text:${root}?.textContent.slice(0,16000),messages:hostMessages.slice(-12),hostErrors})`);} catch {}
  console.error(JSON.stringify({mode, assertions, errors, browserStderr, calls: fixture.calls, state}, null, 2));
  throw error;
} finally {
  clearTimeout(watchdog);stopping = true;
  try {
    await cleanupBrowserFixture({browser, closed, directory,
      requestClose: () => socket?.readyState === WebSocket.OPEN ? rpc('Browser.close') : undefined,
      closeTransport: () => {for (const request of pending.values()) {clearTimeout(request.timer);request.reject(new Error('Publication fixture closed'));}pending.clear();socket?.close();},
      closeServer: () => new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve())), removeProfile: rm});
  } catch (cleanupError) {if (runError) throw new AggregateError([runError, cleanupError], 'Publication assertions and owned-process cleanup failed');throw cleanupError;}
}
console.log(JSON.stringify({mode, checks: assertions.length, assertions, metadataWrites: writes(), calls: fixture.calls, requests, screenshots: [screenshotPath, screenshotPath.replace(/\.png$/, '.published.png')], fixture: 'synthetic host and metadata; no production provider connection', passed: true}));
