import {node,button,selectField,textField} from './dom.js';
import {fieldInstant,fieldSelectionIssue,groupingMetadata} from './field-selection.js';

const words=value=>value.replaceAll('_',' ');
const move=(items,index,offset)=>{const other=index+offset;if(other<0||other>=items.length)return;[items[index],items[other]]=[items[other],items[index]];};

export function renderFieldPicker(parent,session,{locked,edit,change}){
 const view=session.view,f=session.draft.fields,catalog=view.fields;
 const section=node('section',undefined,'field-picker');
 section.append(node('h3','Choose your fields'),node('p','Columns come from this dataset. Reviewed fields keep their published meaning. Nothing runs until you prepare the chart.','metadata'));
 section.append(selectField('Result shape',[{value:'aggregate',label:'Group and aggregate'},{value:'rows',label:'Raw rows · table'}],f.mode,value=>edit(d=>{d.fields.mode=value;if(value==='rows'){d.kind='table';for(const field of d.fields.dimensions){delete field.grain;delete field.calendar;delete field.timezone;}}}),locked));
 const selectedCount=f.dimensions.length+f.measures.length,full=selectedCount>=catalog.max_columns;
 const toolbar=node('div',undefined,'field-search');
 toolbar.append(textField('Find a field',session.fieldSearch||'',value=>{session.fieldSearch=value;change();},{disabled:locked}),node('span',`${selectedCount} / ${catalog.max_columns} result columns`,'metadata'));
 section.append(toolbar);
 const search=(session.fieldSearch||'').trim().toLocaleLowerCase(),matches=item=>!search||[item.name,item.id,item.source_name].some(value=>value?.toLocaleLowerCase().includes(search));
 const library=node('div',undefined,'field-library');
 const add=(label,selected)=>button(label,()=>edit(selected),locked||full);
 const group=(title,items,render)=>{if(!items.length)return;const list=node('section',undefined,'field-library-group');list.append(node('h4',title));for(const item of items)list.append(render(item));library.append(list);};
 group('Columns',catalog.columns.filter(matches),c=>{
  const row=node('div',undefined,'field-library-row'),name=node('div');name.append(node('strong',c.name||c.source_name),node('span',`${c.source_name} · ${c.native_type}${c.nullable?' · nullable':''}`,'metadata'));row.append(name);
  if(!c.supported)row.append(node('span',words(c.reason||'unavailable'),'metadata'));
  else{
   const chosen=f.dimensions.some(d=>d.kind==='column'&&d.field===c.id),control=add(f.mode==='rows'?'Add column':'Group by',d=>{d.fields.dimensions.push({kind:'column',field:c.id});});control.disabled=control.disabled||chosen;row.append(control);
   if(f.mode==='aggregate'&&c.aggregations.length)row.append(add('Aggregate',d=>{d.fields.measures.push({kind:'column',field:c.id,aggregation:''});}));
  }
  return row;
 });
 if(f.mode==='aggregate'){
  group('Reviewed dimensions',catalog.dimensions.filter(matches),d=>{
   const row=node('div',undefined,'field-library-row');row.append(node('strong',d.name));
   if(!d.supported)row.append(node('span',words(d.reason||'unavailable'),'metadata'));
   else{const control=add('Group by',draft=>{draft.fields.dimensions.push({kind:'dimension',field:d.id});});control.disabled=control.disabled||f.dimensions.some(x=>x.kind==='dimension'&&x.field===d.id);row.append(control);}
   return row;
  });
  group('Reviewed measures',view.measures.filter(matches),m=>{
   const row=node('div',undefined,'field-library-row'),name=node('div');name.append(node('strong',m.name),node('span',`${words(m.aggregation)}${m.unit?' · '+m.unit:''} · reviewed`,'metadata'));row.append(name);
   if(!m.supported)row.append(node('span',words(m.reason||'unavailable'),'metadata'));
   else{const control=add('Add measure',d=>{d.fields.measures.push({kind:'measure',field:m.id});});control.disabled=control.disabled||f.measures.some(x=>x.kind==='measure'&&x.field===m.id);row.append(control);}
   return row;
  });
 }
 if(!library.children.length)library.append(node('p','No fields match your search.','metadata'));
 section.append(library);
 if(f.mode==='aggregate'){const count=add('Add row count',d=>{d.fields.measures.push({kind:'count'});});count.disabled=count.disabled||f.measures.some(m=>m.kind==='count');section.append(count);}
 const selections=node('div',undefined,'field-selections');
 const controls=(row,list,index,label)=>{
  row.append(button(`Move ${label} up`,()=>edit(d=>move(d.fields[list],index,-1)),locked||index===0),button(`Move ${label} down`,()=>edit(d=>move(d.fields[list],index,1)),locked||index===f[list].length-1),button(`Remove ${label}`,()=>edit(d=>{d.fields[list].splice(index,1);}),locked));
 };
 if(f.dimensions.length)selections.append(node('h4',f.mode==='rows'?'Selected columns':'Group by · in order'));
 f.dimensions.forEach((d,index)=>{
  const {column,dimension,label}=groupingMetadata(view,d),row=node('section',undefined,'field-selection-row');row.append(node('strong',`${index+1}. ${label}`));
  if(f.mode==='aggregate'&&column?.grains.length){
   const available=dimension?.temporal?column.grains.filter(g=>dimension.temporal.grains.includes(g)):column.grains;
   row.append(selectField(`Date grouping: ${label}`,[{value:'',label:dimension?.temporal?'Choose the reviewed date grain':'Original values · no grouping by date'},...available.map(value=>({value,label:`${words(value)} · Gregorian`}))],d.grain||'',value=>edit(draft=>{
    const item=draft.fields.dimensions[index];delete item.grain;delete item.calendar;delete item.timezone;
    if(value){item.grain=value;item.calendar='gregorian';if(dimension?.temporal?.timezone)item.timezone=dimension.temporal.timezone;}
   }),locked));
   if(d.grain&&fieldInstant(column))row.append(textField(`Timezone (IANA): ${label}`,d.timezone||'',value=>edit(draft=>{const item=draft.fields.dimensions[index];if(value)item.timezone=value;else delete item.timezone;}),{disabled:locked||!!dimension?.temporal}),node('p','Choose the timezone for calendar boundaries, for example UTC or America/Argentina/Buenos_Aires.','metadata'));
  }
  controls(row,'dimensions',index,label);selections.append(row);
 });
 if(f.measures.length)selections.append(node('h4','Measures · in order'));
 f.measures.forEach((m,index)=>{
  const column=catalog.columns.find(c=>c.id===m.field),reviewed=view.measures.find(c=>c.id===m.field),label=m.kind==='count'?'Row count':m.kind==='measure'?reviewed?.name:column?.name,row=node('section',undefined,'field-selection-row');
  row.append(node('strong',`${index+1}. ${label||m.field}`));
  if(m.kind==='column')row.append(selectField(`Aggregation: ${label}`,[{value:'',label:'Choose an aggregation'},...(column?.aggregations||[]).map(value=>({value,label:words(value)}))],m.aggregation||'',value=>edit(d=>{d.fields.measures[index].aggregation=value;}),locked));
  else row.append(node('span',m.kind==='count'?'Count every row, including rows with null values.':`${words(reviewed?.aggregation||'')} · governed definition`,'metadata'));
  controls(row,'measures',index,label||m.field);selections.append(row);
 });
 section.append(selections);
 const issue=fieldSelectionIssue(view,session.draft);if(issue)section.append(node('p',issue,'notice'));
 parent.append(section);
}
