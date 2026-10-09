import test from 'node:test';
import assert from 'node:assert/strict';
import {formattingColumns,formattingDraft,formattingPatch,checkFormattingResult} from './formatting.js';
import {MappingSession} from './mapping.js';
import {formatView,formatSaved} from './formatting-fixture.mjs';
import {mapView,mapClone} from './mapping-fixture.mjs';
import {appError} from './model.js';
const selected=view=>view.block.outputs[0],field=(draft,id)=>draft.fields.find(c=>c.column===id);
const fixture=()=>{let view=formatView();const calls=[];return {calls,get:()=>view,invoke:async(name,args)=>{calls.push({name,args});if(name.endsWith('block_read_v1'))return mapClone(view);view=formatSaved(view,args);return mapClone(view);}};};

test('formatting is derived exclusively from bounded versioned capability roles and canonical columns',()=>{
 const output=selected(formatView());assert.equal(formattingColumns(output).length,4);assert.deepEqual(formattingColumns(output).find(c=>c.column.id==='x').fields,['display_label']);
 for(const mutate of [o=>delete o.presentation,o=>o.presentation.version=2,o=>o.presentation.columns=null,o=>o.presentation.columns=[null],o=>o.presentation.columns.push(o.presentation.columns[0]),o=>o.presentation.columns[0].column='foreign',o=>o.presentation.columns[0].role='legend',o=>o.presentation.columns[0].fields=['format'],o=>o.presentation.columns[0].fields=['fraction_digits'],o=>o.presentation.columns[0].fields=['display_label','display_label'],o=>o.presentation.columns.find(c=>c.column==='x').fields.push('fraction_digits'),o=>o.presentation.columns.find(c=>c.column==='date').fields.push('fraction_digits'),o=>o.mapping.table.columns[0].visible=false]){const bad=mapClone(output);mutate(bad);assert.deepEqual(formattingColumns(bad),[]);}
 const legacy=selected(mapView());legacy.mapping.kind='kpi';legacy.mapping.bindings={value:'amount'};legacy.presentation={version:1,columns:[{column:'amount',role:'value',fields:['fraction_digits']}]};assert.deepEqual(formattingColumns(legacy),[]);legacy.mapping.version=3;assert.equal(formattingColumns(legacy).length,1);legacy.presentation.columns[0].fields.push('display_label');assert.deepEqual(formattingColumns(legacy),[]);
});

test('closed presentation patches distinguish zero, empty label and reset; canonical equivalents normalize to inherit',()=>{
 const output=selected(formatView()),draft=formattingDraft(output);assert.equal(field(draft,'category').display_label,'Territory');assert.equal(field(draft,'amount').fraction_digits,2);assert.throws(()=>formattingPatch(draft,output));
 field(draft,'category').display_label='';field(draft,'amount').fraction_digits=0;
 assert.deepEqual(formattingPatch(draft,output),{version:1,edits:[{column:'category',set:{display_label:''}},{column:'amount',set:{fraction_digits:0}}]});
 delete field(draft,'category').display_label;field(draft,'amount').fraction_digits=3;
 assert.deepEqual(formattingPatch(draft,output),{version:1,edits:[{column:'category',reset:['display_label']},{column:'amount',reset:['fraction_digits']}]});
 const args={block:'source',revision:7,expected_version:8,output:'amount',new_block:'private',presentation:formattingPatch(draft,output)},saved=formatSaved(formatView(),args);assert.equal(selected(saved).mapping.presentation,undefined);checkFormattingResult(formatView(),saved,'amount',args.presentation);
});

test('blank, fractional, nonnumeric, excessive digits, oversized labels and full metadata edits reject before dispatch',()=>{
 const output=selected(formatView());for(const value of [null,'',false,'0',-1,21,1.1,Infinity]){const d=formattingDraft(output);field(d,'amount').fraction_digits=value;assert.throws(()=>formattingPatch(d,output));}
 for(const value of ['é'.repeat(129),'\u0000','\u007f','\ud800','\udc00']){const d=formattingDraft(output);field(d,'category').display_label=value;assert.throws(()=>formattingPatch(d,output));}
 for(const mutate of [d=>d.options.title='Changed',d=>d.mapping={},d=>d.fields.reverse(),d=>d.fields[0].format={currency:'EUR'},d=>d.fields[0].column='amount',d=>d.fields[0].fraction_digits=0,d=>d.fields[0].sql='select 1']){const d=formattingDraft(output);mutate(d);assert.throws(()=>formattingPatch(d,output));}
 const d=formattingDraft(output);field(d,'category').display_label='<img src=x onerror=alert(1)> 😀\n';assert.equal(formattingPatch(d,output).edits[0].set.display_label,field(d,'category').display_label);
});

test('presentation copy shares MappingSession CAS, sends exclusive purpose and never calls chart catalog or data tools',async()=>{
 const f=fixture(),s=new MappingSession(f.invoke,'presentation');await s.open('source',7,'amount');assert.equal(s.draft.options.title,'Revenue');assert(!s.valid());s.edit(d=>field(d,'amount').fraction_digits=0);assert(s.valid());s.newBlock='private';const v=await s.save(),request=f.calls.at(-1);assert.equal(request.name,'reporting_authoring_block_copy_v1');assert.deepEqual(request.args,{block:'source',expected_version:8,revision:7,digest:'a'.repeat(64),output:'amount',presentation:{version:1,edits:[{column:'amount',set:{fraction_digits:0}}]},new_block:'private'});assert.equal(v.block.execution_digest,'b'.repeat(64));assert.equal(v.block.validation,undefined);assert.deepEqual(f.calls.map(c=>c.name),['reporting_authoring_block_read_v1','reporting_authoring_block_copy_v1']);assert(!s.dirty);s.edit(d=>delete field(d,'amount').fraction_digits);await s.save();assert.equal(f.calls.at(-1).name,'reporting_authoring_block_mapping_v1');assert.equal(f.calls.at(-1).args.revision,1);assert.equal(f.calls.at(-1).args.expected_version,1);s.close();
});

test('purpose-only success must preserve schema, binding, provenance, siblings, execution identity and exact normalized overlay',async()=>{
 for(const change of [v=>v.block.execution_digest='e'.repeat(64),v=>selected(v).mapping.columns[0].name='new',v=>selected(v).mapping.columns[1].format.currency='EUR',v=>selected(v).mapping.columns[1].provenance.version=2,v=>selected(v).mapping.bindings.columns.reverse(),v=>selected(v).mapping.version=1,v=>selected(v).mapping.presentation.columns.find(c=>c.column==='amount').fraction_digits=4,v=>selected(v).mapping.presentation.columns.push({column:'foreign',display_label:'bad'}),v=>delete selected(v).presentation,v=>v.block.outputs[1].id='changed',v=>v.block.expected_schema[0].type='integer',v=>v.output_columns[0].columns[0].role='measure']){
  const original=formatView(),s=new MappingSession(async(name,args)=>{if(name.endsWith('block_read_v1'))return original;const v=formatSaved(original,args);change(v);return v;},'presentation');await s.open('source',7,'amount');s.newBlock='private';s.edit(d=>field(d,'amount').fraction_digits=0);await assert.rejects(s.save(),e=>e.unknown===true);assert.equal(s.unknown,true);assert.equal(s.view.block.state.id,'source');await assert.rejects(s.save(),/busy/);
 }
});

test('presentation staged cancel and delayed reads or saves cannot restore metadata after close',async()=>{
 for(const phase of ['cancel','read','save']){const f=fixture();let finish;const s=new MappingSession((name,args)=>phase==='read'&&name.endsWith('block_read_v1')||phase==='save'&&name.endsWith('block_copy_v1')?new Promise(resolve=>{finish=()=>resolve(phase==='read'?formatView():formatSaved(formatView(),args));}):f.invoke(name,args),'presentation');let work;if(phase==='read')work=s.open('source',7,'amount');else{await s.open('source',7,'amount');s.edit(d=>field(d,'amount').fraction_digits=0);s.newBlock='private';if(phase==='save')work=s.save();}s.close();finish?.();await work;assert.equal(s.view,null);assert.equal(s.draft,null);if(phase==='cancel')assert.equal(f.calls.length,1);}
});

test('presentation conflicts, uncertain outcomes and double clicks cannot repeat a mutation; inspection keeps the fence',async()=>{
 for(const code of ['conflict','unavailable']){let writes=0;const s=new MappingSession(async(name)=>{if(name.endsWith('block_read_v1'))return formatView();writes++;throw appError(code,code==='unavailable');},'presentation');await s.open('source',7,'amount');s.newBlock='private';s.edit(d=>field(d,'amount').fraction_digits=0);await assert.rejects(s.save());assert.equal(s.conflict,code==='conflict');assert.equal(s.unknown,code==='unavailable');await s.inspect();await assert.rejects(s.save(),/busy/);assert.equal(writes,1);}
 let finish,writes=0;const s=new MappingSession(async(name,args)=>{if(name.endsWith('block_read_v1'))return formatView();writes++;return new Promise(resolve=>{finish=()=>resolve(formatSaved(formatView(),args));});},'presentation');await s.open('source',7,'amount');s.newBlock='private';s.edit(d=>field(d,'amount').fraction_digits=0);const saving=s.save();await assert.rejects(s.save(),/busy/);assert.throws(()=>s.edit(()=>{}),/busy/);finish();await saving;assert.equal(writes,1);
});

test('newer metadata open wins without older replies clearing its pending state or resurrecting a stage',async()=>{
 const pending=new Map(),s=new MappingSession((_,args)=>new Promise(resolve=>pending.set(args.block,resolve)),'presentation');const first=s.open('first',7,'amount'),second=s.open('second',7,'amount');pending.get('first')(formatView('first'));assert.equal(await first,false);assert.equal(s.pending,true);assert.equal(s.view,null);pending.get('second')(formatView('second'));assert.equal(await second,true);assert.equal(s.view.block.state.id,'second');s.close();
});


test('legacy or malformed stored overlays remain unsupported even under otherwise valid capabilities',()=>{
 for(const mutate of [o=>o.mapping.presentation=null,o=>o.mapping.presentation.version=2,o=>o.mapping.presentation.columns=[],o=>o.mapping.presentation.extra=true,o=>o.mapping.presentation.columns=[null],o=>o.mapping.presentation.columns.push(o.mapping.presentation.columns[0]),o=>o.mapping.presentation.columns.reverse(),o=>o.mapping.presentation.columns[0].extra='x',o=>o.mapping.presentation.columns[0].display_label=null,o=>o.mapping.presentation.columns[0].display_label='Region',o=>o.mapping.presentation.columns[1].fraction_digits=null,o=>o.mapping.presentation.columns[1].fraction_digits=21,o=>o.mapping.presentation.columns[1].fraction_digits=3,o=>o.mapping.presentation.columns[0].column='foreign',o=>o.mapping.presentation.columns[0]={column:'category'}]){const output=selected(formatView());mutate(output);assert.deepEqual(formattingColumns(output),[]);assert.deepEqual(formattingDraft(output).fields,[]);assert.throws(()=>formattingPatch(formattingDraft(output),output));}
});

test('forged rich KPI category, comparison and coordinate capabilities never enable inert formatters',()=>{
 for(const role of ['category','comparison','x','y']){const output=selected(mapView());output.mapping.kind='kpi';output.mapping.version=3;output.mapping.bindings={value:'amount',[role]:'category'};output.mapping.columns[0].type='integer';output.presentation={version:1,columns:[{column:'category',role,fields:['fraction_digits']}]};assert.deepEqual(formattingColumns(output),[]);}
});


test('an older save reply cannot clear a newer metadata read pending fence',async()=>{
 let finishSave,finishRead;const s=new MappingSession((name,args)=>name.endsWith('block_read_v1')?(args.block==='source'?Promise.resolve(formatView()):new Promise(resolve=>{finishRead=()=>resolve(formatView('second'));})):new Promise(resolve=>{finishSave=()=>resolve(formatSaved(formatView(),args));}),'presentation');await s.open('source',7,'amount');s.newBlock='private';s.edit(d=>field(d,'amount').fraction_digits=0);const saving=s.save(),opening=s.open('second',7,'amount');finishSave();assert.equal(await saving,null);assert.equal(s.pending,true);finishRead();await opening;assert.equal(s.view.block.state.id,'second');s.close();
});

test('visual formatting drafts preserve native retained values, units, evidence and stored objects',async()=>{
 const {readFile}=await import('node:fs/promises'),{formattingPreview}=await import('./formatting.js');
 const data=JSON.parse(await readFile(new URL('./testdata/presentation-native.json',import.meta.url),'utf8'));
 for(const widget of ['table','kpi']){
  const view=data.stages.source.views[widget],output=data.source_block.block.outputs.find(o=>o.id===view.output.id),draft=formattingDraft(output),before=JSON.stringify(view),source=JSON.stringify(output);
  const column=formattingColumns(output).find(c=>c.fields.includes('fraction_digits'));draft.fields.find(f=>f.column===column.column.id).fraction_digits=1;
  const result=formattingPreview(view,output,draft),payload=result.output.table||result.output.chart,original=view.output.table||view.output.chart;
  assert.equal(payload.columns.find(c=>c.id===column.column.id).format.fraction_digits,1);
  assert.deepEqual(payload.columns.find(c=>c.id===column.column.id).format,{...original.columns.find(c=>c.id===column.column.id).format,fraction_digits:1});
  assert.deepEqual({...payload,columns:[]},{...original,columns:[]});assert.deepEqual(result.summary,view.summary);assert.deepEqual(result.output.amount_completeness,view.output.amount_completeness);
  assert.equal(JSON.stringify(view),before);assert.equal(JSON.stringify(output),source);
 }
});
test('visual formatting rejects mismatched output, column identity, invalid precision and unsupported fields',async()=>{
 const {readFile}=await import('node:fs/promises'),{formattingPreview}=await import('./formatting.js'),data=JSON.parse(await readFile(new URL('./testdata/presentation-native.json',import.meta.url),'utf8'));
 const view=data.stages.source.views.table,output=data.source_block.block.outputs.find(o=>o.id===view.output.id),draft=formattingDraft(output),numeric=draft.fields.find(c=>c.column==='c1');numeric.fraction_digits=1;
 for(const mutate of [v=>v.output.id='foreign',v=>v.output.table.columns.find(c=>c.id==='c1').name='different',v=>v.output.table.columns.find(c=>c.id==='c1').type='string']){const altered=mapClone(view);mutate(altered);assert.throws(()=>formattingPreview(altered,output,draft));}
 for(const value of [null,21,-1]){numeric.fraction_digits=value;assert.throws(()=>formattingPreview(view,output,draft));}
 numeric.fraction_digits=1;numeric.currency='EUR';assert.throws(()=>formattingPreview(view,output,draft));
});

test('unrounded numeric defaults distinguish explicit zero from inherited precision and reset',async()=>{
 const {formattingPreview}=await import('./formatting.js'),before=formatView(),output=selected(before),column=output.mapping.columns.find(c=>c.id==='amount');
 delete output.mapping.presentation;column.format={fraction_digits:0,preserve_precision:true};
 const draft=formattingDraft(output);field(draft,'amount').fraction_digits=0;
 const patch=formattingPatch(draft,output);assert.deepEqual(patch,{version:1,edits:[{column:'amount',set:{fraction_digits:0}}]});
 const after=mapClone(before);selected(after).mapping.presentation={version:1,columns:[{column:'amount',fraction_digits:0}]};after.block.digest='c'.repeat(64);
 checkFormattingResult(before,after,output.id,patch);assert.equal(formattingColumns(selected(after)).length,4);
 const view={output:{id:output.id,table:{columns:[mapClone(column)],rows:[[{value:'15.5'}]]}}};
 const rounded=formattingPreview(view,output,draft);assert.deepEqual(rounded.output.table.columns[0].format,{fraction_digits:0});
 const reset=formattingDraft(selected(after));delete field(reset,'amount').fraction_digits;
 assert.deepEqual(formattingPatch(reset,selected(after)),{version:1,edits:[{column:'amount',reset:['fraction_digits']}]});
 assert.equal(formattingPreview(rounded,selected(after),reset).output.table.columns[0].format.preserve_precision,true);
 assert.equal(view.output.table.columns[0].format.preserve_precision,true);
});
