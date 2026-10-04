import assert from 'node:assert/strict';
import test from 'node:test';

import {exact} from './app.js';

test('closed display intent is deterministic', () => {
  assert.equal(exact({value:'1234.567'}, {type:'decimal',format:{fraction_digits:2,locale:'es-AR',currency_symbol:'US$'}}), '1.234,57 US$');
  assert.equal(exact({value:'2026-09-22'}, {type:'temporal',format:{date_pattern:'date_short',locale:'es-AR'}}), '22/09/2026');
  assert.equal(exact({value:'2026-09'}, {type:'temporal',format:{date_pattern:'year_month',locale:'es-AR'}}), '09/2026');
  assert.equal(exact({value:'2026-09-22'}, {type:'temporal',format:{date_pattern:'date_long',locale:'en-US'}}), 'September 22, 2026');
  assert.equal(exact({exact:'18.18'}, {type:'decimal',format:{percent:'whole'}}), '18.18%');
});

test('sealed report timezone controls instant calendar formatting', () => {
  const shortDate={type:'temporal',format:{date_pattern:'date_short',locale:'es-AR'}};
  assert.equal(exact({value:'2026-09-22T01:30:00Z'},shortDate,'Missing','America/Argentina/Buenos_Aires'),'21/09/2026');

  const shortTime={type:'temporal',format:{date_pattern:'datetime_short',locale:'en-US'}};
  assert.equal(exact({value:'2026-11-01T05:30:00Z'},shortTime,'Missing','America/New_York'),'Nov 01, 2026 01:30');
  assert.equal(exact({value:'2026-11-01T06:30:00Z'},shortTime,'Missing','America/New_York'),'Nov 01, 2026 01:30');
});

test('numeric display preserves exact precision and native rational spelling',()=>{
 const cases=[['9007199254740993.125',2,'9,007,199,254,740,993.13'],['-9007199254740993.125',2,'-9,007,199,254,740,993.13'],['1.005',2,'1.01'],['-1.005',2,'-1.01'],['+00012.500',2,'12.50'],['-0.000',2,'0.00'],['-0.0001',2,'-0.00'],['1.2345e3',2,'1,234.50'],['12345e-4',3,'1.235'],['-5e-21',20,'-0.00000000000000000001'],['1e3',0,'1,000'],['0000',0,'0']];
 for(const[raw,digits,want]of cases){const cell={value:raw},column={type:'decimal',format:{fraction_digits:digits}},before=JSON.stringify({cell,column});assert.equal(exact(cell,column),want,raw);assert.equal(JSON.stringify({cell,column}),before);}
 assert.equal(exact({value:'1.2345e3'},{type:'decimal',format:{fraction_digits:2,locale:'es-AR',currency:'USD'}}),'1.234,50 USD');
 assert.equal(exact({value:'1e4097'},{type:'decimal',format:{fraction_digits:2}}),'1e4097');
 assert.equal(exact({null:true,value:'1.23'},{type:'decimal',format:{fraction_digits:2}}),'Missing');
});
