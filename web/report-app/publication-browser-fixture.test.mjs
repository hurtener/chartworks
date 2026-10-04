import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import {publicationFrameFits} from './publication-browser-fixture.mjs';

const geometry = (height, frameHeight) => ({root: {top: 0, bottom: height, left: 0, right: 1440, height}, frameHeight, frameWidth: 1440, resizeCount: 1, quietMs: 200});

test('publication screenshot geometry accepts the exact short and tall host heights', () => {
  assert.equal(publicationFrameFits(geometry(446, 500)), true, 'short content keeps the explicit 500px host floor');
  assert.equal(publicationFrameFits(geometry(1160, 1176)), true, 'tall content keeps the explicit 16px host padding');
  assert.equal(publicationFrameFits(geometry(1160.25, 1177)), true, 'fractional root height uses the host ceil rule');
  assert.equal(publicationFrameFits(geometry(1160, 1178)), true, 'bounded two-pixel layout rounding is tolerated');
});

test('publication screenshot geometry rejects actual clipping and incorrect host sizing', () => {
  assert.equal(publicationFrameFits(geometry(510, 500)), false, 'content extending below the frame is clipped');
  assert.equal(publicationFrameFits(geometry(1160, 1100)), false, 'tall content is not cropped to the viewport');
  assert.equal(publicationFrameFits(geometry(446, 600)), false, 'a stale oversized frame fails the exact floor formula');
  assert.equal(publicationFrameFits(geometry(1160, 1180)), false, 'unexpected blank tail above tolerance fails');
  const outside = geometry(446, 500);outside.root.right = 1442;
  assert.equal(publicationFrameFits(outside), false, 'horizontal clipping still fails');
  outside.root.right = 1440;outside.root.top = -1;
  assert.equal(publicationFrameFits(outside), false, 'content scrolled above the frame still fails');
});

test('publication screenshot geometry requires stable bounded content in its serialized browser form', () => {
  assert.equal(publicationFrameFits({...geometry(446, 500), quietMs: 199}), false);
  assert.equal(publicationFrameFits({...geometry(446, 500), resizeCount: 0}), false);
  assert.equal(publicationFrameFits(geometry(2400, 2416)), false, 'the existing maximum-frame bound remains strict');
  assert.equal(publicationFrameFits(geometry(NaN, 500)), false);
  const serialized = vm.runInNewContext(`(${publicationFrameFits.toString()})`);
  assert.equal(serialized(geometry(446, 500)), true, 'the actual CDP predicate has no missing module dependencies');
  assert.equal(serialized(geometry(1160, 1176)), true);
  assert.equal(serialized(geometry(510, 500)), false);
});
