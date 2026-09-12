"""Temporary branch-only editing aid; remove before submitting the final PR."""
from pathlib import Path
import subprocess


def replace(path, old, new):
    file = Path(path)
    text = file.read_text()
    if new in text:
        return
    if text.count(old) != 1:
        raise RuntimeError(f"Expected one edit anchor in {path}: {old[:100]!r}")
    file.write_text(text.replace(old, new, 1))


replace('internal/reporting/compositions_model.go', 'type CompositionPageSummary struct {\n', '''type CompositionPageSummary struct {
	Locale string `json:"locale,omitempty"`
	Timezone string `json:"timezone,omitempty"`
''')
replace('internal/reporting/compositions_summary.go', 'Revision: saved.Revision, Title: saved.Title, Widgets:', 'Revision: saved.Revision, Title: saved.Title, Locale: saved.Locale, Timezone: saved.Timezone, Widgets:')
replace('internal/reporting/delivery.go', 'Revision: revision, Title: report, Widgets:', 'Revision: revision, Title: report, Locale: d.Locale, Timezone: d.Timezone, Widgets:')
replace('internal/reporting/delivery_view.go', '\t\tpageFound = true\n', '\t\tpageFound = true\n\t\tout.Locale, out.Timezone = page.Locale, page.Timezone\n')
for file in Path('test/acceptance').glob('phase31*.go'):
    file.write_text(file.read_text().replace('t testing.TB', 't *testing.T'))
replace('web/report-viewer/resource_test.go', r'`(?i)\bon\w+\s*=`', r'`(?i)<[^>]*\s(on\w+)\s*=`')
files = subprocess.check_output(['git','ls-files','*.go'],text=True).splitlines()
subprocess.run(['gofmt','-w',*files],check=True)
subprocess.run(['node','--check','web/report-viewer/app.js'],check=True)
subprocess.run(['node','--check','web/report-viewer/component.test.mjs'],check=True)
