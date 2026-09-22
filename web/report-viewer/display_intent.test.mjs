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
