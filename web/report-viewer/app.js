import {VERSION, KINDS, boundedJSON, exact, renderChart, amountDisclosureLines, validateRetainedView, renderRetainedOutput, viewerInternals} from './presentation.js';
export {VERSION, KINDS, boundedJSON, exact, renderChart, amountDisclosureLines, validateRetainedView, renderRetainedOutput} from './presentation.js';
const {MAX_MESSAGE, MAX_DATA, TOOLS, ERRORS, words, fail, text, array, id, integer, element, button, retainedRunOutputs}=viewerInternals;

// A deliberately small implementation of the established MCP Apps postMessage
// bridge. It only talks to its exact parent and pins the initialization origin.
export class Bridge {
  constructor(win=window){this.win=win;this.parent=win.parent;this.pending=new Map();this.sequence=0;this.origin=null;this.ready=false;this.closed=false;this.onresult=()=>{};this.oncontext=()=>{};this.oninput=()=>{};this.onfailure=()=>{};this.onclose=()=>{};this.listener=e=>this.receive(e);win.addEventListener('message',this.listener);}
  send(value){if(this.closed)throw fail('unavailable');boundedJSON(value,MAX_MESSAGE);this.parent.postMessage(value,this.origin&&this.origin!=='null'?this.origin:'*');}
  request(method,params){if(this.closed||this.pending.size>=16)return Promise.reject(fail('busy'));const id=++this.sequence;return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{this.pending.delete(id);reject(fail('cancelled_or_timed_out'));},65000);this.pending.set(id,{resolve,reject,timer,method});try{this.send({jsonrpc:'2.0',id,method,params});}catch(e){clearTimeout(timer);this.pending.delete(id);reject(e);}});}
  async connect(){if(this.parent===this.win)throw fail('unavailable');const result=await this.request('ui/initialize',{protocolVersion:'2026-01-26',appInfo:{name:'Chartworks reporting',version:'1'},appCapabilities:{availableDisplayModes:['inline','fullscreen']}});if(result?.protocolVersion!=='2026-01-26')throw fail('unavailable');this.ready=true;this.capabilities=result.hostCapabilities||{};this.oncontext(result.hostContext||{});this.send({jsonrpc:'2.0',method:'ui/notifications/initialized'});return result;}
  async call(name,args){if(!this.ready||!TOOLS.has(name)||!this.capabilities.serverTools)throw fail('forbidden');boundedJSON(args,65536);return this.request('tools/call',{name,arguments:args});}
  receive(event){
    if(this.closed||event.source!==this.parent||this.origin!==null&&event.origin!==this.origin)return;
    try{
      const m=boundedJSON(event.data,MAX_MESSAGE);if(m?.jsonrpc!=='2.0')return;
      if(m.id!==undefined&&(Object.hasOwn(m,'result')||Object.hasOwn(m,'error'))){const p=this.pending.get(m.id);if(!p)return;if(this.origin===null){if(p.method!=='ui/initialize')return;this.origin=event.origin;}this.pending.delete(m.id);clearTimeout(p.timer);if(m.error)p.reject(fail('unavailable'));else p.resolve(m.result);return;}
      if(!this.ready)return;
      if(m.method==='ui/notifications/tool-result')this.onresult(m.params);
      else if(m.method==='ui/notifications/host-context-changed')this.oncontext(m.params);
      else if(m.method==='ui/notifications/tool-input')this.oninput(m.params);
      else if(m.method==='ui/resource-teardown'&&m.id!==undefined){this.send({jsonrpc:'2.0',id:m.id,result:{}});this.close();}
      else if(m.method==='ping'&&m.id!==undefined)this.send({jsonrpc:'2.0',id:m.id,result:{}});
      else if(m.id!==undefined)this.send({jsonrpc:'2.0',id:m.id,error:{code:-32601,message:'not_found'}});
    }catch(e){this.onfailure(fail(e.code));}
  }
  resize(width,height){if(this.ready&&!this.closed&&Number.isFinite(width)&&Number.isFinite(height))this.send({jsonrpc:'2.0',method:'ui/notifications/size-changed',params:{width:Math.min(1600,Math.max(200,Math.ceil(width))),height:Math.min(2400,Math.max(100,Math.ceil(height)))}});}
  close(){if(this.closed)return;this.closed=true;this.win.removeEventListener('message',this.listener);for(const p of this.pending.values()){clearTimeout(p.timer);p.reject(fail('unavailable'));}this.pending.clear();this.onclose();}
}

function unwrap(result){
  boundedJSON(result,MAX_MESSAGE);if(result?.isError){let code='unavailable';const f=result.structuredContent?.error;if(ERRORS.has(f?.code))code=f.code;const err=fail(code);err.unknown=f?.outcome==='unknown';throw err;}
  let body=result?.structuredContent;
  if(!body){const t=array(result?.content).find(c=>c.type==='text');if(!t||typeof t.text!=='string'||t.text.length>MAX_DATA)throw fail('unavailable');try{body=JSON.parse(t.text);}catch{throw fail('unavailable');}}
  if(body?.error)throw fail(body.error.code);
  const value=body?.result;boundedJSON(value);if(value?.version!==VERSION)throw fail('unavailable');return value;
}
function choiceLabel(item,locale) {
  const labels=array(item.metadata), language=(locale||'en').split('-')[0].toLowerCase();
  const label=labels.find(m=>m.locale===locale)||labels.find(m=>text(m.locale).split('-')[0].toLowerCase()===language);
  return text(label?.display_name)||text(item.title)||text(item.id);
}
function selectControl(label,items,current,onchange,locale='en') {
  const l=element('label',label),s=element('select'),w=words[locale.toLowerCase().startsWith('es')?'es':'en'];
  s.setAttribute('aria-label',label);
  for(const item of items){
    const disabled=item.enabled===false,omitted=item.selected===false;
    const suffix=disabled?w.disabled:omitted?w.omitted:'';
    const o=element('option',choiceLabel(item,locale)+(suffix?' — '+suffix:''));
    o.value=item.id;o.selected=item.id===current;o.disabled=disabled||omitted;
    if(item.description)o.title=text(item.description);
    s.append(o);
  }
  s.addEventListener('change',()=>{const chosen=s.selectedOptions[0];if(chosen&&!chosen.disabled)onchange(s.value);});
  l.append(s);return l;
}
function periodValue(raw){let p;try{p=JSON.parse(raw);}catch{throw fail('invalid_request');}boundedJSON(p,4096);const keys=new Set(['mode','unit','count','start','end','from_date','first_occurrence','dst_policy','month_policy']);if(!p||Array.isArray(p)||Object.keys(p).some(k=>!keys.has(k)))throw fail('invalid_request');return {period:p};}

export class Viewer {
  constructor(root,bridge,options={}){this.allowRun=options.allowRun!==false;this.allowGenerated=options.allowGenerated!==false;this.root=root;this.bridge=bridge;this.locale='en';this.value=null;this.generation=0;this.mutationPending=false;this.timer=null;this.closed=false;this.lastSize='';bridge.onresult=result=>this.accept(result);bridge.oninput=()=>this.loading();bridge.oncontext=context=>this.context(context);bridge.onfailure=e=>this.error(e);bridge.onclose=()=>this.close();this.observer=typeof ResizeObserver==='function'?new ResizeObserver(()=>{const box=root.getBoundingClientRect(),key=Math.ceil(box.width)+':'+Math.ceil(box.height);if(key!==this.lastSize){this.lastSize=key;bridge.resize(box.width,box.height);}}):null;this.observer?.observe(root);this.loading();}
  get w(){return words[this.locale];}
  clear(){clearTimeout(this.timer);this.timer=null;this.value=null;this.root.replaceChildren();}
  loading(){this.generation++;this.clear();const p=element('p',this.w.loading,'notice');p.setAttribute('role','status');this.root.append(p);}
  error(e,mutation=false){this.generation++;this.clear();const p=element('p',`${this.w.error}: ${ERRORS.has(e?.code)?e.code:'unavailable'}`,'notice error');p.setAttribute('role','alert');this.root.append(p);if(mutation||e?.unknown)this.root.append(element('p',this.w.unknown));}
  context(c){try{boundedJSON(c,65536);document.documentElement.dataset.theme=c?.theme==='dark'?'dark':'light';if(typeof c?.locale==='string'&&c.locale.length<=64){try{const canonical=Intl.getCanonicalLocales(c.locale)[0];document.documentElement.lang=canonical;this.locale=canonical.toLowerCase().startsWith('es')?'es':'en';}catch{/* Ignore malformed optional host locale. */}}if(this.value)this.draw();}catch(e){this.error(e);}}
  accept(result){if(this.closed)return;try{const v=unwrap(result);if(v.selection&&v.summary)this.show(v);else if(id(v.run)&&['block','report','dashboard'].includes(v.kind))void this.read({kind:v.kind,run:v.run,page:'',widget:'',output:'',offset:0,limit:0});else throw fail('unavailable');}catch(e){this.error(e);}}
  show(v){
    const expiry=validateRetainedView(v);
    this.generation++;this.clear();this.value=v;
    if(v.summary.state==='expired'||expiry<=Date.now()){this.clear();this.root.append(element('p',this.w.expired,'notice'));return;}
    this.timer=setTimeout(()=>{this.clear();this.root.append(element('p',this.w.expired,'notice'));},Math.min(2147483647,expiry-Date.now()));this.draw();
  }
  async read(selection){this.loading();const generation=this.generation;try{const result=await this.bridge.call('reporting_view',selection);if(generation!==this.generation||this.closed)return;this.show(unwrap(result));}catch(e){if(generation===this.generation&&!this.closed)this.error(e);}}
  navigate(change){if(this.value)void this.read({...this.value.selection,...change});}
  draw(){
    const v=this.value;if(!v||this.closed)return;this.root.replaceChildren();const w=this.w;
    this.root.append(element('p','Chartworks · '+w.run,'eyebrow'),element('h1',text(v.summary.target?.id)),element('p',`${w.state}: ${text(v.summary.state)} · ${text(v.summary.kind)} · ${v.summary.target?.revision??''}`,'metadata'));
    if(v.summary.private)this.root.append(element('span',w.private,'badge'));
    if(v.summary.state==='partial')this.root.append(element('span',w.partial,'badge'));
    if(v.redacted)this.root.append(element('p',w.redacted,'notice'));
    const meta=element('div',undefined,'metadata');
    if(v.observed_at)meta.append(element('p',`${w.observed}: ${v.observed_at}`));
    meta.append(element('p',`${w.retained}: ${v.summary.expires_at} · ${text(v.locale)} · ${text(v.timezone)}`));
    if(v.trust)meta.append(element('p',`${w.trust}: ${text(v.trust.publication)} / ${text(v.trust.certification)} / ${text(v.trust.health?.status)}`));
    if(v.query_limits)meta.append(element('p',`${w.limits}: ${v.query_limits.max_rows} rows · ${v.query_limits.max_bytes} bytes · ${v.query_limits.timeout_ms} ms · ${v.query_limits.query_attempts} attempts`));
    if(v.mixed_freshness)meta.append(element('p','mixed_freshness'));
    if(v.summary.scheduled){const s=v.summary.scheduled;meta.append(element('p',`${text(s.schedule_id)} · ${text(s.due_at)} · [${text(s.window_start)}, ${text(s.window_end)})`),element('p',`query: ${text(s.query)} · artifact: ${text(s.artifact)} · catalog: ${text(s.catalog)} · notification: ${text(s.notification)}`));}
    if(v.summary.code)meta.append(element('p',text(v.summary.code)));this.root.append(meta);
    const toolbar=element('div',undefined,'toolbar');
    if(v.pages?.length)toolbar.append(selectControl(w.pages,v.pages,v.selection.page,page=>this.navigate({page,widget:'',output:'',offset:0})));
    const page=array(v.pages).find(p=>p.id===v.selection.page);
    if(page?.widgets?.length)toolbar.append(selectControl(w.widgets,page.widgets,v.selection.widget,widget=>this.navigate({widget,output:'',offset:0})));
    if(v.outputs?.length)toolbar.append(selectControl(w.outputs,v.outputs,v.selection.output,output=>this.navigate({output,offset:0}),document.documentElement.lang||this.locale));
    this.root.append(toolbar);
    const content=element('section');content.setAttribute('aria-label',w.outputs);this.root.append(content);
    try{
      renderRetainedOutput(content,v,{locale:this.locale,onPage:offset=>this.navigate({offset})});
      if(!['succeeded','completed','partial','expired'].includes(v.summary.state))content.append(button(w.refresh,()=>this.navigate({})));
      if(!v.summary.private)this.filters(v);
    }catch(e){this.error(e);}
  }
  filters(v){
    if(!this.allowRun)return;
    const w=this.w,filters=array(v.filters);if(!filters.length)return;
    const details=element('details');details.append(element('summary',w.filters));const fieldset=element('fieldset');fieldset.append(element('legend',w.filters),element('p',w.consent));const grid=element('div',undefined,'filters'),controls=[];
    for(const f of filters){const p=f.parameter;if(!id(p?.name))throw fail('invalid_request');const label=element('label',text(f.label)||p.name);let input;
      if(array(p.enum).length||p.type==='boolean'){input=element('select');const options=p.type==='boolean'?['true','false']:p.enum;const empty=element('option',w.default);empty.value='';input.append(empty);for(const value of options){const o=element('option',value);o.value=value;input.append(o);}}
      else{input=element(p.type==='relative_period'?'textarea':'input');if(input.tagName==='INPUT'){input.type=p.type==='date'?'date':'text';if(['number','integer','top_n'].includes(p.type))input.inputMode='decimal';}input.maxLength=p.type==='relative_period'?4096:4096;input.placeholder=p.default?JSON.stringify(p.default):w.default;}
      input.setAttribute('aria-label',text(f.label)||p.name);const useDefault=element('input');useDefault.type='checkbox';useDefault.checked=true;input.disabled=true;useDefault.addEventListener('change',()=>{input.disabled=useDefault.checked;});const defaultLabel=element('span',undefined,'check');defaultLabel.append(useDefault,element('span',w.default));label.append(defaultLabel,input);if(p.type==='relative_period')label.append(element('span','JSON: mode, unit, count, start, end, dst_policy, month_policy','metadata'));grid.append(label);controls.push({f,input,useDefault});
    }
    fieldset.append(grid);const dynamic=element('input'),narrative=element('input');dynamic.type=narrative.type='checkbox';for(const [input,label]of(this.allowGenerated?[[dynamic,w.dynamic],[narrative,w.narrative]]:[])){const l=element('label',undefined,'check');l.append(input,element('span',label));fieldset.append(l);}
    const run=button(w.apply,async()=>{
      if(this.mutationPending||this.closed)return;this.mutationPending=true;run.disabled=true;const oldSelection={...v.selection},startingGeneration=this.generation;
      try{
        const grouped=new Map();for(const {f,input,useDefault}of controls){if(useDefault.checked)continue;const value=f.parameter.type==='relative_period'?periodValue(input.value):{literal:input.value};if(!grouped.has(f.page))grouped.set(f.page,[]);grouped.get(f.page).push({name:f.parameter.name,value});}
        const description=unwrap(await this.bridge.call('reporting_describe',{target:v.summary.target,locale:text(v.locale),outputs:v.summary.kind==='block'?retainedRunOutputs(v):null}));
        if(this.closed||startingGeneration!==this.generation)return;
        if(v.summary.kind==='block'&&!['published','certified_only'].includes(v.policy))throw fail('stale_validation');
        if(!id(description.resource?.target?.id)||description.resource.target.id!==v.summary.target.id||description.resource.target.kind!==v.summary.target.kind||description.resource.target.revision!==v.summary.target.revision)throw fail('stale_validation');
        const request={target:description.resource.target,key:crypto.randomUUID(),arguments:[],pages:[],outputs:[],policy:v.summary.kind==='block'?v.policy:'',locale:text(v.locale),timezone:text(v.timezone)||description.timezone,narrative:narrative.checked,dynamic:dynamic.checked,partial_failure:''};
        if(v.summary.kind==='block'){request.arguments=Array.from(grouped.values()).flat();request.outputs=retainedRunOutputs(v);request.limits=v.query_limits??null;}
        else request.pages=Array.from(grouped,([page,filters])=>({page,filters,overrides:[]}));
        this.loading();const generation=this.generation;const response=await this.bridge.call('reporting_run',request);if(generation!==this.generation||this.closed)return;const result=unwrap(response);if(!id(result.run))throw fail('unavailable');await this.read({...oldSelection,run:result.run,offset:0});
      }catch(e){if(!this.closed)this.error(e,true);}finally{this.mutationPending=false;if(this.value&&!this.closed)this.draw();}
    },true);run.disabled=this.mutationPending;fieldset.append(run);details.append(fieldset);this.root.append(details);
  }
  close(){this.closed=true;this.generation++;this.clear();this.observer?.disconnect();}
}

const root=typeof document==='undefined'?null:document.getElementById('report-viewer');
if(root){if(window.parent===window){root.replaceChildren(element('p',words.en.host,'notice'));}else{const bridge=new Bridge(),viewer=new Viewer(root,bridge);bridge.connect().catch(e=>viewer.error(e));}}
