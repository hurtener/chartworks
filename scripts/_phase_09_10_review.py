from pathlib import Path
import json

# Audit must describe the actual database-chosen status, including a late cancel.
p=Path('internal/store/postgres/read_executions.go');s=p.read_text()
old='return auditJob(ctx, tx, s, "read."+a.Status, a.ID)'
assert old in s
s=s.replace(old,'''var committed string
 if err=tx.QueryRow(ctx,`SELECT status FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3`,s.Tenant(),s.Actor(),a.ID).Scan(&committed);err!=nil { return err }
 return auditJob(ctx,tx,s,"read."+committed,a.ID)''',1);p.write_text(s)

# Each malformed-body test starts from a valid request, not an incidental null array.
p=Path('test/acceptance/read_api_test.go');s=p.read_text();old='encoded, _ := json.Marshal(input)';assert old in s
s=s.replace(old,'input.Parameters=[]cw.ReadParameter{}\n encoded, _ := json.Marshal(input)',1);p.write_text(s)
p=Path('test/acceptance/read_adversarial_test.go');s=p.read_text()
start=s.index('func TestReadLateCancellationCannotPublishRows(');end=s.index('\ntype lostReadReply',start)
part=s[start:end];needle='\n}\n';at=part.rfind(needle);assert at>=0
part=part[:at]+'''
 meta:=support.Raw(t,f.dsn);var success,cancelled int
 if err:=meta.QueryRow(context.Background(),`SELECT count(*) FILTER(WHERE action='read.succeeded'),count(*) FILTER(WHERE action='read.cancelled') FROM chartworks.audit_events WHERE resource_id=$1`,r.Attempt.ID).Scan(&success,&cancelled);err!=nil || success!=0 || cancelled!=1 { t.Fatal("audit contradicted committed cancellation",err,success,cancelled) }
'''+part[at:];s=s[:start]+part+s[end:];p.write_text(s)

# Preserve original criterion text; update implementation status and consumer details.
p=Path('docs/plans/phase-registry.json');data=json.loads(p.read_text());assert data['phases']['10']['status']=='planned';data['phases']['10']['status']='in_progress';p.write_text(json.dumps(data,indent=2)+'\n')
p=Path('docs/plans/phase-10-exec-read.md');s=p.read_text().replace('Status: planned.','Status: in_progress.',1).replace('No runtime completion is claimed.','Runtime implementation and executable acceptance are supplied on the implementation branch; final source verification remains required.',1)
s+='''
## Implemented contract and review

D-065 and [read-execution.md](../contracts/read-execution.md) specify the actual
PostgreSQL cursor, exact type encodings, server/client/JWT deadlines, separate
source-revision fence, bounded response/optimizer admission and content-free
attempt journal. Migration 006 accompanies its consumers; applied 001–005 remain
unchanged. The [adversarial review](../reviews/phase-09-10-adversarial.md) records
real failures and fixes. HTTP and Go SDK operations use the same plan-only core.

`exec.bytes_default=4194304`, `bytes_ceiling=16777216`, `cancel_grace=2s`,
`planner_cost_ceiling=10000000`, `execution_concurrency=2` and
`max_read_attempts=3` supplement the row/time settings above. Optimizer cost is an
estimate; PostgreSQL does not implement a hard scan-byte ceiling and reports actual
scan bytes as unknown. There is no misleading scan_bytes setting. Attempt records
have a bounded 24-hour replay window and fixed admission capacity; uncertain
records require reconciliation rather than automatic erasure. Values are not retained.

The common core's caps apply to later scheduled/frozen consumers without a mode
bypass. Actual scheduled reporting targets remain phase 30 and are not fabricated
to close phase 10. Phase 09 was already merged and all six of its criteria remain.
''';p.write_text(s)
for name in ['docs/plans/phase-09-sql-validate-core.md','docs/plans/phase-08-sources-core.md','docs/contracts/vector-sources-validation.md']:
 p=Path(name);s=p.read_text();s+='''
## Phase-10 read consumer

The [D-065 read execution contract](../contracts/read-execution.md) extends the
existing validated plan and source adapter without changing their authority or
qualification boundaries. Its [adversarial review](../reviews/phase-09-10-adversarial.md)
records the actual cursor, attempt, cancellation and exact-result regressions.
Earlier statements assigning execution to phase 10 are now realized by that
consumer; other-engine and retained reporting deliverables remain separately owned.
''';p.write_text(s)

# Update active status text only; all 224 source-feature criteria stay mapped.
for name in ['RFC-001-Chartworks.md','docs/plans/README.md','README.md']:
 p=Path(name);s=p.read_text()
 s=s.replace('Phases 01–09 now have runtime implementations; 25 later workstreams remain planned.','Phases 01–09 are merged; phase 10 now has its runtime implementation under final verification. Twenty-four later workstreams remain planned.')
 s=s.replace('Twenty-five later workstreams remain planned.','Phase 10 is implemented under final verification; twenty-four later workstreams remain planned.')
 s=s.replace('Phases01–09 are implemented; the remaining twenty-five are planned.','Phases01–09 are merged, phase10 is implemented under final verification, and the remaining twenty-four are planned.')
 s+='\nRead execution now extends the merged phase-09 validator on the existing source/store seams. See '+('[D-065](docs/contracts/read-execution.md)' if '/' not in name else '[D-065](../contracts/read-execution.md)')+' for exact typed results, bounded attempts and cancellation/reconciliation. Final named acceptance and read-only CI establish readiness, not this status paragraph.\n'
 p.write_text(s)
for name in ['RFC-002-Governed-Reporting.md','00_CHARTWORKS_CONSUMER-REQUEST.md']:
 p=Path(name);s=p.read_text();s+='''
## Validated read execution consumer

[D-065](docs/contracts/read-execution.md) supplies the existing phase-09 validator's
bounded phase-10 execution consumer, exact transport, attempt evidence and
cancellation/reconciliation. Reporting and scheduler consumers must reuse it and
its ceilings. It introduces no new reporting target, retained result cache,
identity-policy owner or model dependency.
''';p.write_text(s)
p=Path('AGENTS.md');s=p.read_text();s+='''
## Phases 09/10 read execution

D-065 and docs/contracts/read-execution.md extend the existing opaque plan, source
adapter and metadata store. Keep the source-revision fence through native cleanup,
short ordinary metadata deadlines and reserved journal/control capacity. Every
consumer uses exact typed results and the same caps; optimizer estimates are not
scan-byte guarantees. Persist cancel intent and only signal the original owned
connection, never a reusable PID. Final receipt/audit commit must resolve late
cancellation before exposing values. Reconcile uncertain physical attempts before
explicit retries; never reconstruct lost values or silently rewrite empty SQL.
''';p.write_text(s);Path('CLAUDE.md').write_text(s)
p=Path('GETTING-STARTED.md');s=p.read_text();s+='''
## Validated read execution (phases 09/10)

Merge the non-secret [execution excerpt](examples/chartworks.execution.json) with
your source/verifier configuration; warehouse aliases remain opt-in. Install/check
forward migration 006 after unchanged 001–005. The metadata pool must have at least
`exec.execution_concurrency + 2` connections. Default HTTP write/client timeouts are
75 seconds; custom proxy/client budgets must leave validation and cleanup room.

Using a current Pengui bearer with source/context and all dataset query scopes,
POST `/v1/sources/sales/execute` with a synthetic registered `sales:v1` context:

```json
{"context":"sales:v1","sql":"SELECT id, amount FROM analytics.sales ORDER BY id","parameters":[],"execution":{"operation":"read-example-001","attempt":1,"preview":false,"rows":0,"bytes":0}}
```

HTTP 200 returns an accepted attempt receipt; check its status before using result
values. `empty` retains schema; `truncated` marks incomplete rows/bytes. Exact
integers/decimals are strings, booleans and null retain JSON types, and JSON columns
contain exact JSON text strings. The same invocation never automatically retries.

After a lost response, GET `/v1/read-operations/read-example-001` to recover its
attempt ID, then GET `/v1/read-executions/{id}`. POST `{}` to that path's `/cancel`
or `/reconcile` suffix. A cancellation request is not termination proof; unknown
remote state remains uncertain. No result values are retained or rerun by these
metadata calls. After a proven interrupted/failed attempt, an explicitly supplied
next attempt number may retry the same immutable operation; changed input conflicts.

Read the complete [read contract](docs/contracts/read-execution.md) before enabling
execution. Actual scan bytes remain unknown; row/response-byte caps are not scan
budgets. The 24-hour content-free receipt window is not phase-28 result retention.
''';p.write_text(s)
p=Path('CHANGELOG.md');s=p.read_text();s+='''
## Phase 10 read execution / phase 09 continuation

- One plan-only PostgreSQL cursor core with exact ordered results and honest empty/truncated/error states.
- Content-free explicit attempt journal, cancellation intent and observed reconciliation; no hidden retries or result cache.
- Source-revision fence through bounded execution/cleanup; API/SDK parity and real adversarial regressions.
- Forward migration 006; applied migrations 001–005 and existing phase-09 criteria preserved.
''';p.write_text(s)
p=Path('docs/decisions.md');s=p.read_text();s+='\nRead execution continuation: [bounded plan-only reads and attempt uncertainty](decisions/2026-09-06-read-execution.md).\n';p.write_text(s)
