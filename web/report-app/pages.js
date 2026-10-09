import {INVALID_REQUEST, CONFLICT} from './error-codes.js';
import {appError, copyData, titleFor, validID} from './model.js';

// These are coordinates inside one document, never separately authorized resources.
export function reportPages(definition) {
  if(definition?.schema_version===2&&Array.isArray(definition.widgets))return [{id:'main',title:titleFor(definition.metadata,definition.locale),widgets:definition.widgets,filters:definition.filters,defaults:definition.defaults,locale:definition.locale,timezone:definition.timezone}];
  if(definition?.schema_version===3&&Array.isArray(definition.report_pages))return definition.report_pages;
  return [];
}
export function pageContent(definition,page) {
  if(definition?.schema_version===2&&page==='main')return definition;
  const found=definition?.schema_version===3&&definition.report_pages?.find(p=>p.id===page);
  if(!found)throw appError(INVALID_REQUEST);
  return found;
}
export function effectivePage(definition,page) {
  const p=pageContent(definition,page);
  return {...copyData(p),locale:p.locale||definition.locale,timezone:p.timezone||definition.timezone};
}
export function upgradeReportPages(definition) {
  if(definition?.schema_version!==2||!Array.isArray(definition.widgets)||definition.pages?.length||definition.sections?.length||definition.report_pages?.length)throw appError(INVALID_REQUEST);
  const d=copyData(definition),page={id:'main',title:titleFor(d.metadata,d.locale),widgets:d.widgets};
  for(const key of ['filters','defaults'])if(Object.hasOwn(d,key))page[key]=d[key];
  delete d.widgets;delete d.filters;delete d.defaults;delete d.pages;delete d.sections;
  d.schema_version=3;d.report_pages=[page];return d;
}
export function editReportPages(definition,change) {
  if(definition?.schema_version!==3||!Array.isArray(definition.report_pages))throw appError(INVALID_REQUEST);
  const d=copyData(definition);change(d.report_pages);
  if(d.report_pages.length<1||d.report_pages.length>100)throw appError(INVALID_REQUEST);
  const seen=new Set();for(const p of d.report_pages){if(!validID(p.id)||seen.has(p.id)||typeof p.title!=='string'||!p.title.trim()||p.title.length>256||!Array.isArray(p.widgets))throw appError(INVALID_REQUEST);seen.add(p.id);}
  return d;
}
export function addReportPage(definition,id,title='Untitled page') {return editReportPages(definition,pages=>pages.push({id,title,widgets:[]}));}
export function renameReportPage(definition,id,title) {return editReportPages(definition,pages=>{const p=pages.find(p=>p.id===id);if(!p)throw appError(INVALID_REQUEST);p.title=title;});}
export function moveReportPage(definition,id,delta) {return editReportPages(definition,pages=>{const index=pages.findIndex(p=>p.id===id),target=index+delta;if(index<0||!Number.isInteger(delta)||target<0||target>=pages.length)throw appError(INVALID_REQUEST);pages.splice(target,0,pages.splice(index,1)[0]);});}
export function removeReportPage(definition,id) {return editReportPages(definition,pages=>{const index=pages.findIndex(p=>p.id===id);if(index<0)throw appError(INVALID_REQUEST);pages.splice(index,1);});}
export function unusedWidgetID(definition,prefix,random=()=>crypto.randomUUID().slice(0,8)) {
  const used=new Set(reportPages(definition).flatMap(p=>p.widgets.map(w=>w.id)));
  for(let attempt=0;attempt<8;attempt++){const id=prefix+'-'+random();if(validID(id)&&!used.has(id))return id;}
  throw appError(CONFLICT);
}
