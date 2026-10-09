import {INVALID_REQUEST} from './error-codes.js';
import {appError,copyData} from './model.js';
import {node, button, textField, selectField} from './dom.js';
const fields=['display_label','fraction_digits'];
const keys=(value,allowed)=>value&&typeof value==='object'&&!Array.isArray(value)&&Object.keys(value).every(key=>allowed.includes(key));
const base=(column,field)=>field==='display_label'?column.display_label??'':column.format?.preserve_precision?null:column.format?.fraction_digits??0;
const validValue=(field,value)=>field==='fraction_digits'?Number.isInteger(value)&&value>=0&&value<=20:typeof value==='string'&&new TextEncoder().encode(value).length<=256&&!/[\x00-\x08\x0b-\x1f\x7f]|[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value);

// Capabilities describe native display consumers, never grant access. Unknown or
// inconsistent metadata fails closed; schema types alone cannot enable a field.
export function formattingColumns(output){
 const p=output?.presentation,m=output?.mapping;
 if(!keys(p,['version','columns'])||p.version!==1||!Array.isArray(p.columns)||p.columns.length>256||!Array.isArray(m?.columns)||!m.columns.length||m.columns.length>256||new Set(m.columns.map(c=>c?.id)).size!==m.columns.length||!m.bindings||m.kind==='kpi'&&m.version!==3)return [];
 const seen=new Set(),result=[];
 for(const entry of p.columns){
  if(!keys(entry,['column','role','fields']))return [];
  const c=m.columns.find(c=>c?.id===entry.column),b=m.bindings,role=entry.role;
  if(!c||seen.has(c.id)||!Array.isArray(entry.fields)||!entry.fields.length||new Set(entry.fields).size!==entry.fields.length)return [];
  const bound=role==='table_column'?m.kind==='table'&&b.columns?.includes(c.id)&&(m.version!==3||m.table?.columns.some(t=>t.column===c.id&&t.visible)):role==='values'?b.values?.includes(c.id):['value','x','y','category','target'].includes(role)&&b[role]===c.id&&(role!=='target'||m.kind==='kpi');
  if(!bound||m.kind==='kpi'&&!['value','target'].includes(role)||entry.fields.some(f=>!fields.includes(f)||f==='display_label'&&role!=='table_column'||f==='fraction_digits'&&(!['integer','decimal','number'].includes(c.type)||!!c.format?.percent)))return [];
  seen.add(c.id);result.push({column:c,role,fields:entry.fields});
 }
 if(Object.hasOwn(m,'presentation')){
  const saved=m.presentation;
  if(!keys(saved,['version','columns'])||saved.version!==1||!Array.isArray(saved.columns)||!saved.columns.length||saved.columns.length>m.columns.length)return [];
  let previous=-1;
  for(const row of saved.columns){
   if(!keys(row,['column',...fields]))return [];
   const entry=result.find(c=>c.column.id===row.column),index=m.columns.findIndex(c=>c.id===row.column),changed=Object.keys(row).filter(k=>k!=='column');
   if(!entry||index<=previous||!changed.length||changed.some(f=>!entry.fields.includes(f)||!validValue(f,row[f])||row[f]===base(entry.column,f)))return [];
   previous=index;
  }
 }
 return result;
}
export function formattingDraft(output){
 return {options:{title:output.mapping.options.title},fields:formattingColumns(output).map(({column,fields})=>{
  const saved=output.mapping.presentation?.columns?.find(c=>c.column===column.id),row={column:column.id};
  for(const field of fields)if(saved&&Object.hasOwn(saved,field))row[field]=saved[field];return row;
 })};
}
export function formattingPatch(draft,output){
 const columns=formattingColumns(output),original=formattingDraft(output);
 if(!columns.length||!keys(draft,['options','fields'])||!keys(draft.options,['title'])||draft.options.title!==original.options.title||!Array.isArray(draft.fields)||draft.fields.length!==columns.length)throw appError(INVALID_REQUEST);
 const edits=[];
 columns.forEach(({column,fields},index)=>{
  const row=draft.fields[index],old=original.fields[index],set={},reset=[];
  if(!keys(row,['column',...fields])||row.column!==column.id)throw appError(INVALID_REQUEST);
  for(const field of fields){
   const present=Object.hasOwn(row,field),value=row[field];
   if(present&&!validValue(field,value))throw appError(INVALID_REQUEST);
   if(!present||value===base(column,field)){if(Object.hasOwn(old,field))reset.push(field);}
   else if(value!==old[field])set[field]=value;
  }
  if(Object.keys(set).length||reset.length)edits.push({column:column.id,...(Object.keys(set).length?{set}:{}),...(reset.length?{reset}:{})});
 });
 if(!edits.length)throw appError(INVALID_REQUEST);return {version:1,edits};
}
// Verify the purpose-only success before adopting any report reference. The
// whole selected mapping except its normalized overlay, sibling outputs, schema
// and execution identity must be unchanged, including reviewed provenance.
const stable=value=>JSON.stringify(value,(_,v)=>v&&typeof v==='object'&&!Array.isArray(v)?Object.fromEntries(Object.keys(v).sort().map(k=>[k,v[k]])):v);
export function checkFormattingResult(before,after,output,patch){
 const expected=copyData(before.block.outputs),mapping=expected.find(o=>o.id===output).mapping,overrides=new Map((mapping.presentation?.columns||[]).map(c=>[c.column,c]));
 for(const edit of patch.edits){
  const column=mapping.columns.find(c=>c.id===edit.column),row=overrides.get(edit.column)||{column:edit.column};
  for(const field of edit.reset||[])delete row[field];
  for(const [field,value]of Object.entries(edit.set||{})){if(value===base(column,field))delete row[field];else row[field]=value;}
  overrides.set(edit.column,row);
 }
 const columns=mapping.columns.map(c=>overrides.get(c.id)).filter(c=>c&&Object.keys(c).length>1);
 if(columns.length)mapping.presentation={version:1,columns};else delete mapping.presentation;
 if(before.block.execution_digest!==after.block.execution_digest||before.block.digest===after.block.digest||stable(expected)!==stable(after.block.outputs)||stable(before.block.expected_schema)!==stable(after.block.expected_schema)||stable(before.output_columns)!==stable(after.output_columns))throw appError(INVALID_REQUEST);
}
// A disposable visual draft over an already authorized retained result. Only
// advertised display fields change; rows, amounts, provenance and admission do not.
export function formattingPreview(view,output,draft){
 formattingPatch(draft,output);
 if(view.output?.id!==output.id)throw appError(INVALID_REQUEST);
 const result=copyData(view),payload=result.output.table||result.output.chart,allowed=formattingColumns(output);
 if(!Array.isArray(payload?.columns))throw appError(INVALID_REQUEST);
 for(const column of payload.columns){
  const entry=allowed.find(item=>item.column.id===column.id);if(!entry)continue;
  const original=entry.column,row=draft.fields.find(item=>item.column===column.id);
  if(column.name!==original.name||column.type!==original.type)throw appError(INVALID_REQUEST);
  for(const field of entry.fields){const value=Object.hasOwn(row,field)?row[field]:base(original,field);
   if(field==='display_label')column.display_label=value;else {
    column.format={...column.format,fraction_digits:value??0};
    if(value===null)column.format.preserve_precision=true;else delete column.format.preserve_precision;
   }
  }
 }
 return result;
}
export function renderFormattingFields(parent,session,change){
 const output=session.view.block.outputs.find(o=>o.id===session.output),columns=formattingColumns(output),name=c=>c.display_label||c.name||c.id;
 parent.append(node('p','Edit supported headers and precision. Units, currency, percent scale, physical fields and exact values stay reviewed. Reset inherits the reviewed field.','metadata'));
 if(!columns.length){parent.append(node('p','Field formatting is unavailable for this output. Missing or unsupported metadata, legacy KPIs, percent precision and date formats cannot be edited.','notice'));return;}
 const index=Math.max(0,columns.findIndex(c=>c.column.id===session.formattingColumn)),{column,role,fields}=columns[index],row=session.draft.fields[index],section=node('section',undefined,'mapping-column-list');
 parent.append(selectField('Format field',columns.map(({column})=>({value:column.id,label:name(column)})),column.id,value=>{if(!session.closed){session.formattingColumn=value;change();}}));
 section.append(node('p',[role.replaceAll('_',' '),column.type,column.format?.currency,column.format?.unit].filter(Boolean).join(' · '),'metadata'));
 for(const field of fields){
  const label=(field==='display_label'?'Table header':'Fraction digits')+' · '+name(column),present=Object.hasOwn(row,field),reviewed=base(column,field),edit=fn=>{session.edit(d=>fn(d.fields[index]));change();},control=textField(label,present?row[field]:reviewed,value=>edit(d=>{d[field]=field==='fraction_digits'?(value.trim()===''?null:Number(value)):value;}),field==='fraction_digits'?{type:'number',min:0,max:20}:{maxLength:256});
  section.append(control,node('p',present?`Override · Reviewed: ${reviewed===null?'Unrounded retained value':reviewed===''?'(empty)':reviewed}`:reviewed===null?'Unrounded retained value · Enter fraction digits to round explicitly':'Inherited from reviewed field','metadata'),button('Reset '+label,()=>{control.children[0].focus?.({preventScroll:true});edit(d=>{delete d[field];});},!present));
 }
 parent.append(section);
}
