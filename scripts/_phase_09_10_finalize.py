from pathlib import Path
import json

p=Path('docs/plans/phase-registry.json');d=json.loads(p.read_text());assert d['phases']['10']['status']=='in_progress';d['phases']['10']['status']='shipped';p.write_text(json.dumps(d,indent=2)+'\n')
p=Path('docs/plans/phase-10-exec-read.md');s=p.read_text().replace('Status: in_progress.','Status: shipped.',1);s=s.replace('Runtime implementation and executable acceptance are supplied on the implementation branch; final source verification remains required.','Runtime implementation and all six executable acceptance criteria are supplied here; final exact-source CI is required before PR readiness.');p.write_text(s)
for name in ['RFC-001-Chartworks.md','docs/plans/README.md','README.md']:
 p=Path(name);s=p.read_text()
 s=s.replace('Phases 01–09 are merged; phase 10 now has its runtime implementation under final verification. Twenty-four later workstreams remain planned.','Phases 01–10 have runtime implementations; twenty-four later workstreams remain planned. Exact-source CI, not the status alone, establishes readiness.')
 s=s.replace('Phase 10 is implemented under final verification; twenty-four later workstreams remain planned.','Phase 10 supplies bounded validated read execution; twenty-four later workstreams remain planned.')
 s=s.replace('Phases01–09 are merged, phase10 is implemented under final verification, and the remaining twenty-four are planned.','Phases01–10 are implemented, and the remaining twenty-four are planned.')
 p.write_text(s)
p=Path('.github/workflows/ci.yml');s=p.read_text()
s=s.replace('feat/phase-07-08-vindex-sources]','feat/phase-07-08-vindex-sources, feat/phase-09-10-read-execution]',1)
s=s.replace("-name '_phase_07_08*'", "-name '_phase_07_08*' -o -name '_phase_09_10*'",1)
s=s.replace('All fifty-eight implemented phase criteria','All sixty-four implemented phase criteria',1)
s=s.replace('          python3 scripts/run_phase_acceptance.py --phase 08','          python3 scripts/run_phase_acceptance.py --phase 08\n          python3 scripts/run_phase_acceptance.py --phase 10',1)
needle='      - name: Working tree must remain unchanged'
s=s.replace(needle,'''      - name: Read-only workflow and source preparation hygiene
        run: |
          test ! -f .github/workflows/phase-09-10-development.yml
          test ! -f .github/workflows/read-snapshot.yml
          test ! -f .github/workflows/read-execution-workspace.yml
'''+needle,1)
p.write_text(s)
p=Path('docs/reviews/phase-09-10-adversarial.md');s=p.read_text();s+='''
## Finalization discipline

All new runtime assertions are retained, including actual PID reuse, credential
context substitution, late-cancellation audit truth and the reference configuration
through the closed decoder with required Pengui verifier settings. Migration 006
is reflected in the cumulative schema-version assertion. No threshold, criterion,
fuzz seed or race instrumentation has been removed to resolve a failure.

Permanent CI now requires all 64 implemented criteria, including phase10, and
rejects source preparation scripts and temporary development/workspace workflows.
Those files are removed before the final verification commit. The final PR records
exact tested source, completed runs and any remaining qualified boundaries.
''';p.write_text(s)
