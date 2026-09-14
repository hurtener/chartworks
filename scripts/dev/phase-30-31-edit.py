"""Temporary exact-source integration aid; removed before final review."""
from pathlib import Path
import json
import subprocess

# Reuse the exact reviewed edit payload, correcting its overly narrow source
# anchor. Fetch only this immutable public repository commit, without credentials.
revision = '776a916d6edf63fde10f39ed71675ed4ddebdce5'
subprocess.run(['git', 'fetch', '--no-tags', '--depth=1', 'origin', revision], check=True)
payload = subprocess.check_output(['git', 'show', revision + ':scripts/dev/phase-30-31-edit.py'], text=True)
old = "replace(p, 'case errors.Is(err, gateway.ErrBudget):', 'case errors.Is(err, gateway.ErrBudget), errors.Is(err, jobs.ErrReportingBudget):')"
new = "replace(p, 'errors.Is(err, gateway.ErrBudget):', 'errors.Is(err, gateway.ErrBudget) || errors.Is(err, jobs.ErrReportingBudget):')"
if payload.count(old) != 1:
    raise RuntimeError('Exact reviewed budget edit anchor changed')
exec(compile(payload.replace(old, new), 'reviewed-phase-30-31-integration.py', 'exec'))

# In-progress work is not allowed to evade the strict acceptance runner by
# retaining a planned label. Shipping is a separate, evidence-backed change.
p = Path('docs/plans/phase-registry.json')
registry = json.loads(p.read_text())
for phase in ['30', '31']:
    registry['phases'][phase]['status'] = 'in_progress'
    plan = next(Path('docs/plans').glob('phase-' + phase + '-*.md'))
    text = plan.read_text()
    text = text.replace('Status: planned', 'Status: in_progress').replace('**Status:** planned', '**Status:** in_progress').replace('| Status | planned |', '| Status | in_progress |')
    plan.write_text(text)
p.write_text(json.dumps(registry, indent=2) + '\n')
subprocess.run(['git', 'diff', '--check'], check=True)
