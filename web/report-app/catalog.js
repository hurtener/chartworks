import {appError, copyData, validID} from './model.js';

const bytes=value=>new TextEncoder().encode(value).length;
const text=(value,max)=>typeof value==='string'&&bytes(value)<=max&&!/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(value);
export const CATALOG_PAGE_SIZE=40, CATALOG_MAX_ITEMS=200, CATALOG_MAX_PAGES=5;

// Every page is one authorized native metadata search, never a source query.
// Sparse pages may carry a continuation. The scan remains explicitly bounded.
export class CatalogSearch {
  constructor(kind,invoke){if(!['report','block'].includes(kind))throw appError('invalid_request');this.kind=kind;this.invoke=invoke;this.generation=0;this.closed=false;this.reset();}
  reset(){this.form=null;this.generation++;this.input='';this.query='';this.locale='';this.items=[];this.next='';this.pages=0;this.cursors=new Set();this.pending=false;this.limited=false;}
  invalidate(){this.generation++;this.pending=false;}
  async load({after='',query=this.input,locale,current=()=>true}={}){
    if(this.closed)throw appError('unavailable');
    if(!text(query,256)||!text(locale,64)||!locale||after&&!validID(after))throw appError('invalid_request');
    if(after&&(this.pending||after!==this.next||query!==this.query||locale!==this.locale||this.pages>=CATALOG_MAX_PAGES||this.items.length>=CATALOG_MAX_ITEMS))throw appError('stale_validation');
    const generation=++this.generation;
    if(!after){this.items=[];this.next='';this.pages=0;this.cursors=new Set();this.limited=false;this.query=query;this.locale=locale;}
    this.pending=true;
    try{
      const r=await this.invoke('reporting_search',{kind:this.kind,query,locale,after,limit:CATALOG_PAGE_SIZE});
      if(this.closed||generation!==this.generation||!current())return null;
      const next=r?.next||'',items=r?.items;
      if(r?.version!=='reporting-view-v1'||!Array.isArray(items)||items.length>CATALOG_PAGE_SIZE||typeof next!=='string'||next&&(!validID(next)||next===after||this.cursors.has(next))||items.some(item=>item?.target?.kind!==this.kind||!validID(item.target.id)||!Number.isSafeInteger(item.target.revision)||item.target.revision<1||item.target.revision>256||item.title!==undefined&&!text(item.title,256)||item.description!==undefined&&!text(item.description,4096)))throw appError('unavailable');
      const all=[...this.items,...items];if(new Set(all.map(item=>item.target.id)).size!==all.length)throw appError('unavailable');
      this.items=copyData(all.slice(0,CATALOG_MAX_ITEMS));this.pages++;if(after)this.cursors.add(after);
      this.limited=!!next&&(this.pages>=CATALOG_MAX_PAGES||this.items.length>=CATALOG_MAX_ITEMS);this.next=this.limited?'':next;
      return {items:this.items,next:this.next};
    }catch(error){if(!this.closed&&(generation!==this.generation||!current()))return null;throw error;}
    finally{if(generation===this.generation)this.pending=false;}
  }
  close(){this.reset();this.closed=true;}
}
