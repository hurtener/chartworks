"""Recover the exact authored documentation patch, then apply explicit CI fixes.
This temporary editing aid is removed before the final PR.
"""
from pathlib import Path
import hashlib
import subprocess
import urllib.request

# Immutable, previously authored source, not downloaded third-party code. Verify
# its Git object identity before execution so the intended patch cannot drift.
url='https://raw.githubusercontent.com/hurtener/chartworks/bac5795eae3a3c63354386898bafac137d8f8e72/scripts/dev/phase-30-31-edit.py'
with urllib.request.urlopen(url,timeout=30) as response:
    data=response.read(100000)
assert hashlib.sha1(b'blob '+str(len(data)).encode()+b'\0'+data).hexdigest()=='2fc26c4e85d8ed5b04ebc0f640513032e16d7c97'
source=data.decode().replace('reporting-execution-v1.md','reviewed-engineering-and-frozen-runs.md').replace('ui://chartworks/report-viewer/v1.html','ui://chartworks/report-viewer/v1')
source=source.replace('All fourteen catalog kinds are rendered: area, bar, column, donut, grouped bar,\nheatmap, KPI, line, pie, scatter, stacked bar, stacked column, table and treemap.', 'All fourteen kinds in the versioned [chart catalog](chart-specifications-v1.md)\nare rendered and exercised by the real browser fixture matrix.')
exec(compile(source,'authored-reporting-docs.py','exec'),{'__name__':'__main__'})

def replace(path,old,new,count=1):
    p=Path(path);text=p.read_text()
    if new in text:return
    if text.count(old)!=count:raise RuntimeError(f'Unexpected anchor {path}: {old[:120]!r}')
    p.write_text(text.replace(old,new))

replace('internal/store/postgres/errors_test.go','"migrations/032_timezone_database.sql", "timezone_database_version"','"migrations/032_timezone_database.sql", "queue_timezone_version"')
# Install the actual component runtime in the ordinary all-package CI, not only
# in a branch-only proof. Node is needed for tests, never in the shipping binary.
p=Path('.github/workflows/ci.yml');text=p.read_text()
start=text.index('  build-test:');end=text.index('\n  lint:',start)
part=text[start:end]
anchor="      - uses: actions/setup-go@v5\n        with:\n          go-version: '1.26.4'"
assert part.count(anchor)==1
part=part.replace(anchor,anchor+"\n      - uses: actions/setup-node@v4\n        with:\n          node-version: '22.14.0'\n      - name: Real reporting component test runtime\n        run: |\n          node --version\n          google-chrome --version\n          node --check web/report-viewer/app.js")
part=part.replace("find cmd internal sdk test -name '*.go'","find cmd internal sdk test web -name '*.go'")
anchor='          python3 scripts/run_phase_acceptance.py --phase 29'
assert part.count(anchor)==1
part=part.replace(anchor,anchor+'\n          python3 scripts/run_phase_acceptance.py --phase 30\n          python3 scripts/run_phase_acceptance.py --phase 31')
anchor='          test ! -f .github/workflows/phase-20-apply.yml'
assert part.count(anchor)==1
part=part.replace(anchor,anchor+'''
          test ! -f .github/workflows/phase-30-31-edit.yml
          test ! -f .github/workflows/phase-30-31-workspace.yml
          test ! -f .github/workflows/phase-30-31-module-transfer.yml
          test ! -f .github/workflows/phase-30-31-local-runtime.yml
          test ! -f .github/workflows/phase-30-31-diagnostics.yml
          test ! -f scripts/dev/phase-30-31-edit.py''')
p.write_text(text[:start]+part+text[end:])
subprocess.run(['gofmt','-w','internal/store/postgres/errors_test.go'],check=True)
subprocess.run(['git','diff','--check'],check=True)
