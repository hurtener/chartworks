import assert from 'node:assert/strict';
import test from 'node:test';
import {amountDisclosureLines} from './app.js';
const evidence=()=>({label:'Known paid amount',evidence:'reviewed_definition',query_outcome:'succeeded',rows_scope:'visible_source_rows',role:'unknown_count',unit:'count',result:{policy:'reviewed-amount-completeness-v1',scope:'returned_query_rows',metric:'known',unknown_count_metric:'counter',status:'incomplete',rows:[{row:7,status:'incomplete',unknown_count:'9007199254740993'}]}});
test('count meaning and exact disclosure do not come from aliases',()=>{
 const d=evidence();d.label='Revenue <script>literal</script>';
 const lines=amountDisclosureLines([d]);
 assert(lines.includes('Unknown amount count (displayed rows): 9007199254740993'));
 assert(lines.includes('Displayed metric unit: count'));
 assert(lines.includes('Evidence: reviewed definition'));
 assert(lines.includes('Scope: returned query rows'));
 assert(lines.some(x=>x.includes('<script>literal</script>'))); // passed as textContent, never HTML
});
test('truncation and unknown counters are not rendered as zero',()=>{
 const d=evidence();d.truncation='row_limit';d.result.status='unknown';d.result.rows=[];
 assert(amountDisclosureLines([d]).includes('Unknown amount count (displayed rows): unknown'));
 assert(amountDisclosureLines([d]).includes('Result truncated; amount completeness is unknown'));
 delete d.truncation;d.result.rows=[{status:'unknown'}];
 assert(amountDisclosureLines([d]).includes('Unknown amount count (displayed rows): unknown'));
 assert(amountDisclosureLines([d],'es').some(x=>x.includes('desconocido')));
});
test('shared amount/count declaration is disclosed once and old outputs stay unchanged',()=>{
 const d=evidence();assert.deepEqual(amountDisclosureLines([d,d]),amountDisclosureLines([d]));assert.deepEqual(amountDisclosureLines(undefined),[]);
});

test('unsupported evidence policy is not displayed as reviewed',()=>{const d=evidence();d.result.policy='untrusted';assert.throws(()=>amountDisclosureLines([d]));});
