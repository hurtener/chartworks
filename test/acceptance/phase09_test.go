package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/jackc/pgx/v5"
)

type unsupportedReadAdapter struct { binding readexec.Binding; explains int }
func (a *unsupportedReadAdapter) Binding(context.Context,identity.Envelope,string,string) (readexec.Binding,error) { return a.binding.Clone(),nil }
func (a *unsupportedReadAdapter) Explain(context.Context,identity.Envelope,readexec.Candidate) error { a.explains++; return nil }

func TestPhase09(t *testing.T) {
	t.Run("AC01",func(t *testing.T) {
		f:=newSourceFixture(t,nil); ctx:=context.Background(); source:=f.create(t,"sales")
		plan:=f.plan(t,source,`SELECT id, amount FROM analytics.sales WHERE id=$1`,readexec.Parameter{Kind:"integer",Value:"1"})
		receipt:=plan.Receipt()
		if !receipt.Validated || receipt.Source!=source.ID || receipt.Context!=source.ContextID || len(receipt.Dependencies)!=1 || len(receipt.Manifest)!=64 { t.Fatal("missing validated binding receipt") }
		for _,typ:=range []reflect.Type{reflect.TypeOf(readexec.Plan{}),reflect.TypeOf(readexec.Candidate{})} { for i:=0;i<typ.NumField();i++ { if typ.Field(i).IsExported() { t.Fatal("caller-constructible proof field") } } }
		binding,err:=f.s.Binding(ctx,f.e,source.ID,source.ContextID); if err!=nil { t.Fatal(err) }
		statement,parameters,err:=plan.SQL(f.e,binding); if err!=nil || statement=="" || len(parameters)!=1 { t.Fatal("sealed arguments missing",err) }
		parameters[0].Value="2"
		rows,err:=f.s.Read(ctx,f.e,plan); if err!=nil || len(rows.Values)!=1 || *rows.Values[0][0]!="1" { t.Fatal("caller mutated bound parameters",err) }
		changed:=binding.Clone(); changed.Relations[0].Columns[0].Name="secret"
		if _,_,err:=plan.SQL(f.e,changed); !errors.Is(err,readexec.ErrBinding) { t.Fatal("changed semantic contract accepted",err) }
		other:=f.actor(t,f.e.Tenant(),"different-actor")
		if _,err:=f.s.Read(ctx,other,plan); !errors.Is(err,readexec.ErrBinding) { t.Fatal("other actor reused plan",err) }
		narrow:=f.token.envelope(t,f.e.Tenant(),f.e.User(),"sources.query","cw.source.query:sales","cw.execution_context.use:sales:v1","cw.dataset.query:*")
		if _,err:=f.s.Read(ctx,narrow,plan); !errors.Is(err,readexec.ErrBinding) { t.Fatal("narrowed envelope reused broader plan",err) }
		encoded,_:=json.Marshal(plan)
		if strings.Contains(string(encoded),statement) || strings.Contains(string(encoded),"parameters") { t.Fatal("opaque plan serialized executable SQL") }
	})
	t.Run("AC02",func(t *testing.T) {
		f:=newSourceFixture(t,nil); ctx:=context.Background(); source:=f.create(t,"sales")
		before:=f.lookups.Load()
		for _,sql:=range []string{
			`WITH x AS (UPDATE analytics.sales SET amount=0 RETURNING id) SELECT id FROM x`,
			`WITH x AS (SELECT id FROM analytics.sales), y AS (DELETE FROM analytics.items RETURNING sale_id) SELECT id FROM x`,
			`SELECT id FROM analytics.sales; DELETE FROM analytics.items`,
			`SELECT pg_catalog.nextval('sequence')`,
			`SELECT pg_read_file('/etc/passwd')`,
			`SELECT set_config('transaction_read_only','off',true)`,
			`SELECT * FROM analytics.sales`,
			`SELECT s.secret FROM analytics.sales s`,
			`SELECT id FROM pg_catalog.pg_class`,
			`SELECT id FROM analytics.sales WHERE EXISTS (SELECT 1 FROM analytics.unapproved)`,
			`SELECT id FROM analytics.sales ORDER BY public.side_effect(id)`,
			`SELECT sum(amount) FILTER (WHERE public.side_effect(id)>0) FROM analytics.sales`,
			`SELECT row_number() OVER (ORDER BY public.side_effect(id)) FROM analytics.sales`,
			`SELECT amount::public.unsafe_type FROM analytics.sales`,
			`SELECT 1 INTO copied`,
			`COPY analytics.sales TO '/tmp/copy'`,
		} {
			if p,err:=f.validator.Validate(ctx,f.e,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:sql}); err==nil || p.Receipt().Validated { t.Fatalf("unproven SQL accepted: %s",sql) }
		}
		if f.lookups.Load()!=before { t.Fatal("unsafe SQL reached native planner") }
		missingDataset:=f.token.envelope(t,f.e.Tenant(),f.e.User(),"sources.query","cw.source.query:sales","cw.execution_context.use:sales:v1")
		if _,err:=f.validator.Validate(ctx,missingDataset,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:`SELECT id FROM analytics.sales`}); err==nil || f.lookups.Load()!=before { t.Fatal("dataset denial happened after protected planning") }
	})
	t.Run("AC03",func(t *testing.T) {
		f:=newSourceFixture(t,nil); ctx:=context.Background(); source:=f.create(t,"sales")
		for _,sql:=range []string{
			`WITH totals AS (SELECT id, sum(amount) AS total FROM analytics.sales GROUP BY id) SELECT t.id,t.total FROM totals t ORDER BY t.id`,
			`SELECT id, row_number() OVER (ORDER BY amount) AS position FROM analytics.sales`,
			`SELECT id FROM analytics.sales UNION ALL SELECT sale_id AS id FROM analytics.items ORDER BY id`,
			`SELECT id FROM analytics.sales INTERSECT SELECT sale_id FROM analytics.items`,
			`SELECT s.id,i.quantity FROM analytics.sales s JOIN analytics.items i ON s.id=i.sale_id`,
			`SELECT q.id FROM (SELECT id FROM analytics.sales WHERE amount>0) q`,
			`SELECT id FROM analytics.sales WHERE id IN (SELECT sale_id FROM analytics.items)`,
			`SELECT id, CASE WHEN active THEN upper(name) ELSE lower(name) END AS label FROM analytics.sales`,
			`SELECT id FROM analytics.sales WHERE name='; DROP TABLE analytics.sales; --' OR id=1`,
		} {
			plan:=f.plan(t,source,sql)
			if _,err:=f.s.Read(ctx,f.e,plan); err!=nil { t.Fatal("qualified grammar failed native read",sql,err) }
		}
		for _,sql:=range []string{`SELECT id FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id`,`WITH q AS (SELECT id FROM analytics.sales) SELECT secret FROM q`,`SELECT items.sale_id FROM analytics.sales`} { if _,err:=f.validator.Validate(ctx,f.e,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:sql}); err==nil { t.Fatal("ambiguous or hidden visibility guessed") } }
	})
	t.Run("AC04",func(t *testing.T) {
		f:=newSourceFixture(t,nil); ctx:=context.Background(); source:=f.create(t,"sales")
		_,err:=f.admin.Exec(ctx,`CREATE TABLE analytics.effects(n integer); CREATE FUNCTION analytics.side_effect() RETURNS integer LANGUAGE plpgsql SECURITY DEFINER AS $$ BEGIN INSERT INTO analytics.effects VALUES(1); RETURN 1; END $$;`)
		if err!=nil { t.Fatal(err) }
		if _,err=f.admin.Exec(ctx,"GRANT EXECUTE ON FUNCTION analytics.side_effect() TO "+pgx.Identifier{f.role}.Sanitize()); err!=nil { t.Fatal(err) }
		before:=f.lookups.Load()
		for _,sql:=range []string{`SELECT analytics.side_effect()`,`EXPLAIN ANALYZE SELECT id FROM analytics.sales`,`EXPLAIN SELECT id FROM analytics.sales`,`BEGIN; SELECT id FROM analytics.sales; COMMIT`,`SELECT id FROM analytics.sales FOR SHARE`} { if _,err:=f.validator.Validate(ctx,f.e,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:sql}); err==nil { t.Fatal("native planning bypass") } }
		if f.lookups.Load()!=before { t.Fatal("bypass sent to warehouse") }
		var count int
		if err=f.admin.QueryRow(ctx,`SELECT count(*) FROM analytics.effects`).Scan(&count); err!=nil || count!=0 { t.Fatal("planning executed side effect",err,count) }
		if p:=f.plan(t,source,`SELECT id FROM analytics.sales`); !p.Receipt().Validated { t.Fatal("native safe planning failed") }
	})
	t.Run("AC05",func(t *testing.T) {
		f:=newSourceFixture(t,nil); ctx:=context.Background(); source:=f.create(t,"sales")
		binding,err:=f.s.Binding(ctx,f.e,source.ID,source.ContextID); if err!=nil { t.Fatal(err) }
		for _,dialect:=range []string{"mysql","sqlserver","duckdb","unknown"} {
			changed:=binding.Clone(); changed.Dialect=dialect
			adapter:=&unsupportedReadAdapter{binding:changed}
			validator,err:=readexec.NewValidator(adapter,config.DefaultReadValidation()); if err!=nil { t.Fatal(err) }
			if _,err=validator.Validate(ctx,f.e,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:`SELECT id FROM analytics.sales`}); !errors.Is(err,readexec.ErrUnsupported) || adapter.explains!=0 { t.Fatal("unqualified dialect fell back",dialect,err) }
		}
		// A source with unproven custom types never acquires an executable proof for them.
		if _,err=f.validator.Validate(ctx,f.e,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:`SELECT domain_value,mood FROM analytics.sales`}); err==nil { t.Fatal("unqualified type executed") }
	})
	t.Run("AC06",func(t *testing.T) {
		f:=newSourceFixture(t,nil); ctx:=context.Background(); source:=f.create(t,"sales")
		if !strings.Contains(readexec.ParserIdentity,"wasm-b511bb3bfd6e") { t.Fatal("parser pin missing") }
		before:=f.lookups.Load()
		for _,request:=range []readexec.Request{
			{Source:source.ID,Context:source.ContextID,SQL:strings.Repeat("x",65537)},
			{Source:source.ID,Context:source.ContextID,SQL:"SELECT "+strings.Repeat("(",257)+"1"+strings.Repeat(")",257)},
			{Source:source.ID,Context:source.ContextID,SQL:"SELECT 1\x00"},
			{Source:source.ID,Context:source.ContextID,SQL:"SELECT $1",Parameters:[]readexec.Parameter{{Kind:"number",Value:"NaN"}}},
			{Source:source.ID,Context:source.ContextID,SQL:"SELECT id FROM analytics.sales",Parameters:[]readexec.Parameter{{Kind:"integer",Value:"1"}}},
			{Source:source.ID,Context:source.ContextID,SQL:"SELECT $2",Parameters:[]readexec.Parameter{{Kind:"integer",Value:"1"}}},
		} { if _,err:=f.validator.Validate(ctx,f.e,request); err==nil || strings.Contains(err.Error(),request.SQL) { t.Fatal("unbounded or unsafe error response") } }
		if f.lookups.Load()!=before { t.Fatal("bad parse input performed source work") }
		cancelled,cancel:=context.WithCancel(ctx); cancel()
		if _,err:=f.validator.Validate(cancelled,f.e,readexec.Request{Source:source.ID,Context:source.ContextID,SQL:`SELECT 1`}); err==nil { t.Fatal("cancelled parser work admitted") }
	})
}
