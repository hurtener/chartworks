import {node, button} from './dom.js';
import {chartLabel} from '../report-viewer/presentation.js';
import {publicationEligible, publishedWidgets, hasPrivatePins, stageLabel, validRejectionNote} from './publication.js';


const actionLabel=action=>({block_publish:'chart publication',review:'review request',publish:'report publication',reject:'feedback',rebind_published:'chart update'})[action]||'change';
function checkbox(label,checked,disabled,fn){const el=node('label',undefined,'publication-confirmation'),input=node('input');input.type='checkbox';input.checked=checked;input.disabled=disabled;input.setAttribute('aria-label',label);input.addEventListener('change',()=>fn(input.checked));el.append(input,node('span',label));return el;}
function confirm(parent,app,action,target,label,buttonLabel,disabled){const p=app.publication;parent.append(checkbox(label,p.confirmed(action,target),disabled,value=>{p.confirm(action,target,value);app.render();}),button(buttonLabel,()=>void app.perform(()=>app.publishAction(action,target)),disabled||!p.confirmed(action,target)));}

// DOM is always rebuilt from an exact server inspection. No count, recipient,
// grant, source result, or publication state is inferred from a private preview.
export function renderPublicationControls(parent,app){
  const p=app.publication,s=app.session;if(!p.available()||!s.state)return;
  const section=node('details',undefined,'publication-controls'),owner=s.state.id,generation=p.generation,epoch=app.epoch,unknown=p.records(owner).some(op=>op.status==='unknown');section.setAttribute('aria-label','Publication and review');section.open=p.panelExpanded||unknown;section.append(node('summary',unknown?'Publish and review · action unconfirmed':'Publish and review'));section.addEventListener('toggle',()=>{if(!app.closed&&app.epoch===epoch&&p.generation===generation&&app.session.state?.id===owner)p.panelExpanded=section.open;});
  const unavailable=app.busy||s.dirty||!!app.mapping||!!app.datasetSession||s.conflict||s.uncertain;
  section.append(button('Check report',()=>void app.perform(()=>app.inspectPublication()),unavailable));
  if(s.dirty)section.append(node('p','Save your changes before reviewing or publishing.','metadata'));
  for(const op of p.records(s.state.id)){
    if(op.status==='unknown')section.append(node('p',`We could not confirm the ${actionLabel(op.action)}. Check its status before trying again.`, 'notice error'),button('Check '+actionLabel(op.action),()=>void app.perform(()=>app.inspectPublicationOperation(op)),app.busy));
    else if(op.action==='block_publish'&&['confirmed','observed'].includes(op.status))section.append(node('p','The chart is published. Editing the report does not unpublish that chart.', 'metadata'));
    if(p.retryEligible(op)){
      section.append(node('p','The report has not changed, but the previous action may still be finishing. Retry checks the same action without creating another copy.','notice'));

      if(op.disclosure){section.append(node('p','This publishes the chart and every item listed below for people who already have access:','metadata'));for(const output of op.disclosure)section.append(node('p',output.title||chartLabel(output.kind),'metadata'));}
      if(op.action==='reject')section.append(node('p',`Your feedback: ${op.args.note}`,'metadata'),node('p','Return the report you reviewed for changes. Keep the newer draft and published report.','notice'));
      if(op.args.widgets)section.append(node('p',`${op.args.widgets.length} selected items will use the published charts.`,'metadata'));
      section.append(checkbox('Retry the same action without creating a duplicate.',p.retryConfirmed(op),unavailable,value=>{p.confirmRetry(op,value);app.render();}),button('Retry this action',()=>void app.perform(()=>app.retryPublicationOperation(op)),unavailable||!p.retryConfirmed(op)));
    }
  }
  if(!p.current(s)){section.append(node('p',s.capabilities.can_save?'Check your report to see what is ready to publish.':'Check this report to review its status and publish when ready.','metadata'));parent.append(section);return;}
  const view=p.view,r=view.report,locked=unavailable||p.blocked(s.state.id);
  const step=!r.private?'Published':view.blocks.some(item=>!item.published_at)?'1 · Publish charts':hasPrivatePins(view)?'2 · Update charts in the report':view.stage==='review'?'4 · Publish report':'3 · Submit for review';
  section.append(node('h2',step),node('p',stageLabel(view.stage),'metadata'));

  if(s.capabilities.can_save&&(view.stage==='review'||r.state.review_revision>0))section.append(node('p','A saved version is awaiting review. Further edits are saved separately until you send them for review.','notice'));
  if(r.state.draft_revision>0&&r.state.draft_revision!==r.revision)section.append(button('Open current draft',()=>app.navigate(()=>app.openDraft(r.state.id,'draft')),app.busy));
  if(view.rejected===true||p.rejected(r.state.id,r.revision))section.append(node('p','This report needs changes. Edit and save it before requesting another review.','notice'));
  if(r.state.review_revision>0&&r.state.review_revision!==r.revision)section.append(button('Open pending review',()=>app.navigate(()=>app.openDraft(r.state.id,'review')),app.busy));
  const batchTargets=p.chartBatchTargets();
  if(batchTargets.length>1&&p.available('block_publish')){
    const group=node('section',undefined,'publication-batch');group.append(node('h3',`${batchTargets.length} charts ready to publish`),node('p','Publishes each chart and every item listed below for people who already have access. If interrupted, charts already published stay published. Check sharing separately to choose access to the report.','notice'));
    group.append(checkbox(`Publish all ${batchTargets.length} charts and every item listed below.`,p.chartBatchConfirmed(),locked,value=>{p.confirmChartBatch(value);app.render();}),button(`Publish ${batchTargets.length} ready charts`,()=>void app.perform(()=>app.publishChartBatch()),locked||!p.chartBatchConfirmed()));section.append(group);
  }
  for(const item of view.blocks){const b=item.block,card=node(item.published_at?'details':'article',undefined,'publication-chart');if(item.published_at)card.append(node('summary',b.metadata?.[0]?.title||'Published chart'));
    card.append(node('h3',b.metadata?.[0]?.title||'Untitled chart'),node('p',`${item.published_at?'Published':'Draft'} · ${b.outputs.length} ${b.outputs.length===1?'item':'items'}`,'metadata'));
    if(batchTargets.length>1&&publicationEligible(item))card.append(node('p','Included in ready charts','metadata'));
    const outputs=node('ul');for(const output of b.outputs)outputs.append(node('li',output.mapping?.options?.title||chartLabel(output.kind)));card.append(outputs);
    if(!item.published_at&&p.available('block_publish')){
      card.append(node('p','Publishes this chart and every item listed below. People who already have access to the chart can see it. Use Share to choose access to the report.','notice'));

      if(publicationEligible(item))confirm(card,app,'block_publish',`${b.state.id}:${b.revision}`,'Publish this chart and every item listed above.','Publish chart',locked);
      else card.append(node('p','This chart is not ready to publish. Check its data and try again.','metadata'));
    }
    section.append(card);
  }
  const options=publishedWidgets(view);
  if(options.length&&s.capabilities.can_save&&r.private&&r.state.draft_revision===r.revision&&p.available('rebind_published')){
    const group=node('section');group.append(node('h3','Update the report with these charts'),node('p','Use these published charts in your draft. Your layout and filters stay the same.','metadata'));
    if(options.length>1)group.append(button(p.selection.size===options.length?'Clear selection':`Select all ${options.length} items`,()=>{const value=p.selection.size!==options.length;for(const option of options)p.select(option.widget,value);app.render();},locked));
    for(const option of options)group.append(checkbox(`${option.page} / ${option.title}`,p.selection.has(option.widget),locked,value=>{p.select(option.widget,value);app.render();}));
    if(p.selection.size)confirm(group,app,'rebind_published',undefined,'Update only the selected items with the published charts.','Update selected charts',locked);
    section.append(group);
  }
  if(hasPrivatePins(view))section.append(node('p','Publish your charts, then update them in this report before requesting a review.','notice'));
  else if(p.available('report_transition')&&r.private){
    if(r.state.draft_revision===r.revision&&!r.state.review_revision&&view.can_review===true&&view.rejected!==true&&!p.rejected(r.state.id,r.revision)){section.append(node('p','Send this draft for review. Readers will still see the last published version.','metadata'));confirm(section,app,'review',undefined,'Send this saved draft for review.','Request review',locked);}
    if(r.state.review_revision===r.revision&&view.can_publish===true){section.append(node('p','Makes this report version available to people with access. Draft previews stay private. Refresh the published report to save updated data for readers.','notice'));confirm(section,app,'publish',undefined,'Publish the reviewed report.','Publish report',locked);}
  }
  if(p.available('report_transition')&&r.private&&r.state.review_revision===r.revision&&view.can_reject===true){
    const group=node('section');group.append(node('h3','Request changes'),node('p','Explain what needs to change. The published report and any newer draft stay unchanged. The author can edit and send it for review again.','notice'));
    const label=node('label','What needs to change?'),input=node('textarea');input.value=p.rejectNote;input.defaultValue=input.value;input.maxLength=2048;input.disabled=locked;input.setAttribute('aria-label','What needs to change?');input.addEventListener('input',()=>{p.setRejectNote(input.value);app.render();});label.append(input);group.append(label,node('p','Add a short note explaining the changes you need.','metadata'));
    if(validRejectionNote(p.rejectNote))confirm(group,app,'reject',undefined,'Return this report with my note.','Send feedback',locked);
    else group.append(button('Send feedback',()=>{},true));
    section.append(group);
  }
  if(r.private&&!hasPrivatePins(view)&&p.available('report_transition')&&(r.state.draft_revision===r.revision&&!r.state.review_revision&&view.can_review!==true||r.state.review_revision===r.revision&&view.can_publish!==true))section.append(node('p','You cannot make this change right now. Check the report’s status and your access, then try again.','metadata'));
  if(!r.private)section.append(button('Open published report',()=>app.navigate(async()=>{const target={kind:'report',id:r.state.id,revision:r.revision},title=r.definition.metadata?.find(m=>m.locale===app.locale)?.title||r.definition.metadata?.[0]?.title;await app.changeMode('consumer');if(app.capabilities.consumer)await app.selectPublished({target,title});else app.message='This publication is unavailable under your current access.';}),app.busy),node('p','Published. Open the report and refresh its data so readers can view it. Draft previews stay private.','notice'));
  parent.append(section);
}
