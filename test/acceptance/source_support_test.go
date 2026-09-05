package acceptance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

const sourcePassword="SYNTHETIC_SOURCE_PASSWORD"

type sourceFixture struct {
	db *postgres.DB
	dsn string
	warehouse string
	admin *pgx.Conn
	role string
	cfg config.Sources
	s *sources.Service
	validator *readexec.Validator
	token *tokenFixture
	e identity.Envelope
	mu sync.RWMutex
	values map[string]string
	lookups atomic.Int64
	writeLookups atomic.Int64
}
func newSourceFixture(t *testing.T,change func(*config.Sources)) *sourceFixture {
	t.Helper()
	f:=&sourceFixture{dsn:support.Database(t),warehouse:support.Database(t),token:newTokenFixture(t),cfg:config.DefaultSources(),values:map[string]string{}}
	f.db=support.Open(t,f.dsn)
	f.admin=support.Raw(t,f.warehouse)
	ctx:=context.Background()
	_,err:=f.admin.Exec(ctx,`CREATE SCHEMA analytics;
CREATE DOMAIN analytics.nonnegative AS numeric CHECK(VALUE>=0);
CREATE TYPE analytics.mood AS ENUM ('ready');
CREATE TABLE analytics.sales(id integer PRIMARY KEY,amount numeric(30,3),cash money,created_at timestamptz,active boolean,name text,document jsonb,payload bytea,domain_value analytics.nonnegative,mood analytics.mood,secret text);
CREATE TABLE analytics.items(sale_id integer,quantity integer);
INSERT INTO analytics.sales VALUES(1,9007199254740993.125,12.25,'2026-01-02T03:04:05Z',true,'one','{"a":1}',decode('cafe','hex'),2,'ready','PRIVATE_COLUMN_CANARY'),(2,5.5,2.25,'2026-01-03T03:04:05Z',false,'two','{"a":2}',NULL,3,'ready','PRIVATE_COLUMN_CANARY');
INSERT INTO analytics.items VALUES(1,3),(2,4);`)
	if err!=nil { t.Fatal("warehouse fixture",err) }
	var seed [8]byte
	if _,err=rand.Read(seed[:]); err!=nil { t.Fatal(err) }
	f.role="cw_reader_"+hex.EncodeToString(seed[:])
	quoted:=pgx.Identifier{f.role}.Sanitize()
	if _,err=f.admin.Exec(ctx,"CREATE ROLE "+quoted+" LOGIN PASSWORD '"+sourcePassword+"' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS"); err!=nil { t.Fatal(err) }
	// Register after the admin connection so cleanup runs while it is still open.
	t.Cleanup(func(){ _,_=f.admin.Exec(context.Background(),"DROP OWNED BY "+quoted); _,_=f.admin.Exec(context.Background(),"DROP ROLE "+quoted) })
	if _,err=f.admin.Exec(ctx,"GRANT USAGE ON SCHEMA analytics TO "+quoted+"; GRANT SELECT ON ALL TABLES IN SCHEMA analytics TO "+quoted); err!=nil { t.Fatal(err) }
	u,err:=url.Parse(f.warehouse); if err!=nil { t.Fatal(err) }
	u.User=url.UserPassword(f.role,sourcePassword)
	f.values["CHARTWORKS_SOURCE_READ"]=u.String()
	f.values["CHARTWORKS_SOURCE_WRITE"]="NEVER_RESOLVE_WRITE_CANARY"
	f.cfg.Enabled=true
	f.cfg.Connections=[]config.SourceConnection{{Tenant:"source-a",ID:"warehouse",Version:"operator-v1",ReadDSN:"env:CHARTWORKS_SOURCE_READ",WriteDSN:"env:CHARTWORKS_SOURCE_WRITE",Relations:[]config.SourceRelation{{Schema:"analytics",Name:"sales",Columns:[]string{"id","amount","cash","created_at","active","name","document","payload","domain_value","mood"}},{Schema:"analytics",Name:"items",Columns:[]string{"sale_id","quantity"}}}}}
	if change!=nil { change(&f.cfg) }
	f.s,err=sources.New(f.db,f.cfg,f.lookup); if err!=nil { t.Fatal("source service",err) }; t.Cleanup(f.s.Close)
	f.validator,err=readexec.NewValidator(f.s,config.DefaultReadValidation()); if err!=nil { t.Fatal(err) }
	f.e=f.actor(t,"source-a","operator")
	return f
}
func (f *sourceFixture) lookup(name string) (string,bool) {
	f.lookups.Add(1)
	if name=="CHARTWORKS_SOURCE_WRITE" { f.writeLookups.Add(1) }
	f.mu.RLock(); defer f.mu.RUnlock()
	v,ok:=f.values[name]; return v,ok
}
func (f *sourceFixture) actor(t *testing.T,tenant,user string) identity.Envelope {
	return f.token.envelope(t,tenant,user,"sources.write","sources.read","sources.rotate","sources.query","cw.tenant.write:"+tenant,"cw.source.read:*","cw.source.write:*","cw.source.query:*","cw.execution_context.use:*","cw.dataset.query:*")
}
func (f *sourceFixture) create(t *testing.T,id string) sources.Source {
	t.Helper()
	out,err:=f.s.Create(context.Background(),f.e,sources.CreateRequest{ID:id,Name:"Synthetic warehouse",Connection:"warehouse"}); if err!=nil { t.Fatal("create source",err) }; return out
}
func (f *sourceFixture) plan(t *testing.T,source sources.Source,sql string,parameters ...readexec.Parameter) readexec.Plan {
	t.Helper()
	p,err:=f.validator.Validate(context.Background(),f.e,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:sql,Parameters:parameters}); if err!=nil { t.Fatal("validate",sql,err) }; return p
}
func (f *sourceFixture) setReadDSN(value string) { f.mu.Lock(); f.values["CHARTWORKS_SOURCE_READ"]=value; f.mu.Unlock() }
func (f *sourceFixture) readDSN() string { f.mu.RLock(); defer f.mu.RUnlock(); return f.values["CHARTWORKS_SOURCE_READ"] }
