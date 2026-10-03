import {appError, copyData, validID} from './model.js';

export const GRID_COLUMNS = 12;
export const GRID_ROWS = 10000;
export const GRID_MAX_HEIGHT = 100;

export function validCell(cell) {
  return !!cell && ['column','row','width','height'].every(key=>Number.isSafeInteger(cell[key])) &&
    cell.column>=0 && cell.row>=0 && cell.width>=1 && cell.height>=1 &&
    cell.column+cell.width<=GRID_COLUMNS && cell.row+cell.height<=GRID_ROWS && cell.height<=GRID_MAX_HEIGHT;
}

export function cellsOverlap(a,b) {
  return a.column<b.column+b.width && a.column+a.width>b.column && a.row<b.row+b.height && a.row+a.height>b.row;
}

export function canPlace(widgets,widgetID,cell) {
  return Array.isArray(widgets) && widgets.length<=100 && validCell(cell) &&
    widgets.every(widget=>widget.id===widgetID || validCell(widget.grid) && !cellsOverlap(cell,widget.grid));
}

function changeCell(widgets,id,patch) {
  if(!Array.isArray(widgets)||!validID(id)||widgets.filter(widget=>widget.id===id).length!==1)throw appError('invalid_request');
  const next=copyData(widgets),target=next.find(widget=>widget.id===id),cell={...target.grid,...patch};
  if(!canPlace(next,id,cell))throw appError('invalid_request');
  target.grid=cell;
  return next;
}

export function moveWidget(widgets,id,column,row) { return changeCell(widgets,id,{column,row}); }
export function resizeWidget(widgets,id,width,height) { return changeCell(widgets,id,{width,height}); }
export function placeWidget(widgets,id,cell) {
  if(!validCell(cell))throw appError('invalid_request');
  return changeCell(widgets,id,{column:cell.column,row:cell.row,width:cell.width,height:cell.height});
}

// Search only arrangement boundaries: a lowest available rectangle starts at
// row zero or the bottom edge of another rectangle. Work stays bounded by the
// document's 100 widgets rather than the maximum grid area.
export function firstFreeCell(widgets,{width,height,column=0,row=0}) {
  if(!Array.isArray(widgets)||widgets.length>=100||!validCell({column,row,width,height})||widgets.some(widget=>!validCell(widget.grid)))throw appError('invalid_request');
  const rows=[...new Set([row,...widgets.map(widget=>widget.grid.row+widget.grid.height).filter(value=>value>=row)])].sort((a,b)=>a-b);
  for(const y of rows) {
    const columns=[column,...Array.from({length:GRID_COLUMNS-width+1},(_,index)=>index).filter(value=>value!==column)];
    for(const x of columns) {
      const cell={column:x,row:y,width,height};
      if(canPlace(widgets,null,cell))return cell;
    }
  }
  throw appError('limit_exceeded');
}

export function duplicateWidget(widgets,id,newID) {
  if(!validID(newID)||!Array.isArray(widgets)||widgets.some(widget=>widget.id===newID)||widgets.filter(widget=>widget.id===id).length!==1)throw appError('invalid_request');
  const original=widgets.find(widget=>widget.id===id),next=copyData(widgets),duplicate=copyData(original);
  duplicate.id=newID;
  duplicate.grid=firstFreeCell(widgets,{...original.grid,row:original.grid.row});
  next.push(duplicate);
  return next;
}

// This whole-layout action is explicit. Individual move/resize never rearranges
// neighbors. Rows advance by their tallest cell, preserving authored heights.
export function arrangeWidgets(widgets,columns) {
  if(![1,2,3].includes(columns)||!Array.isArray(widgets)||widgets.length>100||widgets.some(widget=>!validCell(widget.grid)))throw appError('invalid_request');
  const next=copyData(widgets),width=GRID_COLUMNS/columns;
  let row=0;
  for(let start=0;start<next.length;start+=columns) {
    const group=next.slice(start,start+columns),height=Math.max(...group.map(widget=>widget.grid.height));
    if(row+height>GRID_ROWS)throw appError('limit_exceeded');
    group.forEach((widget,index)=>{widget.grid={column:index*width,row,width,height:widget.grid.height};});
    row+=height;
  }
  return next;
}
