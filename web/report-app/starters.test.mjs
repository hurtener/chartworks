import test from 'node:test';
import assert from 'node:assert/strict';
import {reportStarters,applyStarter,starterPlacement} from './starters.js';
import {newDefinition,validateManualDefinition} from './model.js';
import {canPlace} from './grid.js';

test('both starters use editable native pages and headings without fabricated data or authority',()=>{
  for(const id of ['executive','operations']){const original=newDefinition('Team report','es-AR','America/Argentina/Buenos_Aires'),d=applyStarter(original,id);validateManualDefinition(d);assert.equal(original.report_pages.length,1);assert.equal(original.report_pages[0].widgets.length,0);assert.equal(d.report_pages.length,2);assert.deepEqual(d.metadata,original.metadata);assert.equal(d.locale,original.locale);assert.equal(d.timezone,original.timezone);assert(d.report_pages.every(p=>p.widgets.length===1&&p.widgets[0].kind==='text'));assert(!JSON.stringify(d).includes('block'));assert.deepEqual(Object.keys(d).sort(),Object.keys(original).sort());assert.throws(()=>applyStarter(d,'executive'));}
  assert.throws(()=>applyStarter(newDefinition('Team'),'unrecognized'));const list=reportStarters();list[1].pages[0].title='mutated';assert.equal(reportStarters()[1].pages[0].title,'Overview');
});

test('explicit chart additions follow starter slots, survive reload, and avoid all existing components',()=>{
  for(const starter of reportStarters().filter(s=>s.pages.length)){const d=applyStarter(newDefinition('Team'),starter.id);for(const [index,page] of d.report_pages.entries()){let n=0;for(const slot of starter.pages[index].slots){const grid=starterPlacement(JSON.parse(JSON.stringify(page)),slot.kind,{width:12,height:4});assert.deepEqual(grid,slot.grid);assert(canPlace(page.widgets,undefined,grid));page.widgets.push({id:`chart-${index}-${++n}`,kind:'block',grid,presentation:{title:'Chosen output'},block:{block:'chosen-block',revision:2,outputs:['chosen-output'],policy:'published',narrative:false}});}const extra=starterPlacement(page,'chart',{width:8,height:4});assert(canPlace(page.widgets,undefined,extra));validateManualDefinition(d);}}
});

test('moving headings or components never lets suggested placement overlap or rearrange saved content',()=>{
  const d=applyStarter(newDefinition('Team'),'executive'),page=d.report_pages[0];page.widgets[0].grid.height=5;const before=JSON.stringify(page);const grid=starterPlacement(page,'kpi',{width:4,height:3});assert(canPlace(page.widgets,undefined,grid));assert.equal(JSON.stringify(page),before);assert(grid.row>=5);page.widgets[0].id='custom-heading';assert(canPlace(page.widgets,undefined,starterPlacement(page,'chart',{width:8,height:4})));
});
