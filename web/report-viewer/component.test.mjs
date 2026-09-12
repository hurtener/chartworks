/* Runs the actual bundled resource in Chromium. No npm dependency, mocked DOM,
 * host qualification, external service or credential is used. Go acceptance
 * writes real charts.Build fixtures and invokes this component runner. */
import assert from 'node:assert/strict';
import {readFile,writeFile,mkdtemp,rm} from 'node:fs/promises';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {once} from 'node:events';

const [htmlPath,fixturePath,suite='all'] = process.argv.slice(2);
assert(htmlPath && fixturePath,'actual resource and provider fixtures required');
const html=await readFile(htmlPath,'utf8'), fixtures=JSON.parse(await readFile(fixturePath,'utf8'));
const fixtureWire=JSON.stringify(fixtures).replaceAll('<','\\u003c');
const errors=[],network=[]; let passes=0;
const host = `<!doctype html><meta charset="utf-8"><title>Chartworks component fixture</title><iframe id="viewer" title="Actual reporting app" src="/resource" width="900" height="950"></iframe><script>
window.fixture=${fixtureWire};window.calls=[];window.notifications=[];window.resultOverride=null;window.hostReady=false;
const frame=document.getElementById('viewer');
const copy=v=>JSON.parse(JSON.stringify(v));
const send=m=>frame.contentWindow.postMessage(m,location.origin);
const wrapped=v=>({structuredContent:{result:copy(v)},content:[{type:'text',text:JSON.stringify({result:v})}]});
window.show=v=>{window.current=copy(v);send({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:wrapped(v)});};
window.makeView=(out,tag)=>{const v=copy(fixture.view);v.summary.target.id=tag;v.selection.output=out.kind;v.outputs=[{id:out.kind,kind:out.kind,title:out.kind}];v.output={id:out.kind,kind:out.kind,state:'succeeded',code:'',retained_digest:'fixture'};if(out.kind==='table'){v.output.table={columns:out.columns,rows:out.rows,totals:out.totals,completeness:out.completeness,warnings:out.warnings};v.page_bounds={offset:0,limit:100,total:out.rows.length};}else{v.output.chart=out;v.page_bounds={offset:0,limit:100,total:out.points.length};}v.filters=[];return v;};
window.chartCase=i=>{const c=fixture.cases[i];show(makeView(c.output,'case-'+i));};
window.context=c=>send({jsonrpc:'2.0',method:'ui/notifications/host-context-changed',params:c});
window.addEventListener('message',e=>{
 if(e.source!==frame.contentWindow||e.origin!==location.origin)return;
 const m=e.data;if(m?.jsonrpc!=='2.0')return;
 if(m.method==='ui/initialize'){send({jsonrpc:'2.0',id:m.id,result:{protocolVersion:'2026-01-26',hostInfo:{name:'deterministic fixture',version:'1'},hostCapabilities:{serverTools:{}},hostContext:{theme:'light',locale:'en-US',containerDimensions:{maxWidth:900}}}});return;}
 if(m.method==='ui/notifications/initialized'){window.hostReady=true;show(fixture.view);return;}
 if(m.method==='ui/notifications/size-changed'){notifications.push(copy(m.params));return;}
 if(m.method==='tools/call'){
   calls.push(copy(m.params));let result;
   if(window.resultOverride){result=window.resultOverride;window.resultOverride=null;}
   else if(m.params.name==='reporting_describe')result=wrapped(fixture.description);
   else if(m.params.name==='reporting_run')result=wrapped(fixture.run);
   else if(m.params.name==='reporting_view'){
     const v=copy(fixture.view),a=m.params.arguments;v.selection={...v.selection,...a};v.summary.run=a.run;
     if(a.output==='table-second'){v.output.id='table-second';}
     const offset=a.offset||0,limit=a.limit||2,total=fixture.table.rows.length,end=Math.min(offset+limit,total);
     v.output.table.rows=fixture.table.rows.slice(offset,end);v.page_bounds={offset,limit,total};if(end<total)v.page_bounds.next=end;result=wrapped(v);
   }else result={isError:true,structuredContent:{error:{code:'forbidden',outcome:'not_started'}},content:[]};
   setTimeout(()=>send({jsonrpc:'2.0',id:m.id,result}),15);return;
 }
 if(m.id!==undefined&&m.result!==undefined)window.lastReply=copy(m);
 if(m.id!==undefined&&m.error!==undefined)window.lastReply=copy(m);
});
window.sendRaw=send;
</script>`;
const server=createServer((req,res)=>{
  network.push(req.url);
  if(req.url==='/resource'){res.writeHead(200,{'Content-Type':'text/html; charset=utf-8','Cache-Control':'no-store'});res.end(html);}
  else if(req.url==='/'){res.writeHead(200,{'Content-Type':'text/html; charset=utf-8'});res.end(host);}
  else if(req.url==='/spoof'){res.writeHead(200,{'Content-Type':'text/html'});res.end(`<script>const v=JSON.parse(JSON.stringify(parent.fixture.view));v.summary.target.id='spoofed';parent.document.getElementById('viewer').contentWindow.postMessage({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:{structuredContent:{result:v}}},location.origin);parent.spoofSent=true;</script>`);}
  else if(req.url==='/favicon.ico'){res.writeHead(204);res.end();}
  else{res.writeHead(404);res.end('unexpected fixture request');}
});
server.listen(0,'127.0.0.1');await once(server,'listening');
const origin=`http://127.0.0.1:${server.address().port}`;
const directory=await mkdtemp(join(tmpdir(),'chartworks-viewer-'));
const command=process.env.CHARTWORKS_CHROME_BIN||'google-chrome';
const browser=spawn(command,['--headless=new','--no-sandbox','--disable-gpu','--disable-dev-shm-usage','--disable-background-networking','--disable-component-update','--no-first-run','--no-default-browser-check','--remote-debugging-port=0','--remote-allow-origins=http://localhost','--user-data-dir='+directory,'about:blank'],{stdio:['ignore','ignore','pipe']});
let browserError;browser.on('error',e=>{browserError=e;});browser.stderr.on('data',b=>{if(errors.join('').length<65536)errors.push(b.toString());});
let socket,session,seq=0;const pending=new Map();
const pause=ms=>new Promise(r=>setTimeout(r,ms));
const until=async(fn,message,ms=15000)=>{const end=Date.now()+ms;while(Date.now()<end){if(browserError)throw browserError;try{if(await fn())return;}catch(e){if(Date.now()+100>=end)throw e;}await pause(35);}throw new Error(message);};
function rpc(method,params={}){const id=++seq;return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{pending.delete(id);reject(new Error('CDP timeout: '+method));},10000);pending.set(id,{resolve,reject,timer});socket.send(JSON.stringify({id,method,params}));});}
async function evaluate(expression){const r=await rpc('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(r.exceptionDetails.exception?.description||r.exceptionDetails.text);return r.result?.value;}
async function check(expression,message){assert.equal(await evaluate(expression),true,message);passes++;}
const doc=`document.getElementById('viewer').contentDocument`;
const body=`${doc}.getElementById('report-viewer')`;
const waitTitle=title=>until(()=>evaluate(`${body}?.querySelector('h1')?.textContent===${JSON.stringify(title)}`),'render did not finish: '+title);
const waitCalls=n=>until(()=>evaluate(`calls.length>=${n}`),'bridge request missing');
const restore=async()=>{await evaluate('show(fixture.view)');await waitTitle(fixtures.view.summary.target.id);};
try{
  let port;
  await until(async()=>{try{port=Number((await readFile(join(directory,'DevToolsActivePort'),'utf8')).split('\n')[0]);return port>0;}catch{return false;}},'browser did not expose DevTools');
  const target=await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`,{method:'PUT'})).json();
  socket=new WebSocket(target.webSocketDebuggerUrl);await once(socket,'open');
  socket.addEventListener('message',event=>{const m=JSON.parse(event.data);if(m.id){const p=pending.get(m.id);if(!p)return;pending.delete(m.id);clearTimeout(p.timer);m.error?p.reject(new Error(m.error.message)):p.resolve(m.result);}else if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);});
  await rpc('Page.enable');await rpc('Runtime.enable');
  await rpc('Page.addScriptToEvaluateOnNewDocument',{source:`window.storageTouches=0;window.networkTouches=0;for(const n of ['localStorage','sessionStorage','indexedDB']){try{Object.defineProperty(window,n,{get(){window.storageTouches++;throw new Error('unexpected storage access');}});}catch{}}window.fetch=()=>{window.networkTouches++;throw new Error('unexpected component fetch');};XMLHttpRequest.prototype.open=function(){window.networkTouches++;throw new Error('unexpected component XHR');};`});
  await rpc('Page.navigate',{url:origin});
  await waitTitle(fixtures.view.summary.target.id);
  await check('hostReady===true','established initialization completed');
  await check('calls.length===0','initial retained response did not call a tool');
  await check(`${doc}.querySelector('script[src]')===null&&${doc}.querySelector('link')===null`,'bundle has no remote assets');

  if(suite==='all'||suite==='charts'){
    const seen=new Set();
    for(let i=0;i<fixtures.cases.length;i++){
      const c=fixtures.cases[i];if(c.error){assert.equal(c.error,'unsuitable_binding');continue;}
      seen.add(c.kind);await evaluate(`chartCase(${i})`);await waitTitle('case-'+i);
      if(c.output.state!=='ready'){await check(`${body}.querySelector('.notice')!==null&&${body}.querySelector('svg')===null`,'empty/no-values state '+c.kind+'/'+c.scenario);continue;}
      if(c.kind==='kpi')await check(`${body}.querySelector('.kpi')!==null`,'actual KPI');
      else if(c.kind==='table')await check(`${body}.querySelector('table')!==null&&${body}.querySelector('svg')===null`,'actual table');
      else{
        await check(`${body}.querySelector('svg.chart')?.getBBox().width>0`,'actual geometry '+c.kind+'/'+c.scenario);
        const glyph=['bar','column','grouped_bar','stacked_bar','stacked_column','heatmap','treemap'].includes(c.kind)?'rect':c.kind==='scatter'?'circle':['line','area'].includes(c.kind)?'path':'path,circle';
        await check(`${body}.querySelectorAll('svg.chart ${glyph}').length>0`,'kind-specific glyph '+c.kind+'/'+c.scenario);
      }
      const expected=c.expected_rows;
      if(expected.length){
        const rows=await evaluate(`Array.from(${body}.querySelectorAll('tbody tr'),r=>Array.from(r.querySelectorAll('td'),c=>c.textContent))`);
        assert.equal(rows.length,Math.min(100,expected.length),'exact row count '+c.kind+'/'+c.scenario);
        for(let r=0;r<rows.length;r++)for(let k=0;k<expected[r].length;k++){
          if(expected[r][k]===null)assert.equal(rows[r][k],'Missing','null became a value');
          else assert(rows[r][k].includes(expected[r][k]),`exact cell changed ${c.kind}/${c.scenario} [${r},${k}]: ${rows[r][k]} != ${expected[r][k]}`);
        }
        passes++;
      }
      for(const column of c.output.columns)await check(`${body}.textContent.includes(${JSON.stringify(column.name)})`,'exact full label '+c.kind+'/'+c.scenario);
    }
    assert.equal(seen.size,14,'all fourteen actual kinds');passes++;
    await evaluate(`show(makeView(fixture.precision,'precision'))`);await waitTitle('precision');
    await check(`${body}.textContent.includes('9007199254740993.0100')`,'large exact decimal');
    await evaluate(`show(makeView(fixture.percent,'percent'))`);await waitTitle('percent');
    await check(`${body}.textContent.includes('12.3456789123456789%')`,'fraction percentage without floating-point rounding');
    await check('calls.length===0','chart interpretation and local redraw never execute');
  }

  if(suite==='all'||suite==='interaction'){
    await restore();const before=await evaluate('calls.length');
    await evaluate(`Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Next').click()`);await waitCalls(before+1);await until(()=>evaluate(`${body}.textContent.includes('3–3 / 3')`),'exact table continuation');
    await check(`calls.at(-1).name==='reporting_view'&&calls.at(-1).arguments.offset===2`,'paging is an explicit retained read');
    await check(`Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Next').disabled`,'last page has no invented continuation');
    await check(`calls.every(c=>c.name!=='reporting_run')`,'paging did not execute');
    await evaluate(`const s=${body}.querySelector('select[aria-label="Output"]');s.value='table-second';s.dispatchEvent(new Event('change'));`);await waitCalls(before+2);await waitTitle(fixtures.view.summary.target.id);
    await check(`calls.at(-1).arguments.output==='table-second'&&calls.at(-1).arguments.offset===0`,'output navigation resets table cursor');
    await evaluate(`context({theme:'dark',locale:'es-AR',styles:{variables:{'--color-text-primary':'url(/attack)'},css:{fonts:'@import url(/attack);body{display:none}'}}})`);
    await until(()=>evaluate(`${doc}.documentElement.dataset.theme==='dark'`),'dark theme');
    await check(`${doc}.documentElement.lang==='es-AR'&&${body}.textContent.includes('Siguiente')`,'locale changed UI, not retained values');
    await check(`${body}.querySelector('table')?.getBoundingClientRect().height>0`,'host CSS is not executable');
    await check(`notifications.length>0&&notifications.every(n=>n.width>=200&&n.width<=1600&&n.height>=100&&n.height<=2400)`,'bounded real resize notifications');
    await check(`Array.from(${body}.querySelectorAll('select')).every(s=>s.getAttribute('aria-label'))`,'named native controls');
    await check(`Array.from(${body}.querySelectorAll('button')).every(b=>b.getBoundingClientRect().height>=44)`,'minimum hit targets');
    await evaluate(`${body}.querySelector('select').focus()`);await rpc('Input.dispatchKeyEvent',{type:'keyDown',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});await rpc('Input.dispatchKeyEvent',{type:'keyUp',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});
    await check(`${doc}.activeElement.tagName!=='BODY'`,'keyboard navigation');
    await evaluate(`context({theme:'light',locale:'en-US'});show(fixture.view)`);await waitTitle(fixtures.view.summary.target.id);
    await evaluate(`const d=Array.from(${body}.querySelectorAll('details')).find(d=>d.textContent.includes('Run with different filters'));d.open=true;const useDefault=d.querySelector('input[type=checkbox]');useDefault.checked=false;useDefault.dispatchEvent(new Event('change'));const input=d.querySelector('input[type=text]');input.value='2';input.dispatchEvent(new Event('input',{bubbles:true}));`);
    await check(`calls.every(c=>c.name!=='reporting_run')`,'editing a filter is not execution');
    const n=await evaluate('calls.length');await evaluate(`Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Run with these filters').click()`);await waitCalls(n+3);await waitTitle(fixtures.view.summary.target.id);
    await check(`calls.filter(c=>c.name==='reporting_run').length===1`,'one explicit cost-bearing invocation');
    await check(`calls.filter(c=>c.name==='reporting_run')[0].arguments.arguments[0].value.literal==='2'`,'typed exact filter value');
    await check(`calls.filter(c=>c.name==='reporting_run')[0].arguments.target.revision===fixture.description.resource.target.revision`,'published revision chosen explicitly');
  }

  if(suite==='all'||suite==='security'){
    await restore();await evaluate(`const a=JSON.parse(JSON.stringify(fixture.view));a.summary.target.id='inert-label';a.output.table.columns[0].name='<img src=/attack onerror=parent.hacked=true>';a.output.table.rows[0][0].value='<script>parent.hacked=true</'+'script>';show(a);`);await waitTitle('inert-label');
    await check(`${body}.textContent.includes('<img src=/attack')&&${body}.querySelector('img')===null&&window.hacked!==true`,'labels and values are inert text');
    await evaluate(`const n=JSON.parse(JSON.stringify(fixture.view));n.summary.target.id='inert-narrative';n.output={id:'narrative',kind:'narrative',state:'succeeded',code:'',retained_digest:'fixture',narrative:{text:'<svg onload=parent.hacked=true>https://example.invalid/private</svg>',caveats:['<a href=/attack>not a link</a>']}};show(n);`);await waitTitle('inert-narrative');
    await check(`${body}.querySelector('svg,a,img,iframe')===null&&window.hacked!==true`,'narrative has no executable markup or auto-links');
    await restore();await evaluate(`const f=document.createElement('iframe');f.src='/spoof';document.body.append(f);`);await until(()=>evaluate('window.spoofSent===true'),'spoof fixture did not run');
    await check(`${body}.querySelector('h1').textContent!== 'spoofed'`,'exact parent source is required');
    await evaluate(`sendRaw({jsonrpc:'2.0',id:80001,method:'tools/call',params:{name:'arbitrary_code'}})`);await until(()=>evaluate('window.lastReply?.id===80001'),'unknown bridge method reply');
    await check(`lastReply.error.code===-32601&&!calls.some(c=>c.name==='arbitrary_code')`,'host instructions cannot invent outbound tools');
    await evaluate(`window.resultOverride={isError:true,structuredContent:{error:{code:'forbidden',outcome:'not_started'}},content:[]};Array.from(${body}.querySelectorAll('button')).find(b=>b.textContent==='Next').click()`);
    await until(()=>evaluate(`${body}.textContent.includes('forbidden')`),'denial state');
    await check(`${body}.querySelector('table,svg')===null&&!${body}.textContent.includes('Beta')`,'denial clears prior values and totals');
    await restore();await evaluate(`const e=JSON.parse(JSON.stringify(fixture.view));e.summary.state='expired';e.summary.expires_at='2000-01-01T00:00:00Z';show(e);`);await until(()=>evaluate(`${body}.textContent.includes('expired')`),'expiry state');
    await check(`${body}.querySelector('table,svg')===null`,'expired payload never renders values');
    await restore();await evaluate(`const p=JSON.parse(JSON.stringify(fixture.view));p.summary.private=true;p.summary.state='partial';p.summary.target.id='private-partial';show(p);`);await waitTitle('private-partial');
    await check(`${body}.textContent.includes('Private preview')&&${body}.textContent.includes('Partial result')&&!${body}.textContent.includes('Run with different filters')`,'private partial state remains read-only');
    await restore();await evaluate(`sendRaw({jsonrpc:'2.0',method:'ui/notifications/tool-result',params:{oversized:'X'.repeat(17*1024*1024)}})`);await until(()=>evaluate(`${body}.textContent.includes('limit_exceeded')`),'oversized message bound');
    await check(`${body}.querySelector('table,svg')===null`,'oversized response clears old analytical state');
    await restore();await evaluate(`sendRaw({jsonrpc:'2.0',id:90001,method:'ui/resource-teardown'})`);await until(()=>evaluate('window.lastReply?.id===90001'),'teardown response');
    await check(`${body}.children.length===0`,'teardown erases values and observers');
  }
  await check(`document.getElementById('viewer').contentWindow.storageTouches===0&&document.getElementById('viewer').contentWindow.networkTouches===0`,'no browser state or network bypass');
  assert(!network.some(p=>!['/','/resource','/favicon.ico','/spoof'].includes(p)),'unexpected network request: '+network.join(','));
  assert(passes>=10,'component suite asserted too little');
  console.log(JSON.stringify({suite,passes,kinds:suite==='charts'||suite==='all'?14:undefined,engine:'Chromium actual bundled resource'}));
}catch(e){
  try{await writeFile(join(directory,'failure.json'),JSON.stringify({message:e.message,errors:errors.slice(-10),dom:await evaluate(`${body}?.textContent`)}));}catch{}
  console.error(e.stack||e.message);try{console.error('COMPONENT_DOM='+String(await evaluate(`${body}?.textContent`)).slice(0,6000));}catch{}console.error(errors.slice(-6).join('').slice(-4000));process.exitCode=1;
}finally{
  if(socket)socket.close();for(const p of pending.values()){clearTimeout(p.timer);p.reject(new Error('test shutdown'));}pending.clear();
  browser.kill('SIGTERM');await Promise.race([once(browser,'exit'),pause(2000)]);if(browser.exitCode===null)browser.kill('SIGKILL');
  server.closeAllConnections();server.close();await rm(directory,{recursive:true,force:true,maxRetries:3}).catch(()=>{});
}
