import {appError} from './model.js';
import {editReportPages,pageContent} from './pages.js';

// A small canonical selection, not an inferred list of host-installed locales.
// Existing native document metadata and page values remain selectable as saved.
const locales=[['en','English'],['en-US','English (United States)'],['en-GB','English (United Kingdom)'],['es','Spanish'],['es-AR','Spanish (Argentina)'],['es-ES','Spanish (Spain)'],['es-MX','Spanish (Mexico)']];
const zones=[['UTC','Coordinated Universal Time'],['America/New_York','New York'],['America/Chicago','Chicago'],['America/Denver','Denver'],['America/Los_Angeles','Los Angeles'],['America/Argentina/Buenos_Aires','Buenos Aires'],['America/Mexico_City','Mexico City'],['America/Sao_Paulo','São Paulo'],['Europe/London','London'],['Europe/Madrid','Madrid'],['Europe/Paris','Paris'],['Europe/Berlin','Berlin'],['Asia/Tokyo','Tokyo'],['Asia/Kolkata','Kolkata'],['Australia/Sydney','Sydney']];
export function validPageLocale(value){if(typeof value!=='string'||!value||value.length>64||value==='und')return false;try{return Intl.getCanonicalLocales(value)[0]===value;}catch{return false;}}
export function validPageTimezone(value){if(typeof value!=='string'||!value||value.length>128||value==='Local'||value.includes('..')||!(/^[A-Za-z][A-Za-z0-9_+.-]*(\/[A-Za-z0-9_+.-]+)*$/).test(value))return false;try{new Intl.DateTimeFormat('en',{timeZone:value});return true;}catch{return false;}}
function choices(kind,definition,page){
  const fixed=kind==='locale'?locales:zones,valid=kind==='locale'?validPageLocale:validPageTimezone;
  const values=new Map(fixed);for(const value of [definition[kind],...(kind==='locale'?(definition.metadata||[]).map(m=>m.locale):[]),page[kind]])if(typeof value==='string'&&value&&!values.has(value))values.set(value,`${value.replaceAll('_',' ').replaceAll('/',' / ')}${valid(value)?' · saved metadata':' · saved value, unavailable in this browser'}`);
  const label=value=>values.get(value)||value;
  return [{value:'',label:`Inherit report default · ${label(definition[kind])} (${definition[kind]})`},...[...values].map(([value,title])=>({value,label:`${title} (${value})`,disabled:!valid(value)}))];
}
export function pageSettingChoices(definition,pageID,kind){if(!['locale','timezone'].includes(kind)||definition.schema_version!==3)throw appError('invalid_request');return choices(kind,definition,pageContent(definition,pageID));}
export function setPageSetting(definition,pageID,kind,value){
  if(typeof value!=='string'||!pageSettingChoices(definition,pageID,kind).some(choice=>choice.value===value)||value&&!(kind==='locale'?validPageLocale:validPageTimezone)(value)&&value!==pageContent(definition,pageID)[kind])throw appError('invalid_request');
  return editReportPages(definition,pages=>{const page=pages.find(p=>p.id===pageID);if(value)page[kind]=value;else delete page[kind];});
}
