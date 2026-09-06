from pathlib import Path

def replace(path, old, new):
    p=Path(path); s=p.read_text()
    if new in s: return
    assert old in s, (path, old[:100])
    p.write_text(s.replace(old,new,1))

replace('internal/config/sources.go', 'type ReadValidation struct {', '''type ReadValidation struct {
 RowsDefault int `json:"rows_default"`
 RowsCeiling int `json:"rows_ceiling"`
 PreviewRows int `json:"preview_rows"`
 BytesDefault int `json:"bytes_default"`
 BytesCeiling int `json:"bytes_ceiling"`
 Timeout Duration `json:"timeout"`
 CancelGrace Duration `json:"cancel_grace"`
 PlannerCostCeiling float64 `json:"planner_cost_ceiling"`
 ExecutionConcurrency int `json:"execution_concurrency"`
 MaxReadAttempts int `json:"max_read_attempts"`''')
replace('internal/config/sources.go','return ReadValidation{MaxSQLBytes:', 'return ReadValidation{RowsDefault:10000,RowsCeiling:100000,PreviewRows:200,BytesDefault:4<<20,BytesCeiling:16<<20,Timeout:Duration(time.Minute),CancelGrace:Duration(2*time.Second),PlannerCostCeiling:1e7,ExecutionConcurrency:2,MaxReadAttempts:3,MaxSQLBytes:')
replace('internal/config/sources.go', 'func ValidateReadValidation(v ReadValidation) error {', '''func ValidateReadValidation(v ReadValidation) error {
 if v.RowsDefault<1 || v.RowsDefault>v.RowsCeiling || v.RowsCeiling>100000 || v.PreviewRows<1 || v.PreviewRows>v.RowsDefault || v.BytesDefault<1024 || v.BytesDefault>v.BytesCeiling || v.BytesCeiling>16<<20 || v.Timeout<Duration(time.Millisecond) || v.Timeout>Duration(time.Minute) || v.CancelGrace<Duration(time.Millisecond) || v.CancelGrace>Duration(3*time.Second) || !(v.PlannerCostCeiling>0 && v.PlannerCostCeiling<=1e12) || v.ExecutionConcurrency<1 || v.ExecutionConcurrency>16 || v.MaxReadAttempts<1 || v.MaxReadAttempts>3 { return invalid("exec", "execution bounds exceeded") }''')
# Only the source-revision fence may use a separately bounded transaction timeout.
replace('internal/store/postgres/postgres.go','func (d *DB) transactionOptions(ctx context.Context, options pgx.TxOptions, fn func(context.Context, pgx.Tx) error) error {','''func (d *DB) transactionOptions(ctx context.Context, options pgx.TxOptions, fn func(context.Context, pgx.Tx) error) error {
 return d.transactionDuration(ctx,options,d.timeout,fn)
}
func (d *DB) transactionDuration(ctx context.Context, options pgx.TxOptions, timeout time.Duration, fn func(context.Context, pgx.Tx) error) error {''')
p=Path('internal/store/postgres/postgres.go');s=p.read_text();start=s.index('func (d *DB) transactionDuration(');end=s.index('func checkScope(',start);s=s[:start]+s[start:end].replace('d.timeout','timeout')+s[end:];p.write_text(s)
replace('internal/store/postgres/sources.go','"encoding/json"','"encoding/json"\n "time"')
p=Path('internal/store/postgres/sources.go');s=p.read_text();start=s.index('func (d *DB) WithSource(')
old='return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {'
new='''timeout:=d.timeout
 if until,ok:=ctx.Deadline();ok { timeout=time.Until(until);if timeout>65*time.Second { timeout=65*time.Second };if timeout<=0 { return context.DeadlineExceeded } }
 return d.transactionDuration(ctx,pgx.TxOptions{},timeout,func(ctx context.Context, tx pgx.Tx) error {'''
if new not in s[start:]:
    assert old in s[start:];s=s[:start]+s[start:].replace(old,new,1);p.write_text(s)
replace('internal/store/postgres/postgres.go','[]error{readexec.ErrUnsafe,','[]error{readexec.ErrType,readexec.ErrCancelled,readexec.ErrTimeout,readexec.ErrUncertain,readexec.ErrReplay,readexec.ErrUnsafe,')
replace('internal/sources/sources.go','[]error{context.Canceled,','[]error{readexec.ErrType,readexec.ErrCancelled,readexec.ErrTimeout,readexec.ErrUncertain,readexec.ErrReplay,context.Canceled,')
