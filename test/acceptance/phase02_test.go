package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/hurtener/chartworks/internal/maintenance"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

func hash(s string)string{x:=sha256.Sum256([]byte(s));return hex.EncodeToString(x[:])}
func sql(t testing.TB,c *pgx.Conn,q string,args ...any){t.Helper();if _,e:=c.Exec(context.Background(),q,args...);e!=nil{t.Fatalf("fixture SQL failed (%T)",e)}}
func count(t testing.TB,c *pgx.Conn,q string,args ...any)int64{t.Helper();var n int64;if e:=c.QueryRow(context.Background(),q,args...).Scan(&n);e!=nil{t.Fatalf("fixture count failed (%T)",e)};return n}
func policy(t testing.TB,db *postgres.DB,s store.Scope)store.Policy{t.Helper();p,e:=db.SetPolicy(context.Background(),s,0,store.Policy{AuditDays:7,OperationHours:24});if e!=nil{t.Fatal(e)};return p}
func upgradeFixture(t testing.TB,dsn string)*pgx.Conn{
	t.Helper();c:=support.Raw(t,dsn);m,e:=postgres.Migrations();if e!=nil{t.Fatal(e)}
	sql(t,c,`CREATE SCHEMA chartworks; CREATE TABLE chartworks.schema_migrations(version integer PRIMARY KEY,name text NOT NULL,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	sql(t,c,m[0].SQL);sql(t,c,`INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`,m[0].Version,m[0].Name,m[0].Checksum)
	return c
}
func TestPhase02(t *testing.T){
	ctx:=context.Background()
	t.Run("AC01",func(t *testing.T){
		dsn:=support.Database(t);var wg sync.WaitGroup
		for i:=0;i<8;i++{wg.Add(1);go func(){defer wg.Done();db,e:=postgres.Open(ctx,dsn,postgres.Defaults());if e!=nil{t.Error(e);return};db.Close()}()};wg.Wait()
		c:=support.Raw(t,dsn);if count(t,c,`SELECT count(*) FROM chartworks.schema_migrations`)!=2{t.Fatal("migrations not exactly once")}
		db:=support.Open(t,dsn);if e:=db.Check(ctx);e!=nil{t.Fatal(e)}
		opts:=postgres.Defaults();opts.MigrationPolicy="check";other,e:=postgres.Open(ctx,dsn,opts);if e!=nil{t.Fatal(e)};other.Close()
		upgrade:=support.Database(t);u:=upgradeFixture(t,upgrade)
		sql(t,u,`INSERT INTO chartworks.policy_revisions(tenant_id,revision,audit_days,operation_hours,created_by) VALUES('prior',1,12,48,'actor'); INSERT INTO chartworks.policies VALUES('prior',1)`)
		db2:=support.Open(t,upgrade);p,e:=db2.Policy(ctx,support.Scope(t,"prior","actor"));if e!=nil||p.AuditDays!=12||p.OperationHours!=48{t.Fatal("upgrade lost data")}
		sql(t,c,`UPDATE chartworks.schema_migrations SET checksum=repeat('0',64) WHERE version=1`)
		if _,e=postgres.Open(ctx,dsn,postgres.Defaults());!errors.Is(e,store.ErrMigration){t.Fatal("changed migration accepted")}
		broken:=support.Database(t);bc:=upgradeFixture(t,broken);sql(t,bc,`CREATE TABLE chartworks.operations(dummy text)`)
		if _,e=postgres.Open(ctx,broken,postgres.Defaults());!errors.Is(e,store.ErrMigration){t.Fatal("partial migration accepted")}
		if count(t,bc,`SELECT count(*) FROM chartworks.schema_migrations`)!=1{t.Fatal("failed migration committed history")}
		if count(t,bc,`SELECT count(*) FROM information_schema.columns WHERE table_schema='chartworks' AND table_name='audit_events' AND column_name='operation_id'`)!=0{t.Fatal("failed migration left partial DDL")}
		unknown:=support.Database(t);udb:=support.Open(t,unknown);udb.Close();uc:=support.Raw(t,unknown);sql(t,uc,`INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES(99,'future',repeat('a',64))`)
		if _,e=postgres.Open(ctx,unknown,postgres.Defaults());!errors.Is(e,store.ErrMigration){t.Fatal("future schema accepted")}
		empty:=support.Database(t);if _,e=postgres.Open(ctx,empty,opts);!errors.Is(e,store.ErrMigration){t.Fatal("check mode migrated empty database")}
	})
	t.Run("AC02",func(t *testing.T){
		dsn:=support.Database(t);db:=support.Open(t,dsn);a,b:=support.Scope(t,"tenant-a","actor"),support.Scope(t,"tenant-b","actor");policy(t,db,a);policy(t,db,b)
		for _,bad:=range [][2]string{{"","actor"},{"tenant",""},{"../foreign","actor"},{strings.Repeat("t",129),"actor"}}{if _,e:=store.NewScope(bad[0],bad[1]);e==nil{t.Fatal("invalid scope accepted")}}
		zero:=store.Scope{};checks:=[]func()error{
			func()error{_,e:=db.Policy(ctx,zero);return e},func()error{_,e:=db.SetPolicy(ctx,zero,0,store.Policy{AuditDays:7,OperationHours:24});return e},func()error{_,e:=db.Audits(ctx,zero,10);return e},func()error{_,e:=db.ReserveSweep(ctx,zero,"key",hash("x"),10);return e},func()error{_,e:=db.Claim(ctx,zero,"op","owner",time.Second);return e},func()error{return db.Renew(ctx,zero,store.Lease{},time.Second)},func()error{_,e:=db.CommitSweep(ctx,zero,store.Lease{});return e},
		};for _,f:=range checks{if !errors.Is(f(),store.ErrScope){t.Fatal("unscoped repository method")}}
		if _,e:=db.Policy(ctx,support.Scope(t,"absent","actor"));!errors.Is(e,store.ErrNotFound){t.Fatal("foreign tenant disclosed")}
		op,e:=db.ReserveSweep(ctx,a,"shared-key",hash("x"),10);if e!=nil{t.Fatal(e)}
		if _,e=db.Claim(ctx,b,op.ID,"attacker",time.Second);!errors.Is(e,store.ErrConflict){t.Fatal("cross-tenant lease theft")}
		if _,e=db.Claim(ctx,support.Scope(t,"tenant-a","other-actor"),op.ID,"attacker",time.Second);!errors.Is(e,store.ErrConflict){t.Fatal("cross-actor lease theft")}
		p,e:=db.SetPolicy(ctx,a,1,store.Policy{AuditDays:8,OperationHours:24});if e!=nil||p.Revision!=2{t.Fatal("revision update failed")}
		c:=support.Raw(t,dsn)
		if _,e=c.Exec(ctx,`UPDATE chartworks.policies SET current_revision=2 WHERE tenant_id='tenant-b'`);e==nil{t.Fatal("cross-tenant reference accepted")}
		if _,e=c.Exec(ctx,`INSERT INTO chartworks.policy_revisions(tenant_id,revision,audit_days,operation_hours,created_by) VALUES('',1,7,24,'actor')`);e==nil{t.Fatal("SQL bypassed required tenant")}
		sql(t,c,`DELETE FROM chartworks.audit_events WHERE tenant_id='tenant-a'`)
		records,e:=db.Audits(ctx,b,10);if e!=nil||len(records)!=1{t.Fatal("tenant-targeted deletion affected other partition")}
		if _,e=c.Exec(ctx,`DELETE FROM chartworks.policy_revisions WHERE tenant_id='tenant-b' AND revision=1`);e==nil{t.Fatal("referenced revision could be orphaned")}
	})
	t.Run("AC03",func(t *testing.T){
		dsn:=support.Database(t);db:=support.Open(t,dsn);scope:=support.Scope(t,"cas","actor");policy(t,db,scope)
		var wg sync.WaitGroup;var won atomic.Int64
		for i:=0;i<20;i++{wg.Add(1);go func(){defer wg.Done();_,e:=db.SetPolicy(ctx,scope,1,store.Policy{AuditDays:9,OperationHours:24});if e==nil{won.Add(1)}else if !errors.Is(e,store.ErrConflict){t.Error(e)}}()};wg.Wait();if won.Load()!=1{t.Fatalf("CAS had %d winners",won.Load())}
		c:=support.Raw(t,dsn);if count(t,c,`SELECT count(*) FROM chartworks.policy_revisions`)!=2||count(t,c,`SELECT count(*) FROM chartworks.audit_events`)!=2{t.Fatal("losing writer left state/audit")}
		if _,e:=c.Exec(ctx,`UPDATE chartworks.policy_revisions SET audit_days=1 WHERE tenant_id='cas' AND revision=1`);e==nil{t.Fatal("published revision mutable")}
		sql(t,c,`CREATE FUNCTION chartworks.fail_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END; $$; CREATE TRIGGER reject_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.fail_audit()`)
		if _,e:=db.SetPolicy(ctx,scope,2,store.Policy{AuditDays:10,OperationHours:24});e==nil{t.Fatal("failed audit did not abort transaction")}
		p,e:=db.Policy(ctx,scope);if e!=nil||p.Revision!=2||count(t,c,`SELECT count(*) FROM chartworks.policy_revisions`)!=2{t.Fatal("partial CAS committed")}
	})
	t.Run("AC04",func(t *testing.T){
		dsn:=support.Database(t);db:=support.Open(t,dsn);c:=support.Raw(t,dsn)
		rows,e:=c.Query(ctx,`SELECT table_name FROM information_schema.tables WHERE table_schema='chartworks' ORDER BY table_name`);if e!=nil{t.Fatal("schema inspection failed")}
		var names []string;for rows.Next(){var name string;if e=rows.Scan(&name);e!=nil{t.Fatal("schema scan failed")};names=append(names,name)};rows.Close();if rows.Err()!=nil{t.Fatal("schema rows failed")}
		expected:=[]string{"audit_events","operations","policies","policy_revisions","schema_migrations"};sort.Strings(expected)
		if strings.Join(names,",")!=strings.Join(expected,","){t.Fatalf("unexpected foundation schema: %v",names)}
		if count(t,c,`SELECT count(*) FROM information_schema.columns WHERE table_schema='chartworks' AND (column_name LIKE '%password%' OR column_name LIKE '%secret%' OR column_name LIKE '%token%' OR column_name LIKE '%role%' OR column_name LIKE '%grant%')`)!=0{t.Fatal("local IAM/issuer material in schema")}
		opts:=postgres.Defaults();opts.MaxConns=0;if _,e=postgres.Open(ctx,dsn,opts);!errors.Is(e,store.ErrInvalid){t.Fatal("invalid pool bounds accepted")}
		if _,e=postgres.Open(ctx,"://"+canary,postgres.Defaults());e==nil||strings.Contains(e.Error(),canary){t.Fatal("DSN error leaked")}
		if _,e=db.SetPolicy(ctx,support.Scope(t,"bounds","actor"),0,store.Policy{});!errors.Is(e,store.ErrInvalid){t.Fatal("invalid retention accepted")}
		canceled,cancel:=context.WithCancel(ctx);cancel();if e=db.Check(canceled);e==nil{t.Fatal("cancellation ignored")}
	})
	t.Run("AC05",func(t *testing.T){
		dsn:=support.Database(t);db:=support.Open(t,dsn);s:=support.Scope(t,"operations","actor");policy(t,db,s)
		var wg sync.WaitGroup;ids:=make(chan string,20)
		for i:=0;i<20;i++{wg.Add(1);go func(){defer wg.Done();o,e:=db.ReserveSweep(ctx,s,"retry-key",hash("request"),10);if e!=nil{t.Error(e);return};ids<-o.ID}()};wg.Wait();close(ids);var opID string;for i:=range ids{if opID!=""&&opID!=i{t.Fatal("duplicate operation")};opID=i}
		if _,e:=db.ReserveSweep(ctx,s,"retry-key",hash("different"),10);!errors.Is(e,store.ErrConflict){t.Fatal("request conflict accepted")}
		l,e:=db.Claim(ctx,s,opID,"worker-a",30*time.Millisecond);if e!=nil{t.Fatal(e)}
		time.Sleep(60*time.Millisecond);if e=db.Renew(ctx,s,l,time.Second);!errors.Is(e,store.ErrConflict){t.Fatal("expired lease resurrected")}
		l2,e:=db.Claim(ctx,s,opID,"worker-b",3*time.Second);if e!=nil||l2.Fence<=l.Fence{t.Fatal("fence did not advance")}
		if _,e=db.CommitSweep(ctx,s,l);!errors.Is(e,store.ErrConflict){t.Fatal("stale worker committed")}
		if e=db.Renew(ctx,s,l2,3*time.Second);e!=nil{t.Fatal(e)}
		if _,e=db.Claim(ctx,s,opID,"worker-c",time.Second);!errors.Is(e,store.ErrConflict){t.Fatal("active lease stolen")}
		c:=support.Raw(t,dsn);sql(t,c,`INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id,created_at) VALUES('operations',repeat('a',32),'actor','retention_policy.updated','retention',clock_timestamp()-interval '30 days')`)
		sql(t,c,`CREATE FUNCTION chartworks.reject_sweep() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='retention.sweep' THEN RAISE EXCEPTION 'injected failure'; END IF; RETURN NEW; END; $$; CREATE TRIGGER reject_sweep BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_sweep()`)
		if _,e=db.CommitSweep(ctx,s,l2);e==nil{t.Fatal("failing completion audit committed")}
		if count(t,c,`SELECT count(*) FROM chartworks.audit_events WHERE event_id=repeat('a',32)`)!=1{t.Fatal("rollback lost retained row")}
		sql(t,c,`DROP TRIGGER reject_sweep ON chartworks.audit_events`)
		o,e:=db.CommitSweep(ctx,s,l2);if e!=nil||o.Status!="succeeded"||o.DeletedEvents!=1{t.Fatalf("sweep completion failed: %v",e)}
		db.Close();reopened:=support.Open(t,dsn);replay,e:=reopened.ReserveSweep(ctx,s,"retry-key",hash("request"),10);if e!=nil||replay.ID!=o.ID||replay.Status!="succeeded"||replay.Cutoff!=o.Cutoff{t.Fatal("restart changed accepted manifest/result")}
		service,e:=maintenance.New(reopened,10,3*time.Second);if e!=nil{t.Fatal(e)};first,e:=service.Sweep(ctx,s,"actual-consumer");if e!=nil{t.Fatal(e)};second,e:=service.Sweep(ctx,s,"actual-consumer");if e!=nil||first.ID!=second.ID{t.Fatal("real consumer did not replay")}
		if _,e=reopened.Claim(ctx,s,first.ID,"worker",time.Second);!errors.Is(e,store.ErrConflict){t.Fatal("completed result claimed again")}
	})
	t.Run("AC06",func(t *testing.T){
		dsn:=support.Database(t);db:=support.Open(t,dsn);a,b:=support.Scope(t,"backup-a","actor"),support.Scope(t,"backup-b","actor");policy(t,db,a);policy(t,db,b);c:=support.Raw(t,dsn)
		for _,tenant:=range []string{"backup-a","backup-b"}{sql(t,c,`INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id,created_at) VALUES($1,repeat('b',32),'actor','retention_policy.updated','retention',clock_timestamp()-interval '30 days')`,tenant)}
		service,e:=maintenance.New(db,10,3*time.Second);if e!=nil{t.Fatal(e)};o,e:=service.Sweep(ctx,a,"sweep");if e!=nil||o.DeletedEvents!=1{t.Fatal("bounded retention failed")}
		if count(t,c,`SELECT count(*) FROM chartworks.audit_events WHERE tenant_id='backup-b' AND event_id=repeat('b',32)`)!=1{t.Fatal("retention crossed tenant")}
		archive:=filepath.Join(t.TempDir(),"metadata.dump")
		archiveCommand:=func(uri,action string,extra ...string)([]byte,error){
			commandCtx,cancel:=context.WithTimeout(ctx,30*time.Second);defer cancel();args:=append([]string{"../../scripts/store_archive.py",action,archive},extra...)
			cmd:=exec.CommandContext(commandCtx,"python3",args...);cmd.Env=append(os.Environ(),"CHARTWORKS_STORE_URL="+uri);return cmd.CombinedOutput()
		}
		if output,e:=archiveCommand(dsn,"backup");e!=nil{t.Fatalf("real pg_dump backup failed: %s",output)}
		info,e:=os.Stat(archive);if e!=nil||info.Mode().Perm()!=0600||info.Size()==0{t.Fatal("archive permissions/content invalid")}
		if _,e=archiveCommand(dsn,"backup");e==nil{t.Fatal("existing backup overwritten")}
		if _,e=archiveCommand(dsn,"restore","--confirm-empty");e==nil{t.Fatal("nonempty database restore accepted")}
		restored:=support.Database(t)
		if output,e:=archiveCommand(restored,"restore","--confirm-empty");e!=nil{t.Fatalf("real pg_restore failed: %s",output)}
		rdb:=support.Open(t,restored);p,e:=rdb.Policy(ctx,b);if e!=nil||p.Revision!=1||p.AuditDays!=7{t.Fatal("restored policy incorrect")}
		restoredService,e:=maintenance.New(rdb,10,3*time.Second);if e!=nil{t.Fatal(e)};replay,e:=restoredService.Sweep(ctx,a,"sweep");if e!=nil||replay.ID!=o.ID||replay.DeletedEvents!=o.DeletedEvents{t.Fatal("restored operation lost idempotency/result")}
		raw:=support.Raw(t,restored);if count(t,raw,`SELECT count(*) FROM chartworks.schema_migrations`)!=2{t.Fatal("restored schema history invalid")}
		if _,e:=raw.Exec(ctx,`DELETE FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`,a.Tenant(),o.ID);e==nil{t.Fatal("retained audit reference orphaned after restore")}
		if _,e=db.Audits(ctx,a,0);!errors.Is(e,store.ErrInvalid){t.Fatal("unbounded audit request accepted")}
	})
}
