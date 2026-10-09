// Synthetic presentation metadata and transport responses, not native evidence.
import {mapView,mapClone} from './mapping-fixture.mjs';
export function formatView(id='source',revision=7,isPrivate=false){
 const view=mapView(id,revision,isPrivate),output=view.block.outputs[0],m=output.mapping;
 output.kind='table';m.version=3;m.kind='table';m.bindings={columns:['category','amount','date','x']};m.columns=mapClone(view.output_columns[0].columns.filter(c=>m.bindings.columns.includes(c.id)));
 m.columns.find(c=>c.id==='category').display_label='Region';m.columns.find(c=>c.id==='amount').display_label='Revenue';m.columns.find(c=>c.id==='x').format.percent='ratio';
 m.table={columns:m.bindings.columns.map(column=>({column,visible:true})),page_size:25,show_totals:true};
 m.presentation={version:1,columns:[{column:'category',display_label:'Territory'},{column:'amount',fraction_digits:2}]};
 output.presentation={version:1,columns:m.columns.map(c=>({column:c.id,role:'table_column',fields:c.id==='amount'?['display_label','fraction_digits']:['display_label']}))};
 return view;
}
export function formatSaved(source,args,copy=!!args.new_block){
 const view=mapClone(source),b=view.block;b.state={...b.state,id:copy?args.new_block:args.block,version:copy?1:args.expected_version+1,draft_revision:copy?1:args.revision+1,published_revision:0};b.revision=b.state.draft_revision;b.private=true;b.digest=b.digest==='c'.repeat(64)?'d'.repeat(64):'c'.repeat(64);delete b.validation;
 const m=b.outputs.find(o=>o.id===args.output).mapping,rows=mapClone(m.presentation?.columns||[]);
 for(const edit of args.presentation.edits){let row=rows.find(row=>row.column===edit.column);if(!row){row={column:edit.column};rows.push(row);}Object.assign(row,edit.set);for(const field of edit.reset||[])delete row[field];}
 const columns=[];for(const c of m.columns){const row=rows.find(row=>row.column===c.id);if(!row)continue;if(row.display_label===(c.display_label??''))delete row.display_label;if(row.fraction_digits===(c.format?.fraction_digits??0))delete row.fraction_digits;if(Object.keys(row).length>1)columns.push(row);}
 if(columns.length)m.presentation={version:1,columns};else delete m.presentation;return view;
}
