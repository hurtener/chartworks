"""Temporary branch-only editing aid; remove before submitting the final PR."""
from pathlib import Path
import subprocess


def replace(path, old, new):
    file = Path(path)
    text = file.read_text()
    if new in text:
        return
    if text.count(old) != 1:
        raise RuntimeError(f"Expected exactly one edit anchor in {path}: {old[:100]!r}")
    file.write_text(text.replace(old, new, 1))


path = Path('internal/reporting/delivery_view.go')
s = path.read_text()
a, b = s.split('func (s *Delivery) viewComposition', 1)
b = b.replace('v.State != "succeeded"', 'v.State != "completed"').replace('payload.State != "succeeded"', 'payload.State != "completed"')
path.write_text(a + 'func (s *Delivery) viewComposition' + b)
replace('internal/reporting/delivery_view.go', 'out.Filters = description.Filters', '''out.Filters = []ViewerFilter{}
			for _, filter := range description.Filters {
				if out.Selection.Kind == "block" || filter.Page == out.Selection.Page { out.Filters = append(out.Filters, filter) }
			}''')

js = 'web/report-viewer/app.js'
replace(js, "const array = x => Array.isArray(x) ? x : [];", "const array = x => Array.isArray(x) ? x : [];\nconst shorten = (value, max) => Array.from(value).slice(0,max).join('');")
for old, new in [
    ("name.slice(0,32)", "shorten(name,32)"),
    ("label.slice(0,22)", "shorten(label,22)"),
    ("title.slice(0,40)", "shorten(title,40)"),
    ("cellValue(p.category,w.null).slice(0,Math.floor(width/9))", "shorten(cellValue(p.category,w.null),Math.floor(width/9))"),
]:
    replace(js, old, new)
file = Path(js)
s = file.read_text().replace('label.slice(0,20)', 'shorten(label,20)')
file.write_text(s)
replace(js, "this.locale='en';this.value=null;this.generation=0;", "this.locale='en';this.value=null;this.generation=0;this.mutationPending=false;")
replace(js, "['succeeded','partial','expired'].includes(v.summary.state)", "['succeeded','completed','partial','expired'].includes(v.summary.state)")
replace(js, "input.setAttribute('aria-label',text(f.label)||p.name);label.append(input);", "input.setAttribute('aria-label',text(f.label)||p.name);const useDefault=element('input');useDefault.type='checkbox';useDefault.checked=true;input.disabled=true;useDefault.addEventListener('change',()=>{input.disabled=useDefault.checked;});const defaultLabel=element('span',undefined,'check');defaultLabel.append(useDefault,element('span',w.default));label.append(defaultLabel,input);")
replace(js, "controls.push({f,input});", "controls.push({f,input,useDefault});")
replace(js, "run.disabled=true;const oldSelection={...v.selection};", "if(this.mutationPending||this.closed)return;this.mutationPending=true;run.disabled=true;const oldSelection={...v.selection},startingGeneration=this.generation;")
replace(js, "for(const {f,input}of controls){if(input.value==='')continue;", "for(const {f,input,useDefault}of controls){if(useDefault.checked)continue;")
replace(js, "if(!id(description.resource?.target?.id)", "if(this.closed||startingGeneration!==this.generation)return;\n        if(!id(description.resource?.target?.id)")
replace(js, "}catch(e){this.error(e,true);}\n    },true);fieldset.append(run);", "}catch(e){if(!this.closed)this.error(e,true);}finally{this.mutationPending=false;if(this.value&&!this.closed)this.draw();}\n    },true);run.disabled=this.mutationPending;fieldset.append(run);")
replace('web/report-viewer/component.test.mjs', "d.open=true;const input=d.querySelector('input[type=text]');input.value='2';", "d.open=true;const useDefault=d.querySelector('input[type=checkbox]');useDefault.checked=false;useDefault.dispatchEvent(new Event('change'));const input=d.querySelector('input[type=text]');input.value='2';")
replace('web/report-viewer/component.test.mjs', "console.error(e.stack||e.message);console.error(errors.slice(-6).join('').slice(-4000));", "console.error(e.stack||e.message);try{console.error('COMPONENT_DOM='+String(await evaluate(`${body}?.textContent`)).slice(0,6000));}catch{}console.error(errors.slice(-6).join('').slice(-4000));")

# Show only relevant tracked source anchors for the next reviewed edit.
for directory, needle in [('internal/reporting','CompositionPageSummary{'), ('internal/store/postgres','CompositionPageSummary{')]:
    for file in Path(directory).glob('*.go'):
        if file.name.endswith('_test.go'):
            continue
        lines = file.read_text().splitlines()
        for index, line in enumerate(lines):
            if needle in line:
                print('SOURCE_ANCHOR', file, index+1)
                print('\n'.join(lines[max(0,index-3):index+14]))
files = subprocess.check_output(['git','ls-files','*.go'],text=True).splitlines()
subprocess.run(['gofmt','-w',*files],check=True)
subprocess.run(['node','--check',js],check=True)
subprocess.run(['node','--check','web/report-viewer/component.test.mjs'],check=True)
