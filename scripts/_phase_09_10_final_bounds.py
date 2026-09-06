from pathlib import Path
p=Path('internal/exec/execution.go');s=p.read_text()
for old,new in [('func (l Limits) Valid() bool {','// Valid checks every execution ceiling independently of caller authority.\nfunc (l Limits) Valid() bool {'),('func (q RemoteQuery) Valid() bool {','// Valid checks a canonical native backend identity without exposing its secret.\nfunc (q RemoteQuery) Valid() bool {'),('func (m Manifest) Valid() bool {','// Valid checks bounded retained coordinates and exact validation metadata.\nfunc (m Manifest) Valid() bool {')]:
 assert old in s;s=s.replace(old,new,1)
s+='''
// ValidateOptions rejects unsupported invocation bounds before native validation I/O.
// It is admission only and never authorizes or constructs an executable plan.
func (x *Executor) ValidateOptions(o Options) error {
 _,err:=x.limits(o);return err
}
''';p.write_text(s)
p=Path('internal/sourceapi/execution.go');s=p.read_text();old='''if err = body(w, r, &input); err == nil {
				var p readexec.Plan''';assert old in s;s=s.replace(old,'''if err = body(w,r,&input);err==nil { err=executor.ValidateOptions(input.Execution) }
 if err==nil {
 var p readexec.Plan''',1);p.write_text(s)
p=Path('internal/sources/read.go');s=p.read_text()
s=s.replace('"context"','"bytes"\n "context"',1)
s=s.replace('defer func() {\n\t\tcleanup,','defer func() {\n queryErr:=err\n\t\tcleanup,',1)
old='''out.RemoteState = "stopped"
					if errors.Is(ctx.Err(), context.DeadlineExceeded)''';assert old in s
s=s.replace(old,'''out.RemoteState = "stopped"
 if queryErr!=nil { err=readFailure(ctx,queryErr) }
 if errors.Is(ctx.Err(), context.DeadlineExceeded)''',1)
old='errors.As(err, &pg) && pg.Code == "57014"';assert old in s;s=s.replace(old,'errors.As(err, &pg) && (pg.Code=="57014" || pg.Code=="25P03" || pg.Code=="25P04")',1)
a=s.index('func moneyDecimal(');b=s.index('// ControlRead',a)
s=s[:a]+'''func moneyDecimal(raw []byte) ([]byte,error) {
 if len(raw)!=8 { return nil,readexec.ErrType }
 var n int64
 if err:=binary.Read(bytes.NewReader(raw),binary.BigEndian,&n);err!=nil { return nil,readexec.ErrType }
 var magnitude uint64
 sign:=""
 if n<0 { magnitude=uint64(-(n+1))+1;sign="-" } else { magnitude=uint64(n) }
 fraction:=strconv.FormatUint(magnitude%100,10);if len(fraction)==1 { fraction="0"+fraction }
 return []byte(sign+strconv.FormatUint(magnitude/100,10)+"."+fraction),nil
}

'''+s[b:];p.write_text(s)
p=Path('internal/sources/read_test.go');s=p.read_text();s=s.replace('"context"','"bytes"\n "context"',1)
old='''raw := make([]byte, 8)
		binary.BigEndian.PutUint64(raw, uint64(c.n))
		got, err := moneyDecimal(raw)''';assert old in s
s=s.replace(old,'''var raw bytes.Buffer
 if err:=binary.Write(&raw,binary.BigEndian,c.n);err!=nil { t.Fatal(err) }
 got, err := moneyDecimal(raw.Bytes())''',1)
s+='''
func TestReadNativeTransactionTimeoutCodes(t *testing.T) {
 for _,code:=range []string{"25P03","25P04"} {
  if err:=readFailure(context.Background(),&pgconn.PgError{Code:code,Message:"PRIVATE_ERROR_CANARY"});!errors.Is(err,readexec.ErrTimeout) { t.Fatal("transaction deadline lost its typed category",code,err) }
 }
}
''';p.write_text(s)
p=Path('test/acceptance/read_api_test.go');s=p.read_text();needle='// The execution endpoint has a larger explicit SDK bound';assert needle in s
s=s.replace(needle,'''before=f.lookups.Load()
 invalidBounds:=input;invalidBounds.Execution.Operation="oversized-http";invalidBounds.Execution.Rows=100001
 _,err=client.ExecuteRead(ctx,source.ID,invalidBounds)
 var rejected *cw.StatusError
 if !errors.As(err,&rejected) || rejected.Status!=413 || f.lookups.Load()!=before { t.Fatal("HTTP ceilings did not reject before native planning",err) }
 '''+needle,1);p.write_text(s)
p=Path('docs/reviews/phase-09-10-adversarial.md');s=p.read_text();needle='## Preserved authority and lifecycle checks';assert needle in s
s=s.replace(needle,'''## Additional final-review findings

- `TestReadReconciliationCannotSwitchCredentialContext` changes the actual
  credential/database without rotation. Reconciliation re-probes the current
  technical binding under the source-revision fence before observing an old
  query; a foreign database cannot falsely prove termination.
- `TestReadOldCancellationCannotSignalReusedBackend` proves actual PID reuse,
  cancels the old uncertain attempt and verifies the new tagged transaction is
  still active. Only its own owner can cancel it.
- Retired credential pools are bounded to one per alias and joined on shutdown;
  a short control request no longer synchronously waits for an old 60-second read.
- Late cancellation now also controls the committed audit action. The regression
  requires zero `read.succeeded` records and one `read.cancelled` record.
- Request ceilings are checked before HTTP-triggered native planning, and
  PostgreSQL-17 transaction/idle-transaction SQLSTATEs retain typed timeout errors.
- The whole-suite schema-version assertion now includes actual migration 006;
  no migration/acceptance check or coverage threshold was bypassed.

'''+needle,1);p.write_text(s)
