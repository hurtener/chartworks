import test from 'node:test';
import assert from 'node:assert/strict';
import {applyHostTheme} from './theme.js';

test('host presentation has a closed token map and resets stale overrides', t => {
  const previous = globalThis.CSS;
  t.after(() => { globalThis.CSS = previous; });
  globalThis.CSS = {supports: (property, value) => property === 'color' ? /^#[0-9a-f]{6}$/.test(value) : property === 'font-family' ? value === 'system-ui, sans-serif' : value === '16px'};
  const values = new Map(), root = {dataset: {}, style: {setProperty: (key, value) => values.set(key, value)}};
  applyHostTheme(root, {theme: 'dark', styles: {variables: {'--color-background-primary': '#123456', '--font-sans': 'system-ui, sans-serif', '--font-text-md-size': '16px', '--unexpected': 'anything'}}});
  assert.equal(root.dataset.theme, 'dark');
  assert.equal(values.get('--app-paper'), '#123456');
  assert.equal(values.get('--app-font-sans'), 'system-ui, sans-serif');
  assert.equal(values.get('--app-text-size'), '16px');
  assert(!values.has('--unexpected'));
  for (const candidate of ['url(https://example.test/font)', 'var(--unknown)', '#ffffff;display:none', 'red', 'x'.repeat(513), 16, null, '#123456\\00']) {
    applyHostTheme(root, {styles: {variables: {'--color-background-primary': candidate}}});
    assert.equal(values.get('--app-paper'), '', String(candidate));
    assert.equal(values.get('--app-font-sans'), '');
  }
  assert.equal(root.dataset.theme, 'light');
});
