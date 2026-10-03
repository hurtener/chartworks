import {boundedJSON} from '../report-viewer/app.js';

export const APP_MAX_RESULT = 4 << 20;
export const APP_MAX_WIRE = 16 << 20;
export const AUTHORING_VERSION = 'report-authoring-v1';
export const authoringTool = action => `reporting_authoring_${action}_v1`;
const modelCodes = new Set(['invalid_request','forbidden','conflict','unavailable','busy','stale_validation','limit_exceeded','unauthenticated','not_found','expired','cancelled_or_timed_out']);
export function appError(code, unknown = false) { const e = new Error(modelCodes.has(code) ? code : 'unavailable'); e.code = e.message; e.unknown = unknown; return e; }
export function copyData(value) { boundedJSON(value, 2 << 20); return JSON.parse(JSON.stringify(value)); }
export function validID(value) { return typeof value === 'string' && /^[A-Za-z0-9_.:-]{1,128}$/.test(value); }
export function unpack(result) {
  boundedJSON(result, APP_MAX_WIRE);
  let body = result?.structuredContent;
  if (!body) { const text = result?.content?.find(item => item.type === 'text')?.text; if (typeof text !== 'string' || text.length > APP_MAX_RESULT) throw appError('unavailable'); try { body = JSON.parse(text); } catch { throw appError('unavailable'); } }
  if (result.isError || body?.error) throw appError(body?.error?.code, body?.error?.outcome === 'unknown');
  if (!body || !Object.hasOwn(body,'result')) throw appError('unavailable');
  boundedJSON(body.result, APP_MAX_RESULT);
  return JSON.parse(JSON.stringify(body.result));
}
export function titleFor(metadata, locale = 'en-US') { return metadata?.find(m => m.locale === locale)?.title || metadata?.[0]?.title || 'Untitled report'; }
export function newDefinition(title, locale='en-US', timezone='UTC') {
  if (typeof title !== 'string' || !title.trim() || title.length > 256) throw appError('invalid_request');
  return {schema_version:2, metadata:[{locale,title:title.trim()}],locale,timezone,partial_failure:'fail_closed',widgets:[],filters:[]};
}
export function manualDocument(definition) { return definition?.schema_version === 2 && !definition.pages?.length && !definition.sections?.length && Array.isArray(definition.widgets) && definition.widgets.every(w => w.kind === 'text' || w.kind === 'block' && w.block?.narrative === false); }
export function layoutWidgets(widgets, columns=1) {
  if (![1,2,3].includes(columns)) throw appError('invalid_request');
  return widgets.map((widget,index) => ({...widget,grid:{column:index%columns*(12/columns),row:Math.floor(index/columns),width:12/columns,height:1}}));
}
export function validateManualDefinition(definition) {
  boundedJSON(definition, 1 << 20);
  if (!manualDocument(definition) || definition.widgets.length < 1 || definition.widgets.length > 100 || (definition.filters?.length || 0) > 100 || !titleFor(definition.metadata,definition.locale).trim()) throw appError('invalid_request');
  const ids = new Set();
  for (const w of definition.widgets) {
    if (!validID(w.id) || ids.has(w.id)) throw appError('invalid_request'); ids.add(w.id);
    const g=w.grid;
    if (!g || !['column','row','width','height'].every(k=>Number.isSafeInteger(g[k])) || g.column<0 || g.column>11 || g.row<0 || g.row>9999 || g.width<1 || g.column+g.width>12 || g.height<1 || g.height>100 || g.row+g.height>10000) throw appError('invalid_request');
    if (w.kind==='text' && (w.block || !['plain','markdown'].includes(w.text?.format) || typeof w.text.text!=='string' || w.text.text.length>32768)) throw appError('invalid_request');
    if (w.kind==='block' && (w.text || !validID(w.block.block) || !Number.isSafeInteger(w.block.revision) || w.block.revision<1 || !Array.isArray(w.block.outputs) || !w.block.outputs.length || w.block.outputs.some(o=>!validID(o)) || new Set(w.block.outputs).size!==w.block.outputs.length || !['published','certified_only'].includes(w.block.policy))) throw appError('invalid_request');
    if (w.query) throw appError('invalid_request');
  }
  for (let a=0;a<definition.widgets.length;a++) for(let b=a+1;b<definition.widgets.length;b++) { const x=definition.widgets[a].grid,y=definition.widgets[b].grid; if(x.column<y.column+y.width&&x.column+x.width>y.column&&x.row<y.row+y.height&&x.row+x.height>y.row)throw appError('invalid_request'); }
  return definition;
}
// This is a disposable editor buffer over the canonical DocumentDefinition DTO.
// Server capabilities are display hints. Every call still needs current authority.
export class DraftSession {
  constructor(invoke) { this.invoke=invoke; this.generation=0; this.closed=false; this.pending=false; this.definition=null; this.baseline=null; this.state=null; this.revision=0; this.dirty=false; this.conflict=false; this.uncertain=false; this.capabilities={}; }
  setCapabilities(c) { if(c?.version!==AUTHORING_VERSION)throw appError('unavailable'); this.capabilities=copyData(c); }
  replace(view) { if(!validID(view?.state?.id)||!Number.isSafeInteger(view.state.version)||!Number.isSafeInteger(view.revision)||!view.definition)throw appError('unavailable'); this.state=copyData(view.state);this.definition=copyData(view.definition);this.baseline=copyData(view.definition);this.revision=view.revision;this.dirty=false;this.conflict=false;this.uncertain=false; }
  new(title,locale,timezone) { if(this.closed||this.pending||!this.capabilities.can_create)throw appError('forbidden');this.generation++;this.definition=newDefinition(title,locale,timezone);this.state=null;this.baseline=null;this.revision=0;this.dirty=true;this.conflict=false;this.uncertain=false; }
  edit(fn) { if(this.pending||this.closed||this.conflict||this.uncertain||!this.definition||!(this.state?this.capabilities.can_save:this.capabilities.can_create)||!manualDocument(this.definition))throw appError('forbidden');const next=copyData(this.definition);fn(next);boundedJSON(next,1<<20);this.definition=next;this.dirty=true; }
  async open(report) { if(this.pending||!validID(report))throw appError('invalid_request'); const generation=++this.generation;const [capabilities,view]=await Promise.all([this.invoke(authoringTool('capabilities'),{report}),this.invoke(authoringTool('read'),{report,revision:0})]);if(this.closed||generation!==this.generation)return false;this.setCapabilities(capabilities);this.replace(view);return true; }
  async save(id) {
    if(this.pending||this.closed||this.conflict||this.uncertain||!this.dirty||!this.definition)throw appError('busy');
    if(!(this.state?this.capabilities.can_save:this.capabilities.can_create))throw appError('forbidden');
    const definition=copyData(validateManualDefinition(this.definition)),generation=this.generation;
    const creating=!this.state;if(creating&&!validID(id))throw appError('invalid_request');
    const args=creating?{id,definition}:{report:this.state.id,expected_version:this.state.version,revision:this.revision,definition};
    this.pending=true;
    try { const state=await this.invoke(authoringTool(creating?'create':'save'),args); if(this.closed||generation!==this.generation)return false; if(!validID(state?.id)||state.id!==(creating?id:args.report)||!Number.isSafeInteger(state.version)||!Number.isSafeInteger(state.draft_revision)||state.draft_revision<1)throw appError('unavailable',true);this.state=copyData(state);this.revision=state.draft_revision;this.baseline=copyData(this.definition);this.dirty=false;return true; }
    catch(e) { if(!this.closed&&generation===this.generation){this.conflict=e.code==='conflict';this.uncertain=e.unknown===true||['unavailable','cancelled_or_timed_out'].includes(e.code);} throw e; }
    finally { this.pending=false; }
  }
  canSaveWidget(widget) {
    if(!this.state||!this.baseline||!this.dirty||this.pending||this.conflict||this.uncertain||!this.capabilities.can_save)return false;
    const current=this.definition.widgets.find(w=>w.id===widget),original=this.baseline.widgets.find(w=>w.id===widget);if(!current||!original)return false;
    const projected=copyData(this.definition),target=projected.widgets.find(w=>w.id===widget);target.presentation=copyData(original.presentation);if(original.text)target.text=copyData(original.text);
    return JSON.stringify(projected)===JSON.stringify(this.baseline);
  }
  async saveWidget(widget) {
    if(this.closed||!this.canSaveWidget(widget))throw appError('forbidden');
    const current=this.definition.widgets.find(w=>w.id===widget),patch={presentation:copyData(current.presentation)};if(current.text)patch.text=copyData(current.text);
    const generation=this.generation;this.pending=true;
    try{const state=await this.invoke(authoringTool('widget'),{report:this.state.id,widget,expected_version:this.state.version,revision:this.revision,patch});if(this.closed||generation!==this.generation)return false;if(state?.id!==this.state.id||!Number.isSafeInteger(state.version)||!Number.isSafeInteger(state.draft_revision)||state.draft_revision<1)throw appError('unavailable',true);this.state=copyData(state);this.revision=state.draft_revision;this.baseline=copyData(this.definition);this.dirty=false;return true;}
    catch(e){if(!this.closed&&generation===this.generation){this.conflict=e.code==='conflict';this.uncertain=e.unknown===true||['unavailable','cancelled_or_timed_out'].includes(e.code);}throw e;}finally{this.pending=false;}
  }
  close() { this.closed=true;this.generation++;this.definition=null;this.baseline=null;this.state=null; }
}
