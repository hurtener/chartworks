import test from 'node:test';
import assert from 'node:assert/strict';
import {DatasetSession, newPreparationOperation, datasetIntent} from './dataset.js';
import {appError, unpack} from './model.js';
import {datasetClone, datasetFixture, datasetView, datasetPrepared, datasetCreated, datasetOperation} from './dataset-fixture.mjs';

function staged(invoke, operation = newPreparationOperation) {
  const session = new DatasetSession(invoke, {operation});
  session.view = datasetClone(datasetView);
  session.newBlock = 'private-chart';
  session.edit(draft => Object.assign(draft, {title: 'Revenue', dimensions: ['region'], measure: 'revenue'}));
  return session;
}

test('Prepare key uses canonical seconds and lowercase 128-bit UUID spelling only at explicit dispatch', async t => {
  let now = 1791095400123, keys = 0;
  t.mock.method(Date, 'now', () => now);
  t.mock.method(crypto, 'randomUUID', () => { keys++; return '01234567-89ab-4def-8012-3456789abcde'; });
  const f = datasetFixture(), session = staged(f.invoke);
  assert.equal(keys, 0);
  now += 86400000; // A long staged editing session must not age an uncreated key.
  session.edit(draft => { draft.title = 'Reviewed revenue'; });
  assert.equal(keys, 0);
  await session.prepare();
  assert.equal(keys, 1);
  assert.equal(session.request.operation, `prepare:${Math.floor(now / 1000)}:0123456789ab4def80123456789abcde`);
  assert.equal(session.request.operation_version, 'prepare-v1');
  assert.equal(session.custody.operation_version, 'prepare-v1');
  assert.deepEqual(f.calls[0].args, session.request);
  now += 86400000;
  await session.inspect('inspect');
  await session.create();
  assert.equal(keys, 1, 'status, control and Create never rotate the key');
  assert.equal(session.request.operation_version, 'prepare-v1');
  assert.deepEqual(f.calls[1].args, {new_block: 'private-chart', preparation: 'preparation-a', action: 'inspect'});
  assert.deepEqual(f.calls[2].args, {new_block: 'private-chart', preparation: 'preparation-a', digest: 'b'.repeat(64)});
});

test('malformed, legacy and noncanonical generated preparation keys fail before dispatch', async () => {
  for (const operation of ['legacy-operation', '', null, 123, datasetOperation.toUpperCase(), datasetOperation.replace(':179', ':0179'), datasetOperation.replace(':1791095400:', ':0:'), datasetOperation.replace(':1791095400:', ':-1:'), datasetOperation.replace(':1791095400:', ':1.5:'), datasetOperation.replace(':1791095400:', ':9007199254740993:'), datasetOperation.slice(0, -1), datasetOperation + '0', datasetOperation + '\n']) {
    const calls = [], session = staged((...args) => { calls.push(args); }, () => operation);
    await assert.rejects(session.prepare(), /invalid_request/);
    assert.deepEqual(calls, []);
    assert.equal(session.request, null);
    assert.equal(session.custody, null);
  }
});

test('client does not grant freshness: canonical old or future timestamps reach the native admission decision', async () => {
  for (const timestamp of [1, 9999999999]) {
    const operation = `prepare:${timestamp}:${'a'.repeat(32)}`, calls = [];
    const session = staged(async (name, args) => { calls.push({name, args}); throw appError('preparation_operation_expired'); }, () => operation);
    await assert.rejects(session.prepare(), /preparation_operation_expired/);
    assert.equal(calls.length, 1);
    assert.equal(calls[0].args.operation, operation);
    assert.equal(session.rejected, 'preparation_operation_expired');
    assert.equal(session.unknown, '');
    assert.equal(session.canCreate(), false);
  }
});

test('typed pre-admission rejections require review then another explicit Prepare without silent retry', async () => {
  for (const code of ['preparation_contract_required', 'preparation_operation_expired']) {
    let keys = 0;
    const calls = [], session = staged(async (name, args) => {
      calls.push({name, args: datasetClone(args)});
      throw unpack({isError: true, structuredContent: {error: {code, outcome: 'not_started'}}});
    }, () => `prepare:1791095400:${String(++keys).padStart(32, '0')}`);
    await assert.rejects(session.prepare(), e => e.code === code);
    const pinned = datasetClone(session.request);
    assert.equal(session.rejected, code);
    assert.equal(session.unknown, '');
    assert.equal(session.locked, true);
    await assert.rejects(session.prepare(), /busy/);
    await assert.rejects(session.inspect(), /busy/);
    await assert.rejects(session.create(), /busy/);
    assert.equal(keys, 1);
    session.suspend();
    assert.deepEqual(session.request, pinned);
    assert.equal(session.rejected, code);
    session.reviewPreparation();
    assert.equal(session.custody, null);
    assert.equal(session.view, null);
    assert.deepEqual(session.request, pinned, 'the rejected request is retained until a deliberate new Prepare');
    assert.equal(calls.length, 1);
    assert.equal(keys, 1);
    session.view = datasetClone(datasetView);
    await assert.rejects(session.prepare(), e => e.code === code);
    assert.equal(keys, 2);
    assert.equal(calls.length, 2);
    assert.notEqual(session.request.operation, pinned.operation);
    assert.equal(session.request.operation_version, pinned.operation_version);
  }
});

test('unknown or conflicting-outcome replies never enable the pre-admission reset', async () => {
  for (const [code, unknown] of [['unavailable', true], ['cancelled_or_timed_out', true], ['forbidden', false], ['conflict', false], ['preparation_contract_required', true], ['preparation_operation_expired', true]]) {
    let keys = 0, calls = 0;
    const session = staged(async () => { calls++; throw appError(code, unknown); }, () => { keys++; return datasetOperation; });
    await assert.rejects(session.prepare());
    const request = datasetClone(session.request), custody = datasetClone(session.custody);
    assert.equal(session.unknown, 'prepare');
    assert.equal(session.rejected, '');
    session.suspend();
    assert.throws(() => session.reviewPreparation(), /busy/);
    await assert.rejects(session.prepare(), /busy/);
    await assert.rejects(session.inspect());
    assert.deepEqual(session.request, request);
    assert.deepEqual(session.custody, custody);
    assert.equal(calls, 2, 'one Prepare and one inspection only');
    assert.equal(keys, 1);
  }
});

test('a rejected generator cannot reuse the previous operation after explicit review', async () => {
  let calls = 0;
  const session = staged(async () => { calls++; throw appError('preparation_operation_expired'); }, () => datasetOperation);
  await assert.rejects(session.prepare());
  session.reviewPreparation();
  session.view = datasetClone(datasetView);
  await assert.rejects(session.prepare(), /invalid_request/);
  assert.equal(calls, 1);
});

test('retained legacy custody can still inspect and Create without a new key or version', async () => {
  const intent = datasetIntent(datasetView, {title: 'Revenue', kind: 'bar', dimensions: ['region'], measure: 'revenue'});
  const prepared = datasetPrepared({new_block: 'private-chart', operation: 'retained-legacy-operation', intent});
  const calls = [], session = staged(async (name, args) => {
    calls.push({name, args});
    return name.includes('create_prepared') ? datasetCreated(prepared) : prepared;
  }, () => assert.fail('Legacy recovery cannot generate a fresh key'));
  session.custody = {new_block: prepared.new_block, operation: prepared.operation};
  session.acceptedIntent = intent;
  session.unknown = 'prepare';
  await session.inspect();
  assert.equal(session.canCreate(), true);
  await session.create();
  assert.deepEqual(calls[0].args, {new_block: prepared.new_block, operation: prepared.operation});
  assert.deepEqual(calls[1].args, {new_block: prepared.new_block, preparation: prepared.preparation, digest: prepared.digest});
  assert.equal(session.request, null);
});


test('hosted synthetic dataset fixture projects the exact current native capture without a legacy copy', async () => {
  const {readFile} = await import('node:fs/promises');
  const {browserDatasetSamples, projectDatasetSamples} = await import('./browser_fixture.mjs');
  const native = JSON.parse(await readFile(new URL('./testdata/dataset-native-public.json', import.meta.url), 'utf8'));
  assert.deepEqual(browserDatasetSamples, projectDatasetSamples(native));
  assert.equal(browserDatasetSamples.request.operation_version, 'prepare-v1');
  assert.match(browserDatasetSamples.request.operation, /^prepare:[1-9][0-9]*:[a-f0-9]{32}$/);
  assert.equal(browserDatasetSamples.preparation.operation, native.request.operation);
  assert.deepEqual(browserDatasetSamples.view_root.selection, native.view_root.selection);
  assert.equal(browserDatasetSamples.view_root.output.chart.points[0].value.exact, '9007199254740998.625');
});
