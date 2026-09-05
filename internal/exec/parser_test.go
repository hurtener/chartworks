package exec

import (
	"encoding/json"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	pgquery "github.com/wasilibs/go-pgquery"
)

func parserBinding() Binding {
	return Binding{Tenant:"tenant",Source:"source",Context:"source:v1",Revision:1,Dialect:"postgres",Contract:"contract",Fingerprint:Hash("synthetic"),Relations:[]Relation{
		{ID:"sales",Schema:"analytics",Name:"sales",Columns:[]Column{{Name:"id",NativeType:"integer",Safe:true},{Name:"amount",NativeType:"numeric",Safe:true},{Name:"name",NativeType:"text",Safe:true},{Name:"created_at",NativeType:"timestamp",Safe:true},{Name:"active",NativeType:"boolean",Safe:true},{Name:"custom",NativeType:"custom_type",Safe:false}}},
		{ID:"items",Schema:"analytics",Name:"items",Columns:[]Column{{Name:"sale_id",NativeType:"integer",Safe:true},{Name:"quantity",NativeType:"integer",Safe:true}}},
	}}
}
func resolveFixture(sql string) ([]string,error,string) {
	raw,err:=pgquery.ParseToJSON(sql); if err!=nil { return nil,err,"" }
	var tree map[string]any
	if err=json.Unmarshal([]byte(raw),&tree); err!=nil { return nil,err,raw }
	statements:=array(tree["stmts"]); if len(statements)!=1 { return nil,ErrUnsafe,raw }
	r:=sqlResolver{binding:parserBinding(),dependencies:map[string]bool{},parameters:map[int]bool{},parameterCount:1}
	cols,err:=r.selectStatement(object(statements[0])["stmt"],nil)
	return cols,err,raw
}
func TestSQLResolverGrammar(t *testing.T) {
	for _,sql:=range []string{
		`SELECT id, amount FROM analytics.sales WHERE amount > $1 ORDER BY id LIMIT 10`,
		`WITH totals AS (SELECT id, sum(amount) AS amount FROM analytics.sales GROUP BY id) SELECT t.id, t.amount, row_number() OVER (ORDER BY t.amount DESC) AS position FROM totals t`,
		`SELECT id FROM analytics.sales UNION ALL SELECT id FROM analytics.sales ORDER BY id`,
		`SELECT s.id FROM analytics.sales AS s JOIN analytics.items AS i ON s.id=i.sale_id`,
		`SELECT x.id FROM (SELECT id FROM analytics.sales WHERE amount>0) AS x`,
		`SELECT CASE WHEN amount IS NULL THEN 0 ELSE amount END AS value FROM analytics.sales`,
		`SELECT coalesce(amount,0), greatest(amount,1) FROM analytics.sales`,
		`SELECT pg_catalog.date_trunc('month',created_at) AS month FROM analytics.sales`,
		`SELECT count(*) FROM analytics.sales`,
		`SELECT sum(amount) FILTER (WHERE amount>0) OVER (PARTITION BY active ORDER BY id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS total FROM analytics.sales`,
		`SELECT amount::numeric(18,2) FROM analytics.sales`,
		`SELECT id FROM analytics.sales WHERE id IN (SELECT sale_id FROM analytics.items)`,
		`SELECT id FROM analytics.sales WHERE id IN (1,2,3)`,
		`SELECT id FROM analytics.sales WHERE name LIKE 'a%'`,
		`SELECT current_date`,
		`SELECT 1 + 2`,
		`WITH q(a) AS (SELECT id FROM analytics.sales) SELECT a FROM q`,
		`SELECT row_number() OVER w AS n FROM analytics.sales WINDOW w AS (ORDER BY id)`,
		`SELECT id, sum(amount) FROM analytics.sales GROUP BY GROUPING SETS ((id),())`,
		`VALUES (1, 'a'), (2, 'b')`,
	} {
		t.Run(sql,func(t *testing.T){ cols,err,raw:=resolveFixture(sql); if err!=nil || len(cols)==0 { t.Fatalf("safe grammar rejected: %v\nAST: %s",err,raw) } })
	}
}
func TestSQLResolverAdversarial(t *testing.T) {
	for _,sql:=range []string{
		`DELETE FROM analytics.sales`,
		`SELECT id FROM analytics.sales; SELECT 1`,
		`WITH x AS (DELETE FROM analytics.sales RETURNING id) SELECT id FROM x`,
		`SELECT id INTO stolen FROM analytics.sales`,
		`SELECT id FROM analytics.sales FOR UPDATE`,
		`SELECT pg_sleep(1) FROM analytics.sales`,
		`SELECT pg_catalog.pg_read_file('/etc/passwd')`,
		`SELECT set_config('search_path','public',true)`,
		`SELECT nextval('sequence')`,
		`SELECT pg_advisory_lock(1)`,
		`SELECT public.dangerous_immutable(amount) FROM analytics.sales`,
		`SELECT amount OPERATOR(public.+) amount FROM analytics.sales`,
		`SELECT name::public.external_type FROM analytics.sales`,
		`SELECT * FROM analytics.sales`,
		`SELECT sales.* FROM analytics.sales`,
		`SELECT secret FROM analytics.sales`,
		`SELECT custom FROM analytics.sales`,
		`SELECT id FROM sales`,
		`SELECT id FROM foreign_database.analytics.sales`,
		`SELECT id FROM public.unregistered`,
		`SELECT * FROM dblink('secret','SELECT 1')`,
		`SELECT id FROM analytics.sales NATURAL JOIN analytics.items`,
		`SELECT id FROM analytics.sales ORDER BY id USING OPERATOR(public.<)`,
		`SELECT id FROM analytics.sales WHERE amount>$2`,
		`SELECT id FROM analytics.sales WHERE amount>$0`,
		`WITH RECURSIVE q AS (SELECT 1 AS n UNION ALL SELECT n+1 FROM q) SELECT n FROM q`,
	} {
		t.Run(sql,func(t *testing.T){ _,err,_:=resolveFixture(sql); if err==nil { t.Fatal("unsafe/unqualified SQL accepted") } })
	}
}
func TestSQLTreeBounds(t *testing.T) {
	count:=0
	limits:=config.DefaultReadValidation(); limits.MaxASTNodes=2
	if boundedTree(map[string]any{"a":[]any{1,2}},0,&count,limits) { t.Fatal("AST node bound ignored") }
	count=0; limits=config.DefaultReadValidation(); limits.MaxASTDepth=1
	if boundedTree(map[string]any{"a":[]any{1,2}},0,&count,limits) { t.Fatal("AST depth bound ignored") }
	if !parserBinding().Valid() { t.Fatal("fixture binding invalid") }
}

func FuzzSQLTree(f *testing.F) {
	for _,sql:=range []string{`SELECT id FROM analytics.sales`,`WITH q AS (SELECT id FROM analytics.sales) SELECT id FROM q`,`SELECT pg_sleep(1)`,`SELECT amount+$1 FROM analytics.sales`} { f.Add(sql) }
	f.Fuzz(func(t *testing.T,sql string){
		if len(sql)>4096 { return }
		raw,err:=pgquery.ParseToJSON(sql); if err!=nil { return }
		var tree map[string]any
		if json.Unmarshal([]byte(raw),&tree)!=nil { t.Fatal("native parser emitted invalid JSON") }
		count:=0; if !boundedTree(tree,0,&count,config.DefaultReadValidation()) { return }
		_,_,_=resolveFixture(sql)
	})
}
