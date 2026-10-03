// Actual compiled resource + deterministic host/provider fixture in Chromium.
// The fixture does not issue credentials or establish live host admission.
import assert from 'node:assert/strict';
import {readFile,writeFile,mkdtemp,rm} from 'node:fs/promises';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {once} from 'node:events';
import {cleanupBrowserFixture} from './browser_cleanup.mjs';
import {capturedViews,initializeSyntheticHost} from './browser_fixture.mjs';
const [htmlPath,screenshotPath,mode="mcp"]=process.argv.slice(2);
assert(["mcp","embedded"].includes(mode),"explicit transport mode required");
assert(htmlPath,'provide actual Go-compiled resource HTML');
const html=await readFile(htmlPath,'utf8');
assert(capturedViews?.heading&&capturedViews?.kpi&&capturedViews?.trend&&capturedViews?.table&&capturedViews?.tableNext,'genuine retained fixture outputs required');
assert.equal(capturedViews.kpi.output.chart.kind,'kpi');assert.deepEqual(capturedViews.kpi.output.chart.mapping.kpi.thresholds,[],'ordinary no-threshold KPI is a supported retained shape');
assert.equal(capturedViews.trend.output.chart.kind,'line');assert.equal(capturedViews.trend.output.chart.points.length,2);
assert.equal(capturedViews.table.output.table.rows[0][1].value,'9007199254740993.125');assert.equal(capturedViews.tableNext.output.table.rows[0][1].value,'5.500');
const host=`<!doctype html><meta charset="utf-8"><title>Synthetic report app browser fixture</title><style>body{margin:0}#fixture-label{padding:6px 16px;background:#20372d;color:white;font:12px system-ui,sans-serif}iframe{border:0;width:100%;height:1500px}</style><div id="fixture-label">Synthetic host fixture · KPI, trend and table · No production provider connection</div><iframe id="app" title="Chartworks report app" src="/resource"></iframe><script>(${initializeSyntheticHost.toString()})(${JSON.stringify(capturedViews).replaceAll('<','\\u003c')},${mode==='embedded'});</script>`;
const requests=[];const server=createServer((req,res)=>{requests.push(req.url);if(req.url==='/'){res.writeHead(200,{'Content-Type':'text/html'});res.end(host);}else if(req.url==='/resource'){res.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'});res.end(html);}else{res.writeHead(404);res.end();}});server.listen(0,'127.0.0.1');await once(server,'listening');
if(process.env.CHARTWORKS_COMPONENT_SERVE_ONLY==='1'){console.log('http://127.0.0.1:'+server.address().port);await new Promise(()=>{});}
const directory=await mkdtemp(join(tmpdir(),'chartworks-app-browser-')),errors=[],pending=new Map();let seq=0,socket;
const browser=spawn(process.env.CHARTWORKS_CHROME_BIN||'chromium',['--headless=new','--no-sandbox','--disable-gpu','--disable-dev-shm-usage','--disable-background-networking','--disable-component-update','--no-first-run','--no-default-browser-check','--remote-debugging-port=0','--user-data-dir='+directory,'about:blank'],{stdio:['ignore','ignore','pipe']});let browserStderr='';browser.stderr.on('data',data=>{if(browserStderr.length<16384)browserStderr+=data.toString();});const browserClosed=new Promise(resolve=>browser.once('close',resolve));let browserError;browser.on('error',e=>browserError=e);
const pause=ms=>new Promise(r=>setTimeout(r,ms));async function until(fn,message){const end=Date.now()+15000;while(Date.now()<end){if(browserError)throw browserError;if(browser.exitCode!==null||browser.signalCode!==null)throw new Error('Chromium exited: '+browserStderr);if(await fn())return;await pause(25);}throw new Error(message);}
function rpc(method,params={}){const id=++seq;return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{pending.delete(id);reject(new Error('CDP timeout: '+method));},10000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}));});}
async function evaluate(expression){const r=await rpc('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result?.value;}
const doc="document.getElementById('app').contentDocument",body=`${doc}.getElementById('report-app')`;
async function click(label){await evaluate(`(()=>{const b=Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent===${JSON.stringify(label)}&&!b.disabled);if(!b)throw new Error('Missing enabled button '+${JSON.stringify(label)});for(let p=b.parentElement;p;p=p.parentElement)if(p.tagName==='DETAILS')p.open=true;b.click();})()`);}
async function fill(label,value){await evaluate(`(()=>{const i=Array.from(${body}.querySelectorAll('input,textarea')).find(i=>i.getAttribute('aria-label')===${JSON.stringify(label)});if(!i)throw new Error('Missing field');for(let p=i.parentElement;p;p=p.parentElement)if(p.tagName==='DETAILS')p.open=true;i.value=${JSON.stringify(value)};i.dispatchEvent(new Event('change',{bubbles:true}));})()`);}
async function textHas(text){await until(()=>evaluate(`${body}?.textContent.includes(${JSON.stringify(text)})`),'missing text: '+text);}
async function ready(){await until(()=>evaluate(`${body}&&!Array.from(${body}.querySelectorAll('[role=status]')).some(p=>p.textContent==='Working…')`),'operation did not settle');}
const exactAmount='9,007,199,254,740,993.125 USD revenue';
const exactTotal='9,007,199,254,740,998.625 USD revenue';
const canvas=`${body}.querySelector('.composition-canvas.retained-preview')`;
const output=id=>`${canvas}?.querySelector('section[data-output="${id}"]')`;
const executions="calls.filter(c=>['reporting_run','reporting_authoring_preview_v1','reporting_authoring_execute_v1'].includes(c.name)).length";
async function chooseLayout(value){await evaluate(`(()=>{const s=${body}.querySelector('select[aria-label="Layout"]');if(!s)throw new Error('Missing layout');s.value=${JSON.stringify(value)};s.dispatchEvent(new Event('change',{bubbles:true}));})()`);}
async function pageTable(label,wait=true){await evaluate(`(()=>{const b=Array.from(${output('table-main')}.querySelectorAll('button')).find(b=>b.textContent===${JSON.stringify(label)}&&!b.disabled);if(!b)throw new Error('Missing table page button');b.click();})()`);if(wait)await ready();}
async function checkCanvas(label){
 await check(`(()=>{const c=${canvas};return !!c&&c.querySelectorAll('article[data-widget]').length===4&&c.querySelectorAll('section[data-output]').length===3&&!!c.querySelector('.heading-card')&&!!c.querySelector('[data-output="kpi-main"] .kpi')&&!!c.querySelector('[data-output="trend-main"] svg.chart path.line')&&!!c.querySelector('[data-output="table-main"] table');})()`,label+' renders heading and distinct genuine KPI, line and table outputs');
 await check(`${output('kpi-main')}.querySelector('.kpi').textContent===${JSON.stringify(exactAmount)}&&${output('table-main')}.textContent.includes(${JSON.stringify(exactTotal)})`,label+' preserves exact decimal/currency/unit value and whole-result total');
 await check(`${output('trend-main')}.querySelectorAll('svg.chart circle').length===2&&Array.from(${output('trend-main')}.querySelectorAll('svg.chart circle title')).some(t=>t.textContent.includes(${JSON.stringify(exactAmount)}))`,label+' draws both approved trend observations with exact accessible labels');
 await check(`${output('kpi-main')}.textContent.includes('Synthetic known revenue: incomplete')&&${output('kpi-main')}.textContent.includes('Evidence: reviewed definition')&&${output('kpi-main')}.textContent.includes('Unknown amount count (returned rows): 2')`,label+' preserves carried amount completeness and unknown-count disclosure');
}
async function checkGrid(definition,label){
 await check(`(()=>{const c=${body}.querySelector('.composition-canvas');return ${definition}.widgets.every(w=>{const e=c.querySelector('[data-widget="'+w.id+'"]'),g=w.grid;return e&&e.style.gridColumn===(g.column+1)+' / span '+g.width&&e.style.gridRow===(g.row+1)+' / span '+g.height&&['row','column','width','height'].every(k=>e.dataset[k]===String(g[k]));});})()`,label+' uses each saved row, column, width and height');
 await check(`(()=>{const rects=Array.from(${body}.querySelectorAll('.composition-canvas article[data-widget]'),e=>e.getBoundingClientRect());return rects.every((a,i)=>rects.slice(i+1).every(b=>a.right<=b.left+1||b.right<=a.left+1||a.bottom<=b.top+1||b.bottom<=a.top+1));})()`,label+' places actual widget rectangles without overlap');
}
async function checkRetainedFitsFrame(label){
 const visible=`(()=>{const frame=document.getElementById('app'),d=frame.contentDocument,c=d.querySelector('.composition-canvas.retained-preview'),root=d.getElementById('report-app').getBoundingClientRect();if(!c||!window.resizeCount||performance.now()-window.lastResizeAt<100)return false;const elements=[c,...c.querySelectorAll('article[data-widget],.kpi,svg.chart,[data-output="table-main"] .scroll,[data-output="table-main"] td')];return elements.every(e=>{const b=e.getBoundingClientRect();return b.width>0&&b.height>0&&b.top>=0&&b.bottom<=frame.clientHeight+1&&b.left>=0&&b.right<=frame.clientWidth+1;})&&root.bottom<=frame.clientHeight+1&&d.documentElement.scrollWidth<=frame.clientWidth+1;})()`;
 await until(()=>evaluate(visible),label+' canvas or exact amount is clipped after host resize settles');
 await check(visible,label+' heading, KPI, trend, table and exact cells fit the settled iframe width and height');
 await check("resizeMessages.length>0&&resizeMessages.every(s=>s.width>=200&&s.width<=1600&&s.height>=100&&s.height<=2400)",label+' uses bounded host resize messages');
}
async function captureProof(path){const metrics=await rpc('Page.getLayoutMetrics'),size=metrics.cssContentSize||metrics.contentSize;const proof=await rpc('Page.captureScreenshot',{format:'png',captureBeyondViewport:true,fromSurface:true,clip:{x:0,y:0,width:Math.min(1600,Math.ceil(size.width)),height:Math.min(6000,Math.ceil(size.height)),scale:1}});await writeFile(path,Buffer.from(proof.data,'base64'));}
let checks=0,runError;const assertions=[];async function check(expression,message){assert.equal(await evaluate(expression),true,message);checks++;assertions.push(message);}
try{
 let port;await until(async()=>{try{port=Number((await readFile(join(directory,'DevToolsActivePort'),'utf8')).split('\n')[0]);return port>0;}catch{return false;}},'Chromium not ready');
 const target=await(await fetch(`http://127.0.0.1:${port}/json/new?about:blank`,{method:'PUT'})).json();socket=new WebSocket(target.webSocketDebuggerUrl);await once(socket,'open');socket.addEventListener('message',event=>{const m=JSON.parse(event.data);if(m.id){const p=pending.get(m.id);if(!p)return;clearTimeout(p.timer);pending.delete(m.id);m.error?p.reject(new Error(m.error.message)):p.resolve(m.result);}else if(m.method==='Fetch.requestPaused'){const u=new URL(m.params.request.url);requests.push(u.pathname);const content=u.pathname==='/resource'?html:u.pathname==='/'?host:'';void rpc('Fetch.fulfillRequest',{requestId:m.params.requestId,responseCode:content?200:404,responseHeaders:[{name:'Content-Type',value:'text/html'}],body:Buffer.from(content).toString('base64')});}else if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.exception?.description||m.params.exceptionDetails.text);});
 await rpc('Runtime.enable');await rpc('Page.enable');await rpc('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});await rpc('Page.addScriptToEvaluateOnNewDocument',{source:`window.storageTouches=0;for(const n of ['localStorage','sessionStorage','indexedDB']){Object.defineProperty(window,n,{get(){window.storageTouches++;throw new Error('unexpected browser storage');}});}`});
 if(mode==='embedded')await rpc('Fetch.enable',{patterns:[{urlPattern:'https://report-host.example/*'}]});
 await rpc('Page.navigate',{url:mode==='embedded'?'https://report-host.example/':`http://127.0.0.1:${server.address().port}`});
 await textHas('Weekly operations');await ready();await click('Weekly operations');await textHas('Retained runs');await ready();
 await click('Open retained run');await textHas(exactAmount);await ready();
 await check("calls.every(c=>!['reporting_run','reporting_authoring_execute_v1'].includes(c.name))",'catalog and retained read make zero execution calls');
 await checkCanvas('Consumer');await checkGrid('publishedDefinition','Consumer custom saved grid');
 await check(`(()=>{const c=${canvas},tools=${body}.querySelector('.consumer-tools');return !!tools&&!tools.open&&!!(c.compareDocumentPosition(tools)&Node.DOCUMENT_POSITION_FOLLOWING)&&!${body}.querySelector('select[aria-label="Widget"]')&&!${body}.querySelector('select[aria-label="Output"]');})()`,'Consumer places the complete saved layout before collapsed run tools rather than a per-output picker');
 await click('Close retained view');await fill('Maximum rows','25');await click('Run with these filters');await textHas(exactAmount);await ready();
 await check("calls.find(c=>c.name==='reporting_run').arguments.pages[0].filters[0].value.literal==='25'",'explicit consumer filter run');
 await click('Build');await textHas('Private drafts');await ready();await click('Weekly operations');await textHas('Draft revision 3 loaded.');await ready();
 await fill('Report title','Edited weekly operations');await click('Browse');await textHas('You have unsaved edits.');await click('Keep editing');
 await check(`${body}.querySelector('input[aria-label="Report title"]').value==='Edited weekly operations'`,'cancel navigation preserves edits');
 for(const label of ['Add kpi: Synthetic revenue KPI','Add chart: Synthetic revenue trend','Add table: <img src=x onerror=alert(1)>']){
  await click('Add published output');await textHas('Approved business metrics');await ready();await click('Approved business metrics');await textHas('Add kpi: Synthetic revenue KPI');await ready();
  await check(`!${body}.textContent.includes('Add narrative:')`,'narrative output remains absent from the manual output library');await click(label);
 }
 await check(`${body}.querySelector('img')===null`,'untrusted output title remains inert text');await fill('Widget title','Synthetic revenue detail');
 await click('Choose business filter');await textHas('Add filter: Maximum rows');await ready();await click('Add filter: Maximum rows');await fill('Filter label','Rows shown');await fill('Default value','30');await chooseLayout('2');
 await click('Save report');await textHas('Saved private draft revision 4.');await ready();
 await check("calls.filter(c=>c.name==='reporting_authoring_save_v1').length===1&&calls.find(c=>c.name==='reporting_authoring_save_v1').arguments.expected_version===4",'one explicit report CAS save');
 await check("definition.filters[0].label==='Rows shown'&&definition.filters[0].parameter.default.literal==='30'&&definition.widgets[3].bindings[0].parameter==='maximum'",'typed filter and exact binding saved');
 await check("definition.widgets.length===4&&definition.widgets.slice(1).every((w,i)=>w.block.revision===7&&w.block.outputs.length===1&&w.block.outputs[0]===['kpi-main','trend-main','table-main'][i])",'all three genuine published output pins are preserved');
 await check(`${body}.querySelector('select[aria-label="Layout"]').value==='2'`,'layout selector reflects saved two-column canonical grid');
 await click('Reload latest');await textHas('Draft revision 4 loaded.');await ready();
 await check(`${body}.querySelector('select[aria-label="Layout"]').value==='2'`,'reopening saved two-column report preserves layout selector');
 await evaluate('denyPrivate=true');await click('Private preview');await textHas('Your host must refresh access');await ready();
 await check("calls.filter(c=>c.name==='reporting_authoring_preview_v1').length===1&&calls.filter(c=>c.name==='reporting_authoring_execute_v1').length===1",'private admission executes only once');
 await check(`!${body}.querySelector('.retained-output .kpi,.retained-output svg,.retained-output table')&&!${body}.textContent.includes(${JSON.stringify(exactAmount)})`,'private run read denial exposes no retained values');
 await evaluate("denyPrivate=false;denial='initial-revision'");await click('Inspect last retained run');await textHas('Report action unavailable: stale_validation');await ready();
 await check(`!${body}.querySelector('.retained-output .kpi,.retained-output svg,.retained-output table')&&calls.filter(c=>c.name==='reporting_authoring_execute_v1').length===1`,'initial private read rejects a foreign revision without retaining values or replaying execution');
 await evaluate("denial=''");await click('Inspect last retained run');await textHas('Private preview. This retained result stays private.');await ready();
 await check("calls.filter(c=>c.name==='reporting_authoring_execute_v1').length===1",'authority refresh retry reads without another execution');
 await checkCanvas('Builder private preview');await checkGrid('definition','Builder persisted two-column grid');
 await check(`${body}.querySelector('.preview-provenance').textContent.includes('Values from revision 4 · run private-one')`,'Builder retains the exact private revision/run provenance');
 await click('Synthetic revenue detail');
 await check(`${canvas}.querySelectorAll('.widget-select[aria-pressed="true"]').length===1&&${canvas}.querySelector('.widget-select[aria-pressed="true"]').textContent==='Synthetic revenue detail'&&${body}.querySelectorAll('.inspector').length===1&&${body}.querySelector('.inspector input[aria-label="Widget title"]').value==='Synthetic revenue detail'`,'compact widget selection opens one matching inspector without replacing the canvas');
 await checkRetainedFitsFrame('Builder');
 if(screenshotPath)await captureProof(screenshotPath);
 const beforePaging=await evaluate(executions),beforeViews=await evaluate("calls.filter(c=>c.name==='reporting_view').length");
 await pageTable('Next');await textHas('5.500 USD revenue');await ready();
 await check(`${output('table-main')}.querySelector('tbody').textContent.includes('5.500 USD revenue')&&!${output('table-main')}.querySelector('tbody').textContent.includes(${JSON.stringify(exactAmount)})&&${output('table-main')}.textContent.includes(${JSON.stringify(exactTotal)})`,'table next page has its own exact retained row while preserving whole-result totals');
 await pageTable('Previous');await ready();
 await check(`${output('table-main')}.querySelector('tbody').textContent.includes(${JSON.stringify(exactAmount)})&&${executions}===${beforePaging}&&calls.filter(c=>c.name==='reporting_view').length===${beforeViews+2}`,'table paging only reads the exact selected output, with zero new executions');
 const beforeRedraw=await evaluate('calls.length');
 await chooseLayout('3');await chooseLayout('2');await fill('Widget title','Synthetic detail, locally retitled');await click('Synthetic weekly overview');await fill('Heading text','Synthetic weekly overview, locally edited');
 await check(`${canvas}.textContent.includes('Synthetic detail, locally retitled')&&${canvas}.textContent.includes('Synthetic weekly overview, locally edited')&&${output('kpi-main')}.querySelector('.kpi').textContent===${JSON.stringify(exactAmount)}&&calls.length===${beforeRedraw}`,'layout, heading and presentation redraw reuse exact retained values without new reads or executions');
 await check(`${body}.querySelector('.preview-provenance').textContent.includes('Values from revision 4 · run private-one')`,'local presentation changes do not relabel the retained revision');
 await fill('Default value','31');await textHas('Preview is stale. Save and preview to update retained values.');
 await check(`!${canvas}.querySelector('.kpi,svg,table')&&!${canvas}.textContent.includes(${JSON.stringify(exactAmount)})&&calls.length===${beforeRedraw}`,'semantic filter edit hides all stale retained values and never autoqueries');
 await fill('Default value','30');await textHas(exactAmount);
 await check(`!!${canvas}.querySelector('.kpi')&&calls.length===${beforeRedraw}`,'returning to exact admitted semantics can reuse the same disposable retained values');
 await click('Close retained view');
 await check(`Array.from(${body}.querySelectorAll('button')).some(b=>b.textContent==='Private preview'&&!b.disabled)===false`,'unsaved presentation edits still require an explicit save before another preview');
 await click('Reload latest');await textHas('You have unsaved edits.');await click('Discard and continue');await textHas('Draft revision 4 loaded.');await ready();
 await check(`Array.from(${body}.querySelectorAll('button')).some(b=>b.textContent==='Private preview'&&!b.disabled)`,'terminal private read releases only the resolved execution fence');
 await fill('Report title','Unsaved conflict title');await evaluate('rejectSave=true');await click('Save report');await textHas('This draft changed elsewhere.');await ready();
 await check(`${body}.querySelector('input[aria-label="Report title"]').value==='Unsaved conflict title'`,'conflict preserves local buffer');await click('Reload latest');await textHas('You have unsaved edits.');await click('Discard and continue');await textHas('Draft revision 4 loaded.');await ready();
 await check(`${body}.querySelector('input[aria-label="Report title"]').value==='Edited weekly operations'`,'reload restores authoritative latest');
 await fill('Report title','Weekly operations');await evaluate('saveDelay=300');
 await evaluate(`(()=>{const b=Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Save report');b.click();b.click();Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Browse').click();})()`);await textHas('Saved private draft revision 5.');await ready();
 await check("calls.filter(c=>c.name==='reporting_authoring_save_v1').length===3",'double save emits one mutation and pending navigation stays fenced');
 await check(`${body}.textContent.includes('Private drafts')`,'pending save cannot navigate away');
 // A separately persisted custom grid must be displayed as custom on reopen;
 // selecting a preset is never an implicit save or an implicit rewrite on read.
 await evaluate('definition.widgets.forEach((w,i)=>w.grid=structuredClone(publishedDefinition.widgets[i].grid));state={...state,version:state.version+1,draft_revision:state.draft_revision+1}');await click('Reload latest');await textHas('Draft revision 6 loaded.');await ready();
 await check(`${body}.querySelector('select[aria-label="Layout"]').value==='custom'&&calls.filter(c=>c.name==='reporting_authoring_save_v1').length===3`,'reopening a persisted custom grid preserves custom layout and performs no implicit save');
 await checkGrid('definition','Builder reopened custom grid');
 await click('New report');await textHas('Proposed report ID');await fill('Proposed report ID','new-report');await click('Check access and start');await textHas('This new draft is only in this window');await ready();await click('Save report');await textHas('Saved private draft revision 1.');await ready();
 await check("calls.filter(c=>c.name==='reporting_authoring_create_v1').length===1&&calls.find(c=>c.name==='reporting_authoring_create_v1').arguments.id==='new-report'",'new report checks exact proposed target and saves once');
 await fill('Report title','Delayed close');await evaluate('saveDelay=400');await click('Save report');await evaluate('closeApp()');await textHas('This report app is closed.');await pause(500);
 await check(`${body}.textContent==='This report app is closed. Reopen it through your authorized host.'`,'close fences delayed save result and clears private content');
 await evaluate('consumerOnly=true;document.getElementById("app").src="/resource"');await textHas('Weekly operations');await ready();
 await check(`Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Build').disabled`,'consumer cannot enter builder');await click('Weekly operations');await textHas('Current access permits retained reads only.');await ready();await click('Open retained run');await textHas(exactAmount);await ready();
 await checkCanvas('Read-only Consumer');await checkGrid('publishedDefinition','Read-only Consumer saved custom grid');
 await check(`!${body}.querySelector('.retained-preview fieldset')`,'shared retained renderer respects read-only mode');
 await check(`document.getElementById('app').contentWindow.storageTouches===0`,'resource uses no browser storage');await checkRetainedFitsFrame('Consumer');
 if(screenshotPath)await captureProof(screenshotPath.replace(/\.png$/,'.consumer.png'));
 const beforeDenials=await evaluate(executions);
 for(const denial of ['target','run','context']){
  await evaluate(`denial=${JSON.stringify(denial)}`);await pageTable('Next');await textHas('Report action unavailable:');await ready();
  await check(`!${body}.querySelector('.retained-output .kpi,.retained-output svg,.retained-output table')&&!${body}.textContent.includes(${JSON.stringify(exactAmount)})&&${executions}===${beforeDenials}`,'retained '+denial+' mismatch or typed denial clears the entire value buffer without executing');
  await evaluate("denial=''");await click('Open retained run');await textHas(exactAmount);await ready();
 }
 const beforeWrongTarget=await evaluate("calls.filter(c=>c.name==='reporting_view').length");
 await evaluate("denial='initial-target'");await click('Open retained run');await textHas('Report action unavailable: stale_validation');await ready();
 await check(`!${body}.querySelector('.retained-output .kpi,.retained-output svg,.retained-output table')&&calls.filter(c=>c.name==='reporting_view').length===${beforeWrongTarget+1}&&${executions}===${beforeDenials}`,'initial retained read rejects another report before output fanout and clears preceding values');
 await evaluate("denial=''");await click('Open retained run');await textHas(exactAmount);await ready();
 await evaluate('viewDelay=400');const beforeDelayedRead=await evaluate('calls.length');
 await pageTable('Next',false);await until(()=>evaluate(`calls.length>${beforeDelayedRead}`),'delayed retained page request did not reach the host');await evaluate('closeApp()');await textHas('This report app is closed.');await pause(500);
 await check(`${body}.textContent==='This report app is closed. Reopen it through your authorized host.'&&${executions}===${beforeDenials}`,'close fences a delayed retained page reply and removes all report values');
 assert.deepEqual(errors,[],'no browser exceptions');assert(requests.every(path=>['/','/resource','/favicon.ico'].includes(path)),'no remote or injected asset requests');
}catch(error){runError=error;if(socket&&screenshotPath){try{await captureProof(screenshotPath.replace(/\.png$/,'.failure.png'));}catch{}}let failureState;try{failureState=socket?await evaluate(`({text:${body}?.textContent.slice(0,16000),calls:calls.slice(-12)})`):undefined;}catch{}console.error(JSON.stringify({mode,checks,assertions,browserErrors:errors,browserStderr,failureState},null,2));throw error;}finally{
 try{
  await cleanupBrowserFixture({browser,closed:browserClosed,directory,
   requestClose:()=>socket?.readyState===WebSocket.OPEN?rpc('Browser.close'):undefined,
   closeTransport:()=>{for(const p of pending.values()){clearTimeout(p.timer);p.reject(new Error('test closed'));}pending.clear();socket?.close();},
   closeServer:()=>new Promise((resolve,reject)=>server.close(error=>error?reject(error):resolve())),removeProfile:rm});
 }catch(cleanupError){if(runError)throw new AggregateError([runError,cleanupError],'Browser assertions and fixture cleanup failed');throw cleanupError;}
}
console.log(JSON.stringify({mode,checks,assertions,requests,fixture:'synthetic host; genuine retained output shapes',passed:true}));
