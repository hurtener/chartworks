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
