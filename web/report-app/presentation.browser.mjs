// Source-only until approved publication runs the ordinary hosted Chrome job.
// Actual production HTML; exact PostgreSQL DTO replay; no live provider claim.
import assert from 'node:assert/strict';
import {exact} from '../report-viewer/presentation.js';
import {runHostedBrowser} from './hosted-browser-harness.mjs';
import {presentationBrowserFixture,presentationBrowserTools,presentationStageNames,presentationMutations} from './presentation-browser-fixture.mjs';
const [htmlPath,screenshotPath,mode]=process.argv.slice(2);
const fixture=await presentationBrowserFixture(),data=fixture.data,screenshots=[],scenarios=[];
const proof=await runHostedBrowser({htmlPath,screenshotPath,mode,fixture,tools:presentationBrowserTools,label:'Synthetic host · Native PostgreSQL presentation DTOs · No live provider connection'},async h=>{
 const {root,button,check,equal,ready,textHas,clickElement,click,capture,evaluate,until}=h;
 const field=label=>`Array.from(${root}.querySelectorAll('input')).find(e=>e.getAttribute('aria-label')===${JSON.stringify(label)})`;
 const card=id=>`${root}.querySelector('.editing-canvas .widget-card[data-widget="${id}"]')`;
 const inspector=`${root}.querySelector('.mapping-editor')`;
 const writes=()=>fixture.calls.filter(call=>presentationMutations.includes(call.name));
 const title=data.initial_report.definition.metadata.find(value=>value.locale==='en-US').title;
 const noCalls=async(message,action)=>{const before=structuredClone(fixture.calls),snapshot=fixture.snapshot(),counts=fixture.counts();await action();equal(fixture.calls,before,message);equal(fixture.snapshot(),snapshot,message+' preserves native snapshots');equal(fixture.counts(),counts,message+' represents no query');};
 const noSource=async(message,action)=>{const count=fixture.counts().nativeSourceReadsRepresented;await action();equal(fixture.counts().nativeSourceReadsRepresented,count,message);};
 const captureAs=async suffix=>{const path=screenshotPath.replace(/\.png$/,'.'+suffix+'.png');await capture(path);screenshots.push(path);};
 async function setField(label,value){await evaluate(`(()=>{const e=${field(label)};if(!e||e.disabled||e.closest('fieldset')?.disabled)throw new Error('Missing enabled presentation field');e.value=${JSON.stringify(String(value))};e.dispatchEvent(new Event('change',{bubbles:true}));})()`);await ready();}
 async function select(id){await clickElement(`${card(id)}.querySelector('.widget-select')`);}
 async function arm(request){await evaluate(`document.getElementById('app').contentWindow.armNativeRequest(${JSON.stringify({...request,key:request.key??null})},'preview')`);}
 async function open(){
  await until(()=>evaluate(`!!${root}&&${button('Build')}?.disabled===false`),'Native builder capability did not arrive');
  await check(`hostMessages.filter(m=>m.method===${JSON.stringify(mode==='embedded'?'initialize':'ui/initialize')}).length===1&&!hostMessages.some(m=>m.method===${JSON.stringify(mode==='embedded'?'ui/initialize':'initialize')})`,'Exact '+mode+' adapter handshakes once without fallback');
  await click('Build');await click(title);await textHas(`Private draft revision ${data.initial_report.revision} loaded.`);
  await check(`${root}.dataset.mode==='builder'&&${card('table')}?.textContent.includes('Revenue table')&&${card('kpi')}?.textContent.includes('Revenue KPI')`,'Named report opens its two exact native cards');
 }
 async function editor(id='table'){
  await select(id);await click('Field formatting');await check(`${inspector}?.querySelector('h2')?.textContent==='Field formatting'`,'The selected '+id+' chart has a dedicated Field formatting inspector');
  await check(`!Array.from(${inspector}.querySelectorAll('input')).some(e=>/new.*id|block.*id|copy.*id/i.test(e.getAttribute('aria-label')||''))&&${inspector}.textContent.includes('Units, currency, percent scale, physical fields and exact values stay reviewed.')`,'Formatting keeps technical copy IDs out of entry controls and discloses semantic preservation');
 }
 function beforeView(name){return name==='formatted_kpi'?data.source_block:data.stages[presentationStageNames[presentationStageNames.indexOf(name)-1]].block;}
 async function applyPatch(name){
  const request=data.stages[name].mutation_request,mapping=beforeView(name).block.outputs.find(output=>output.id===request.output).mapping;
  await noCalls(name+' field changes and Reset are local and invoke no tool',async()=>{
   for(const edit of request.presentation.edits){const c=mapping.columns.find(column=>column.id===edit.column),name=c.display_label||c.name||c.id;
    for(const [key,value]of Object.entries(edit.set||{}))await setField((key==='display_label'?'Table header':'Fraction digits')+' · '+name,value);
    for(const key of edit.reset||[])await click('Reset '+(key==='display_label'?'Table header':'Fraction digits')+' · '+name);
   }
  });
 }
 async function assertLayout(stageName){
  const page=data.stages[stageName].report.definition.report_pages.find(page=>page.id==='analysis');
  const layout=await evaluate(`Array.from(${root}.querySelectorAll('.editing-canvas .widget-card'),c=>({id:c.dataset.widget,row:Number(c.dataset.row),column:Number(c.dataset.column),width:Number(c.dataset.width),height:Number(c.dataset.height)}))`);
  equal(layout,page.widgets.map(widget=>({id:widget.id,...widget.grid})).sort((a,b)=>a.row-b.row||a.column-b.column),'Saved '+stageName+' retains exact native card layout');
 }
 async function assertValues(name){
  const stage=data.stages[name],table=stage.views.table.output.table,kpi=stage.views.kpi.output.chart;
  const valueColumn=kpi.columns.find(column=>column.id===kpi.mapping.bindings.value),value=kpi.kpi_result.value;
  await check(`${root}.querySelector('.preview-provenance')?.textContent.includes('Private preview')&&${root}.querySelector('.preview-provenance')?.textContent.includes('Values from revision ${stage.report.revision}')&&!${root}.textContent.includes('Retained output unavailable.')`,name+' displays its exact saved actor-private retained revision');
  const headers=await evaluate(`Array.from(${card('table')}.querySelectorAll('.retained-output thead th'),e=>e.textContent)`);
  equal(headers,[...(table.row_indices?.length?['Returned row']:[]),...table.columns.map(column=>column.display_label||column.name)],name+' table headers use only the effective native column labels');
  equal(await evaluate(`Array.from(${card('table')}.querySelectorAll('.retained-output tbody td'),e=>e.firstChild?.textContent)`),table.rows.flatMap(row=>row.map((cell,index)=>exact(cell,table.columns[index],'Missing',stage.views.table.timezone))),name+' table digits round exact native cells without float conversion');
  equal(await evaluate(`${card('kpi')}.querySelector('.kpi')?.getAttribute('aria-label')`),exact(value,valueColumn),name+' KPI formatted value preserves reviewed unit and currency');
  equal(await evaluate(`${card('kpi')}.querySelector('.kpi-value')?.title`),value.exact??value.value,name+' KPI title retains its exact numeric string');
  const largeAmount={source:'9,007,199,254,740,993.125 USD revenue',copied_table:'9,007,199,254,740,993.13 USD revenue',amended_table:'9,007,199,254,740,993 USD revenue',reset_table:'9,007,199,254,740,993.125 USD revenue',formatted_kpi:'9,007,199,254,740,993.125 USD revenue'};
  equal(await evaluate(`${card('table')}.querySelector('tbody tr')?.querySelectorAll('td')[1]?.firstChild?.textContent`),largeAmount[name],name+' independently checks decimal rounding above the binary safe-integer range');
  equal(await evaluate(`${card('kpi')}.querySelector('.kpi-value')?.textContent`),name==='formatted_kpi'?'0':'0.000',name+' independently checks scientific-value display precision');
  const percent='Percent delta: '+(kpi.kpi_result.percent_delta.exact??kpi.kpi_result.percent_delta.value)+'%';
  await check(`${card('kpi')}.textContent.includes(${JSON.stringify(percent)})`,name+' derived percent delta is unchanged by value precision');
  for(const [label,key]of [['Comparison','comparison'],['Delta','delta'],['Target difference','target_difference']])if(kpi.kpi_result[key])await check(`${card('kpi')}.textContent.includes(${JSON.stringify(label+': '+exact(kpi.kpi_result[key],valueColumn))})`,name+' '+label+' uses the native exact derived value and effective precision');
  if(kpi.kpi_result.sparkline?.length)equal(await evaluate(`${card('kpi')}.querySelector('[aria-label="Sparkline exact values"]')?.textContent`),kpi.kpi_result.sparkline.map(value=>exact(value,valueColumn,'Missing',stage.views.kpi.timezone)).join(' → '),name+' trend values preserve exact native ordering and units');
  for(const id of ['table','kpi']){
   const view=stage.views[id];await check(`${card(id)}.querySelector('.output-provenance')?.textContent.includes(${JSON.stringify('Observed: '+view.observed_at)})&&${card(id)}.querySelector('.output-provenance')?.textContent.includes(${JSON.stringify('Publication / certification / current health: '+view.trust.publication+' / '+view.trust.certification+' / '+(view.trust.health?.status||'unknown'))})`,name+' '+id+' retains separate observed, publication, certification and health provenance');
  }
  await assertLayout(name);
 }
 async function preview(name){
  const stage=data.stages[name];await arm(stage.preview_request);const before=fixture.counts();await click('Private preview');
  equal(fixture.counts().privateExecutions,before.privateExecutions+1,name+' has one separately requested native private execution');
  equal(fixture.counts().nativeSourceReadsRepresented,before.nativeSourceReadsRepresented+stage.counts.explicit_preview_execution.source_reads,name+' represents only the captured native source reads');
  await assertValues(name);
 }
 async function saveReopenValidate(name){
  const stage=data.stages[name];
  await noSource(name+' Save report and reopen are metadata-only',async()=>{await click('Save report');equal(fixture.snapshot().report,stage.report,'Save matches the actual persisted '+name+' public DTO');await click('Reload latest');await textHas(`Private draft revision ${stage.report.revision} loaded.`);});
  await assertLayout(name);
  const id=name==='formatted_kpi'?'kpi':'table';await editor(id);
  const mapping=stage.block.block.outputs.find(output=>output.id===stage.mutation_request.output).mapping;
  for(const edit of stage.mutation_request.presentation.edits){const column=mapping.columns.find(column=>column.id===edit.column),saved=mapping.presentation?.columns.find(row=>row.column===column.id),label=column.display_label||column.name||column.id;
   for(const key of [...Object.keys(edit.set||{}),...(edit.reset||[])])equal(await evaluate(`${field((key==='display_label'?'Table header':'Fraction digits')+' · '+label)}.value`),String(saved?.[key]??(key==='display_label'?column.display_label??'':column.format?.fraction_digits??0)),name+' reopened field inherits or shows its exact saved override');
  }
  await noCalls(name+' reopening Cancel does not mutate or execute',()=>click('Cancel formatting'));
  await arm(stage.validation_request);const before=fixture.counts();await click('Validate data');equal(fixture.counts().validations,before.validations+1,name+' explicitly validates only its changed private chart');
  equal(fixture.counts().nativeSourceReadsRepresented,before.nativeSourceReadsRepresented+stage.counts.explicit_validation.source_reads,name+' validation accounts for its actual native source read');
  for(const widget of stage.report.definition.report_pages.find(page=>page.id==='analysis').widgets.filter(widget=>widget.block?.policy==='private_preview')){
   await select(widget.id);await noSource('Checking '+widget.id+' fresh native validation is metadata-only',()=>click('Check chart status'));
  }
  await preview(name);
 }
 async function precisionDetails(name){
  const stage=data.stages[name],table=stage.views.table.output.table,kpi=stage.views.kpi.output.chart;
  await noCalls('Per-card exact values and scientific precision disclosures never run tools',async()=>{
   const rawTable=table.rows.flatMap((row,index)=>row.flatMap((cell,column)=>{const raw=cell.exact??cell.value,c=table.columns[column],m=/^[+-]?\d+(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(raw||'');return !cell.null&&['integer','decimal','number'].includes(c.type)&&!c.format?.percent&&m&&(m[2]!==undefined||(m[1]?.length||0)>c.format.fraction_digits)?[raw]:[];}));
   equal(await evaluate(`Array.from(${card('table')}.querySelectorAll('.cell-precision .raw-retained-value'),e=>e.textContent)`),rawTable.map(raw=>'Unrounded retained value: '+raw),'Each rounded or scientific table value has its own exact precision disclosure');
   assert(rawTable.some(raw=>/[eE]/.test(raw)),'Native capture must contain a scientific value for this actual Chrome check');
   await clickElement(`${card('table')}.querySelector('.cell-precision > summary')`);
   await check(`${card('table')}.querySelector('.cell-precision')?.open===true`,'Table Precision opens on the actual table card');
   await clickElement(`${card('kpi')}.querySelector('.retained-values > summary')`);
   const raw=kpi.kpi_result.value.exact??kpi.kpi_result.value.value;
   await check(`${card('kpi')}.querySelector('.retained-values')?.open===true&&${card('kpi')}.querySelector('.retained-values > .raw-retained-value')?.textContent===${JSON.stringify('Unrounded retained value: '+raw)}`,'The KPI card opens its own exact retained main value separately');
   for(const id of ['table','kpi'])await clickElement(`${card(id)}.querySelector('.output-provenance > summary')`);
  });
  await captureAs('precision-details');
 }
 await open();equal(fixture.counts().nativeSourceReadsRepresented,0,'Report opening and field metadata perform no source work');
 await preview('source');await captureAs('baseline-private');
 await editor();await captureAs('table-inspector');
 const initialDigits=await evaluate(`${field('Fraction digits · Revenue')}.value`),initialHeader=await evaluate(`${field('Table header · Revenue')}.value`);
 await noCalls('Empty, invalid and cancelled formatting stages invoke no tool',async()=>{
  await setField('Fraction digits · Revenue','');await check(`${button('Save formatting')}.disabled===true`,'Empty precision is invalid rather than coerced to zero');
  await setField('Fraction digits · Revenue','21');await check(`${button('Save formatting')}.disabled===true`,'Precision above 20 is unavailable');
  await setField('Fraction digits · Revenue','0');await setField('Table header · Revenue','Cancelled label');await click('Cancel formatting');
 });
 await editor();equal(await evaluate(`${field('Fraction digits · Revenue')}.value`),initialDigits,'Cancel preserves the reviewed digits');equal(await evaluate(`${field('Table header · Revenue')}.value`),initialHeader,'Cancel preserves the reviewed table header');
 for(const name of presentationStageNames.slice(1)){
  if(name!=='copied_table'){await editor(name==='formatted_kpi'?'kpi':'table');if(name==='formatted_kpi')await captureAs('kpi-inspector');}
  await applyPatch(name);const reportBefore=fixture.snapshot().report,sourceBefore=fixture.counts().nativeSourceReadsRepresented,writeCount=writes().length;
  if(name==='copied_table'){
   fixture.hold('reporting_authoring_block_copy_v1');await click('Save formatting',{wait:false,repeat:2});await until(()=>fixture.held()===1,'The exact native copy reply did not reach the bounded hold');
   equal(writes().length,writeCount+1,'Rapid physical Save clicks issue one exact native copy');equal(fixture.allocations.length,1,'Repeated click reserves only one stable host target');await fixture.release();await h.drain();await ready();
  }else await click('Save formatting');
  await textHas('Private chart saved. It is unvalidated.');equal(fixture.snapshot().report,reportBefore,'Saving '+name+' does not implicitly save the report');
  equal(fixture.counts().nativeSourceReadsRepresented,sourceBefore,'Saving '+name+' presentation calls no source or model');
  await check(`${button('Private preview')}.disabled===true&&${button('Save report')}.disabled===false`,'Presentation save requires separate report save and fresh validation before preview');
  equal(writes().at(-1).args,data.stages[name].mutation_request,name+' sends only the exact native presentation patch and CAS');
  await saveReopenValidate(name);await captureAs(name+'-saved-private');
 }
 await precisionDetails('formatted_kpi');
 await noCalls('Saved retained layout redraw and page switching do not call tools',async()=>{
  await select('table');await select('kpi');await click('Notes');await check(`${root}.textContent.includes(${JSON.stringify(data.stages.formatted_kpi.report.definition.report_pages.find(page=>page.id==='notes').widgets[0].text.text)})`,'Untargeted native Notes text remains visible');await click('Analysis');
 });
 equal(fixture.allocations.map(value=>value.id),[data.stages.copied_table.mutation_request.new_block,data.stages.formatted_kpi.mutation_request.new_block],'Only the table and KPI native copy targets were allocated');
 equal(await evaluate('hostCalls.map(c=>({name:c.name,args:c.arguments}))'),fixture.calls,'Both adapter tool calls use only the exact native replay ledger');
 equal(await evaluate('hostAllocations'),fixture.allocations.map(value=>value.request),'Only exact adapter copy requests reached the host allocation seam');
 await check(`hostErrors.length===0&&document.getElementById('app').contentWindow.storageTouches===0`,'No protocol error, credential channel or browser persistence');
 scenarios.push({name:'happy-private-retained',counts:fixture.counts(),calls:structuredClone(fixture.calls),allocations:structuredClone(fixture.allocations)});
 // This native fixture has no report publication. Browse is deliberately not
 // used as a substitute for public Consumer qualification.
 await h.close();await textHas('This report app is closed. Reopen it through your authorized host.');
 for(const scenario of ['conflict','unknown','late-read','late-save']){
  fixture.reset(scenario);await h.navigate();await open();
  if(scenario==='late-read'){
   await select('table');fixture.hold('reporting_authoring_block_read_v1');await click('Field formatting',{wait:false});await until(()=>fixture.held()===1,'Pending metadata read did not reach hold');
  }else{
   await editor();await applyPatch('copied_table');if(scenario==='late-save')fixture.hold('reporting_authoring_block_copy_v1');
   await click('Save formatting',{wait:scenario!=='late-save'});
  }
  if(scenario.startsWith('late-')){
   if(scenario==='late-save')await until(()=>fixture.held()===1,'Pending save did not reach hold');
   await h.close();await textHas('This report app is closed. Reopen it through your authorized host.');await fixture.release();await h.drain();await ready();
   await check(`${root}.textContent==='This report app is closed. Reopen it through your authorized host.'&&!${inspector}`,'Closing during '+scenario+' ignores the exact late native reply');
   equal(fixture.snapshot().report,data.initial_report,'A late '+scenario+' response cannot save or rebind the report');
  }else{
   await check(`${button('Save formatting')}.disabled===true`,scenario+' fences repeated formatting writes');
   await noCalls(scenario+' disabled Save cannot repeat a mutation',async()=>{await evaluate(`${button('Save formatting')}.click()`);await ready();});
   equal(writes().length,1,scenario+' makes exactly one attempted native mutation');
   if(scenario==='unknown'){
    await textHas('The save outcome is unknown.');const target=data.stages.copied_table.block.block;
    await click('Inspect chart state');equal(fixture.calls.at(-1),{name:'reporting_authoring_block_read_v1',args:{block:target.state.id,revision:target.revision}},'Unknown inspection uses the original stable target and pinned revision');
    await check(`${button('Save formatting')}.disabled===true&&${inspector}.textContent.includes('outcome is unknown')`,'Successful metadata inspection does not resolve unknown write attribution');await captureAs('unknown-fenced');
    await click('Close chart editor');await check(`${button('Field formatting')}.disabled===true`,'Unknown close preserves the report-local editing fence');
   }else{await textHas('This chart changed elsewhere.');await captureAs('cas-conflict');const nativeUnknown=data.response_contract.errors.duplicate_copy.mcp.structuredContent.error.outcome==='unknown';await click(nativeUnknown?'Close chart editor':'Cancel formatting');if(nativeUnknown)await check(`${button('Field formatting')}.disabled===true`,'Native CAS conflict preserves its recorded unknown-outcome editing fence');}
   equal(fixture.snapshot().report,data.initial_report,scenario+' never adopts or saves an uncertain/conflicting chart pin');
   equal(fixture.allocations.length,1,scenario+' keeps one original host allocation');await h.close();await textHas('This report app is closed. Reopen it through your authorized host.');
  }
  equal(fixture.counts().nativeSourceReadsRepresented,0,scenario+' performs no source or model execution');
  scenarios.push({name:scenario,counts:fixture.counts(),calls:structuredClone(fixture.calls),allocations:structuredClone(fixture.allocations)});
 }
});
console.log(JSON.stringify({...proof,nativeCapture:fixture.provenance,scenarios,screenshots,scope:'actual HTML in hosted Chrome using native PostgreSQL public DTO replay; private retained only; no live provider or published Consumer claim'}));
