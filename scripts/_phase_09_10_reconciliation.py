from pathlib import Path
p=Path('internal/store/postgres/read_executions.go');s=p.read_text();old="deadline + interval '3 seconds'<clock_timestamp()";assert old in s;s=s.replace(old,"deadline + $12::bigint * interval '1 microsecond'<clock_timestamp()",1)
old='readexec.Hash(a.Manifest), reconcile)';assert old in s;s=s.replace(old,'readexec.Hash(a.Manifest), reconcile,a.Manifest.Limits.CancelGrace.Microseconds())',1);p.write_text(s)
p=Path('internal/sources/read.go');s=p.read_text();a=s.index('func (s *Service) executeNative(');b=s.index('func remainingReadTime(',a);part=s[a:b]
part=part.replace('\tout.RemoteState = "not_issued"','\tout.RemoteState = "not_issued"\n var remote readexec.RemoteQuery',1)
part=part.replace('remote := readexec.RemoteQuery{Tag: "cw-read:" + id}','remote = readexec.RemoteQuery{Tag: "cw-read:" + id}',1)
old='''		if rollback != nil {
			_ = conn.Conn().Close(cleanup)
		}'''
new='''		if rollback != nil {
 _ = conn.Conn().Close(cleanup)
 conn.Release()
 // Closing a socket alone does not prove remote termination. Independently
 // observe the exact tagged backend identity under the already accepted
 // operation's bounded cleanup allowance; never signal a reusable PID.
 if remote.Valid() && out.RemoteState=="unknown" {
  var active bool
  observeErr:=pool.QueryRow(cleanup,`SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE pid=$1 AND backend_start=$2 AND application_name=$3 AND usename=current_user AND datname=current_database())`,remote.PID,remote.Started,remote.Tag).Scan(&active)
  if observeErr==nil && !active { out.RemoteState="stopped";if errors.Is(ctx.Err(),context.DeadlineExceeded) { err=readexec.ErrTimeout } else if errors.Is(ctx.Err(),context.Canceled) { err=readexec.ErrCancelled } }
 }
}'''
assert old in part;part=part.replace(old,new,1);s=s[:a]+part+s[b:];p.write_text(s)
