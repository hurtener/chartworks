// Synthetic metadata for browser controls. This fixture is not source or host proof.
export const typedFieldCatalog={compiler:'typed-dataset-postgres-v3',supported:true,max_columns:32,dimensions:[],columns:[
 {id:'specimen',name:'Specimen',source_name:'specimen_id',native_type:'text',type:'text',category:'text',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:[]},
 {id:'instrument',name:'Instrument',source_name:'instrument_id',native_type:'text',type:'text',category:'text',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:[]},
 {id:'observed',name:'Observed on',source_name:'observed_on',native_type:'date',type:'date',category:'temporal',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:['day','week','month','quarter','year']},
 {id:'reading',name:'Reading',source_name:'reading',native_type:'numeric',type:'number',category:'numeric',nullable:true,supported:true,aggregations:['count','distinct_count','sum','average','minimum','maximum'],grains:[]},
 {id:'year',name:'Year',source_name:'year_id',native_type:'int4',type:'number',category:'numeric',nullable:false,supported:true,aggregations:['count','distinct_count','sum','average','minimum','maximum'],grains:[]},
]};

// Synthetic native metadata projection; production capabilities come from the server.
for(const column of typedFieldCatalog.columns){
 const type=column.native_type==='int4'?'integer':column.type,temporal=['date','timestamp','instant'].includes(type);
 column.filters={type,kinds:temporal?['range']:['integer','number'].includes(type)?['select','multi_select','range']:['select','multi_select'],option_lookup:false,...(temporal?{calendar_required:true}:{max_set_size:16}),...(type==='instant'?{timezone_required:true}:{})};
}
export const physicalFilterCatalog={...structuredClone(typedFieldCatalog),columns:[...structuredClone(typedFieldCatalog.columns),
 {id:'accepted',name:'Accepted',source_name:'accepted',native_type:'boolean',type:'boolean',category:'boolean',nullable:true,supported:true,aggregations:['count','distinct_count'],grains:[],filters:{type:'boolean',kinds:['select','multi_select'],max_set_size:16,option_lookup:false}},
 {id:'recorded',name:'Recorded at',source_name:'recorded_at',native_type:'timestamptz',type:'instant',category:'temporal',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:['minute','hour','day','week','month','quarter','year'],filters:{type:'instant',kinds:['range'],calendar_required:true,timezone_required:true,option_lookup:false}},
]};
