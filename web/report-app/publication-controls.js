import {node, button} from './dom.js';
import {publicationEligible, publishedWidgets, hasPrivatePins, stageLabel, validRejectionNote} from './publication.js';


function checkbox(label,checked,disabled,fn){const el=node('label',undefined,'publication-confirmation'),input=node('input');input.type='checkbox';input.checked=checked;input.disabled=disabled;input.setAttribute('aria-label',label);input.addEventListener('change',()=>fn(input.checked));el.append(input,node('span',label));return el;}
function confirm(parent,app,action,target,label,buttonLabel,disabled){const p=app.publication;parent.append(checkbox(label,p.confirmed(action,target),disabled,value=>{p.confirm(action,target,value);app.render();}),button(buttonLabel,()=>void app.perform(()=>app.publishAction(action,target)),disabled||!p.confirmed(action,target)));}

// DOM is always rebuilt from an exact server inspection. No count, recipient,
// grant, source result, or publication state is inferred from a private preview.
export function renderPublicationControls(parent,app){
  const p=app.publication,s=app.session;if(!p.available()||!s.state)return;
  const section=node('details',undefined,'publication-controls'),owner=s.state.id,generation=p.generation,epoch=app.epoch,unknown=p.records(owner).some(op=>op.status==='unknown');section.setAttribute('aria-label','Publication and review');section.open=p.panelExpanded||unknown;section.append(node('summary',unknown?'Publish and review · action unconfirmed':'Publish and review'));section.addEventListener('toggle',()=>{if(!app.closed&&app.epoch===epoch&&p.generation===generation&&app.session.state?.id===owner)p.panelExpanded=section.open;});
  const unavailable=app.busy||s.dirty||!!app.mapping||!!app.datasetSession||s.conflict||s.uncertain;
  section.append(button('Inspect publication status',()=>void app.perform(()=>app.inspectPublication()),unavailable));
  if(s.dirty)section.append(node('p','Save report before inspecting or confirming publication.','metadata'));
  for(const op of p.records(s.state.id)){
    if(op.status==='unknown')section.append(node('p',`Unknown ${op.action.replaceAll('_',' ')} outcome at revision ${op.args.revision}. Repeating the change is blocked. Inspect its exact original revision.`, 'notice error'),button('Inspect uncertain '+op.action.replaceAll('_',' '),()=>void app.perform(()=>app.inspectPublicationOperation(op)),app.busy));
    else if(op.action==='block_publish'&&['confirmed','observed'].includes(op.status))section.append(node('p',`Chart ${op.args.block}, revision ${op.args.revision}, is published. A later report edit cannot roll that publication back.`, 'metadata'));
    if(p.retryEligible(op)){
      section.append(node('p','The head and version are unchanged, but the request may still be completing. Confirming retry sends the same metadata-only request and version check; it cannot commit twice, validate data or run queries.','notice'));
      section.append(node('p',`Original ${op.action.replaceAll('_',' ')}: ${op.args.block||op.args.report} · revision ${op.args.revision} · expected version ${op.args.expected_version} · digest ${op.args.digest||op.digest}${op.args.evidence?' · evidence '+op.args.evidence:''}`,'metadata'));
      if(op.disclosure){section.append(node('p','The ENTIRE immutable chart revision and ALL these outputs become available to currently centrally authorized readers:','metadata'));for(const output of op.disclosure)section.append(node('p',`${output.id} · ${output.kind}${output.title?' · '+output.title:''}`,'metadata'));}
      if(op.action==='reject')section.append(node('p',`Original rejection note: ${op.args.note}`,'metadata'),node('p','Return only this reviewed revision for amendment. The newer draft and current publication are preserved.','notice'));
      if(op.args.widgets)for(const pin of op.args.widgets)section.append(node('p',`Selected widget ${pin.widget} · ${pin.block} · revision ${pin.revision} · digest ${pin.digest}`,'metadata'));
      section.append(checkbox('I confirm retrying this identical inspected request with its original version and revision.',p.retryConfirmed(op),unavailable,value=>{p.confirmRetry(op,value);app.render();}),button('Retry identical inspected request',()=>void app.perform(()=>app.retryPublicationOperation(op)),unavailable||!p.retryConfirmed(op)));
    }
  }
  if(!p.current(s)){section.append(node('p',s.capabilities.can_save?'Check this saved revision, then publish charts, link them, submit for review and publish the report.':'Inspect this saved revision to see its review status and available publication actions.','metadata'));parent.append(section);return;}
  const view=p.view,r=view.report,locked=unavailable||p.blocked(s.state.id);
  const step=!r.private?'Published':view.blocks.some(item=>!item.published_at)?'1 · Publish charts':hasPrivatePins(view)?'2 · Link published charts':view.stage==='review'?'4 · Publish report':'3 · Submit for review';
  section.append(node('h2',step),node('p',`${stageLabel(view.stage)} · revision ${r.revision}`,'metadata'));
  const coordinates=node('details');coordinates.append(node('summary','Inspected report coordinates'),node('p',`Report ${r.state.id} · revision ${r.revision} · version ${r.state.version} · digest ${r.digest}`,'metadata'));section.append(coordinates);
  if(s.capabilities.can_save&&(view.stage==='review'||r.state.review_revision>0))section.append(node('p',`Revision ${r.state.review_revision} is pending review. Editing and saving creates a new draft and preserves that independent review revision.`,'notice'));
  if(r.state.draft_revision>0&&r.state.draft_revision!==r.revision)section.append(button('Open current draft',()=>app.navigate(()=>app.openDraft(r.state.id,'draft')),app.busy));
  if(view.rejected===true||p.rejected(r.state.id,r.revision))section.append(node('p','This revision was returned for amendment. Edit and save a new revision before resubmitting it for review.','notice'));
  if(r.state.review_revision>0&&r.state.review_revision!==r.revision)section.append(button('Open pending review',()=>app.navigate(()=>app.openDraft(r.state.id,'review')),app.busy));
  const batchTargets=p.chartBatchTargets();
  if(batchTargets.length>1&&p.available('block_publish')){
    const group=node('section',undefined,'publication-batch');group.append(node('h3',`${batchTargets.length} charts ready to publish`),node('p','Includes the entire revision and every listed output of each ready chart below. Currently authorized readers can access those publications. Audience details are unavailable. Each chart is published separately; an interruption stops the remaining changes.','notice'));
    group.append(checkbox(`I confirm publishing all ${batchTargets.length} ready chart revisions and every listed output.`,p.chartBatchConfirmed(),locked,value=>{p.confirmChartBatch(value);app.render();}),button(`Publish ${batchTargets.length} ready charts`,()=>void app.perform(()=>app.publishChartBatch()),locked||!p.chartBatchConfirmed()));section.append(group);
  }
  for(const item of view.blocks){const b=item.block,card=node(item.published_at?'details':'article',undefined,'publication-chart');if(item.published_at)card.append(node('summary',b.metadata?.[0]?.title||'Published chart'));
    card.append(node('h3',b.metadata?.[0]?.title||b.state.id),node('p',`Chart revision ${b.revision} · ${item.published_at?'Published':'Private'} · ${b.outputs.length} outputs`,'metadata'));
    if(batchTargets.length>1&&publicationEligible(item))card.append(node('p','Included in ready charts','metadata'));
    const outputs=node('ul');for(const output of b.outputs)outputs.append(node('li',`${output.id} · ${output.kind}${output.mapping?.options?.title?' · '+output.mapping.options.title:''}`));card.append(outputs);
    if(!item.published_at&&p.available('block_publish')){
      card.append(node('p','Publishes this ENTIRE immutable chart revision and ALL listed outputs to authorized readers. Audience details are unavailable. Report publication and sharing stay separate.','notice'));
      const evidence=node('details');evidence.append(node('summary','Exact publication and validation coordinates'),node('p',`Block ${b.state.id} · version ${b.state.version} · revision ${b.revision} · digest ${b.digest} · evidence ${b.validation?.id||'missing'}`,'metadata'));card.append(evidence);
      if(publicationEligible(item))confirm(card,app,'block_publish',`${b.state.id}:${b.revision}`,'I confirm publishing the entire chart revision and all listed outputs.','Publish entire chart revision',locked);
      else card.append(node('p','This revision cannot be published yet. Check status, validate private data if required, and inspect again. Access and evidence are rechecked.','metadata'));
    }
    section.append(card);
  }
  const options=publishedWidgets(view);
  if(options.length&&s.capabilities.can_save&&r.private&&r.state.draft_revision===r.revision&&p.available('rebind_published')){
    const group=node('section');group.append(node('h3','Use published charts in this report'),node('p','Link selected widgets to these published charts in a new private draft. Their outputs, filters, parameters and layout stay intact.','metadata'));
    if(options.length>1)group.append(button(p.selection.size===options.length?'Clear selection':`Select all ${options.length} components`,()=>{const value=p.selection.size!==options.length;for(const option of options)p.select(option.widget,value);app.render();},locked));
    for(const option of options)group.append(checkbox(`${option.page} / ${option.title} · ${option.widget} · ${option.block} revision ${option.revision}`,p.selection.has(option.widget),locked,value=>{p.select(option.widget,value);app.render();}));
    if(p.selection.size)confirm(group,app,'rebind_published',undefined,'I confirm rebinding only the selected widgets to their exact published chart revisions.','Rebind selected widgets',locked);
    section.append(group);
  }
  if(hasPrivatePins(view))section.append(node('p','Private chart pins must be explicitly rebound to published revisions before report review.','notice'));
  else if(p.available('report_transition')&&r.private){
    if(r.state.draft_revision===r.revision&&!r.state.review_revision&&view.can_review===true&&view.rejected!==true&&!p.rejected(r.state.id,r.revision)){section.append(node('p','Moves this saved draft into review. You can return it for amendment; it is not published yet.','metadata'));confirm(section,app,'review',undefined,'I confirm submitting this exact report revision for review.','Submit report for review',locked);}
    if(r.state.review_revision===r.revision&&view.can_publish===true){section.append(node('p','Publishes this reviewed revision to authorized readers. Access is checked again. Private previews stay private; no public result is created.','notice'));confirm(section,app,'publish',undefined,'I confirm publishing this exact reviewed report revision.','Publish reviewed report',locked);}
  }
  if(p.available('report_transition')&&r.private&&r.state.review_revision===r.revision&&view.can_reject===true){
    const group=node('section');group.append(node('h3','Return review for amendment'),node('p','Reject this reviewed revision with a note. Keep any newer draft and the current publication. If no draft exists, return this revision to draft. Edit and save before resubmitting.','notice'));
    const label=node('label','Reason for returning review'),input=node('textarea');input.value=p.rejectNote;input.defaultValue=input.value;input.maxLength=2048;input.disabled=locked;input.setAttribute('aria-label','Reason for returning review');input.addEventListener('input',()=>{p.setRejectNote(input.value);app.render();});label.append(input);group.append(label,node('p','A note is required, up to 2,048 UTF-8 bytes.','metadata'));
    if(validRejectionNote(p.rejectNote))confirm(group,app,'reject',undefined,'I confirm rejecting this exact reviewed revision with the note above.','Return reviewed report',locked);
    else group.append(button('Return reviewed report',()=>{},true));
    section.append(group);
  }
  if(r.private&&!hasPrivatePins(view)&&p.available('report_transition')&&(r.state.draft_revision===r.revision&&!r.state.review_revision&&view.can_review!==true||r.state.review_revision===r.revision&&view.can_publish!==true))section.append(node('p','This transition is unavailable under the current lifecycle state or native authority. Refresh access through your host if needed, then inspect again.','metadata'));
  if(!r.private)section.append(button('Open published report',()=>app.navigate(async()=>{const target={kind:'report',id:r.state.id,revision:r.revision},title=r.definition.metadata?.find(m=>m.locale===app.locale)?.title||r.definition.metadata?.[0]?.title;await app.changeMode('consumer');if(app.capabilities.consumer)await app.selectPublished({target,title});else app.message='This publication is unavailable under your current access.';}),app.busy),node('p','Published. Open the report and run it explicitly to create a result for readers. Earlier private previews stay private.','notice'));
  parent.append(section);
}
