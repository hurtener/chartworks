import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
import * as presentation from './presentation.js';
import * as viewer from './app.js';
test('viewer exports the exact shared presentation functions',()=>{for(const name of ['boundedJSON','exact','renderChart','amountDisclosureLines','validateRetainedView','renderRetainedOutput'])assert.equal(viewer[name],presentation[name]);});
test('presentation imports do not initialize a host or register browser listeners',async()=>{const source=await readFile(new URL('./presentation.js',import.meta.url),'utf8');for(const absent of ['class Bridge','class Viewer','postMessage(','window.addEventListener(','document.addEventListener(','getElementById(','ui/initialize','tools/call'])assert.equal(source.includes(absent),false,absent);const context=vm.createContext({get document(){throw Error('presentation accessed DOM while loading');},get window(){throw Error('presentation accessed host while loading');}});new vm.Script(source.replaceAll('export ','')).runInContext(context);});
test('self-contained viewer compilation keeps the bridge and one presentation implementation',async()=>{const common=await readFile(new URL('./presentation.js',import.meta.url),'utf8'),host=await readFile(new URL('./app.js',import.meta.url),'utf8'),names='VERSION, KINDS, boundedJSON, exact, renderChart, amountDisclosureLines, validateRetainedView, renderRetainedOutput, reportStateLabel, reportDate, viewerInternals';const script=`const {${names}}=(()=>{\n${common.replaceAll('export ','')}\nreturn {${names}};\n})();\n`+host.split('\n').filter(line=>!line.startsWith('import ')&&!line.startsWith('export {')).map(line=>line.replaceAll('export ','')+'\n').join('');new vm.Script(script).runInNewContext({});for(const name of ['class Bridge','class Viewer','function renderRetainedOutput','function renderChart'])assert.equal(script.split(name).length-1,1);assert.equal(script.includes('ui/initialize'),true);assert.equal(script.includes('import '),false);});

test('new numeric defaults preserve retained precision while existing zero precision still rounds',()=>{
 const column={type:'decimal',format:{fraction_digits:0,preserve_precision:true}};
 for(const value of ['15.5','12.2500','9007199254740993.123456789012345678901','1.25e-12'])assert.equal(presentation.exact({value},column),value);
 delete column.format.preserve_precision;assert.equal(presentation.exact({value:'15.5'},column),'16');
});

test('client labels preserve warnings, unknown states and dates without exposing transport codes',()=>{
 assert.equal(presentation.reportStateLabel('partial'),'Some data is missing');
 assert.equal(presentation.reportStateLabel('failed','es-AR'),'No se pudo actualizar');
 assert.equal(presentation.reportStateLabel('unexpected_internal_state'),'Status unavailable');
 assert.match(presentation.reportDate('2026-10-09T12:30:00Z','en-US','UTC'),/Oct 9, 2026/);
 assert.equal(presentation.reportDate('not a date'),'Date unavailable');
 assert.match(presentation.reportDate('2026-10-09T12:30:00Z','bad_locale','bad_zone'),/UTC$/);
 assert.match(presentation.chartWarning('truncated_result_not_full_source'),/does not include all data/);
 assert.match(presentation.chartWarning('missing_values_preserved','es'),/No se consideran iguales a cero/);
 assert.match(presentation.chartWarning('new_server_warning'),/Some data may not be shown correctly/);
 assert.equal(presentation.chartLabel('kpi'),'Summary number');
});
