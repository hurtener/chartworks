// Synthetic metadata only. No provider, source, model or credential adapter.
import assert from 'node:assert/strict';
import {publicationFixture, clone} from './publication-fixture.mjs';

export const publicationTools = ['reporting_authoring_capabilities_v1', 'reporting_search', 'reporting_describe', 'reporting_view', 'reporting_runs',
  ...['drafts', 'read', 'save', 'block_read', 'lifecycle', 'block_publish', 'rebind_published', 'report_transition'].map(name => `reporting_authoring_${name}_v1`)];
export const publicationWrites = ['reporting_authoring_block_publish_v1', 'reporting_authoring_rebind_published_v1', 'reporting_authoring_report_transition_v1'];

export function publicationBrowserFixture() {
  const native = publicationFixture();
  // Pre-existing private history, never created/executed by this journey. Its
  // immutable privacy/revision must survive later metadata publication.
  const privatePreview = {kind: 'report', run: 'synthetic-private-before-publication', target: {kind: 'report', id: 'report-a', revision: 1},
    state: 'completed', private: true, created_at: '2026-10-04T06:00:00Z', expires_at: '2099-01-01T00:00:00Z'};
  const beforePrivatePreview = clone(privatePreview);
  return {...native, privatePreview, beforePrivatePreview, async invoke(name, args) {
    if (name === 'reporting_describe' || name === 'reporting_runs') {
      native.calls.push({name, args: clone(args)});
      assert.equal(native.state().published_revision, 2, 'Consumer metadata is read only after actual publication');
      if (name === 'reporting_describe') {
        assert.deepEqual(args, {target: {kind: 'report', id: 'report-a', revision: 2}, locale: 'en-US', outputs: null});
        return {version: 'reporting-view-v1', resource: {target: clone(args.target), metadata: [{locale: 'en-US', title: 'Lifecycle report'}]},
          locale: 'en-US', timezone: 'UTC', filters: [], parameters: [], outputs: []};
      }
      assert.deepEqual(args, {kind: 'report', resource: 'report-a', after: '', limit: 40});
      return {version: 'reporting-view-v1', items: [clone(privatePreview)], next: ''};
    }
    return native.invoke(name, args);
  }};
}

// This function is serialized into the synthetic parent page. The CDP binding
// relays only tool calls to the same Node publicationFixture used by bundle tests.
export function initializePublicationHost(embedded, names) {
  const frame = document.getElementById('app'), frameID = 'publication-frame', generation = 1;
  const challenge = 'synthetic-publication-challenge', pending = new Map();
  let sequence = 0;
  Object.assign(window, {hostMessages: [], hostCalls: [], hostErrors: [], resizeCount: 0, lastResizeAt: 0});
  const envelope = message => embedded ? {protocol: 'chartworks-report-app-v1', frame: frameID, generation, ...message} : {jsonrpc: '2.0', ...message};
  const send = message => frame.contentWindow.postMessage(envelope(message), location.origin);
  window.completePublicationCall = (id, result) => {
    const finish = pending.get(id);
    if (!finish) throw new Error('Uncorrelated synthetic publication reply');
    pending.delete(id);finish(result);
  };
  const invoke = params => new Promise(resolve => {
    const id = ++sequence;pending.set(id, resolve);
    window.publicationInvoke(JSON.stringify({id, name: params.name, args: params.arguments}));
  });
  frame.addEventListener('load', () => {
    if (embedded) send({method: 'bootstrap', params: {challenge}});
  });
  window.addEventListener('message', event => {
    if (event.source !== frame.contentWindow || event.origin !== location.origin) return;
    const message = event.data;
    if (embedded ? message?.protocol !== 'chartworks-report-app-v1' || message.frame !== frameID || message.generation !== generation : message?.jsonrpc !== '2.0') return;
    hostMessages.push(message);
    if (message.method === 'initialize' && embedded) {
      if (message.params?.challenge !== challenge) throw new Error('Incorrect embedded challenge');
      send({id: message.id, result: {challenge, tools: true, capabilities: {supported_tools: names}, context: {theme: 'light', locale: 'en-US'}}});return;
    }
    if (message.method === 'ui/initialize' && !embedded) {
      if (message.params?.protocolVersion !== '2026-01-26') throw new Error('Incorrect MCP protocol');
      send({id: message.id, result: {protocolVersion: '2026-01-26', hostCapabilities: {serverTools: {}}, hostContext: {theme: 'light', locale: 'en-US', 'chartworks/supported-tools': names}}});return;
    }
    if (message.method === 'ui/notifications/size-changed') {
      lastResizeAt = performance.now();resizeCount++;
      frame.style.height = Math.min(2416, Math.max(500, Math.ceil(message.params.height) + 16)) + 'px';return;
    }
    if (message.method === 'tools/call') {
      hostCalls.push(message.params);
      void invoke(message.params).then(result => send({id: message.id, result})).catch(error => hostErrors.push(String(error)));return;
    }
    if (!['ui/notifications/initialized', undefined].includes(message.method)) hostErrors.push('Unexpected host request: ' + message.method);
  });
  window.closeApp = () => send(embedded ? {method: 'close'} : {id: 900, method: 'ui/resource-teardown'});
}
