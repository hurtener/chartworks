import test from 'node:test';
import assert from 'node:assert/strict';
import {node, button, selectField, textField} from './dom.js';
import * as codes from './error-codes.js';
import {appError} from './model.js';

class Element {
  constructor(tag) { this.tagName = tag; this.children = []; this.attributes = {}; this.listeners = {}; }
  set innerHTML(_) { assert.fail('Shared controls must not parse labels or values as HTML'); }
  set outerHTML(_) { assert.fail('Shared controls must not parse labels or values as HTML'); }
  append(...children) { this.children.push(...children); }
  setAttribute(name, value) { this.attributes[name] = String(value); }
  addEventListener(name, listener) { this.listeners[name] = listener; }
}
function installDOM(t) {
  const previous = globalThis.document;
  globalThis.document = {createElement: tag => new Element(tag)};
  t.after(() => { if (previous === undefined) delete globalThis.document; else globalThis.document = previous; });
}

test('shared nodes and buttons preserve text, class, disabled state and explicit click events', t => {
  installDOM(t);
  const text = '<script>inert title</script>', element = node('p', text, 'notice error');
  assert.equal(element.textContent, text);
  assert.equal(element.className, 'notice error');
  assert.equal(node('section').textContent, undefined);
  assert.equal(node('span', 0).textContent, '0');
  assert.equal(node('span', null).textContent, 'null');
  let calls = 0;
  const action = button(text, () => calls++, true);
  assert.equal(action.type, 'button');
  assert.equal(action.disabled, true);
  assert.equal(action.textContent, text);
  assert.equal(calls, 0);
  action.listeners.click();
  assert.equal(calls, 1);
  assert.equal(button('Enabled', () => {}).disabled, false);
});

test('shared selects keep exact values, option availability and change-only dispatch', t => {
  installDOM(t);
  const events = [], field = selectField('Reviewed measure', [
    {value: '', label: 'Choose field'},
    {value: 'amount', label: '<b>Amount</b>'},
    {value: 'blocked', label: 'Unavailable', disabled: true},
  ], 'amount', value => events.push(value), true), select = field.children[0];
  assert.equal(field.textContent, 'Reviewed measure');
  assert.equal(select.attributes['aria-label'], 'Reviewed measure');
  assert.equal(select.disabled, true);
  assert.deepEqual(select.children.map(option => [option.value, option.selected, option.disabled]), [
    ['', false, false], ['amount', true, false], ['blocked', false, true],
  ]);
  assert.equal(select.children[1].textContent, '<b>Amount</b>');
  assert.deepEqual(events, []);
  select.value = '';
  select.listeners.change();
  assert.deepEqual(events, ['']);
});

test('shared fields retain distinct text-length and numeric bounds plus edit defaults', t => {
  installDOM(t);
  const events = [], field = textField('Table page size', 100, value => events.push(value), {
    type: 'number', maxLength: 4, min: 1, max: 1000, disabled: true,
  }), input = field.children[0];
  assert.equal(input.tagName, 'input');
  assert.equal(input.type, 'number');
  assert.equal(input.value, 100);
  assert.equal(input.defaultValue, 100);
  assert.equal(input.maxLength, 4);
  assert.equal(input.min, 1);
  assert.equal(input.max, 1000);
  assert.equal(input.disabled, true);
  assert.equal(input.attributes['aria-label'], 'Table page size');
  input.value = '20';
  assert.deepEqual(events, []);
  input.listeners.change();
  assert.deepEqual(events, ['20']);
  assert.equal(input.defaultValue, 100);
  const heading = textField('Heading', '<script>plain</script>', () => {}, {type: 'textarea', maxLength: 32768}).children[0];
  assert.equal(heading.tagName, 'textarea');
  assert.equal(heading.type, undefined);
  assert.equal(heading.maxLength, 32768);
  assert.equal(heading.value, '<script>plain</script>');
  const empty = textField('Optional title', null, () => {}).children[0];
  assert.equal(empty.value, '');
  assert.equal(empty.maxLength, 256);
  assert.equal(empty.disabled, false);
  assert.equal(empty.min, undefined);
  assert.equal(empty.max, undefined);
});

test('shared error spellings preserve the wire contract and unknown-outcome flags', () => {
  assert.deepEqual(Object.values(codes).sort(), [
    'busy', 'cancelled_or_timed_out', 'conflict', 'forbidden',
    'invalid_request', 'limit_exceeded', 'stale_validation', 'unavailable',
  ]);
  for (const code of Object.values(codes)) {
    const failure = appError(code);
    assert.equal(failure.code, code);
    assert.equal(failure.message, code);
    assert.equal(failure.unknown, false);
    assert.equal(appError(code, true).unknown, true);
    assert.notEqual(appError(code), failure);
  }
  assert.equal(appError('unregistered_error').code, 'unavailable');
});

test('typing commits before blur and duplicate change cannot replace the clicked action', t => {
  installDOM(t);
  const values = [], input = textField('Chart title', 'Before', value => values.push(value)).children[0];
  input.value = 'Typed';
  input.listeners.input();
  assert.deepEqual(values, ['Typed']);
  input.listeners.change();
  assert.deepEqual(values, ['Typed']);
  input.value = 'Changed programmatically';
  input.listeners.change();
  assert.deepEqual(values, ['Typed', 'Changed programmatically']);
});
