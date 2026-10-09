import {appError,copyData,validID} from './model.js';
import {STALE_VALIDATION} from './error-codes.js';
const fail=()=>{throw appError(STALE_VALIDATION);};
const hash=v=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
export function sourceDatasetPin(value){
 if(!value||typeof value!=='object'||Object.keys(value).length!==5||!['source','context','dataset'].every(k=>validID(value[k]))||!Number.isSafeInteger(value.source_revision)||value.source_revision<1||!hash(value.schema_digest))fail();
 return copyData(value);
}
export function sameSourceDataset(a,b){
 return !!a&&!!b&&['source','context','dataset','source_revision','schema_digest'].every(key=>a[key]===b[key]);
}
export function checkSource(item){
 if(!item||!validID(item.id)||!validID(item.context_id)||typeof item.name!=='string'||!item.name.trim()||item.name.length>256||!Number.isSafeInteger(item.revision)||item.revision<1||typeof item.dialect!=='string'||item.status!=='registered')fail();
 return item;
}
export function checkRegisteredDataset(item,source){
 const pin=sourceDatasetPin({source:item?.source,context:item?.context,dataset:item?.relation?.id,source_revision:item?.source_revision,schema_digest:item?.schema_digest});
 if(pin.source!==source.id||pin.context!==source.context_id||pin.source_revision!==source.revision||typeof item.relation.name!=='string'||!item.relation.name.trim()||item.relation.name.length>256)fail();
 return pin;
}
// Both native metadata arrays and dependency-filtered host pages are supported.
// Never infer the end of a sparse host page from its number of visible items.
export function checkCatalogPage(page,after,limit,key,check,{array=false}={}){
 const explicit=!Array.isArray(page);
 if(!explicit&&!array)fail();
 const items=explicit?page?.items:page;
 if(!Array.isArray(items)||items.length>limit||explicit&&(!page||Object.keys(page).some(k=>!['items','next'].includes(k))||page.next!==undefined&&typeof page.next!=='string'))fail();
 const next=explicit?page.next||'':items.length===limit?key(items.at(-1)):'';
 for(let i=0;i<items.length;i++){check(items[i]);const id=key(items[i]);if(!validID(id)||id<=after||i>0&&id<=key(items[i-1]))fail();}
 if(next&&(!validID(next)||next<=after||items.length&&next<key(items.at(-1))))fail();
 return {items:copyData(items),next};
}
export function catalogHistory(after,history){
 if(!after)return [''];
 const index=history.indexOf(after);
 return index>=0?history.slice(0,index+1):[...history,after];
}
