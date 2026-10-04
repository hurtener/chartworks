import {INVALID_REQUEST, STALE_VALIDATION, FORBIDDEN, UNAVAILABLE, BUSY, LIMIT_EXCEEDED, CONFLICT, CANCELLED_OR_TIMED_OUT} from './error-codes.js';
import {appError, authoringTool, AUTHORING_VERSION, copyData, validID, manualDocument} from './model.js';
import {reportPages} from './pages.js';
import {boundedJSON} from '../report-viewer/presentation.js';

export const PUBLICATION_METADATA_BYTES=3<<20;
export const PUBLICATION_CUSTODY_BYTES=16<<20;
export const PUBLICATION_CUSTODY_RECORDS=64;
const custodyReserve=4096;
const reportTransition=action=>['review','publish','reject'].includes(action);
const reportHint=action=>({review:'can_review',publish:'can_publish',reject:'can_reject'})[action];
export const validRejectionNote=note=>typeof note==='string'&&!!note.trim()&&new TextEncoder().encode(note).length<=2048&&!/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(note);
const terminalStatuses=new Set(['confirmed','observed','changed','failed']);
const publicationClone=value=>{boundedJSON(value,PUBLICATION_METADATA_BYTES);return JSON.parse(JSON.stringify(value));};
const pick=(value,keys)=>Object.fromEntries(keys.filter(k=>Object.hasOwn(value||{},k)).map(k=>[k,value[k]]));
const stateCoordinates=value=>pick(value,['id','version','latest_revision','draft_revision','review_revision','published_revision','draft_state','archived']);
const outputDisclosure=outputs=>outputs.map(output=>({id:output.id,kind:output.kind,title:output.mapping?.options?.title||output.title||''}));
// A fixed small reserve covers mutable status/error/state coordinates. Full
// request, disclosure, report base and optional retry proof count as UTF-8 JSON.
const custodySize=op=>new TextEncoder().encode(JSON.stringify(pick(op,['id','action','args','report','disclosure','digest','base','retryView','prior']))).length+custodyReserve;
function retrySnapshot(view){
  const result={rejected:view.rejected===true,can_review:view.can_review===true,can_publish:view.can_publish===true,can_reject:view.can_reject===true,blocks:view.blocks.map(item=>{const b=item.block;return {published_at:item.published_at||'',validation_fresh:item.validation_fresh,can_publish:item.can_publish,block:{state:stateCoordinates(b.state),revision:b.revision,digest:b.digest,execution_digest:b.execution_digest,private:b.private,outputs:outputDisclosure(b.outputs),...(b.validation?{validation:pick(b.validation,['id','revision','definition_digest','execution_digest','expires_at'])}:{})}};})};
  if(view.report){const r=view.report,widgets=page=>page.widgets.filter(w=>w.block).map(w=>({id:w.id,block:pick(w.block,['block','revision','digest','policy'])}));result.report={state:stateCoordinates(r.state),revision:r.revision,digest:r.digest,private:r.private,definition:r.definition.schema_version===3?{schema_version:3,report_pages:reportPages(r.definition).map(p=>({id:p.id,title:'',widgets:widgets(p)}))}:{schema_version:2,widgets:widgets(r.definition)}};}
  return publicationClone(result);
}

const hash=value=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const integer=(value,max=Number.MAX_SAFE_INTEGER)=>Number.isSafeInteger(value)&&value>0&&value<=max;
const key=block=>`${block.state.id}:${block.revision}`;
const canonical=value=>JSON.stringify(value,(_,v)=>v&&typeof v==='object'&&!Array.isArray(v)?Object.fromEntries(Object.keys(v).sort().map(k=>[k,v[k]])):v);
const uncertain=error=>error?.unknown===true||[UNAVAILABLE,CANCELLED_OR_TIMED_OUT].includes(error?.code);
export function reportStage(view) {
  if(view?.private===false)return 'published';
  if(view?.state?.review_revision===view?.revision)return 'review';
  if(view?.state?.draft_revision===view?.revision)return 'draft';
  return 'private_revision';
}
export function stageLabel(stage){return ({draft:'Private draft',review:'Pending review',published:'Published report',private_revision:'Private revision'})[stage]||'Private revision';}
export function checkLifecycle(value,target) {
  const view=publicationClone(value);
  if(view?.version!==AUTHORING_VERSION||!Array.isArray(view.blocks)||view.blocks.length>100)throw appError(STALE_VALIDATION);
  const seen=new Set();
  for(const item of view.blocks){const b=item?.block;
    if(!validID(b?.state?.id)||!integer(b.state.version)||!integer(b.revision,256)||!hash(b.digest)||!hash(b.execution_digest)||typeof b.private!=='boolean'||typeof item.can_publish!=='boolean'||typeof item.validation_fresh!=='boolean'||item.publication_scope!=='entire_revision'||item.audience_effect!=='existing_authorized_readers'||!Array.isArray(b.outputs)||!b.outputs.length||b.outputs.length>64||new Set(b.outputs.map(o=>o.id)).size!==b.outputs.length||b.outputs.some(o=>!validID(o.id)||typeof o.kind!=='string')||seen.has(key(b)))throw appError(STALE_VALIDATION);
    if(item.published_at!==undefined&&!Number.isFinite(Date.parse(item.published_at))||!!item.published_at===b.private)throw appError(STALE_VALIDATION);
    seen.add(key(b));
  }
  if(target.block){if(view.report||view.blocks.length!==1||view.blocks[0].block.state.id!==target.block||view.blocks[0].block.revision!==target.revision)throw appError(STALE_VALIDATION);}
  else {const r=view.report;
    if(r?.state?.id!==target.report||!integer(r.state.version)||!integer(r.revision,256)||target.revision&&r.revision!==target.revision||!hash(r.digest)||typeof r.private!=='boolean'||!manualDocument(r.definition)||view.stage!==reportStage(r)||view.rejected!==undefined&&typeof view.rejected!=='boolean'||view.rejected===true&&!r.private)throw appError(STALE_VALIDATION);
    for(const page of reportPages(r.definition))for(const widget of page.widgets){if(!widget.block)continue;const b=view.blocks.find(item=>item.block.state.id===widget.block.block&&item.block.revision===widget.block.revision)?.block;if(!b||widget.block.policy==='private_preview'&&widget.block.digest!==b.digest||widget.block.outputs.some(output=>!b.outputs.some(o=>o.id===output)))throw appError(STALE_VALIDATION);}
  }
  return view;
}
export function publicationEligible(item) {
  const b=item?.block,e=b?.validation;
  return !!item?.can_publish&&item.validation_fresh&&b.private&&!item.published_at&&!b.state.archived&&b.state.draft_revision===b.revision&&b.state.draft_state==='validated'&&validID(e?.id)&&e.revision===b.revision&&e.definition_digest===b.digest&&e.execution_digest===b.execution_digest&&Date.parse(e.expires_at)>Date.now();
}
export function publishedWidgets(view) {
  if(!view?.report||view.report.definition.schema_version!==3)return [];
  return reportPages(view.report.definition).flatMap(page=>page.widgets.filter(w=>w.block?.policy==='private_preview').flatMap(widget=>{
    const item=view.blocks.find(item=>item.block.state.id===widget.block.block&&item.block.revision===widget.block.revision&&item.block.digest===widget.block.digest);
    return item?.published_at&&!item.block.private?[{widget:widget.id,block:widget.block.block,revision:widget.block.revision,digest:widget.block.digest,page:page.title,title:widget.presentation?.title||widget.id}]:[];
  }));
}
export function hasPrivatePins(view){return reportPages(view?.report?.definition).some(p=>p.widgets.some(w=>w.block?.policy==='private_preview'));}

// Inspection is disposable; mutation custody survives navigation. No mutation
// is retried automatically, and an unchanged read cannot settle an in-flight CAS.
export class PublicationSession {
  constructor(invoke,supports=()=>true){this.invoke=invoke;this.supports=supports;this.closed=false;this.generation=0;this.pending=false;this.view=null;this.selection=new Set();this.confirmations=new Set();this.operations=new Map();this.message='';this.rejectNote='';this.panelExpanded=false;}
  available(action='lifecycle'){return !this.closed&&this.supports(authoringTool('lifecycle'))&&this.supports(authoringTool(action));}
  invalidate(){this.generation++;this.view=null;this.selection.clear();this.confirmations.clear();for(const op of this.operations.values())op.retryView=null;this.message='';this.rejectNote='';this.panelExpanded=false;}
  current(session){const r=this.view?.report;return !!r&&!session.dirty&&r.state.id===session.state?.id&&r.revision===session.revision&&r.state.version===session.state.version&&r.digest===session.digest;}
  blocked(report){return this.pending||Array.from(this.operations.values()).some(op=>op.status==='unknown'&&(op.report===report||op.args.report===report));}
  records(report){return Array.from(this.operations.values()).filter(op=>op.report===report||op.args.report===report);}
  get custodyBytes(){return Array.from(this.operations.values()).reduce((sum,op)=>sum+custodySize(op),0);}
  retain(op,patch={}){
    const candidate={...op,...patch},removed=[];let bytes=custodySize(candidate),count=1;
    for(const [id,record] of this.operations)if(id!==op.id){bytes+=custodySize(record);count++;}
    for(const [id,record] of this.operations){if(bytes<=PUBLICATION_CUSTODY_BYTES&&count<=PUBLICATION_CUSTODY_RECORDS)break;if(id!==op.id&&terminalStatuses.has(record.status)){removed.push(id);bytes-=custodySize(record);count--;}}
    if(bytes>PUBLICATION_CUSTODY_BYTES||count>PUBLICATION_CUSTODY_RECORDS){this.message='Publication recovery storage is full. Inspect unresolved operations before submitting another change.';throw appError(LIMIT_EXCEEDED);}
    for(const id of removed)this.operations.delete(id);Object.assign(op,patch);this.operations.set(op.id,op);
  }
  async read(target){if(!this.available())throw appError(FORBIDDEN);return checkLifecycle(await this.invoke(authoringTool('lifecycle'),copyData(target)),target);}
  async open(report,revision){this.panelExpanded=true;if(this.closed||this.pending||!validID(report)||!integer(revision,256))throw appError(BUSY);const generation=++this.generation;this.view=null;this.rejectNote='';this.selection.clear();this.confirmations.clear();const view=await this.read({report,revision});if(this.closed||generation!==this.generation)return null;this.view=view;return view;}
  rejected(report,revision){return this.records(report).some(op=>op.action==='reject'&&op.args.revision===revision&&['confirmed','observed'].includes(op.status));}
  setRejectNote(value){if(this.pending||this.closed)return;if(typeof value!=='string'||value.length>2048)throw appError(INVALID_REQUEST);this.rejectNote=value;this.confirmations.clear();}
  request(action,target){const r=this.view?.report;if(!r||r.state.archived)throw appError(FORBIDDEN);
    if(action==='block_publish'){const item=this.view.blocks.find(item=>key(item.block)===target);if(!publicationEligible(item))throw appError(STALE_VALIDATION);const b=item.block;return {block:b.state.id,expected_version:b.state.version,revision:b.revision,digest:b.digest,evidence:b.validation.id};}
    if(action==='rebind_published'){if(!r.private||r.state.draft_revision!==r.revision)throw appError(CONFLICT);const widgets=publishedWidgets(this.view).filter(w=>this.selection.has(w.widget)).map(({widget,block,revision,digest})=>({widget,block,revision,digest}));if(!widgets.length||widgets.length!==this.selection.size)throw appError(INVALID_REQUEST);return {report:r.state.id,expected_version:r.state.version,revision:r.revision,digest:r.digest,widgets};}
    if(!reportTransition(action)||this.view[reportHint(action)]!==true)throw appError(FORBIDDEN);
    if(action!=='reject'&&hasPrivatePins(this.view)||!r.private||action==='review'&&(r.state.draft_revision!==r.revision||r.state.review_revision>0||this.view.rejected===true||this.rejected(r.state.id,r.revision))||['publish','reject'].includes(action)&&r.state.review_revision!==r.revision)throw appError(CONFLICT);
    if(action==='reject'&&!validRejectionNote(this.rejectNote))throw appError(INVALID_REQUEST);
    return {report:r.state.id,expected_version:r.state.version,revision:r.revision,operation:action,note:action==='reject'?this.rejectNote:''};
  }
  token(action,target){return JSON.stringify([action,this.view?.report?.digest,this.request(action,target),action==='block_publish'?outputDisclosure(this.view.blocks.find(item=>key(item.block)===target).block.outputs):[]]);}
  confirm(action,target,value){const token=this.token(action,target);if(value)this.confirmations.add(token);else this.confirmations.delete(token);}
  confirmed(action,target){try{return this.confirmations.has(this.token(action,target));}catch{return false;}}
  select(widget,value){if(this.pending)return;if(value)this.selection.add(widget);else this.selection.delete(widget);this.confirmations.clear();}
  async mutate(action,target){
    const tool=reportTransition(action)?'report_transition':action;
    if(this.closed||!this.available(tool)||this.blocked(this.view?.report?.state.id)||!this.confirmed(action,target))throw appError(BUSY);
    const args=this.request(action,target);if(Array.from(this.operations.values()).some(op=>op.status==='unknown'&&(args.block&&op.args.block===args.block||args.report&&op.args.report===args.report)))throw appError(BUSY);const generation=this.generation,report=this.view.report.state.id,id=JSON.stringify([action,args]),op={id,action,args:copyData(args),report,status:'unknown',disclosure:action==='block_publish'?publicationClone(outputDisclosure(this.view.blocks.find(item=>key(item.block)===target).block.outputs)):null,digest:this.view.report.digest,prior:action==='reject'?pick(this.view.report.state,['draft_revision','published_revision','latest_revision']):null,base:action==='rebind_published'?publicationClone(this.view.report.definition):null};
    this.retain(op);return this.dispatch(op,generation);
  }
  async dispatch(op,generation,retry=false){
    const {action,args}=op,tool=reportTransition(action)?'report_transition':action;
    this.pending=true;op.retryView=null;this.confirmations.clear();
    try{const state=await this.invoke(authoringTool(tool),copyData(args));if(this.closed)return null;
      if(state?.id!==(args.block||args.report)||!integer(state.version)||state.version<=args.expected_version||action==='block_publish'&&state.published_revision!==args.revision||action==='review'&&state.review_revision!==args.revision||action==='publish'&&state.published_revision!==args.revision||action==='reject'&&(state.review_revision!==0||state.draft_revision!==(op.prior.draft_revision||args.revision)||state.published_revision!==op.prior.published_revision)||action==='rebind_published'&&(!integer(state.draft_revision,256)||state.draft_revision<=args.revision))throw appError(UNAVAILABLE,true);
      op.status='confirmed';op.state=copyData(pick(state,['id','version',...['draft_revision','review_revision','published_revision'].filter(k=>Number.isSafeInteger(state[k]))]));if(generation===this.generation)this.message=action==='block_publish'?'The entire chart revision is published. The report still has its private pin. Select widgets and confirm a separate rebind.':action==='rebind_published'?'Selected widgets now use their published chart revisions. The report is still private.':action==='review'?'Report submitted for review. It remains private.':action==='reject'?'Review returned for amendment. The newer draft and current publication are preserved. Edit and save a new revision before resubmitting this rejected revision.':'Reviewed report published. Existing private previews remain private. Browse lists the publication; Run is a separate source action.';
      if(generation!==this.generation)return null;return op;
    }catch(error){if(!this.closed){op.status=retry||uncertain(error)?'unknown':'failed';op.code=typeof error.code==='string'&&error.code.length<=64?error.code:UNAVAILABLE;if(generation===this.generation)this.message=op.status==='unknown'?'The outcome is unknown. Inspect the original exact revision before another change.':action==='rebind_published'?'Rebind did not complete. Published chart revisions remain published; the report was not rolled back. Inspect its current state before confirming another edit.':'The change did not complete. Inspect current state and confirm again.';if(generation===this.generation)this.view=null;}throw error;}
    finally{this.pending=false;}
  }
  retryEligible(op){
    const v=op.retryView,a=op.args,exact=a.block?v?.blocks?.[0]?.block:v?.report;
    if(op.status!=='unknown'||!exact||exact.state.version!==a.expected_version||exact.digest!==(a.digest||op.digest)||exact.state.archived)return false;
    if(a.block)return publicationEligible(v.blocks[0])&&exact.validation.id===a.evidence&&canonical(outputDisclosure(exact.outputs))===canonical(op.disclosure);
    if(op.action==='rebind_published')return exact.private&&exact.state.draft_revision===a.revision&&a.widgets.every(pin=>publishedWidgets(v).some(w=>['widget','block','revision','digest'].every(k=>w[k]===pin[k])));
    return v[reportHint(op.action)]===true&&(op.action==='reject'||!hasPrivatePins(v))&&exact.private&&(op.action==='review'?exact.state.draft_revision===a.revision&&!exact.state.review_revision&&!v.rejected:exact.state.review_revision===a.revision)&&(op.action!=='reject'||validRejectionNote(a.note));
  }
  retryToken(op){return JSON.stringify(['retry',op.id,op.digest,op.disclosure]);}
  confirmRetry(op,value){if(!this.retryEligible(op))throw appError(FORBIDDEN);if(value)this.confirmations.add(this.retryToken(op));else this.confirmations.delete(this.retryToken(op));}
  retryConfirmed(op){return this.retryEligible(op)&&this.confirmations.has(this.retryToken(op));}
  async retry(op){const tool=reportTransition(op.action)?'report_transition':op.action;if(this.closed||this.pending||this.operations.get(op.id)!==op||!this.available(tool)||!this.retryConfirmed(op))throw appError(BUSY);return this.dispatch(op,this.generation,true);}
  async inspectOperation(op){
    this.panelExpanded=true;
    if(this.closed||this.pending||this.operations.get(op.id)!==op)throw appError(BUSY);const generation=this.generation,args=op.args;op.retryView=null;this.confirmations.clear();
    const target=args.block?{block:args.block,revision:args.revision}:{report:args.report,revision:args.revision};
    let view=await this.read(target);if(this.closed)return null;
    const exact=args.block?view.blocks[0].block:view.report;
    if(exact.digest!==(args.digest||op.digest))throw appError(STALE_VALIDATION);
    let achieved=op.action==='block_publish'?!!view.blocks[0].published_at:op.action==='review'?view.report.state.review_revision===args.revision:op.action==='publish'?!view.report.private:op.action==='reject'?view.rejected===true&&view.report.private:false;
    if(op.action==='rebind_published'&&view.report.state.draft_revision>args.revision){
      const next=await this.read({report:args.report,revision:view.report.state.draft_revision});if(this.closed)return null;
      const expected=publicationClone(op.base);for(const page of reportPages(expected))for(const widget of page.widgets)if(args.widgets.some(pin=>pin.widget===widget.id)){widget.block.policy='published';delete widget.block.digest;}
      achieved=canonical(next.report.definition)===canonical(expected);if(achieved)view=next;
    }
    // Version advancement fences the old CAS. A still-identical private head
    // does not prove a timed-out mutation has stopped. Only a new explicit
    // confirmation may repeat that identical metadata-only CAS after inspection.
    if(achieved)op.status='observed';else if(exact.state.version>args.expected_version)op.status='changed';
    if(generation!==this.generation)return null;
    if(op.status==='unknown')this.retain(op,{retryView:retrySnapshot(view)});
    this.confirmations.clear();this.selection.clear();if(view.report)this.view=view;else this.view=null;
    this.message=achieved&&op.action==='reject'?'Exact inspection confirms this revision was returned for amendment and remains private. Current draft and publication pointers are shown. Edit and save a new revision before resubmission. This observation does not identify which caller made the change.':achieved?'Exact inspection confirms the requested state. It does not attribute an earlier ambiguous response to a caller.':op.status==='changed'?'The head changed, so the original version can no longer be applied. Reinspect current content before confirming any new change.':this.retryEligible(op)?'The exact head and version are unchanged, but the earlier request may still be completing. Its outcome is still unknown. You may inspect again or separately confirm one retry of the identical metadata-only request; its version check prevents a second commit.':'The original outcome is still unknown, and current evidence or authority does not permit retry. Refresh access or reconcile through your host, then inspect again.';
    return view;
  }
  close(){this.closed=true;this.invalidate();this.operations.clear();}
}
