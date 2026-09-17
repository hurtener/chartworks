from pathlib import Path


def edit(path,before,after,count=1):
 p=Path(path);text=p.read_text()
 if text.count(before)!=count:raise SystemExit(f'{path}: changed anchor {text.count(before)}')
 p.write_text(text.replace(before,after))


def create(path,text):
 p=Path(path)
 if p.exists():raise SystemExit(f'{path}: exists')
 p.write_text(text)

edit('internal/exec/business_model.go','''		return category == "integer" || category == "number"''','''  if strings.HasPrefix(native,"float") || strings.HasPrefix(native,"double") || native=="real" {return false}
		return category == "integer" || category == "number"''')

create('internal/exec/business_sql_test.go',r'''package exec

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "reflect"
 "strings"
 "sync"
 "testing"

 pgquery "github.com/wasilibs/go-pgquery"
)

func businessFixtureConstraint() BusinessConstraint {
 return BusinessConstraint{Resolution:Hash("reviewed-threshold"),Dataset:"sales",Column:"amount",SourceRevision:1,Kind:"number",Operator:"gte",Nulls:"exclude",Unit:"USD",Precision:38,Scale:6,Value:"9007199254740993.125"}
}

func TestBusinessBindingConjunctionAndParameterReceipts(t *testing.T) {
 ctx:=context.Background();binding:=parserBinding();constraint:=businessFixtureConstraint()
 cases:=[]struct{name,sql,fragment string;params []Parameter;count int}{
  {"plain","SELECT id FROM analytics.sales ORDER BY id",`"sales"."amount" >=`,nil,1},
  {"or-preserved","SELECT id FROM analytics.sales WHERE id=1 OR id=2 ORDER BY id",`WHERE (id=1 OR id=2) AND (`,nil,1},
  {"alias","SELECT s.id FROM analytics.sales AS s WHERE s.name='a''b'",`"s"."amount" >=`,nil,1},
  {"join","SELECT s.id FROM analytics.sales s JOIN analytics.items i ON s.id=i.sale_id",`"s"."amount" >=`,nil,1},
  {"group-and-order","SELECT active,sum(amount) FROM analytics.sales GROUP BY active ORDER BY active",`WHERE`,nil,1},
  {"repeated-parameter","SELECT id FROM analytics.sales WHERE id=$1 OR id=$1 LIMIT 10",`WHERE (id=$1 OR id=$2) AND (`,[]Parameter{{Kind:"integer",Value:"1"}},3},
  {"parameter-before-filter","SELECT $1 AS label,id FROM analytics.sales ORDER BY id LIMIT $2",`WHERE`,[]Parameter{{Kind:"text",Value:"public label"},{Kind:"integer",Value:"20"}},3},
  {"trailing-semicolon","SELECT id FROM analytics.sales;",`WHERE`,nil,1},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  originalBinding:=binding.Clone();originalConstraint:=constraint
  out,err:=BindBusinessConstraints(ctx,binding,tc.sql,tc.params,[]BusinessConstraint{constraint})
  if err!=nil || !strings.Contains(out.SQL,tc.fragment) || len(out.Parameters)!=tc.count || out.Receipt.Validation!=nil || len(out.Receipt.Bindings)!=1 {t.Fatalf("closed conjunction or receipt failed: %v",err)}
  if _,err:=pgquery.ParseToJSON(out.SQL);err!=nil {t.Fatal("transformation did not produce parsable PostgreSQL",err)}
  indexes:=out.Receipt.Bindings[0].Parameters
  if len(indexes)!=1 || indexes[0]<1 || indexes[0]>len(out.Parameters) || out.Parameters[indexes[0]-1].Value!=constraint.Value {t.Fatal("binding receipt does not identify exact scalar")}
  if out.Receipt.SourceBinding!=Hash(binding) || out.Receipt.Statement!=Hash([]any{out.SQL,out.Parameters}) {t.Fatal("receipt lost exact source or parameter seal")}
  if constraint!=originalConstraint || !reflect.DeepEqual(binding,originalBinding){t.Fatal("shared inputs mutated")}
  wire,_:=json.Marshal(out)
  if strings.Contains(string(wire),constraint.Value) || strings.Contains(fmt.Sprintf("%v %#v",out,out),constraint.Value) {t.Fatal("ordinary bound-query projection leaked scalar")}
 })}
}

func TestBusinessBindingAggregatesAndNulls(t *testing.T){
 binding:=parserBinding();ctx:=context.Background()
 for _,aggregation:=range []string{"sum","average","minimum","maximum","count","distinct_count"}{
  t.Run(aggregation,func(t *testing.T){
   c:=businessFixtureConstraint();c.Aggregation=aggregation
   out,err:=BindBusinessConstraints(ctx,binding,"SELECT active,sum(amount) FROM analytics.sales GROUP BY active HAVING count(*)>0 OR sum(amount)>0 ORDER BY active",nil,[]BusinessConstraint{c})
   if err!=nil || !strings.Contains(out.SQL,"HAVING (count(*)>0 OR sum(amount)>0) AND (") || strings.Contains(out.SQL," WHERE ") || out.Receipt.Bindings[0].Aggregation!=aggregation {t.Fatal("aggregate constraint fell into row-filter scope",err)}
   if _,err:=pgquery.ParseToJSON(out.SQL);err!=nil{t.Fatal(err)}
  })
 }
 for _,nulls:=range []string{"exclude","include","only"}{
  t.Run(nulls,func(t *testing.T){
   c:=businessFixtureConstraint();c.Nulls=nulls
   if nulls=="only"{c.Null=true;c.Value=""}
   out,err:=BindBusinessConstraints(ctx,binding,"SELECT id FROM analytics.sales WHERE id=1 OR id=2",nil,[]BusinessConstraint{c})
   if err!=nil{t.Fatal(err)}
   if nulls=="include" && !strings.Contains(out.SQL,"IS NULL OR") {t.Fatal("NULL inclusion not explicit")}
   if nulls=="only" && (!strings.Contains(out.SQL,"IS NULL") || len(out.Parameters)!=0){t.Fatal("null predicate bound an unused scalar")}
   if nulls=="exclude" && strings.Contains(out.SQL,"IS NULL OR"){t.Fatal("ordinary comparison widened NULL semantics")}
   if _,err:=pgquery.ParseToJSON(out.SQL);err!=nil{t.Fatal(err)}
  })
 }
 for _,bounds:=range []string{"[]","[)","(]","()"}{
  c:=businessFixtureConstraint();c.Operator="range";c.Bounds=bounds;c.Value="1.5";c.Upper="9.75"
  out,err:=BindBusinessConstraints(ctx,binding,"SELECT id FROM analytics.sales",nil,[]BusinessConstraint{c})
  if err!=nil || len(out.Parameters)!=2 || out.Parameters[0].Value!="1.5" || out.Parameters[1].Value!="9.75"{t.Fatal("exact interval bound order lost",err)}
  lower,upper:=">","<";if bounds[0]=='['{lower=">="};if bounds[1]==']'{upper="<="}
  if !strings.Contains(out.SQL,`"amount" `+lower) || !strings.Contains(out.SQL,`"amount" `+upper){t.Fatal("interval inclusivity changed")}
 }
}

func TestBusinessBindingRejectsAmbiguityAndForeignInputs(t *testing.T){
 binding:=parserBinding();c:=businessFixtureConstraint();ctx:=context.Background()
 for _,sql:=range []string{
  "WITH q AS (SELECT * FROM analytics.sales) SELECT * FROM q",
  "SELECT * FROM (SELECT * FROM analytics.sales) s",
  "SELECT id FROM analytics.sales UNION ALL SELECT id FROM analytics.sales",
  "SELECT a.id FROM analytics.sales a JOIN analytics.sales b ON a.id=b.id",
  "SELECT id FROM sales",
  "SELECT id FROM private.sales",
  "SELECT id FROM analytics.items",
  "SELECT id FROM analytics.sales --comment",
  "SELECT id FROM analytics.sales /*comment*/",
  "SELECT id FROM analytics.sales; SELECT 1",
  "SELECT id FROM analytics.sales WHERE name=$$private$$",
  "SELECT id FROM analytics.sales WHERE id=\x1f0\x1f",
  "SELECT id FROM analytics.sales WHERE name='a\\b'",
 }{
  out,err:=BindBusinessConstraints(ctx,binding,sql,nil,[]BusinessConstraint{c})
  if err==nil || out.SQL!="" || len(out.Parameters)!=0 || out.Receipt.Validation!=nil{t.Fatal("unsupported shape returned partial bound output")}
 }
 for _,mutate:=range []func(*BusinessConstraint){
  func(c *BusinessConstraint){c.Dataset="foreign"},func(c *BusinessConstraint){c.Column="custom"},
  func(c *BusinessConstraint){c.SourceRevision++},func(c *BusinessConstraint){c.Resolution="bad"},
  func(c *BusinessConstraint){c.Nulls="coalesce"},func(c *BusinessConstraint){c.Value="1e6"},
  func(c *BusinessConstraint){c.Value="NaN"},func(c *BusinessConstraint){c.Operator="execute"},
  func(c *BusinessConstraint){c.Precision=77},func(c *BusinessConstraint){c.Scale=39},
  func(c *BusinessConstraint){c.Kind="sql"},func(c *BusinessConstraint){c.Value="1'; DROP TABLE x;--"},
 }{
  changed:=c;mutate(&changed)
  if err:=ValidateBusinessConstraints(binding,[]BusinessConstraint{changed});err==nil {t.Fatal("invalid typed constraint passed admission")}
 }
 unsafe:=binding.Clone();unsafe.Relations[0].Columns[1].NativeType="double precision";unsafe.Relations[0].Columns[1].Category="numeric"
 if err:=ValidateBusinessConstraints(unsafe,[]BusinessConstraint{c});!errors.Is(err,ErrUnsupported){t.Fatal("exact threshold admitted an approximate warehouse column",err)}
 cancel,cancelled:=context.WithCancel(ctx);cancelled()
 if _,err:=BindBusinessConstraints(cancel,binding,"SELECT id FROM analytics.sales",nil,[]BusinessConstraint{c});!errors.Is(err,context.Canceled){t.Fatal("cancelled binding continued",err)}
 for _,params:=range [][]Parameter{{{Kind:"number",Value:"NaN"}},{{Kind:"integer",Value:"1"}}}{
  if _,err:=BindBusinessConstraints(ctx,binding,"SELECT id FROM analytics.sales",params,[]BusinessConstraint{c});err==nil{t.Fatal("invalid or unused parameter accepted")}
 }
 var zero Plan
 if _,_,err:=zero.SQL(zero.candidate.owner,binding);err==nil{t.Fatal("business evidence replaced validator proof")}
}

func TestBusinessBindingDialectTransportAndConcurrentReuse(t *testing.T){
 for _,dialect:=range []string{"postgres","mysql","sqlserver","bigquery","snowflake","databricks"}{
  t.Run(dialect,func(t *testing.T){
   binding:=parserBinding();binding.Dialect=dialect;c:=businessFixtureConstraint()
   // Quoted identifiers preserve the reviewed case across warehouse dialects.
   sql:="SELECT "+businessQuote(dialect,"id")+" FROM "+businessQuote(dialect,"analytics")+"."+businessQuote(dialect,"sales")
   var wg sync.WaitGroup
   for i:=0;i<12;i++{wg.Add(1);go func(){defer wg.Done()
    out,err:=BindBusinessConstraints(context.Background(),binding,sql,nil,[]BusinessConstraint{c})
    if err!=nil || len(out.Parameters)!=1 || out.Parameters[0].Value!=c.Value || !strings.Contains(out.SQL,businessPlaceholder(dialect,1)){t.Error("dialect transport lost exact value",err)}
   }()};wg.Wait()
  })
 }
}

func FuzzBusinessConstraintStatement(f *testing.F){
 for _,sql:=range []string{"SELECT id FROM analytics.sales","SELECT id FROM analytics.sales WHERE id=1 OR id=2","SELECT '\\' FROM analytics.sales","SELECT * FROM analytics.sales;SELECT 1","","SELECT id FROM analytics.sales WHERE id=$1"}{f.Add(sql)}
 f.Fuzz(func(t *testing.T,sql string){
  out,err:=BindBusinessConstraints(context.Background(),parserBinding(),sql,nil,[]BusinessConstraint{businessFixtureConstraint()})
  if err!=nil {if out.SQL!="" || len(out.Parameters)!=0 {t.Fatal("failed transformation returned partial constraints")};return}
  if len(out.Parameters)!=1 || out.Parameters[0].Value!=businessFixtureConstraint().Value || out.Receipt.Validation!=nil || strings.ContainsRune(out.SQL,0x1f){t.Fatal("binding lost typed scalar or fabricated execution proof")}
 })
}
''')

create('test/acceptance/cw01_binding_test.go',r'''package acceptance

import (
 "context"
 "testing"

 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/nlqexec"
)

func cw01BindingAcceptance(t *testing.T,f *cw01Fixture){
 t.Helper();ctx:=context.Background()
 defer f.model.mode.Store(phase18RawResponse(t,"SELECT id, amount FROM analytics.sales ORDER BY id"))
 for _,tc:=range []struct{name,sql string}{
  {"disjunction","SELECT id,amount FROM analytics.sales WHERE id=1 OR id=2 ORDER BY id"},
  {"reviewed-alias","SELECT s.id,s.amount FROM analytics.sales AS s ORDER BY s.id"},
  {"existing-filter","SELECT id,amount FROM analytics.sales WHERE amount>=0 ORDER BY id"},
 }{t.Run(tc.name,func(t *testing.T){
  f.model.mode.Store(phase18RawResponse(t,tc.sql))
  plan:=f.plan(t,f.question("Show large sales",nlq.LanguageEnglish),"amount-required",cw01Number("10"))
  f.run(t,plan,1,true)
 })}
 t.Run("unsupported-shape-no-read",func(t *testing.T){
  f.model.mode.Store(phase18RawResponse(t,"WITH values AS (SELECT id,amount FROM analytics.sales) SELECT id,amount FROM values"))
  q:=f.question("Show large sales",nlq.LanguageEnglish);pending:=f.preflight(t,q)
  q.AnswerContext=pending.Route.AnswerContext;q.ClarificationQuery=pending.QueryID;q.Answers=append(q.Answers,f.answer(t,"amount-required",cw01Number("10")))
  out,err:=f.query.Plan(ctx,f.e,nlqexec.PlanRequest{QuestionRequest:q})
  if err==nil || out.QueryID!="" || out.Bindings!=nil {t.Fatal("unsupported shape silently dropped its typed constraint")}
 })
}
''')
edit('test/acceptance/cw01_test.go','''		if strings.Contains(string(raw), "Bearer") {
			t.Fatal("authority retained in replay request")
		}
''','''		if strings.Contains(string(raw), "Bearer") {
			t.Fatal("authority retained in replay request")
		}
  cw01BindingAcceptance(t,f)
''')
