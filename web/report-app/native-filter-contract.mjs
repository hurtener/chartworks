// Reads an optional local synthetic DTO export from the real PostgreSQL journey.
// No network, source execution, persistent values or browser claim is made here.
import {readFile} from 'node:fs/promises';
import assert from 'node:assert/strict';
import {checkDatasetView,datasetIntent} from './dataset.js';
import {FilterOptionLookup} from './filters.js';
import {filterInputState,filterInputValue} from './filter-controls.js';
const path=process.argv[2];assert(path,'native synthetic DTO path required');
const bytes=await readFile(path);assert(bytes.length<=512<<10,'bounded DTO capture');
const data=JSON.parse(bytes),request=data.prepare_request,intent=request.intent;
checkDatasetView(data.dataset,intent.topic,intent.dataset);
const draft={kind:intent.mapping.kind,dimensions:intent.dimensions,measure:intent.measure,title:intent.mapping.options.title,pageSize:intent.mapping.table?.page_size||20,filters:intent.filters};
assert.deepEqual(datasetIntent(data.dataset,draft).filters,intent.filters);
for(const filter of intent.filters){const parameter=data.created.block.parameters.find(p=>p.dimension?.dimension===filter.dimension);assert(parameter,'created native parameter');assert.deepEqual(filterInputValue(filterInputState(parameter,filter.default)),filter.default);}
let calls=0;const native=data.option_request,lookup=new FilterOptionLookup(async(name,args)=>{calls++;assert.equal(name,'reporting_authoring_dataset_options_v1');assert.deepEqual(args,native);return data.option_response;},native.target,{operation:()=>native.operation,locale:native.locale});
await lookup.search(native.search,native.cursor);assert.equal(calls,1);assert.deepEqual(lookup.values,data.option_response.options);assert.equal(lookup.unknown,false);lookup.close();assert.equal(lookup.values.length,0);
console.log('PASS: actual filtered Dataset DTO, canonical intent/defaults, and exact explicit lookup request/response decoded by UI modules. No source/model calls from Node.');
