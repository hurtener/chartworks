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
  en: {loading:'Opening retained report…',error:'Report unavailable',expired:'Retained values have expired. Opening this report does not regenerate them.',empty:'No values in this retained output.',private:'Private preview',partial:'Partial result',pages:'Page',widgets:'Widget',outputs:'Output',previous:'Previous',next:'Next',values:'Exact retained values',null:'Missing',filters:'Run with different filters',apply:'Run with these filters',consent:'Running queries may incur cost and creates a new artifact. This does not alter the retained result.',dynamic:'Allow dynamic query generation',narrative:'Allow narrative generation',refresh:'Read status again',host:'Open this viewer through an authorized MCP Apps host.',unknown:'The operation outcome is unknown. Inspect its run history before submitting another run.',geometry:'Drawing coordinates are approximate. Display values use the saved format, which may round; unrounded values remain available in retained values.',default:'Use the published default',observed:'Observed',retained:'Retained until',trust:'Publication / certification / current health',redacted:'Some content is not visible under the current authority.',noscript:'The host did not provide a supported reporting result.',scope:'Total scope',table:'Table page',run:'Run',state:'State',limits:'Accepted query ceilings',disabled:'Disabled',omitted:'Not included in this run'},
  es: {loading:'Abriendo el informe guardado…',error:'Informe no disponible',expired:'Los valores guardados vencieron. Abrir el informe no los vuelve a generar.',empty:'No hay valores en esta salida guardada.',private:'Vista previa privada',partial:'Resultado parcial',pages:'Página',widgets:'Componente',outputs:'Salida',previous:'Anterior',next:'Siguiente',values:'Valores exactos guardados',null:'Sin dato',filters:'Ejecutar con otros filtros',apply:'Ejecutar con estos filtros',consent:'La ejecución puede generar costos y crea un nuevo resultado. No modifica el resultado guardado.',dynamic:'Permitir generación de consultas dinámicas',narrative:'Permitir generación narrativa',refresh:'Leer el estado nuevamente',host:'Abrí este visor desde un host MCP Apps autorizado.',unknown:'El resultado de la operación es incierto. Revisá su historial antes de ejecutar nuevamente.',geometry:'Las coordenadas del gráfico son aproximadas. Los valores usan el formato guardado, que puede redondear; los valores sin redondear están disponibles en los valores guardados.',default:'Usar el valor predeterminado publicado',observed:'Observado',retained:'Guardado hasta',trust:'Publicación / certificación / estado actual',redacted:'Parte del contenido no es visible con la autorización actual.',noscript:'El host no proporcionó un resultado compatible.',scope:'Alcance del total',table:'Página de tabla',run:'Ejecución',state:'Estado',limits:'Límites aceptados de consulta',disabled:'Deshabilitada',omitted:'No incluida en esta ejecución'}
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
export function exact(cell, column, missing = 'Missing', timezone = 'UTC') {
  const raw = cellValue(cell, missing);
  if (cell?.null || !cell) return raw;
  const f = column?.format || {};
  let value = raw;
	if (column?.type === 'temporal' && f.date_pattern) value = formatDate(raw,f.date_pattern,f.locale,column?.display_timezone || timezone);
  if (f.percent === 'fraction') { const shifted = percentShift(raw); value = shifted === null ? raw + ' (fraction)' : shifted + '%'; }
  if (f.percent === 'whole') value += '%';
	if (!f.percent && !f.preserve_precision && ['integer','decimal','number'].includes(column?.type)) value = formatDecimal(value,f.fraction_digits,f.locale);
	return [value,text(f.currency_symbol)||text(f.currency),text(f.unit)].filter(Boolean).join(' ');
}
function formatDecimal(raw,digits,locale) {
  const parsed=/^([+-]?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(raw);
  if (!integer(digits,0,20) || !parsed || raw.length>16384 || Math.abs(Number(parsed[4]||0))>4096) return raw;
  const coefficient=parsed[2]+(parsed[3]||''), dot=parsed[2].length+Number(parsed[4]||0);
  // Native rational formatting discards a leading plus and exact signed zero,
  // but keeps the sign of a nonzero negative value rounded to displayed zero.
  const sign=parsed[1]==='-'&&/[1-9]/.test(coefficient)?'-':'';
  let whole=dot<=0?'0':dot>=coefficient.length?coefficient+'0'.repeat(dot-coefficient.length):coefficient.slice(0,dot);
  let fraction=dot<=0?'0'.repeat(-dot)+coefficient:dot>=coefficient.length?'':coefficient.slice(dot);
  whole=whole.replace(/^0+(?=\d)/,'');
  if (fraction.length>digits) { const round=fraction[digits]>='5'; let scaled=BigInt(whole+(fraction.slice(0,digits)||'')); if(round)scaled+=1n; let s=scaled.toString().padStart(digits+1,'0'); whole=digits?s.slice(0,-digits):s; fraction=digits?s.slice(-digits):''; }
  else fraction=fraction.padEnd(digits,'0');
  const spanish=text(locale).toLowerCase().startsWith('es'), group=spanish?'.':',', decimal=spanish?',':'.';
  whole=whole.replace(/\B(?=(\d{3})+(?!\d))/g,group); return sign+whole+(digits?decimal+fraction:'');
}
function formatDate(raw,pattern,locale,timezone) {
  let m=/^(\d{4})-(\d{2})(?:-(\d{2})(?:[T ](\d{2}):(\d{2}))?)?/.exec(raw); if(!m||pattern!=='year_month'&&!m[3])return raw;
  if (/(?:Z|[+-]\d{2}:\d{2})$/.test(raw)) {
    const instant=new Date(raw); if(!Number.isFinite(instant.valueOf()))return raw;
    try {
      const values=Object.fromEntries(new Intl.DateTimeFormat('en-US',{timeZone:timezone,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hourCycle:'h23'}).formatToParts(instant).map(p=>[p.type,p.value]));
      m=['',values.year,values.month,values.day,values.hour,values.minute];
    } catch { return raw; }
  }
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

function appendRawPrecision(parent,cell,column,w){const raw=cellValue(cell,w.null),digits=column?.format?.fraction_digits;if(cell?.null||column?.format?.preserve_precision||!['integer','decimal','number'].includes(column?.type)||!integer(digits,0,20)||column.format?.percent)return;const numeric=/^[+-]?\d+(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/.exec(raw);if(!numeric||numeric[2]===undefined&&(numeric[1]?.length||0)<=digits)return;const disclosure=element('details',undefined,'cell-precision');disclosure.append(element('summary',w===words.es?'Precisión':'Precision'),element('p',`${w===words.es?'Valor guardado sin redondear':'Unrounded retained value'}: ${raw}`,'metadata raw-retained-value'));parent.append(disclosure);}

function renderTable(parent, columns, rows, w, caption, totals = [], rowIndices = [], timezone = 'UTC') {
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
    row.forEach((c,i) => {const td=element('td',exact(c,columns[i],w.null,timezone));appendRawPrecision(td,c,columns[i],w);tr.append(td);});
    body.append(tr);
  }
  table.append(body); scroll.append(table); parent.append(scroll);
  for (const total of totals) {
    const c = columns.find(c => c.id === total.column);
    if (c) {parent.append(element('p',`${columnLabel(c)}: ${exact(total.value,c,w.null,timezone)} — ${w.scope}: ${text(total.scope)}`,'metadata'));appendRawPrecision(parent,total.value,c,w);}
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
  draw(); details.append(body,controls); parent.append(details);return details;
}
function accessiblePoints(parent, chart, w) {
  const columns = array(chart.columns), points = array(chart.points), rich = chart.version === 2;
  // The wide rows, including omitted coordinates, are the exact retained result.
  // Normalized observations are separate: an absent observation has no source row.
  const retainedValues=pagedValues(parent,columns,rich ? array(chart.rows) : points.map(p => columns.map(c => pointCell(chart,p,c))),w,w.values,array(chart.totals),rich ? array(chart.row_indices) : [],'retained-values');
  if(chart.kind==='kpi'&&chart.state==='ready'){const value=chart.kpi_result?.value||points[0]?.value;retainedValues.append(element('p',`${w===words.es?'Valor guardado sin redondear':'Unrounded retained value'}: ${cellValue(value,w.null)}`,'metadata raw-retained-value'));}
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
  if(groups.size===1){const values=points.filter(p=>coordinate(p.value)!==null);if(values.length>1)for(const [i,p]of [values[0],values.at(-1)].entries()){const label=exact(p.value,array(c.columns).find(col=>col.id===p.measure)||columnFor(c,'value'),w.null);if(label.length<=32){const t=svg('text',{x:x(p),y:Math.max(24,y(coordinate(p.value))-18),'text-anchor':i?'end':'start',class:'chart-value-label'});t.textContent=label;s.append(t);}}}
  categories.forEach((label,i) => { if (i % Math.max(1,Math.ceil(categories.length/10)) === 0) { const t = svg('text',{x:65+i*680/Math.max(1,categories.length-1),y:385,class:'chart-axis-label','text-anchor':i===0?'start':i===categories.length-1?'end':'middle'}); t.textContent = shorten(label,22); t.append(svg('title',{},label)); s.append(t); } });
  const axis=element('div',undefined,'chart-axis');axis.setAttribute('aria-label',columnLabel(columnFor(c,'category'))||w.category);const step=Math.max(1,Math.ceil((categories.length-1)/4));categories.forEach((label,i)=>{if(i===0||i===categories.length-1||i%step===0)axis.append(element('span',label));});parent.append(axis);
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

export function renderChart(parent, chart, language='en', timezone='UTC', contextTitle='') {
  boundedJSON(chart); const w=words[language==='es'?'es':'en'];
  if(!KINDS.includes(chart.kind)||array(chart.points).length>MAX_POINTS||array(chart.columns).length>256)throw fail('invalid_request');
  validateRetainedChart(chart);
  chart={...chart,columns:array(chart.columns).map(column=>({...column,display_timezone:timezone}))};
  if(chart.mapping?.options?.title&&chart.mapping.options.title!==contextTitle)parent.append(element('h2',chart.mapping.options.title));
  const ready = chart.state === 'ready';
  if(!ready)parent.append(element('p',`${w.empty} ${text(chart.state)}`,'notice'));
  if(chart.kind==='table'){renderTable(parent,array(chart.columns),array(chart.rows),w,w.values,array(chart.totals),[],timezone);return;}
  if(ready && chart.kind==='kpi'){
    const valueColumn=columnFor(chart,'value'),targetColumn=columnFor(chart,'target'),percentColumn={type:'decimal',format:{percent:'whole'}},k=chart.kpi_result;
    const cell=k?.value||chart.points[0]?.value,formatted=exact(cell,valueColumn,w.null),format=valueColumn?.format||{},number=exact(cell,{...valueColumn,format:{...format,currency:'',currency_symbol:'',unit:''}},w.null),units=cell?.null?'':[text(format.currency_symbol)||text(format.currency),text(format.unit)].filter(Boolean).join(' '),display=element('p',undefined,'kpi'),value=element('span',number,'kpi-value');display.setAttribute('aria-label',formatted);display.style.setProperty('--kpi-width',String(Math.max(8,Array.from(number).length*.66)));value.setAttribute('title',cellValue(cell,w.null));display.append(value);if(units)display.append(element('span',' '+units,'kpi-unit'));parent.append(display);
    for(const [label,v,column] of [['Comparison',k?.comparison,valueColumn],['Delta',k?.delta,valueColumn],['Percent delta',k?.percent_delta,percentColumn],['Target',k?.target,targetColumn],['Target difference',k?.target_difference,valueColumn]])if(v)parent.append(element('p',`${label}: ${exact(v,column,w.null)}`,'metadata'));
    if(k?.threshold_state)parent.append(element('p',`${text(k.threshold_label)||k.threshold_state} · ${k.threshold_state}`,'badge'));
    if(array(k?.sparkline).length){const values=element('details',undefined,'kpi-trend-values'),line=element('p',k.sparkline.map(v=>exact(v,valueColumn,w.null,timezone)).join(' → '),'metadata');line.setAttribute('aria-label','Sparkline exact values');values.append(element('summary',language==='es'?'Valores de tendencia':'Trend values'),line);parent.append(values);}
  }
  else if(ready && ['bar','column','grouped_bar','stacked_bar','stacked_column'].includes(chart.kind))renderScales(parent,chart,w,renderBars);
  else if(ready && ['line','area'].includes(chart.kind))renderScales(parent,chart,w,renderLines);
  else if(ready && ['pie','donut'].includes(chart.kind))renderPie(parent,chart,w);
  else if(ready && chart.kind==='scatter')renderScatter(parent,chart,w);
  else if(ready && chart.kind==='heatmap')renderHeatmap(parent,chart,w);
  else if(ready && chart.kind==='treemap')(chart.version === 2 ? renderHierarchy : renderTreemap)(parent,chart,w);
  const diagnostics=element('details',undefined,'rendering-details');diagnostics.append(element('summary',language==='es'?'Detalles del gráfico':'Chart details'));if(chart.kind!=='kpi')diagnostics.append(element('p',w.geometry,'metadata'));
  if (chart.version === 2) {
    const t = chart.transformation;
    if(t.missing_points||t.gap_points||chart.omitted_rows)parent.append(element('p',`${w.missingValues}: ${t.missing_points} · ${w.gaps}: ${t.gap_points} · ${w.omittedRows}: ${chart.omitted_rows}`,'notice output-warning'));
    diagnostics.append(element('p',`${w.missingValues}: ${t.missing_points} · ${w.gaps}: ${t.gap_points} · ${w.omittedRows}: ${chart.omitted_rows} / ${chart.input_rows} · ${w.zeroSize}: ${t.zero_size_points} · ${w.scope}: ${t.scope}`,'transformation'),element('p',`${t.method} · ${t.null_policy} · ${t.duplicate_policy}`,'metadata'),element('p',w.resolution,'metadata'));
    if (['line','area'].includes(chart.kind)) diagnostics.append(element('p',`${columnFor(chart,'category')?.name} · ${w.grain}: ${columnFor(chart,'category')?.grain || 'unspecified'} / ${chart.mapping.order[0]?.direction}`,'metadata'));
  }
  for(const warning of array(chart.warnings))if(warning!=='geometry_approximate_labels_exact')parent.append(element('p',text(warning),'notice output-warning'));
  accessiblePoints(parent,chart,w);if(chart.kind!=='kpi'||chart.version===2)parent.append(diagnostics);
}

// Display order and accepted execution order are different contracts. A new,
// explicitly requested filter run retains the latter and the accepted caps.
// Reviewed completeness is independent of transport truncation and display labels.
export function amountDisclosureLines(disclosures, locale='en') {return amountDisclosureGroups(disclosures,locale).flat();}
function amountDisclosureGroups(disclosures, locale='en') {
  const es=locale.toLowerCase().startsWith('es'), groups=[], seen=new Set(), input=array(disclosures);
  if(input.length>128)throw fail('unavailable');
  for(const d of input){
    const r=d?.result??{}, key=[text(d?.declaration),text(r.metric),text(r.unknown_count_metric)].join('\u0000');
    const validOrigin=(d?.evidence==='reviewed_definition'&&r.policy==='reviewed-amount-completeness-v1')||(d?.evidence==='analytical_receipt'&&r.policy==='proved-known-amount-result-v1');
    if(!validOrigin||r.scope!=='returned_query_rows'||!['returned_query_rows','visible_source_rows'].includes(d?.rows_scope)||!['amount','unknown_count'].includes(d?.role)||(d?.role==='unknown_count'&&d?.unit!=='count'))throw fail('unavailable');
    if(seen.has(key))continue;seen.add(key);const lines=[];groups.push(lines);
    const label=shorten(text(d?.label)|| (es?'Importe conocido':'Known amount'),256);
    const status=!d?.truncation&&['complete','incomplete'].includes(r.status)?r.status:'unknown';
    const translated=es?({complete:'completo',incomplete:'incompleto',unknown:'desconocido'})[status]:status;
    const evidence=d?.evidence==='reviewed_definition'?(es?'definición revisada':'reviewed definition'):d?.evidence==='analytical_receipt'?(es?'evidencia analítica':'analytical receipt'):(es?'desconocida':'unknown');
    lines.push(label+': '+translated,(es?'Evidencia: ':'Evidence: ')+evidence,es?'Alcance: filas devueltas por la consulta':'Scope: returned query rows');
    const rows=array(r.rows);let known=rows.length>0||(d?.query_outcome==='empty'&&status==='complete'), total=0n;
    if(rows.length>100000)throw fail('unavailable');
    for(const row of rows){const value=text(row?.unknown_count);if(row?.status==='unknown'||value.length>4096||!/^(0|[1-9][0-9]*)$/.test(value)){known=false;break;}total+=BigInt(value);}
    const scope=d?.rows_scope==='visible_source_rows'?(es?'filas mostradas':'displayed rows'):(es?'filas devueltas':'returned rows');
    const count=known&&!d?.truncation?total.toString():(es?'desconocido':'unknown');
    lines.push((es?'Cantidad de importes desconocidos':'Unknown amount count')+' ('+scope+'): '+count);
    if(d?.truncation)lines.push(es?'Resultado truncado; la completitud del importe es desconocida':'Result truncated; amount completeness is unknown');
    if(d?.role==='unknown_count')lines.push(es?'Unidad de la métrica mostrada: cantidad':'Displayed metric unit: count');
  }
  return groups;
}

function retainedRunOutputs(v) {
  const choices=array(v.outputs),selected=v.accepted_selection?.selected;
  if(selected!==undefined){
    if(!Array.isArray(selected)||!selected.length||selected.length>64||new Set(selected).size!==selected.length||selected.some(value=>!id(value)||!choices.some(o=>o.id===value&&o.enabled!==false&&o.selected!==false)))throw fail('invalid_request');
    return selected.slice();
  }
  return choices.filter(o=>o.enabled!==false&&o.selected!==false).map(o=>o.id);
}
// Pure retained presentation shared by the viewer and the report canvas.
// The caller owns current authorization, selection, expiry and read lifetimes.
export function validateRetainedView(v) {
  boundedJSON(v);
  if(v.version!==VERSION||!id(v.summary?.run)||!['block','report','dashboard'].includes(v.summary.kind)||!v.selection||array(v.outputs).length>64||array(v.pages).length>100||array(v.filters).length>100)throw fail('invalid_request');
  try{if(!text(v.timezone))throw fail('invalid_request');new Intl.DateTimeFormat('en-US',{timeZone:v.timezone}).format(0);}catch{throw fail('invalid_request');}
  if(v.selection.run!==v.summary.run||v.selection.kind!==v.summary.kind||v.summary.target?.kind!==v.summary.kind||!id(v.summary.target?.id)||!integer(v.summary.target?.revision,1,256))throw fail('invalid_request');
  if(!integer(v.page_bounds?.offset,0,100000)||!integer(v.page_bounds?.limit,1,1000)||!integer(v.page_bounds?.total,0,100000))throw fail('invalid_request');
  const expiry=Date.parse(v.summary.expires_at);if(!Number.isFinite(expiry))throw fail('invalid_request');
  return expiry;
}

export function renderRetainedOutput(content,v,{locale='en',onPage=null,contextTitle=''}={}) {
  validateRetainedView(v);
  const language=locale.toLowerCase().startsWith('es')?'es':'en',w=words[language];
  if(v.text){content.append(element('p',text(v.text.content??v.text.text),'narrative'));return;}
  if(v.output?.state!=='succeeded'){content.append(element('p',`${text(v.output?.state)||text(v.summary.state)} ${text(v.output?.code)}`,'notice'));return;}
  if(v.output.table){
    const t=v.output.table;
    renderTable(content,array(t.columns),array(t.rows),w,w.table,array(t.totals),array(t.row_indices),v.timezone);
    if(t.completeness?.status){const line=element('p',`Result completeness: ${text(t.completeness.status)}${t.completeness.reason?' · '+text(t.completeness.reason):''}`,t.completeness.status==='complete_result'?'metadata':'notice output-warning');if(t.completeness.status==='complete_result'){const details=element('details',undefined,'result-details');details.append(element('summary',language==='es'?'Resultado completo':'Complete returned result'),line);content.append(details);}else content.append(line);}
    for(const warning of array(t.warnings))content.append(element('p',text(warning),'notice output-warning'));
    const b=v.page_bounds,pager=element('div',undefined,'pager');
    pager.append(element('span',`${b.offset+Math.min(1,array(t.rows).length)}–${b.offset+array(t.rows).length} / ${b.total}`));
    const prev=button(w.previous,()=>onPage?.(Math.max(0,b.offset-b.limit)));prev.disabled=!onPage||b.offset===0;
    const next=button(w.next,()=>onPage?.(b.next));next.disabled=!onPage||!integer(b.next,b.offset+1,b.total);
    pager.append(prev,next);content.append(pager);
  }else if(v.output.chart)renderChart(content,v.output.chart,language,v.timezone,contextTitle);
  else if(v.output.narrative){
    content.append(element('p',text(v.output.narrative.text),'narrative'));
    for(const caveat of array(v.output.narrative.caveats))content.append(element('p',text(caveat),'notice'));
    content.append(element('p',`${text(v.output.narrative.model_version)} · ${text(v.output.narrative.prompt_version)} · ${text(v.output.narrative.locale)}`,'metadata'));
  }
  for(const lines of amountDisclosureGroups(v.output.amount_completeness,language)){const panel=element('div',undefined,'amount-disclosure'),evidence=element('details',undefined,'amount-evidence');evidence.append(element('summary',language==='es'?'Evidencia del importe':'Amount evidence'));lines.forEach((line,index)=>{const secondary=index===1||index===2;(secondary?evidence:panel).append(element('p',line,secondary?'metadata':'amount-warning'));});panel.append(evidence);content.append(panel);}
}


export const viewerInternals=Object.freeze({MAX_MESSAGE, MAX_DATA, TOOLS, ERRORS, words, fail, text, array, id, integer, element, button, retainedRunOutputs});
