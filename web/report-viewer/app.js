/* Static MCP Apps view. No tokens, network clients, remote code or persistent
 * analytical state. Every record arrives through an authorized provider tool. */
export const VERSION = 'reporting-view-v1';
export const KINDS = Object.freeze(['area','bar','column','donut','grouped_bar','heatmap','kpi','line','pie','scatter','stacked_bar','stacked_column','table','treemap']);
const NS = 'http://www.w3.org/2000/svg';
const MAX_MESSAGE = 16 * 1024 * 1024;
const MAX_DATA = 4 * 1024 * 1024;
const MAX_POINTS = 10000;
const TOOLS = new Set(['reporting_search','reporting_describe','reporting_run','reporting_runs','reporting_view']);
const ERRORS = new Set(['output_selection_empty','output_duplicate','output_unknown','output_disabled','output_not_selected','narrative_policy_unsupported','invalid_request','unauthorized','unauthenticated','forbidden','not_found','conflict','stale_validation','incomplete','expired','limit_exceeded','invalid_query','busy','unavailable','cancelled_or_timed_out']);
const words = {
  en: {loading:'Opening retained report…',error:'Report unavailable',expired:'Retained values have expired. Opening this report does not regenerate them.',empty:'No values in this retained output.',private:'Private preview',partial:'Partial result',pages:'Page',widgets:'Widget',outputs:'Output',previous:'Previous',next:'Next',values:'Exact retained values',null:'Missing',filters:'Run with different filters',apply:'Run with these filters',consent:'Running queries may incur cost and creates a new artifact. This does not alter the retained result.',dynamic:'Allow dynamic query generation',narrative:'Allow narrative generation',refresh:'Read status again',host:'Open this viewer through an authorized MCP Apps host.',unknown:'The operation outcome is unknown. Inspect its run history before submitting another run.',geometry:'Drawing coordinates are approximate; labels and values below are exact.',default:'Use the published default',observed:'Observed',retained:'Retained until',trust:'Publication / certification / current health',redacted:'Some content is not visible under the current authority.',noscript:'The host did not provide a supported reporting result.',scope:'Total scope',table:'Table page',run:'Run',state:'State',limits:'Accepted query ceilings',disabled:'Disabled',omitted:'Not included in this run'},
  es: {loading:'Abriendo el informe guardado…',error:'Informe no disponible',expired:'Los valores guardados vencieron. Abrir el informe no los vuelve a generar.',empty:'No hay valores en esta salida guardada.',private:'Vista previa privada',partial:'Resultado parcial',pages:'Página',widgets:'Componente',outputs:'Salida',previous:'Anterior',next:'Siguiente',values:'Valores exactos guardados',null:'Sin dato',filters:'Ejecutar con otros filtros',apply:'Ejecutar con estos filtros',consent:'La ejecución puede generar costos y crea un nuevo resultado. No modifica el resultado guardado.',dynamic:'Permitir generación de consultas dinámicas',narrative:'Permitir generación narrativa',refresh:'Leer el estado nuevamente',host:'Abrí este visor desde un host MCP Apps autorizado.',unknown:'El resultado de la operación es incierto. Revisá su historial antes de ejecutar nuevamente.',geometry:'Las coordenadas del gráfico son aproximadas; las etiquetas y los valores son exactos.',default:'Usar el valor predeterminado publicado',observed:'Observado',retained:'Guardado hasta',trust:'Publicación / certificación / estado actual',redacted:'Parte del contenido no es visible con la autorización actual.',noscript:'El host no proporcionó un resultado compatible.',scope:'Alcance del total',table:'Página de tabla',run:'Ejecución',state:'Estado',limits:'Límites aceptados de consulta',disabled:'Deshabilitada',omitted:'No incluida en esta ejecución'}
};
Object.assign(words.en, {row:'Returned row',series:'Series',seriesID:'Series identity',measure:'Measure',category:'Category',value:'Value',path:'Hierarchy path',depth:'Level',aggregation:'Aggregation',members:'Contributing returned rows',seriesValues:'Exact series observations',hierarchyValues:'Exact hierarchy aggregates',independent:'Independent scale',unitless:'No declared unit',missingValues:'Missing measure values',gaps:'Absent observations',omittedRows:'Undrawn returned rows',zeroSize:'Zero-area bubbles',grain:'Time grain / order',resolution:'Zero or subpixel geometry may be invisible; the exact values below are retained.'});
Object.assign(words.es, {row:'Fila devuelta',series:'Serie',seriesID:'Identidad de serie',measure:'Medida',category:'Categoría',value:'Valor',path:'Ruta jerárquica',depth:'Nivel',aggregation:'Agregación',members:'Filas devueltas contribuyentes',seriesValues:'Observaciones exactas por serie',hierarchyValues:'Agregados jerárquicos exactos',independent:'Escala independiente',unitless:'Sin unidad declarada',missingValues:'Valores de medida faltantes',gaps:'Observaciones ausentes',omittedRows:'Filas devueltas no dibujadas',zeroSize:'Burbujas de área cero',grain:'Grano temporal / orden',resolution:'La geometría nula o inferior a un píxel puede no verse; abajo se conservan los valores exactos.'});
const fail = code => { const e = new Error(ERRORS.has(code) ? code : 'unavailable'); e.code = e.message; return e; };
const text = x => typeof x === 'string' ? x : '';
const array = x => Array.isArray(x) ? x : [];
const shorten = (value, max) => Array.from(value).slice(0,max).join('');
const id = x => typeof x === 'string' && /^[A-Za-z0-9_.:-]{1,128}$/.test(x);
const integer = (x, min, max) => Number.isSafeInteger(x) && x >= min && x <= max;

// Bound traversal before serialization, including keys, depth and node count.
// Structured-clone messages can contain cycles and non-JSON values; reject them.
export function boundedJSON(value, maxBytes = MAX_DATA) {
  const stack = [[value,0]], seen = new Set();
  let nodes = 0, chars = 0;
  while (stack.length) {
    const [v,depth] = stack.pop();
    if (++nodes > 400000 || depth > 32) throw fail('limit_exceeded');
    if (v === null || typeof v === 'boolean') continue;
    if (typeof v === 'number') { if (!Number.isFinite(v)) throw fail('invalid_request'); continue; }
    if (typeof v === 'string') { chars += v.length; if (chars > maxBytes) throw fail('limit_exceeded'); continue; }
    if (typeof v !== 'object' || seen.has(v)) throw fail('invalid_request');
    seen.add(v);
    if (Array.isArray(v)) {
      if (v.length > 10000) throw fail('limit_exceeded');
      for (const child of v) stack.push([child,depth+1]);
    } else {
      const prototype = Object.getPrototypeOf(v);
      if (prototype !== Object.prototype && prototype !== null) throw fail('invalid_request');
      const keys = Object.keys(v);
      if (keys.length > 256) throw fail('limit_exceeded');
      for (const key of keys) { chars += key.length; stack.push([v[key],depth+1]); }
    }
  }
  const wire = JSON.stringify(value);
  if (new TextEncoder().encode(wire).length > maxBytes) throw fail('limit_exceeded');
  return value;
}

function element(tag, value, cls) {
  const e = document.createElement(tag);
  if (value !== undefined) e.textContent = String(value);
  if (cls) e.className = cls;
  return e;
}
function svg(tag, attrs = {}, label) {
  const e = document.createElementNS(NS, tag);
  for (const [key,value] of Object.entries(attrs)) {
    if (typeof value === 'number' && !Number.isFinite(value)) throw fail('invalid_request');
    e.setAttribute(key, String(value));
  }
  if (label !== undefined) { const t = document.createElementNS(NS,'title'); t.textContent = label; e.append(t); }
  return e;
}
function button(label, fn, primary = false) {
  const b = element('button',label,primary ? 'primary' : '');
  b.type = 'button'; b.addEventListener('click',fn); return b;
}
function cellValue(cell, missing) { return cell?.null || !cell ? missing : text(cell.value ?? cell.exact); }
function percentShift(s) {
  const m = /^([+-]?)(\d+)(?:\.(\d*))?(?:[eE]([+-]?\d+))?$/.exec(s);
  if (!m || Math.abs(Number(m[4] || 0)) > 512) return null;
  const digits = m[2] + (m[3] || '');
  const dot = m[2].length + Number(m[4] || 0) + 2;
  const number = dot <= 0 ? '0.' + '0'.repeat(-dot) + digits : dot >= digits.length ? digits + '0'.repeat(dot-digits.length) : digits.slice(0,dot) + '.' + digits.slice(dot);
  return m[1] + number.replace(/^0+(?=\d)/,'');
}
export function exact(cell, column, missing = 'Missing') {
  const raw = cellValue(cell, missing);
  if (cell?.null || !cell) return raw;
  const f = column?.format || {};
  let value = raw;
	if (column?.type === 'temporal' && f.date_pattern) value = formatDate(raw,f.date_pattern,f.locale);
  if (f.percent === 'fraction') { const shifted = percentShift(raw); value = shifted === null ? raw + ' (fraction)' : shifted + '%'; }
  if (f.percent === 'whole') value += '%';
	if (!f.percent && ['integer','decimal','number'].includes(column?.type)) value = formatDecimal(value,f.fraction_digits,f.locale);
	return [value,text(f.currency_symbol)||text(f.currency),text(f.unit)].filter(Boolean).join(' ');
}
function formatDecimal(raw,digits,locale) {
  if (!integer(digits,0,20) || !/^[+-]?\d+(?:\.\d+)?$/.test(raw)) return raw;
  const sign=raw.startsWith('-')?'-':raw.startsWith('+')?'+':'', unsigned=sign?raw.slice(1):raw, parts=unsigned.split('.');
  let fraction=parts[1]||'', whole=parts[0];
  if (fraction.length>digits) { const round=fraction[digits]>='5'; let scaled=BigInt(whole+(fraction.slice(0,digits)||'')); if(round)scaled+=1n; let s=scaled.toString().padStart(digits+1,'0'); whole=digits?s.slice(0,-digits):s; fraction=digits?s.slice(-digits):''; }
  else fraction=fraction.padEnd(digits,'0');
  const spanish=text(locale).toLowerCase().startsWith('es'), group=spanish?'.':',', decimal=spanish?',':'.';
  whole=whole.replace(/\B(?=(\d{3})+(?!\d))/g,group); return sign+whole+(digits?decimal+fraction:'');
}
function formatDate(raw,pattern,locale) {
  const m=/^(\d{4})-(\d{2})(?:-(\d{2})(?:[T ](\d{2}):(\d{2}))?)?/.exec(raw); if(!m||pattern!=='year_month'&&!m[3])return raw;
  const spanish=text(locale).toLowerCase().startsWith('es'), months=spanish?['ene','feb','mar','abr','may','jun','jul','ago','sep','oct','nov','dic']:['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'],longMonths=spanish?['enero','febrero','marzo','abril','mayo','junio','julio','agosto','septiembre','octubre','noviembre','diciembre']:['January','February','March','April','May','June','July','August','September','October','November','December'];
  if(pattern==='year_month')return spanish?`${m[2]}/${m[1]}`:`${m[1]}-${m[2]}`;
  if(pattern==='date_short')return spanish?`${m[3]}/${m[2]}/${m[1]}`:`${m[2]}/${m[3]}/${m[1]}`;
  const names=pattern==='date_long'?longMonths:months,date=spanish?`${m[3]} ${names[Number(m[2])-1]} ${m[1]}`:`${names[Number(m[2])-1]} ${m[3]}, ${m[1]}`;
  return pattern==='datetime_short'&&m[4]?`${date} ${m[4]}:${m[5]}`:date;
}
function columnLabel(column){return text(column?.display_label)||text(column?.name);}
function columnFor(chart, slot) { return array(chart.columns).find(c => c.id === chart.mapping?.bindings?.[slot]); }
function pointCell(chart, p, column) {
  const b = chart.mapping?.bindings || {};
  for (const slot of ['category','series','parent','x','y','value','size']) if (b[slot] === column.id) return p[slot];
  if (array(b.values).includes(column.id) && p.measure === column.id) return p.value;
  const level = array(b.hierarchy).indexOf(column.id);
  if (level >= 0) return array(p.path)[level];
  return {null:true,value:''};
}
function coordinate(value) { return value && !value.null && typeof value.coordinate === 'number' && Number.isFinite(value.coordinate) ? value.coordinate : null; }
function scale(values) {
  let magnitude = 1;
  for (const v of values) if (v !== null) magnitude = Math.max(magnitude,Math.abs(v));
  return magnitude;
}
function categoryKey(cell) { return JSON.stringify([cell?.null ?? true,text(cell?.value ?? cell?.exact)]); }

function renderTable(parent, columns, rows, w, caption, totals = [], rowIndices = []) {
  if (columns.length > 256 || rows.length > 1000) throw fail('limit_exceeded');
  if (rows.length === 0) { const notice = element('p',w.empty,'notice'); notice.setAttribute('role','status'); parent.append(notice); }
  const scroll = element('div',undefined,'scroll');
  scroll.tabIndex = 0;
  const table = element('table');
  table.append(element('caption',caption));
  const head = element('thead'), header = element('tr');
  if (rowIndices.length) { const th = element('th',w.row); th.scope = 'col'; header.append(th); }
  for (const column of columns) { const th = element('th',columnLabel(column)); th.scope = 'col'; header.append(th); }
  head.append(header); table.append(head);
  const body = element('tbody');
  for (const [index,row] of rows.entries()) {
    if (!Array.isArray(row) || row.length !== columns.length) throw fail('invalid_request');
    const tr = element('tr');
    if (rowIndices.length) { const th = element('th',rowIndices[index]+1); th.scope = 'row'; tr.dataset.sourceRow = String(rowIndices[index]); tr.append(th); }
    row.forEach((c,i) => tr.append(element('td',exact(c,columns[i],w.null))));
    body.append(tr);
  }
  table.append(body); scroll.append(table); parent.append(scroll);
  for (const total of totals) {
    const c = columns.find(c => c.id === total.column);
    if (c) parent.append(element('p',`${columnLabel(c)}: ${exact(total.value,c,w.null)} — ${w.scope}: ${text(total.scope)}`,'metadata'));
  }
}

function pagedValues(parent, columns, rows, w, caption, totals=[], rowIndices=[], cls='') {
  const details = element('details',undefined,cls); details.append(element('summary',caption));
  const body = element('div'), controls = element('div',undefined,'pager');
  let offset = 0;
  const draw = () => {
    body.replaceChildren(); controls.replaceChildren();
    renderTable(body,columns,rows.slice(offset,offset+100),w,`${offset+Math.min(1,rows.length)}–${Math.min(offset+100,rows.length)} / ${rows.length}`,totals,rowIndices.slice(offset,offset+100));
    const previous = button(w.previous,() => { offset = Math.max(0,offset-100); draw(); }); previous.disabled = offset === 0;
    const next = button(w.next,() => { offset += 100; draw(); }); next.disabled = offset+100 >= rows.length;
    controls.append(previous,next);
  };
  draw(); details.append(body,controls); parent.append(details);
}
function accessiblePoints(parent, chart, w) {
  const columns = array(chart.columns), points = array(chart.points), rich = chart.version === 2;
  // The wide rows, including omitted coordinates, are the exact retained result.
  // Normalized observations are separate: an absent observation has no source row.
  pagedValues(parent,columns,rich ? array(chart.rows) : points.map(p => columns.map(c => pointCell(chart,p,c))),w,w.values,array(chart.totals),rich ? array(chart.row_indices) : [],'retained-values');
  const cell = value => ({null:false,value:String(value)}), missing = () => ({null:true,value:''});
  const defs = new Map(array(chart.series).map(s => [s.id,s]));
  if (rich && defs.size) {
    const headers = [w.series,w.seriesID,w.measure,w.category,w.value,w.row,w.state].map((name,i) => ({id:'observation-'+i,name}));
    const rows = points.map(p => {
      const def = defs.get(p.series_id), column = columns.find(c => c.id === def?.measure);
      return [cell(def?.name || ''),cell(p.series_id),cell(column?.name || ''),p.category || missing(),
        cell(exact(chart.kind === 'scatter' ? p.y : p.value,column,w.null)),p.row < 0 ? missing() : cell(p.row+1),
        cell(p.row < 0 ? 'absent_observation' : p.value?.null && chart.kind !== 'scatter' ? 'missing_value' : 'retained_observation')];
    });
    pagedValues(parent,headers,rows,w,w.seriesValues,[],[],'series-values');
  }
  if (rich && array(chart.hierarchy).length) {
    const headers = [w.depth,w.path,w.value,w.aggregation,w.scope,w.members].map((name,i) => ({id:'hierarchy-'+i,name}));
    const rows = chart.hierarchy.map(n => [cell(n.depth+1),cell(n.path.map(p=>cellValue(p,w.null)).join(' / ')),
      cell(exact(n.value,columnFor(chart,'value'),w.null)),cell(n.aggregation),cell(n.scope),cell(n.rows.map(i=>i+1).join(', '))]);
    pagedValues(parent,headers,rows,w,w.hierarchyValues,[],[],'hierarchy-values');
  }
}

function seriesKey(p) { return text(p.series_id) || categoryKey(p.series); }
function pointCategoryKey(p) { return text(p.category_key) || categoryKey(p.category); }
function seriesNames(c,w) {
  if (array(c.series).length) return new Map(c.series.map(s=>[s.id,s.name]));
  const names = new Map();
  for (const p of c.points) if (!names.has(seriesKey(p))) names.set(seriesKey(p),cellValue(p.series,columnFor(c,'value')?.name || w.null));
  return names;
}
function palette(c,p) { return Math.max(0,array(c.palette || c.series).findIndex(s=>s.id===p.series_id)); }
function unitLabel(format,w) { return [text(format?.currency),text(format?.unit),format?.percent ? '%' : ''].filter(Boolean).join(' ') || w.unitless; }
function renderScales(parent,c,w,draw) {
  if (c.version !== 2) { draw(parent,c,w); return; }
  const groups = new Map();
  for (const def of array(c.series)) {
    const f = def.format || {}, key = JSON.stringify([f.unit || '',f.currency || '',f.percent || '']);
    if (!groups.has(key)) groups.set(key,[]);
    groups.get(key).push(def);
  }
  for (const defs of groups.values()) {
    const panel = element('section',undefined,'scale-panel'), names = new Set(defs.map(d=>d.id)), label = unitLabel(defs[0].format,w);
    panel.setAttribute('aria-label',`${w.independent}: ${label}`);
    panel.append(element('h3',`${w.independent}: ${label}`)); parent.append(panel);
    draw(panel,{...c,series:defs,palette:c.series,points:c.points.filter(p=>names.has(p.series_id))},w);
  }
}

// This is a bounded consumer check, not a second query/authority validator. An
// unsupported or inconsistent retained version must never fall back to scalar.
function validateRetainedChart(c) {
  if (![1,2,3].includes(c.version) || c.mapping?.version !== c.version || c.mapping.kind !== c.kind) throw fail('invalid_request');
  if (c.version === 1) return;
	if(c.version===3){if(!['kpi','table'].includes(c.kind))throw fail('invalid_request');if(c.kind==='kpi'&&!c.kpi_result||c.kind==='table'&&!integer(c.table_page_size,1,1000))throw fail('invalid_request');return;}
  const b = c.mapping.bindings, columns = array(c.columns), points = array(c.points), rows = array(c.rows), indices = array(c.row_indices);
  if (!b || c.transformation?.version !== 1 || !integer(c.input_rows,0,MAX_POINTS) || rows.length !== c.input_rows || indices.length !== rows.length || new Set(indices).size !== indices.length) throw fail('invalid_request');
  const ids = new Set(columns.map(col=>col.id)), defs = new Map();
  if (ids.size !== columns.length || array(c.series).length > 128) throw fail('invalid_request');
  for (const [i,row] of rows.entries()) if (!Array.isArray(row) || row.length !== columns.length || !integer(indices[i],0,c.input_rows-1)) throw fail('invalid_request');
  for (const def of array(c.series)) {
    if (!id(def.id) || defs.has(def.id) || !ids.has(def.measure) || typeof def.name !== 'string') throw fail('invalid_request');
    defs.set(def.id,def);
  }
  const tuples = new Set();
  for (const p of points) {
    if (!integer(p.row,-1,c.input_rows-1) || p.row === -1 && (!['line','area'].includes(c.kind) || !p.value?.null)) throw fail('invalid_request');
    if (c.kind !== 'treemap') {
      if (!defs.has(p.series_id) || c.kind !== 'scatter' && (p.measure !== defs.get(p.series_id).measure || !id(p.category_key))) throw fail('invalid_request');
      const tuple = JSON.stringify([p.category_key,p.series_id]);
      if (c.kind !== 'scatter' && tuples.has(tuple)) throw fail('invalid_request');
      tuples.add(tuple);
    }
    if (b.size && (coordinate(p.size) === null || coordinate(p.size) <= 0 || c.transformation.size_encoding !== 'area')) throw fail('invalid_request');
  }
  if (c.kind === 'treemap') {
    if (array(b.hierarchy).length < 1 || b.hierarchy.length > 8 || array(c.hierarchy).length > MAX_POINTS) throw fail('invalid_request');
    const nodes = new Map();
    for (const node of array(c.hierarchy)) {
      if (!id(node.id) || nodes.has(node.id) || !integer(node.depth,0,b.hierarchy.length-1) || array(node.path).length !== node.depth+1 || coordinate(node.value) === null || coordinate(node.value) < 0) throw fail('invalid_request');
      const parent = nodes.get(node.parent);
      if (node.depth === 0 ? node.parent !== '' : !parent || parent.depth+1 !== node.depth || JSON.stringify(node.path.slice(0,-1)) !== JSON.stringify(parent.path)) throw fail('invalid_request');
      if (!array(node.rows).length || node.rows.some(i=>!integer(i,0,c.input_rows-1))) throw fail('invalid_request');
      nodes.set(node.id,node);
    }
    for (const p of points) if (!nodes.has(p.category_key) || JSON.stringify(p.path) !== JSON.stringify(nodes.get(p.category_key).path)) throw fail('invalid_request');
  } else if (!['line','area','bar','column','grouped_bar','scatter'].includes(c.kind)) throw fail('invalid_request');
}

function chartRoot(parent, chart, w) {
  const s = svg('svg',{viewBox:'0 0 800 420',role:'img',class:'chart','aria-label':text(chart.mapping?.options?.title) || text(chart.kind)});
  const desc = document.createElementNS(NS,'desc'); desc.textContent = w.geometry; s.append(desc); parent.append(s); return s;
}
function titleFor(chart, p, w) {
  const def = array(chart.series).find(s=>s.id===p.series_id), labels = [];
  if (def) labels.push(`${w.series}: ${def.name}`,`${w.seriesID}: ${def.id}`);
  if (chart.version === 2) labels.push(`${w.row}: ${p.row < 0 ? w.gaps : p.row+1}`);
  for (const c of array(chart.columns)) {
    if (array(chart.mapping?.bindings?.values).includes(c.id) && c.id !== p.measure) continue;
    labels.push(`${c.name}: ${exact(pointCell(chart,p,c),c,w.null)}`);
  }
  return labels.join(' · ');
}
function legend(parent, values) { const d = element('div',undefined,'legend'); values.forEach((v,i) => d.append(element('span',`${i+1}. ${v}`))); parent.append(d); }

function renderBars(parent, c, w) {
  const points = c.points, horizontal = ['bar','grouped_bar','stacked_bar'].includes(c.kind), stacked = c.kind.startsWith('stacked_'), grouped = c.kind === 'grouped_bar' || c.version === 2 && array(c.series).length > 1;
  const categories = [], catIndex = new Map(), names = seriesNames(c,w), series = Array.from(names.values()), seriesIndex = new Map(Array.from(names.keys(),(key,i)=>[key,i]));
  for (const p of points) {
    const ck = pointCategoryKey(p), sk = seriesKey(p);
    if (!catIndex.has(ck)) { catIndex.set(ck,categories.length); categories.push(cellValue(p.category,w.null)); }
    if (!seriesIndex.has(sk)) { seriesIndex.set(sk,series.length); series.push(cellValue(p.series,w.null)); }
  }
  const magnitude = scale(points.map(p => coordinate(p.value)));
  const positions = [], accum = new Map(); let lo = 0, hi = 0;
  points.forEach((p,i) => {
    const n = coordinate(p.value); if (n === null) return;
    const value = n/magnitude, index = catIndex.get(pointCategoryKey(p));
    let start = 0;
    if (stacked) { const key = index+':'+(value >= 0 ? '+' : '-'); start = accum.get(key) || 0; accum.set(key,start+value); }
    const end = start+value; lo = Math.min(lo,start,end); hi = Math.max(hi,start,end);
    positions.push({p,i,index,series:seriesIndex.get(seriesKey(p)),start,end});
  });
  if (hi === lo) hi = lo+1;
  const s = chartRoot(parent,c,w), width = 640, height = 320;
  const x = v => 130+(v-lo)/(hi-lo)*width, y = v => 350-(v-lo)/(hi-lo)*height;
  if (horizontal) s.append(svg('line',{x1:x(0),y1:20,x2:x(0),y2:350,class:'axis'}));
  else s.append(svg('line',{x1:60,y1:y(0),x2:770,y2:y(0),class:'axis'}));
  const stride = (horizontal ? height : 710)/Math.max(1,categories.length), slot = stride*.75/(grouped ? Math.max(1,series.length) : 1);
  for (const q of positions) {
    const sub = grouped ? q.series*slot : 0;
    const attrs = horizontal ? {x:Math.min(x(q.start),x(q.end)),y:24+q.index*stride+sub,width:Math.abs(x(q.end)-x(q.start)),height:Math.max(0,slot-Math.min(2,slot*.2))} : {x:64+q.index*stride+sub,y:Math.min(y(q.start),y(q.end)),width:Math.max(0,slot-Math.min(2,slot*.2)),height:Math.abs(y(q.end)-y(q.start))};
    s.append(svg('rect',{...attrs,class:'swatch-'+((c.version === 2 ? palette(c,q.p) : stacked || grouped ? q.series : q.index)%8),'data-series-id':seriesKey(q.p),'data-measure':text(q.p.measure),'data-category-key':pointCategoryKey(q.p)},titleFor(c,q.p,w)));
  }
  categories.forEach((name,i) => { const t = svg('text',horizontal ? {x:8,y:30+i*stride} : {x:64+i*stride,y:375}); t.textContent = shorten(name,32); const full = svg('title'); full.textContent = name; t.append(full); s.append(t); });
  if ((stacked || grouped || c.version === 2) && c.mapping?.options?.legend?.visible !== false) legend(parent,series);
}

function renderLines(parent,c,w) {
  const s = chartRoot(parent,c,w), points = c.points, magnitude = scale(points.map(p => coordinate(p.value)));
  let lo = 0, hi = 0;
  for (const p of points) { const n = coordinate(p.value); if (n !== null) { lo = Math.min(lo,n/magnitude); hi = Math.max(hi,n/magnitude); } }
  if (hi === lo) hi = lo+1;
  const categories = [], indices = new Map(), names = seriesNames(c,w), groups = new Map(Array.from(names.keys(),key=>[key,[]]));
  for (const p of points) {
    const key = pointCategoryKey(p); if (!indices.has(key)) { indices.set(key,categories.length); categories.push(cellValue(p.category,w.null)); }
    const sk = seriesKey(p); if (!groups.has(sk)) groups.set(sk,[]); groups.get(sk).push(p);
  }
  const x = p => 65+indices.get(pointCategoryKey(p))*680/Math.max(1,categories.length-1), y = v => 350-(v/magnitude-lo)/(hi-lo)*315;
  s.append(svg('line',{x1:55,y1:y(0),x2:755,y2:y(0),class:'axis'}));
  let color = 0;
  for (const [key,group] of groups) {
    if (!group.length) continue;
    if (c.version === 2) color = palette(c,group[0]);
    let segment = [];
    const flush = () => {
      if (!segment.length) return;
      const d = segment.map((p,i) => `${i ? 'L' : 'M'} ${x(p)} ${y(coordinate(p.value))}`).join(' ');
      if (c.kind === 'area') s.append(svg('path',{d:d+` L ${x(segment.at(-1))} ${y(0)} L ${x(segment[0])} ${y(0)} Z`,class:`swatch-${color%8} area`,'data-series-id':key}));
      s.append(svg('path',{d,class:`swatch-${color%8} line`,'data-series-id':key})); segment = [];
    };
    for (const p of group) {
      if (coordinate(p.value) === null) { flush(); continue; }
      segment.push(p); s.append(svg('circle',{cx:x(p),cy:y(coordinate(p.value)),r:3,class:'swatch-'+(color%8),'data-series-id':key,'data-measure':text(p.measure),'data-category-key':pointCategoryKey(p)},titleFor(c,p,w)));
    }
    flush(); color++;
  }
  categories.forEach((label,i) => { if (i % Math.max(1,Math.ceil(categories.length/10)) === 0) { const t = svg('text',{x:65+i*680/Math.max(1,categories.length-1),y:380}); t.textContent = shorten(label,22); t.append(svg('title',{},label)); s.append(t); } });
  if ((groups.size > 1 || c.version === 2) && c.mapping?.options?.legend?.visible !== false) legend(parent,Array.from(names.values()));
}

function renderPie(parent,c,w) {
  const s = chartRoot(parent,c,w), points = c.points.filter(p => coordinate(p.value) > 0), magnitude = scale(points.map(p => coordinate(p.value)));
  const sum = points.reduce((n,p) => n+coordinate(p.value)/magnitude,0);
  let angle = -Math.PI/2;
  points.forEach((p,i) => {
    const sweep = coordinate(p.value)/magnitude/sum*2*Math.PI, next = angle+sweep;
    const start = [260+160*Math.cos(angle),210+160*Math.sin(angle)], end = [260+160*Math.cos(next),210+160*Math.sin(next)];
    if (points.length === 1) s.append(svg('circle',{cx:260,cy:210,r:160,class:'swatch-'+(i%8)},titleFor(c,p,w)));
    else s.append(svg('path',{d:`M 260 210 L ${start[0]} ${start[1]} A 160 160 0 ${sweep>Math.PI?1:0} 1 ${end[0]} ${end[1]} Z`,class:'swatch-'+(i%8)},titleFor(c,p,w)));
    angle = next;
  });
  if (c.kind === 'donut') s.append(svg('circle',{cx:260,cy:210,r:92,fill:'var(--bg)',stroke:'none'}));
  if (c.mapping?.options?.legend?.visible !== false) legend(parent,points.map(p => `${cellValue(p.category,w.null)}: ${exact(p.value,columnFor(c,'value'),w.null)}`));
}

function renderScatter(parent,c,w) {
  const points = c.points.filter(p => coordinate(p.x) !== null && coordinate(p.y) !== null), mx = scale(points.map(p => coordinate(p.x))), my = scale(points.map(p => coordinate(p.y)));
  let minx = Infinity,maxx = -Infinity,miny = Infinity,maxy = -Infinity;
  for (const p of points) { minx=Math.min(minx,p.x.coordinate/mx); maxx=Math.max(maxx,p.x.coordinate/mx); miny=Math.min(miny,p.y.coordinate/my); maxy=Math.max(maxy,p.y.coordinate/my); }
  if (!points.length) return;
  const s = chartRoot(parent,c,w), names = seriesNames(c,w), colors = new Map(Array.from(names.keys(),(key,i)=>[key,i])), bubble = !!c.mapping?.bindings?.size;
  const maxSize = bubble ? Math.max(...points.map(p=>coordinate(p.size))) : 1;
  for (const p of points) s.append(svg('circle',{cx:65+(p.x.coordinate/mx-minx)/(maxx-minx || 1)*680,cy:350-(p.y.coordinate/my-miny)/(maxy-miny || 1)*310,r:bubble ? 28*Math.sqrt(coordinate(p.size)/maxSize) : 5,class:'swatch-'+(colors.get(seriesKey(p))%8),'data-series-id':seriesKey(p),'data-size':text(p.size?.exact)},titleFor(c,p,w)));
  if (c.mapping?.bindings?.series && c.mapping?.options?.legend?.visible !== false) legend(parent,Array.from(names.values()));
  if (bubble) parent.append(element('p',`${columnFor(c,'size')?.name}: ${unitLabel(columnFor(c,'size')?.format,w)} · area ∝ size`,'metadata'));
  const labels = [columnFor(c,'x')?.name,columnFor(c,'y')?.name];
  labels.forEach((label,i) => { const t = svg('text',i ? {x:12,y:22} : {x:350,y:395}); t.textContent = text(label); s.append(t); });
}

function renderHeatmap(parent,c,w) {
  const xs=[],ys=[],xi=new Map(),yi=new Map();
  for (const p of c.points) { const x=categoryKey(p.x),y=categoryKey(p.y); if(!xi.has(x)){xi.set(x,xs.length);xs.push(cellValue(p.x,w.null));} if(!yi.has(y)){yi.set(y,ys.length);ys.push(cellValue(p.y,w.null));} }
  const s=chartRoot(parent,c,w),cw=650/Math.max(1,xs.length),ch=310/Math.max(1,ys.length),magnitude=scale(c.points.map(p=>coordinate(p.value)));
  for(const p of c.points){const v=coordinate(p.value);if(v===null)continue;s.append(svg('rect',{x:120+xi.get(categoryKey(p.x))*cw,y:25+yi.get(categoryKey(p.y))*ch,width:cw-1,height:ch-1,class:v<0?'swatch-4':'swatch-0','fill-opacity':.15+.85*Math.abs(v)/magnitude},titleFor(c,p,w)));}
  xs.forEach((label,i)=>{const t=svg('text',{x:120+i*cw,y:365});t.textContent=shorten(label,20);s.append(t);});
  ys.forEach((label,i)=>{const t=svg('text',{x:8,y:40+i*ch});t.textContent=shorten(label,20);s.append(t);});
}

function renderTreemap(parent,c,w) {
  const s=chartRoot(parent,c,w),points=c.points.filter(p=>coordinate(p.value)>0),magnitude=scale(points.map(p=>coordinate(p.value))),groups=new Map();
  for(const p of points){const key=categoryKey(p.parent);if(!groups.has(key))groups.set(key,[]);groups.get(key).push(p);}
  const total=points.reduce((n,p)=>n+p.value.coordinate/magnitude,0);let left=20,color=0;
  for(const group of groups.values()){
    const weight=group.reduce((n,p)=>n+p.value.coordinate/magnitude,0),width=760*weight/total;let top=40;
    const title=cellValue(group[0].parent,'');const label=svg('text',{x:left+5,y:25});label.textContent=shorten(title,40);s.append(label);
    for(const p of group){const height=350*(p.value.coordinate/magnitude)/weight;s.append(svg('rect',{x:left+2,y:top,width:Math.max(0,width-4),height:Math.max(0,height-2),class:'swatch-'+(color%8)},titleFor(c,p,w)));if(width>70&&height>30){const t=svg('text',{x:left+8,y:top+20});t.textContent=shorten(cellValue(p.category,w.null),Math.floor(width/9));s.append(t);}top+=height;color++;}
    left+=width;
  }
}

function renderHierarchy(parent,c,w) {
  const root = chartRoot(parent,c,w), children = new Map();
  for (const node of c.hierarchy) {
    if (!children.has(node.parent)) children.set(node.parent,[]);
    children.get(node.parent).push(node);
  }
  const draw = (parentID,box,depth,color) => {
    if (depth >= 8) return;
    const nodes = (children.get(parentID) || []).filter(n=>coordinate(n.value)>0);
    const magnitude = scale(nodes.map(n=>coordinate(n.value))), total = nodes.reduce((sum,n)=>sum+coordinate(n.value)/magnitude,0);
    let offset = 0;
    nodes.forEach((node,i)=>{
      const fraction = (coordinate(node.value)/magnitude)/total;
      const rect = depth%2 ? {x:box.x,y:box.y+offset,width:box.width,height:box.height*fraction} : {x:box.x+offset,y:box.y,width:box.width*fraction,height:box.height};
      offset += depth%2 ? rect.height : rect.width;
      const shade = depth === 0 ? i : color, branch = children.has(node.id), path = node.path.map(p=>cellValue(p,w.null)).join(' / ');
      const title = `${path}: ${exact(node.value,columnFor(c,'value'),w.null)} · ${node.aggregation} · ${node.scope}`;
      root.append(svg('rect',{...rect,class:`swatch-${shade%8} ${branch ? 'hierarchy-branch' : 'hierarchy-leaf'}`,'data-depth':depth,'data-node-id':node.id},title));
      if (rect.width > 55 && rect.height > 24) { const label = svg('text',{x:rect.x+4,y:rect.y+15}); label.textContent = shorten(cellValue(node.path.at(-1),w.null),Math.max(1,Math.floor(rect.width/8)-1)); label.append(svg('title',{},title)); root.append(label); }
      if (branch) draw(node.id,{x:rect.x+Math.min(3,rect.width/4),y:rect.y+Math.min(20,rect.height/4),width:Math.max(0,rect.width-6),height:Math.max(0,rect.height-23)},depth+1,shade);
    });
  };
  draw('',{x:12,y:12,width:776,height:396},0,0);
}

export function renderChart(parent, chart, language='en') {
  boundedJSON(chart); const w=words[language==='es'?'es':'en'];
  if(!KINDS.includes(chart.kind)||array(chart.points).length>MAX_POINTS||array(chart.columns).length>256)throw fail('invalid_request');
  validateRetainedChart(chart);
  if(chart.mapping?.options?.title)parent.append(element('h2',chart.mapping.options.title));
  const ready = chart.state === 'ready';
  if(!ready)parent.append(element('p',`${w.empty} ${text(chart.state)}`,'notice'));
  if(chart.kind==='table'){renderTable(parent,array(chart.columns),array(chart.rows),w,w.values,array(chart.totals));return;}
  if(ready && chart.kind==='kpi'){
    const valueColumn=columnFor(chart,'value'),targetColumn=columnFor(chart,'target'),percentColumn={type:'decimal',format:{percent:'whole'}},k=chart.kpi_result;
    parent.append(element('p',exact(k?.value||chart.points[0]?.value,valueColumn,w.null),'kpi'));
    for(const [label,v,column] of [['Comparison',k?.comparison,valueColumn],['Delta',k?.delta,valueColumn],['Percent delta',k?.percent_delta,percentColumn],['Target',k?.target,targetColumn],['Target difference',k?.target_difference,valueColumn]])if(v)parent.append(element('p',`${label}: ${exact(v,column,w.null)}`,'metadata'));
    if(k?.threshold_state)parent.append(element('p',`${text(k.threshold_label)||k.threshold_state} · ${k.threshold_state}`,'badge'));
    if(array(k?.sparkline).length){const line=element('p',k.sparkline.map(v=>v.null?w.null:v.exact).join(' → '),'metadata');line.setAttribute('aria-label','Sparkline exact values');parent.append(line);}
  }
  else if(ready && ['bar','column','grouped_bar','stacked_bar','stacked_column'].includes(chart.kind))renderScales(parent,chart,w,renderBars);
  else if(ready && ['line','area'].includes(chart.kind))renderScales(parent,chart,w,renderLines);
  else if(ready && ['pie','donut'].includes(chart.kind))renderPie(parent,chart,w);
  else if(ready && chart.kind==='scatter')renderScatter(parent,chart,w);
  else if(ready && chart.kind==='heatmap')renderHeatmap(parent,chart,w);
  else if(ready && chart.kind==='treemap')(chart.version === 2 ? renderHierarchy : renderTreemap)(parent,chart,w);
  parent.append(element('p',w.geometry,'metadata'));
  if (chart.version === 2) {
    const t = chart.transformation;
    parent.append(element('p',`${w.missingValues}: ${t.missing_points} · ${w.gaps}: ${t.gap_points} · ${w.omittedRows}: ${chart.omitted_rows} / ${chart.input_rows} · ${w.zeroSize}: ${t.zero_size_points} · ${w.scope}: ${t.scope}`,'transformation'),element('p',`${t.method} · ${t.null_policy} · ${t.duplicate_policy}`,'metadata'),element('p',w.resolution,'metadata'));
    if (['line','area'].includes(chart.kind)) parent.append(element('p',`${columnFor(chart,'category')?.name} · ${w.grain}: ${columnFor(chart,'category')?.grain || 'unspecified'} / ${chart.mapping.order[0]?.direction}`,'metadata'));
  }
  for(const warning of array(chart.warnings))parent.append(element('p',text(warning),'metadata'));
  accessiblePoints(parent,chart,w);
}

// A deliberately small implementation of the established MCP Apps postMessage
// bridge. It only talks to its exact parent and pins the initialization origin.
export class Bridge {
  constructor(win=window){this.win=win;this.parent=win.parent;this.pending=new Map();this.sequence=0;this.origin=null;this.ready=false;this.closed=false;this.onresult=()=>{};this.oncontext=()=>{};this.oninput=()=>{};this.onfailure=()=>{};this.onclose=()=>{};this.listener=e=>this.receive(e);win.addEventListener('message',this.listener);}
  send(value){if(this.closed)throw fail('unavailable');boundedJSON(value,MAX_MESSAGE);this.parent.postMessage(value,this.origin&&this.origin!=='null'?this.origin:'*');}
  request(method,params){if(this.closed||this.pending.size>=16)return Promise.reject(fail('busy'));const id=++this.sequence;return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{this.pending.delete(id);reject(fail('cancelled_or_timed_out'));},65000);this.pending.set(id,{resolve,reject,timer,method});try{this.send({jsonrpc:'2.0',id,method,params});}catch(e){clearTimeout(timer);this.pending.delete(id);reject(e);}});}
  async connect(){if(this.parent===this.win)throw fail('unavailable');const result=await this.request('ui/initialize',{protocolVersion:'2026-01-26',appInfo:{name:'Chartworks reporting',version:'1'},appCapabilities:{availableDisplayModes:['inline','fullscreen']}});if(result?.protocolVersion!=='2026-01-26')throw fail('unavailable');this.ready=true;this.capabilities=result.hostCapabilities||{};this.oncontext(result.hostContext||{});this.send({jsonrpc:'2.0',method:'ui/notifications/initialized'});return result;}
  async call(name,args){if(!this.ready||!TOOLS.has(name)||!this.capabilities.serverTools)throw fail('forbidden');boundedJSON(args,65536);return this.request('tools/call',{name,arguments:args});}
  receive(event){
    if(this.closed||event.source!==this.parent||this.origin!==null&&event.origin!==this.origin)return;
    try{
      const m=boundedJSON(event.data,MAX_MESSAGE);if(m?.jsonrpc!=='2.0')return;
      if(m.id!==undefined&&(Object.hasOwn(m,'result')||Object.hasOwn(m,'error'))){const p=this.pending.get(m.id);if(!p)return;if(this.origin===null){if(p.method!=='ui/initialize')return;this.origin=event.origin;}this.pending.delete(m.id);clearTimeout(p.timer);if(m.error)p.reject(fail('unavailable'));else p.resolve(m.result);return;}
      if(!this.ready)return;
      if(m.method==='ui/notifications/tool-result')this.onresult(m.params);
      else if(m.method==='ui/notifications/host-context-changed')this.oncontext(m.params);
      else if(m.method==='ui/notifications/tool-input')this.oninput(m.params);
      else if(m.method==='ui/resource-teardown'&&m.id!==undefined){this.send({jsonrpc:'2.0',id:m.id,result:{}});this.close();}
      else if(m.method==='ping'&&m.id!==undefined)this.send({jsonrpc:'2.0',id:m.id,result:{}});
      else if(m.id!==undefined)this.send({jsonrpc:'2.0',id:m.id,error:{code:-32601,message:'not_found'}});
    }catch(e){this.onfailure(fail(e.code));}
  }
  resize(width,height){if(this.ready&&!this.closed&&Number.isFinite(width)&&Number.isFinite(height))this.send({jsonrpc:'2.0',method:'ui/notifications/size-changed',params:{width:Math.min(1600,Math.max(200,Math.ceil(width))),height:Math.min(2400,Math.max(100,Math.ceil(height)))}});}
  close(){if(this.closed)return;this.closed=true;this.win.removeEventListener('message',this.listener);for(const p of this.pending.values()){clearTimeout(p.timer);p.reject(fail('unavailable'));}this.pending.clear();this.onclose();}
}

function unwrap(result){
  boundedJSON(result,MAX_MESSAGE);if(result?.isError){let code='unavailable';const f=result.structuredContent?.error;if(ERRORS.has(f?.code))code=f.code;const err=fail(code);err.unknown=f?.outcome==='unknown';throw err;}
  let body=result?.structuredContent;
  if(!body){const t=array(result?.content).find(c=>c.type==='text');if(!t||typeof t.text!=='string'||t.text.length>MAX_DATA)throw fail('unavailable');try{body=JSON.parse(t.text);}catch{throw fail('unavailable');}}
  if(body?.error)throw fail(body.error.code);
  const value=body?.result;boundedJSON(value);if(value?.version!==VERSION)throw fail('unavailable');return value;
}
function choiceLabel(item,locale) {
  const labels=array(item.metadata), language=(locale||'en').split('-')[0].toLowerCase();
  const label=labels.find(m=>m.locale===locale)||labels.find(m=>text(m.locale).split('-')[0].toLowerCase()===language);
  return text(label?.display_name)||text(item.title)||text(item.id);
}
function selectControl(label,items,current,onchange,locale='en') {
  const l=element('label',label),s=element('select'),w=words[locale.toLowerCase().startsWith('es')?'es':'en'];
  s.setAttribute('aria-label',label);
  for(const item of items){
    const disabled=item.enabled===false,omitted=item.selected===false;
    const suffix=disabled?w.disabled:omitted?w.omitted:'';
    const o=element('option',choiceLabel(item,locale)+(suffix?' — '+suffix:''));
    o.value=item.id;o.selected=item.id===current;o.disabled=disabled||omitted;
    if(item.description)o.title=text(item.description);
    s.append(o);
  }
  s.addEventListener('change',()=>{const chosen=s.selectedOptions[0];if(chosen&&!chosen.disabled)onchange(s.value);});
  l.append(s);return l;
}
// Display order and accepted execution order are different contracts. A new,
// explicitly requested filter run retains the latter and the accepted caps.
function retainedRunOutputs(v) {
  const choices=array(v.outputs),selected=v.accepted_selection?.selected;
  if(selected!==undefined){
    if(!Array.isArray(selected)||!selected.length||selected.length>64||new Set(selected).size!==selected.length||selected.some(value=>!id(value)||!choices.some(o=>o.id===value&&o.enabled!==false&&o.selected!==false)))throw fail('invalid_request');
    return selected.slice();
  }
  return choices.filter(o=>o.enabled!==false&&o.selected!==false).map(o=>o.id);
}
function periodValue(raw){let p;try{p=JSON.parse(raw);}catch{throw fail('invalid_request');}boundedJSON(p,4096);const keys=new Set(['mode','unit','count','start','end','from_date','first_occurrence','dst_policy','month_policy']);if(!p||Array.isArray(p)||Object.keys(p).some(k=>!keys.has(k)))throw fail('invalid_request');return {period:p};}

export class Viewer {
  constructor(root,bridge){this.root=root;this.bridge=bridge;this.locale='en';this.value=null;this.generation=0;this.mutationPending=false;this.timer=null;this.closed=false;this.lastSize='';bridge.onresult=result=>this.accept(result);bridge.oninput=()=>this.loading();bridge.oncontext=context=>this.context(context);bridge.onfailure=e=>this.error(e);bridge.onclose=()=>this.close();this.observer=typeof ResizeObserver==='function'?new ResizeObserver(()=>{const box=root.getBoundingClientRect(),key=Math.ceil(box.width)+':'+Math.ceil(box.height);if(key!==this.lastSize){this.lastSize=key;bridge.resize(box.width,box.height);}}):null;this.observer?.observe(root);this.loading();}
  get w(){return words[this.locale];}
  clear(){clearTimeout(this.timer);this.timer=null;this.value=null;this.root.replaceChildren();}
  loading(){this.generation++;this.clear();const p=element('p',this.w.loading,'notice');p.setAttribute('role','status');this.root.append(p);}
  error(e,mutation=false){this.generation++;this.clear();const p=element('p',`${this.w.error}: ${ERRORS.has(e?.code)?e.code:'unavailable'}`,'notice error');p.setAttribute('role','alert');this.root.append(p);if(mutation||e?.unknown)this.root.append(element('p',this.w.unknown));}
  context(c){try{boundedJSON(c,65536);document.documentElement.dataset.theme=c?.theme==='dark'?'dark':'light';if(typeof c?.locale==='string'&&c.locale.length<=64){try{const canonical=Intl.getCanonicalLocales(c.locale)[0];document.documentElement.lang=canonical;this.locale=canonical.toLowerCase().startsWith('es')?'es':'en';}catch{/* Ignore malformed optional host locale. */}}if(this.value)this.draw();}catch(e){this.error(e);}}
  accept(result){if(this.closed)return;try{const v=unwrap(result);if(v.selection&&v.summary)this.show(v);else if(id(v.run)&&['block','report','dashboard'].includes(v.kind))void this.read({kind:v.kind,run:v.run,page:'',widget:'',output:'',offset:0,limit:0});else throw fail('unavailable');}catch(e){this.error(e);}}
  show(v){
    boundedJSON(v);if(v.version!==VERSION||!id(v.summary?.run)||!['block','report','dashboard'].includes(v.summary.kind)||!v.selection||array(v.outputs).length>64||array(v.pages).length>100||array(v.filters).length>100)throw fail('invalid_request');
    if(v.selection.run!==v.summary.run||v.selection.kind!==v.summary.kind||v.summary.target?.kind!==v.summary.kind||!id(v.summary.target?.id)||!integer(v.summary.target?.revision,1,256))throw fail('invalid_request');
    if(!integer(v.page_bounds?.offset,0,100000)||!integer(v.page_bounds?.limit,1,1000)||!integer(v.page_bounds?.total,0,100000))throw fail('invalid_request');
    this.generation++;this.clear();this.value=v;
    const expiry=Date.parse(v.summary.expires_at);if(!Number.isFinite(expiry))throw fail('invalid_request');
    if(v.summary.state==='expired'||expiry<=Date.now()){this.clear();this.root.append(element('p',this.w.expired,'notice'));return;}
    this.timer=setTimeout(()=>{this.clear();this.root.append(element('p',this.w.expired,'notice'));},Math.min(2147483647,expiry-Date.now()));this.draw();
  }
  async read(selection){this.loading();const generation=this.generation;try{const result=await this.bridge.call('reporting_view',selection);if(generation!==this.generation||this.closed)return;this.show(unwrap(result));}catch(e){if(generation===this.generation&&!this.closed)this.error(e);}}
  navigate(change){if(this.value)void this.read({...this.value.selection,...change});}
  draw(){
    const v=this.value;if(!v||this.closed)return;this.root.replaceChildren();const w=this.w;
    this.root.append(element('p','Chartworks · '+w.run,'eyebrow'),element('h1',text(v.summary.target?.id)),element('p',`${w.state}: ${text(v.summary.state)} · ${text(v.summary.kind)} · ${v.summary.target?.revision??''}`,'metadata'));
    if(v.summary.private)this.root.append(element('span',w.private,'badge'));
    if(v.summary.state==='partial')this.root.append(element('span',w.partial,'badge'));
    if(v.redacted)this.root.append(element('p',w.redacted,'notice'));
    const meta=element('div',undefined,'metadata');
    if(v.observed_at)meta.append(element('p',`${w.observed}: ${v.observed_at}`));
    meta.append(element('p',`${w.retained}: ${v.summary.expires_at} · ${text(v.locale)} · ${text(v.timezone)}`));
    if(v.trust)meta.append(element('p',`${w.trust}: ${text(v.trust.publication)} / ${text(v.trust.certification)} / ${text(v.trust.health?.status)}`));
    if(v.query_limits)meta.append(element('p',`${w.limits}: ${v.query_limits.max_rows} rows · ${v.query_limits.max_bytes} bytes · ${v.query_limits.timeout_ms} ms · ${v.query_limits.query_attempts} attempts`));
    if(v.mixed_freshness)meta.append(element('p','mixed_freshness'));
    if(v.summary.scheduled){const s=v.summary.scheduled;meta.append(element('p',`${text(s.schedule_id)} · ${text(s.due_at)} · [${text(s.window_start)}, ${text(s.window_end)})`),element('p',`query: ${text(s.query)} · artifact: ${text(s.artifact)} · catalog: ${text(s.catalog)} · notification: ${text(s.notification)}`));}
    if(v.summary.code)meta.append(element('p',text(v.summary.code)));this.root.append(meta);
    const toolbar=element('div',undefined,'toolbar');
    if(v.pages?.length)toolbar.append(selectControl(w.pages,v.pages,v.selection.page,page=>this.navigate({page,widget:'',output:'',offset:0})));
    const page=array(v.pages).find(p=>p.id===v.selection.page);
    if(page?.widgets?.length)toolbar.append(selectControl(w.widgets,page.widgets,v.selection.widget,widget=>this.navigate({widget,output:'',offset:0})));
    if(v.outputs?.length)toolbar.append(selectControl(w.outputs,v.outputs,v.selection.output,output=>this.navigate({output,offset:0}),document.documentElement.lang||this.locale));
    this.root.append(toolbar);
    const content=element('section');content.setAttribute('aria-label',w.outputs);this.root.append(content);
    try{
      if(v.text)content.append(element('p',text(v.text.content??v.text.text),'narrative'));
      else if(v.output?.state==='succeeded'){
        if(v.output.table){const t=v.output.table;renderTable(content,array(t.columns),array(t.rows),w,w.table,array(t.totals));for(const warning of array(t.warnings))content.append(element('p',text(warning),'metadata'));const b=v.page_bounds;const pager=element('div',undefined,'pager');pager.append(element('span',`${b.offset+Math.min(1,array(t.rows).length)}–${b.offset+array(t.rows).length} / ${b.total}`));const prev=button(w.previous,()=>this.navigate({offset:Math.max(0,b.offset-b.limit)}));prev.disabled=b.offset===0;const next=button(w.next,()=>this.navigate({offset:b.next}));next.disabled=!integer(b.next,b.offset+1,b.total);pager.append(prev,next);content.append(pager);}
        else if(v.output.chart)renderChart(content,v.output.chart,this.locale);
        else if(v.output.narrative){content.append(element('p',text(v.output.narrative.text),'narrative'));for(const caveat of array(v.output.narrative.caveats))content.append(element('p',text(caveat),'notice'));content.append(element('p',`${text(v.output.narrative.model_version)} · ${text(v.output.narrative.prompt_version)} · ${text(v.output.narrative.locale)}`,'metadata'));}
      }else content.append(element('p',`${text(v.output?.state)||text(v.summary.state)} ${text(v.output?.code)}`,'notice'));
      if(!['succeeded','completed','partial','expired'].includes(v.summary.state))content.append(button(w.refresh,()=>this.navigate({})));
      if(!v.summary.private)this.filters(v);
    }catch(e){this.error(e);}
  }
  filters(v){
    const w=this.w,filters=array(v.filters);if(!filters.length)return;
    const details=element('details');details.append(element('summary',w.filters));const fieldset=element('fieldset');fieldset.append(element('legend',w.filters),element('p',w.consent));const grid=element('div',undefined,'filters'),controls=[];
    for(const f of filters){const p=f.parameter;if(!id(p?.name))throw fail('invalid_request');const label=element('label',text(f.label)||p.name);let input;
      if(array(p.enum).length||p.type==='boolean'){input=element('select');const options=p.type==='boolean'?['true','false']:p.enum;const empty=element('option',w.default);empty.value='';input.append(empty);for(const value of options){const o=element('option',value);o.value=value;input.append(o);}}
      else{input=element(p.type==='relative_period'?'textarea':'input');if(input.tagName==='INPUT'){input.type=p.type==='date'?'date':'text';if(['number','integer','top_n'].includes(p.type))input.inputMode='decimal';}input.maxLength=p.type==='relative_period'?4096:4096;input.placeholder=p.default?JSON.stringify(p.default):w.default;}
      input.setAttribute('aria-label',text(f.label)||p.name);const useDefault=element('input');useDefault.type='checkbox';useDefault.checked=true;input.disabled=true;useDefault.addEventListener('change',()=>{input.disabled=useDefault.checked;});const defaultLabel=element('span',undefined,'check');defaultLabel.append(useDefault,element('span',w.default));label.append(defaultLabel,input);if(p.type==='relative_period')label.append(element('span','JSON: mode, unit, count, start, end, dst_policy, month_policy','metadata'));grid.append(label);controls.push({f,input,useDefault});
    }
    fieldset.append(grid);const dynamic=element('input'),narrative=element('input');dynamic.type=narrative.type='checkbox';for(const [input,label]of[[dynamic,w.dynamic],[narrative,w.narrative]]){const l=element('label',undefined,'check');l.append(input,element('span',label));fieldset.append(l);}
    const run=button(w.apply,async()=>{
      if(this.mutationPending||this.closed)return;this.mutationPending=true;run.disabled=true;const oldSelection={...v.selection},startingGeneration=this.generation;
      try{
        const grouped=new Map();for(const {f,input,useDefault}of controls){if(useDefault.checked)continue;const value=f.parameter.type==='relative_period'?periodValue(input.value):{literal:input.value};if(!grouped.has(f.page))grouped.set(f.page,[]);grouped.get(f.page).push({name:f.parameter.name,value});}
        const description=unwrap(await this.bridge.call('reporting_describe',{target:v.summary.target,locale:text(v.locale),outputs:v.summary.kind==='block'?retainedRunOutputs(v):null}));
        if(this.closed||startingGeneration!==this.generation)return;
        if(v.summary.kind==='block'&&!['published','certified_only'].includes(v.policy))throw fail('stale_validation');
        if(!id(description.resource?.target?.id)||description.resource.target.id!==v.summary.target.id||description.resource.target.kind!==v.summary.target.kind||description.resource.target.revision!==v.summary.target.revision)throw fail('stale_validation');
        const request={target:description.resource.target,key:crypto.randomUUID(),arguments:[],pages:[],outputs:[],policy:v.summary.kind==='block'?v.policy:'',locale:text(v.locale),timezone:text(v.timezone)||description.timezone,narrative:narrative.checked,dynamic:dynamic.checked,partial_failure:''};
        if(v.summary.kind==='block'){request.arguments=Array.from(grouped.values()).flat();request.outputs=retainedRunOutputs(v);request.limits=v.query_limits??null;}
        else request.pages=Array.from(grouped,([page,filters])=>({page,filters,overrides:[]}));
        this.loading();const generation=this.generation;const response=await this.bridge.call('reporting_run',request);if(generation!==this.generation||this.closed)return;const result=unwrap(response);if(!id(result.run))throw fail('unavailable');await this.read({...oldSelection,run:result.run,offset:0});
      }catch(e){if(!this.closed)this.error(e,true);}finally{this.mutationPending=false;if(this.value&&!this.closed)this.draw();}
    },true);run.disabled=this.mutationPending;fieldset.append(run);details.append(fieldset);this.root.append(details);
  }
  close(){this.closed=true;this.generation++;this.clear();this.observer?.disconnect();}
}

const root=typeof document==='undefined'?null:document.getElementById('report-viewer');
if(root){if(window.parent===window){root.replaceChildren(element('p',words.en.host,'notice'));}else{const bridge=new Bridge(),viewer=new Viewer(root,bridge);bridge.connect().catch(e=>viewer.error(e));}}
