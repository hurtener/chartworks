// Synthetic metadata for browser controls. This fixture is not source or host proof.
export const typedFieldCatalog={compiler:'typed-dataset-postgres-v3',supported:true,max_columns:32,dimensions:[],columns:[
 {id:'specimen',name:'Specimen',source_name:'specimen_id',native_type:'text',type:'text',category:'text',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:[]},
 {id:'instrument',name:'Instrument',source_name:'instrument_id',native_type:'text',type:'text',category:'text',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:[]},
 {id:'observed',name:'Observed on',source_name:'observed_on',native_type:'date',type:'date',category:'temporal',nullable:false,supported:true,aggregations:['count','distinct_count'],grains:['day','week','month','quarter','year']},
 {id:'reading',name:'Reading',source_name:'reading',native_type:'numeric',type:'number',category:'numeric',nullable:true,supported:true,aggregations:['count','distinct_count','sum','average','minimum','maximum'],grains:[]},
 {id:'year',name:'Year',source_name:'year_id',native_type:'int4',type:'number',category:'numeric',nullable:false,supported:true,aggregations:['count','distinct_count','sum','average','minimum','maximum'],grains:[]},
]};
