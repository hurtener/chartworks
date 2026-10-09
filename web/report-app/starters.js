import {appError,copyData} from './model.js';
import {INVALID_REQUEST} from './error-codes.js';
import {canPlace,firstFreeCell} from './grid.js';
import {node,button} from './dom.js';

const slot=(kind,column,row,width,height)=>({kind,grid:{column,row,width,height}});
const starters=[
  {id:'blank',title:'Blank report',description:'A clear canvas for your own story.',pages:[]},
  {id:'executive',title:'Executive review',description:'Key measures first, then trends and the drivers behind them.',pages:[
    {id:'main',title:'Overview',heading:'Performance overview',guide:'Add up to three key measures, followed by a trend chart.',slots:[slot('kpi',0,1,4,2),slot('kpi',4,1,4,2),slot('kpi',8,1,4,2),slot('chart',0,3,12,4)]},
    {id:'drivers',title:'Drivers & detail',heading:'Drivers and detail',guide:'Compare two breakdowns, then add the supporting detail table.',slots:[slot('chart',0,1,6,4),slot('chart',6,1,6,4),slot('table',0,5,12,5)]}
  ]},
  {id:'operations',title:'Operational scorecard',description:'Monitor the current position and investigate the detail.',pages:[
    {id:'main',title:'Scorecard',heading:'Operational performance',guide:'Start with two measures and compare two operational breakdowns.',slots:[slot('kpi',0,1,6,2),slot('kpi',6,1,6,2),slot('chart',0,3,6,4),slot('chart',6,3,6,4)]},
    {id:'detail',title:'Detail',heading:'Operational detail',guide:'Add a detail table, then a chart to explain its distribution. Choose filters that apply to these components.',slots:[slot('table',0,1,12,5),slot('chart',0,6,12,4)]}
  ]}
];
export const reportStarters=()=>copyData(starters);
export function applyStarter(definition,id){
  const starter=starters.find(s=>s.id===id);if(!starter||definition.schema_version!==3||definition.report_pages.some(p=>p.widgets.length))throw appError(INVALID_REQUEST);
  const d=copyData(definition);if(id==='blank')return d;
  d.report_pages=starter.pages.map(p=>({id:p.id,title:p.title,widgets:[{id:`starter-${id}-${p.id}`,kind:'text',grid:{column:0,row:0,width:12,height:1},presentation:{},text:{format:'plain',text:p.heading}}]}));return d;
}
function pageStarter(page){return starters.flatMap(s=>s.pages.map(p=>({...p,starter:s.id}))).find(p=>p.id===page.id&&page.widgets.some(w=>w.id===`starter-${p.starter}-${p.id}`&&w.kind==='text'));}
export function starterPlacement(page,kind,size){
  const recommended=pageStarter(page)?.slots.find(s=>s.kind===kind&&canPlace(page.widgets,undefined,s.grid));return recommended?copyData(recommended.grid):firstFreeCell(page.widgets,size);
}
export function renderStarterGuide(parent,page){const starter=pageStarter(page);if(!starter||page.widgets.some(w=>w.kind==='block'))return;const guide=node('div',undefined,'starter-guide');guide.append(node('strong','Make this page yours'),node('p',starter.guide),node('p','Choose reviewed data or approved outputs in Components. Only headings are saved by the starter; no data is connected yet.','metadata'));parent.append(guide);}
export function renderStarterChoices(parent,selected,disabled,choose){
  const group=node('div',undefined,'starter-choices');group.setAttribute('role','group');group.setAttribute('aria-label','Report starting point');
  for(const starter of starters){const card=button('',()=>choose(starter.id),disabled);card.className='starter-choice';card.setAttribute('aria-pressed',String(selected===starter.id));card.setAttribute('aria-label',starter.title);const preview=node('div',undefined,`starter-preview starter-preview-${starter.id}`);preview.setAttribute('aria-hidden','true');for(const slot of starter.pages[0]?.slots||[]) {const shape=node('span');shape.style.gridColumn=`${slot.grid.column+1} / span ${slot.grid.width}`;shape.style.gridRow=`${slot.grid.row} / span ${slot.grid.height}`;shape.className=slot.kind;preview.append(shape);}card.append(preview,node('strong',starter.title),node('span',starter.description),node('small',starter.pages.length?`${starter.pages.length} pages · connect your data`:'1 page · start from scratch'));group.append(card);}
  parent.append(group);
}
