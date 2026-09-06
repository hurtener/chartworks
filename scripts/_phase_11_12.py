from pathlib import Path
import re

# Reviewed integration edits, applied before committing and testing. This file
# and the temporary development workflow are removed before final exact-head CI.
def edit(path, fn):
    p = Path(path)
    p.write_text(fn(p.read_text()))

def once(text, old, new):
    if new in text:
        return text
    if text.count(old) != 1:
        raise SystemExit('Missing unique integration anchor: ' + old[:100])
    return text.replace(old, new)

def config_sources(t):
    if 'ManagedSchema ' not in t:
        t=t.replace('type SourceConnection struct {', 'type SourceConnection struct {\n\tManagedSchema string `json:"managed_schema,omitempty"`')
    t=once(t,'seen[key] || len(c.Relations) < 1 || len(c.Relations) > 32','seen[key] || c.ManagedSchema == "" && len(c.Relations) < 1 || len(c.Relations) > 32')
    t=once(t,'\t\tseen[key] = true\n','''\t\tif c.ManagedSchema != "" && (!sourceSQLName(c.ManagedSchema) || !strings.HasPrefix(c.ManagedSchema, "cw_") || len(c.ManagedSchema)>30 || c.WriteDSN=="" || len(c.Relations)!=0) { return invalid("sources.connections.managed_schema", "explicit isolated workspace required") }
\t\tseen[key] = true
''')
    return t
edit('internal/config/sources.go',config_sources)

def sources_service(t):
    t=t.replace('s.connection(e.Tenant(), old.Connection)','s.recordConnection(old)').replace('s.connection(e.Tenant(), record.Connection)','s.recordConnection(record)')
    anchor='connection, err := s.connection(e.Tenant(), r.Connection)\n\tif err != nil {\n\t\treturn out, err\n\t}'
    t=once(t,anchor,anchor+'\n\tif connection.ManagedSchema != "" { return out, store.ErrInvalid }')
    return t
edit('internal/sources/sources.go',sources_service)
edit('internal/sources/read.go',lambda t:t.replace('s.connection(e.Tenant(), record.Connection)','s.recordConnection(record)'))

def source_pool(t):
    if 'ParseApprovedDSN(dsn)' not in t:
        a=t.index('\tu, err := url.Parse(dsn)');b=t.index('\tkey := c.Tenant',a)
        t=t[:a]+'\t_, location, err := ParseApprovedDSN(dsn)\n\tif err != nil { return nil, "", err }\n'+t[b:]
        t=t.replace('u.Host + u.EscapedPath()','location').replace('\n\t"net"','').replace('\n\t"net/url"','')
    return t
edit('internal/sources/postgres.go',source_pool)

def source_store(t):
    if 'func putSourceTx(' not in t:
        start=t.index('func (d *DB) PutSource(');end=t.index('// ReadSource',start)
        section=t[start:end];marker='return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {'
        a=section.index(marker);tail=section.rindex('\n\t})')
        body=section[a+len(marker):tail]
        header=section[:a]
        replacement=header+'return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error { return putSourceTx(ctx,tx,s,expected,r,false) })\n}\n\n'
        replacement+='func putSourceTx(ctx context.Context, tx pgx.Tx, s store.Scope, expected int64, r sources.Record, managed bool) error {\n'
        replacement+='if expected==0 && !managed { var reserved bool; if err:=tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.uploads WHERE tenant_id=$1 AND source_id=$2)`,s.Tenant(),r.Source.ID).Scan(&reserved); err!=nil { return err }; if reserved { return store.ErrConflict } }\n'
        replacement+=body+'\n}\n\n'
        t=t[:start]+replacement+t[end:]
    t=t.replace('WHERE s.tenant_id=$1','WHERE NOT s.deleted AND s.tenant_id=$1')
    t=t.replace('AND current_revision=$3`','AND current_revision=$3 AND NOT deleted`')
    return t
edit('internal/store/postgres/sources.go',source_store)

def quota(t):
    if 'accounted_bytes bigint' not in t:
        t=t.replace(' operation_id text,\n receipt jsonb',' accounted_bytes bigint NOT NULL CHECK(accounted_bytes BETWEEN 0 AND 268435456),\n operation_id text,\n receipt jsonb')
    return t
edit('internal/store/postgres/migrations/007_uploads_requests.sql',quota)

def upload_revision(t):
    if 'SourceRevision int64' not in t:
        t=t.replace('Tenant,Actor,Session string','Tenant,Actor,Session string\n\tSourceRevision int64')
        t=t.replace('"strings"','"strings"\n\t"strconv"')
    t=t.replace('Revision:1,ContextID:r.Spec.ID+":v1"','Revision:r.SourceRevision,ContextID:r.Spec.ID+":v"+strconv.FormatInt(r.SourceRevision,10)')
    t=t.replace('if r.Receipt!=nil{partition=id+":v1"}','if r.SourceRevision>0{partition=id+":v"+strconv.FormatInt(r.SourceRevision,10)}')
    if 'limits.Lease=time.Duration(values.Jobs.Lease)' not in t:
        t=t.replace('limits.Workers=min(limits.Workers,limits.GlobalConcurrency)','limits.Workers=values.Jobs.Workers;limits.Lease=time.Duration(values.Jobs.Lease);limits.Heartbeat=time.Duration(values.Jobs.Heartbeat);limits.Poll=time.Duration(values.Jobs.Poll);limits.Backoff=time.Duration(values.Jobs.Backoff);limits.Batch=values.Jobs.Batch;limits.AttemptTimeout=time.Duration(values.Jobs.AttemptTimeout)')
    return t
edit('internal/engineering/uploads.go',upload_revision)

# Keep same exact executor, global slots and journal while concrete consumers
# can lower effective budgets. The public Execute entry point is in caps.go.
def execution(t):
    old='func (x *Executor) Execute(ctx context.Context, e identity.Envelope, p Plan, o Options) (ExecutionReport, error) {'
    new='func (x *Executor) execute(ctx context.Context, e identity.Envelope, p Plan, o Options, caps *Caps) (ExecutionReport, error) {'
    t=once(t,old,new)
    anchor='\tctx, cancel := context.WithTimeout(ctx, limits.Timeout)'
    insert='\tif caps != nil { limits.Rows=min(limits.Rows,caps.Rows); limits.Bytes=min(limits.Bytes,caps.Bytes); limits.Timeout=min(limits.Timeout,caps.Timeout); limits.PlannerCost=min(limits.PlannerCost,caps.PlannerCost) }\n'+anchor
    return once(t,anchor,insert)
edit('internal/exec/execution.go',execution)

def parquet(t):
    t=t.replace('case 0, 2, 3, 4, 8:', 'case 0, 2, 3, 4, 6, 8:')
    anchor='\t\tcase 2, 8:\n'
    insert='\t\tcase 6:\n\t\t\tif column.physical != 6 { return ErrFormat }; if err=guardDeltaLengths(data,int(nonNull),p.limits.MaxCellBytes); err!=nil { return err }\n'+anchor
    return once(t,anchor,insert)
edit('internal/engineering/parquet_guard.go',parquet)

def shared_jobs(t):
    t=t.replace("WHERE dispatch_mode='queued' AND status IN ('pending','retry','running')`, tenant", "WHERE dispatch_mode IN ('queued','request') AND status IN ('pending','retry','running')`, tenant")
    t=t.replace("WHERE dispatch_mode='queued' AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`", "WHERE dispatch_mode IN ('queued','request') AND status='running' AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`")
    t=t.replace("WHERE a.dispatch_mode='queued' AND a.tenant_id=o.tenant_id", "WHERE a.dispatch_mode IN ('queued','request') AND a.tenant_id=o.tenant_id")
    if 'fence, attempt, until, e := claimOperationLease' not in t:
        start=t.index("\t\tif _, e = tx.Exec(ctx, `UPDATE chartworks.operation_attempts SET state='abandoned'", t.index('func (d *DB) ClaimJob'))
        end=t.index('\t\tj.State = "running"',start)
        t=t[:start]+'''\t\tfence, attempt, until, e := claimOperationLease(ctx,tx,j.Tenant,j.ID,owner,l.Lease)
\t\tif e != nil { return e }
\t\tj.Attempts=attempt
'''+t[end:]
    for name,call in [('HeartbeatJob','renewOperationLease(ctx,tx,operationLease{tenant:lease.Job.Tenant,id:lease.Job.ID,owner:lease.Owner,manifest:lease.Job.ManifestHash,mode:"queued",fence:lease.Fence},ttl)'),('FinishAttempt','failOperationLease(ctx,tx,operationLease{tenant:lease.Job.Tenant,id:lease.Job.ID,owner:lease.Owner,manifest:lease.Job.ManifestHash,mode:"queued",fence:lease.Fence},code,permanent,delay)')]:
        start=t.index('func (d *DB) '+name+'(');end=t.index('\n}\n',start)+3
        section=t[start:end]
        if call.split('(')[0] not in section:
            marker='return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {';a=section.index(marker)
            section=section[:a]+marker+' return '+call+' })\n}\n'
            t=t[:start]+section+t[end:]
    return t
edit('internal/store/postgres/jobs.go',shared_jobs)
edit('internal/store/postgres/request_jobs.go',lambda t:t.replace("SELECT status,lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp() FROM", "SELECT status,COALESCE(lease_owner=$3 AND fence=$4 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp(),false) FROM"))

def safe_errors(t):
    if 'internal/engineering"' not in t:
        t=t.replace('"github.com/hurtener/chartworks/internal/access"','"github.com/hurtener/chartworks/internal/access"\n\t"github.com/hurtener/chartworks/internal/engineering"')
        t=t.replace('[]error{readexec.ErrType,','[]error{engineering.ErrInvalid, engineering.ErrLimit, engineering.ErrFormat, engineering.ErrChecksum, engineering.ErrOwnership, engineering.ErrUnavailable, engineering.ErrState, readexec.ErrType,')
    return t
edit('internal/store/postgres/postgres.go',safe_errors)

def writer_bounds(t):
    if 'pgconn/ctxwatch' not in t:
        t=t.replace('"github.com/jackc/pgx/v5/pgtype"','"github.com/jackc/pgx/v5/pgtype"\n\t"github.com/jackc/pgx/v5/pgconn"\n\t"github.com/jackc/pgx/v5/pgconn/ctxwatch"\n\t"github.com/jackc/pgx/v5/pgproto3"')
        t=t.replace('w.ConnectTimeout=time.Duration(s.values.Sources.ConnectTimeout)', '''w.ConnectTimeout=time.Duration(s.values.Sources.ConnectTimeout)
        w.Fallbacks=nil
        w.BuildContextWatcherHandler=func(conn *pgconn.PgConn)ctxwatch.Handler{return &pgconn.CancelRequestContextWatcherHandler{Conn:conn,CancelRequestDelay:0,DeadlineDelay:2*time.Second}}
        w.BuildFrontend=func(reader io.Reader,writer io.Writer)*pgproto3.Frontend{frontend:=pgproto3.NewFrontend(reader,writer);frontend.SetMaxBodyLen(int(s.values.Uploads.MaxBytes)+(1<<20));return frontend}
        w.OnNotice=nil;w.OnNotification=nil''')
    return t
edit('internal/engineering/workspace.go',writer_bounds)
