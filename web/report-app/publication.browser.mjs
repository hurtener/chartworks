// Actual production HTML in hosted Chromium, with synthetic publication metadata only.
import assert from 'node:assert/strict';
import {runHostedBrowser} from './hosted-browser-harness.mjs';
import {publicationBrowserFixture, publicationTools, publicationWrites} from './publication-browser-fixture.mjs';
const [htmlPath,screenshotPath,mode]=process.argv.slice(2);
const fixture=publicationBrowserFixture(),writes=()=>fixture.calls.filter(call=>publicationWrites.includes(call.name));
const snapshot=()=>structuredClone({state:fixture.state(),definitions:[...fixture.definitions],blocks:[...fixture.blocks],privatePreview:fixture.privatePreview});
const original=snapshot();
const proof=await runHostedBrowser({htmlPath,screenshotPath,mode,fixture,tools:publicationTools,label:'Synthetic host fixture · Publication metadata only · No production provider connection'},async ({root,button,check,equal,ready,textHas,clickElement,click,capture,evaluate,requests,assertions})=>{
const panel=`${root}.querySelector('details[aria-label="Publication and review"]')`;
const checkbox=label=>`Array.from(${root}.querySelectorAll('input[type="checkbox"]')).find(e=>e.getAttribute('aria-label')===${JSON.stringify(label)})`;
async function unchanged(message, action) {const before = snapshot(), count = writes().length;await action();equal(snapshot(), before, message);equal(writes().length, count, message + ' makes no metadata write');}
async function confirm(label) {await unchanged('Confirmation checkbox is local until its action is clicked', async () => {await clickElement(checkbox(label));await check(`${checkbox(label)}?.checked===true`, label);});}
async function disabled(label) {
  await unchanged(label + ' requires its own explicit confirmation', async () => {
    await check(`${button(label)}?.disabled===true`, label + ' is disabled before confirmation');
    await evaluate(`${button(label)}.click()`);await ready();
  });
}
  await textHas('No reports have been shared with you yet.');
  await check(`hostMessages.filter(m=>m.method===${JSON.stringify(mode === 'embedded' ? 'initialize' : 'ui/initialize')}).length===1&&!hostMessages.some(m=>m.method===${JSON.stringify(mode === 'embedded' ? 'ui/initialize' : 'initialize')})`, 'Exact requested adapter handshakes once without fallback');
  await click('Build');await textHas('Private drafts');await click('Lifecycle report');await textHas('Private draft revision 1 loaded.');
  equal(snapshot(), original, 'Opening the native draft catalog and selecting its report are read-only');
  await check(`${panel}?.open===false&&${panel}.querySelector('summary').textContent==='Publish and review'`, 'Publication is compact by default');
  await unchanged('Expanding publication controls is local', () => clickElement(`${panel}.querySelector('summary')`));
  await check(`${panel}.open===true`, 'Deliberate expansion exposes publication controls');
  await unchanged('Inspection is metadata-only', () => click('Check report'));
  await textHas('Publication status inspected.');
  await check(`${panel}.open&&${panel}.textContent.includes('ENTIRE immutable chart revision')&&${panel}.textContent.includes('every output listed above')&&${panel}.textContent.includes('audience names and count are not provided')&&JSON.stringify(Array.from(${panel}.querySelectorAll('.publication-chart li'),e=>e.textContent))===JSON.stringify(['amount · chart · Revenue','Table'])`, 'Whole-chart disclosure includes the unselected table output and bounded audience effect');
  await disabled('Publish chart');
  await capture(screenshotPath);
  await confirm('Publish this chart and every item listed above.');
  await click('Publish chart');await textHas('The entire chart revision is published.');
  equal(writes().length, 1, 'Only chart publication was written');
  equal(fixture.blocks.get('chart-a:2').block.private, false, 'Exact validated chart revision is published');
  equal(fixture.report(1).definition, original.definitions[0][1], 'Chart publication does not rewrite any report pin');
  equal(fixture.state().published_revision, 0, 'Chart publication does not publish the report');
  await confirm('Summary / first');
  await confirm('Summary / second');
  await disabled('Update selected charts');
  await confirm('Update only the selected items with the published charts.');
  await click('Update selected charts');await textHas('Selected widgets now use their published chart revisions.');
  equal(writes().length, 2, 'Only one separate rebind is written');
  const expected = structuredClone(original.definitions[0][1]);
  for (const widget of expected.report_pages[0].widgets) {widget.block.policy = 'published';delete widget.block.digest;}
  equal(fixture.report(2).definition, expected, 'Rebind preserves all outputs, filters, literals, grids and the untargeted Notes page');
  equal(fixture.definitions.get(1), original.definitions[0][1], 'Original private report revision is immutable');
  equal(fixture.state().published_revision, 0, 'Rebound revision remains private');
  await disabled('Request review');
  await confirm('Send this saved draft for review.');
  await click('Request review');await textHas('Report submitted for review. It remains private.');
  equal(writes().length, 3, 'Only the separately confirmed review transition is written');
  equal([fixture.state().draft_revision, fixture.state().review_revision, fixture.state().published_revision], [0, 2, 0], 'Review has its independent private pointer');
  await check(`${panel}.textContent.includes('Pending review')&&${panel}.textContent.includes('Existing private preview results stay private. No public data run occurs here.')`, 'Review discloses retained privacy and no public run');
  await disabled('Publish report');
  await confirm('Publish the reviewed report.');
  await click('Publish report');await textHas('Reviewed report published. Existing private previews remain private.');
  equal([fixture.state().draft_revision, fixture.state().review_revision, fixture.state().published_revision], [0, 0, 2], 'Exactly the reviewed report revision is published');
  equal(fixture.report(2).private, false, 'Native publication is reflected in report metadata');
  equal(fixture.privatePreview, fixture.beforePrivatePreview, 'Earlier retained run stays private at its original revision');
  const beforeBrowse = fixture.calls.length;
  await unchanged('Browse reads the new publication without writes', () => click('Browse'));
  await textHas('Choose a report');
  await unchanged('Reopening the collapsed catalog is local', () => click('Show reports'));
  await check(`${root}.dataset.mode==='consumer'&&${button('Lifecycle report')}?.className==='catalog-item'&&${button('Lifecycle report')}.getClientRects().length>0&&!${root}.querySelector('.retained-preview,.preview-provenance')`, 'Consumer catalog lists the actual publication without adopting private preview values');
  await unchanged('Selecting the publication reads metadata and private history only', () => click('Lifecycle report'));
  await textHas('Current published revision 2');
  await check(`${root}.querySelectorAll('.run-row').length===1&&${root}.querySelector('.run-row').textContent.includes('Private preview')&&!${root}.querySelector('.retained-preview,.preview-provenance')&&${button('View saved copy')}?.disabled===false`, 'Pre-existing private history stays explicitly private and requires a separate retained-open action');
  equal(fixture.calls.slice(beforeBrowse).map(call => call.name), ['reporting_authoring_capabilities_v1', 'reporting_search', 'reporting_describe', 'reporting_runs', 'reporting_authoring_capabilities_v1'], 'Browse and selection make only exact public catalog/metadata/history reads');
  await capture(screenshotPath.replace(/\.png$/, '.published.png'));
  equal(writes().map(call => ({name: call.name, args: call.args})), [
    {name: 'reporting_authoring_block_publish_v1', args: {block: 'chart-a', expected_version: 8, revision: 2, digest: 'a'.repeat(64), evidence: 'validation-a'}},
    {name: 'reporting_authoring_rebind_published_v1', args: {report: 'report-a', expected_version: 1, revision: 1, digest: '1'.repeat(64), widgets: ['first', 'second'].map(widget => ({widget, block: 'chart-a', revision: 2, digest: 'a'.repeat(64)}))}},
    {name: 'reporting_authoring_report_transition_v1', args: {report: 'report-a', expected_version: 2, revision: 2, operation: 'review', note: ''}},
    {name: 'reporting_authoring_report_transition_v1', args: {report: 'report-a', expected_version: 3, revision: 2, operation: 'publish', note: ''}}
  ], 'Exactly four metadata writes preserve exact validation, revision, version and selected-widget pins');
  assert(fixture.calls.every(call => ['reporting_authoring_capabilities_v1', 'reporting_search', 'reporting_describe', 'reporting_runs', 'reporting_authoring_drafts_v1', 'reporting_authoring_read_v1', 'reporting_authoring_lifecycle_v1', ...publicationWrites].includes(call.name)), 'No save, source/model, validation, preparation, execution, or retained-value invocation');
  assertions.push('No hidden save, source/model, validation, preparation, execution or retained-value invocation');
  equal(fixture.report(2).definition.report_pages[1], original.definitions[0][1].report_pages[1], 'Untargeted page remains byte-for-byte unchanged through all four writes');
  await check(`hostErrors.length===0&&document.getElementById('app').contentWindow.storageTouches===0`, 'No protocol error, storage access or credential channel');
  equal(await evaluate('hostCalls.map(c=>({name:c.name,args:c.arguments}))'), fixture.calls, 'All real adapter calls are accounted for by the synthetic metadata ledger');
  await evaluate('closeApp()');await textHas('This report app is closed.');
  assert(requests.some(url => new URL(url).pathname === '/resource'), 'Browser loaded the exact compiled resource');
});
console.log(JSON.stringify({...proof,metadataWrites:writes(),screenshots:[screenshotPath,screenshotPath.replace(/\.png$/,'.published.png')],fixture:'synthetic host and metadata; no production provider connection'}));
