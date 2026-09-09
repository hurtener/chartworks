package reporting

import (
	"context"
	"strings"
	"testing"
)

func TestPeriodAssistancePreservesUnrelatedSQL(t *testing.T){
	ctx:=context.Background()
	for _,sql:=range []string{
		"SELECT s.amount AS revenue FROM analytics.sales s WHERE s.created_at >= '2024-01-01' AND s.created_at < '2024-04-01' AND s.region = 'do not rewrite 2024-01-01' ORDER BY s.amount",
		"SELECT s.amount FROM analytics.sales s WHERE s.region = $1 AND (s.created_at >= '2024-01-01'::date AND s.created_at < '2024-04-01'::date) /* preserve comment */",
	}{
		out,err:=parameterizePeriod(ctx,sql,[]string{"s","created_at"},2)
		if err!=nil{t.Fatalf("assistance rejected supported predicate: %v",err)}
		want:=strings.Replace(sql,"'2024-01-01'","$2",1);want=strings.Replace(want,"'2024-04-01'","$3",1)
		if out!=want{t.Fatalf("unrelated SQL changed\nwant=%s\ngot=%s",want,out)}
	}
	for _,sql:=range []string{
		"SELECT amount FROM analytics.sales WHERE created_at >= '2024-01-01' OR created_at < '2024-04-01'",
		"SELECT amount FROM analytics.sales WHERE created_at >= '2024-01-01' AND created_at < '2024-04-01' AND created_at < '2024-02-01'",
		"SELECT amount FROM analytics.sales WHERE created_at >= 'not-a-date' AND created_at < '2024-04-01'",
		"SELECT amount FROM analytics.sales WHERE created_at BETWEEN '2024-01-01' AND '2024-04-01'",
		"DELETE FROM analytics.sales WHERE created_at >= '2024-01-01' AND created_at < '2024-04-01'",
		"SELECT 1; SELECT 2",
	}{if _,err:=parameterizePeriod(ctx,sql,[]string{"created_at"},1);err==nil{t.Fatal("unsafe ambiguous assistance accepted",sql)}}
	if _,err:=parameterizePeriod(ctx,"SELECT 1",[]string{"x"},0);err==nil{t.Fatal("invalid slot accepted")}
	cancelled,cancel:=context.WithCancel(ctx);cancel()
	if _,err:=parameterizePeriod(cancelled,"SELECT 1",[]string{"x"},1);err==nil{t.Fatal("cancelled parse accepted")}
}

func TestRenameAssistanceUsesASTAndPreservesOutputNames(t *testing.T){
	ctx:=context.Background()
	deps:=[]Dependency{{Dataset:"sales",Schema:"analytics",Name:"sales"}}
	rename:=[]Rename{{Dataset:"sales",Column:"amount",From:"amount",To:"net_amount"}}
	cases:=[]struct{before,after string}{
		{"SELECT id, amount FROM analytics.sales WHERE amount > 0 ORDER BY id",`SELECT id, "net_amount" AS "amount" FROM analytics.sales WHERE "net_amount" > 0 ORDER BY id`},
		{"SELECT s.amount AS revenue FROM analytics.sales s WHERE s.amount > $1 AND s.note = 'amount' /* amount */",`SELECT s."net_amount" AS revenue FROM analytics.sales s WHERE s."net_amount" > $1 AND s.note = 'amount' /* amount */`},
		{`SELECT "amount" FROM "analytics"."sales" ORDER BY "amount"`, `SELECT "net_amount" AS "amount" FROM "analytics"."sales" ORDER BY "net_amount"`},
		{"SELECT id FROM analytics.sales","SELECT id FROM analytics.sales"},
	}
	for _,tt:=range cases{
		out,err:=renameSQL(ctx,tt.before,rename,deps)
		if err!=nil || out!=tt.after{t.Fatalf("exact rename failed: %v\nwant=%s\ngot=%s",err,tt.after,out)}
	}
	for _,sql:=range []string{
		"SELECT * FROM analytics.sales",
		"SELECT a.amount FROM analytics.sales a JOIN analytics.other b ON a.id=b.id",
		"SELECT amount FROM sales",
		"SELECT amount FROM analytics.other",
		"WITH q AS (SELECT amount FROM analytics.sales) SELECT amount FROM q",
		"SELECT amount FROM analytics.sales WHERE EXISTS (SELECT 1)",
		"SELECT id AS amount FROM analytics.sales ORDER BY amount",
		"SELECT amount FROM analytics.sales UNION SELECT amount FROM analytics.sales",
	}{if _,err:=renameSQL(ctx,sql,rename,deps);err==nil{t.Fatal("ambiguous rename accepted",sql)}}
	if _,err:=renameSQL(ctx,"SELECT amount FROM analytics.sales",[]Rename{{Dataset:"wrong",From:"amount",To:"x"}},deps);err==nil{t.Fatal("foreign dependency rename accepted")}
	if quoteSQLIdentifier(`unusual"name`)!=`"unusual""name"`{t.Fatal("identifier escaping")}
}

func TestASTEditBoundsAndEquivalence(t *testing.T){
	ctx:=context.Background()
	document,_,err:=parseAssistance(ctx,"SELECT id FROM analytics.sales");if err!=nil{t.Fatal(err)}
	if _,err:=applySQLEdits(ctx,"SELECT id FROM analytics.sales",document,[]sqlEdit{{start:7,end:9,text:"amount"}});err==nil{t.Fatal("unproved AST edit accepted")}
	for _,edit:=range []sqlEdit{{start:-1,end:1,text:"x"},{start:1,end:999,text:"x"},{start:3,end:1,text:"x"}}{if _,err:=applySQLEdits(ctx,"SELECT id FROM analytics.sales",document,[]sqlEdit{edit});err==nil{t.Fatal("invalid span accepted")}}
	if _,_,err:=parseAssistance(ctx,strings.Repeat("x",65537));err==nil{t.Fatal("unbounded input accepted")}
	budget:=0;if walkAST(map[string]any{},0,&budget,func(map[string]any)error{return nil})==nil{t.Fatal("unbounded AST walk")}
	if _,_,err:=identifierSpan(`"unterminated`,0);err==nil{t.Fatal("unterminated identifier accepted")}
	if _,err:=quotedLiteralSpan("'other'",0,"different");err==nil{t.Fatal("literal span was not tied to parsed value")}
}

func FuzzSQLAssistSpanSafety(f *testing.F){
	f.Add("SELECT amount FROM analytics.sales WHERE note = 'amount'")
	f.Add("SELECT id FROM analytics.sales WHERE created_at >= '2024-01-01' AND created_at < '2024-02-01'")
	f.Fuzz(func(t *testing.T,sql string){
		if len(sql)>4096{t.Skip()}
		ctx:=context.Background()
		for _,assist:=range []func()(string,error){func()(string,error){return parameterizePeriod(ctx,sql,[]string{"created_at"},1)},func()(string,error){return renameSQL(ctx,sql,[]Rename{{Dataset:"sales",Column:"amount",From:"amount",To:"net_amount"}},[]Dependency{{Dataset:"sales",Schema:"analytics",Name:"sales"}})}}{
			out,err:=assist();if err==nil{if _,_,err:=parseAssistance(ctx,out);err!=nil{t.Fatal("assistant produced invalid SQL")}}
		}
	})
}
