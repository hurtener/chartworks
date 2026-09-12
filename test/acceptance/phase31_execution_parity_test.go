package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

func testPhase31ExplicitExecution(t *testing.T) {
	f:=newPhase31Fixture(t,true)
	d:=phase27Copy(t,f.domain.base)
	d.SQL="SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
	d.Parameters=[]reporting.Parameter{{Name:"minimum",Type:"integer",Required:true,Default:&reporting.Value{Literal:"1"},Min:"1",Max:"2"}}
	f.domain.block(t,"p31-filters",d)
	request:=phase31Request("block","p31-filters","p31-filter-first")
	request.Outputs=[]string{"table-second","table-main"}
	before:=f.domain.attemptCount(t)
	first,err:=f.service.Run(t.Context(),f.domain.execute,request)
	if err!=nil || first.State!="succeeded" || f.domain.attemptCount(t)!=before+1 { t.Fatal("initial actual execution",first,err) }
	request.Key="p31-filter-changed"
	request.Arguments=[]reporting.Argument{{Name:"minimum",Value:reporting.Value{Literal:"2"}}}
	before=f.domain.attemptCount(t)
	second,err:=f.service.Run(t.Context(),f.domain.execute,request)
	if err!=nil || second.Run==first.Run || f.domain.attemptCount(t)!=before+1 { t.Fatal("changed filters did not request a new single run",second,err) }
	reader:=f.reader(t,"p31-filters","")
	old,err:=f.service.View(t.Context(),reader,reporting.ReportingViewRequest{Kind:"block",Run:first.Run,Output:"table-second",Limit:10})
	if err!=nil || old.PageBounds.Total!=2 || old.Outputs[0].ID!="table-second" { t.Fatal("old artifact changed with filters/output order",old,err) }
	newer,err:=f.service.View(t.Context(),reader,reporting.ReportingViewRequest{Kind:"block",Run:second.Run,Output:"table-second",Limit:10})
	if err!=nil || newer.PageBounds.Total!=1 || newer.Output.Table.Rows[0][0].Value!="2" { t.Fatal("typed filter did not affect actual query",newer,err) }
	before=f.domain.attemptCount(t)
	if replay,err:=f.service.Run(t.Context(),f.domain.execute,request); err!=nil || replay.Run!=second.Run || f.domain.attemptCount(t)!=before { t.Fatal("idempotent receipt re-executed",replay,err) }
	request.Key="p31-read-only-attempt"
	if _,err:=f.service.Run(t.Context(),reader,request); !errors.Is(err,access.ErrForbidden) || f.domain.attemptCount(t)!=before { t.Fatal("read-only/viewer authority could execute",err) }
	request.Key="p31-invalid-filter"
	request.Arguments[0].Value.Literal="3"
	if _,err:=f.service.Run(t.Context(),f.domain.execute,request); !errors.Is(err,reporting.ErrInvalid) || f.domain.attemptCount(t)!=before { t.Fatal("out-of-domain filter reached warehouse",err) }

	// A newer published revision gains a dynamic query after the consumer has
	// described revision one. That publication must not change the already
	// explicit run target, or silently acquire dynamic-generation consent.
	report:=phase29Text("Exact published consent")
	report.Widgets=append(report.Widgets,phase29BlockWidget("frozen","p31-filters",1,"table-main"))
	state:=f.domain.report(t,"p31-consent",report,true)
	original,err:=f.service.Describe(t.Context(),f.domain.execute,reporting.ReportingDescribeRequest{Target:reporting.DeliveryTarget{Kind:"report",ID:state.ID},Outputs:[]string{}})
	if err!=nil || original.Dynamic || original.Resource.Target.Revision!=1 { t.Fatal(original,err) }
	report.Widgets=append(report.Widgets,f.domain.queryWidget())
	state,err=f.domain.documents.Edit(t.Context(),f.domain.author,"report",state.ID,state.Version,reporting.DocumentReference{},report)
	if err!=nil { t.Fatal(err) }
	state=phase29Publish(t,f.domain.documents,f.domain.author,state)
	beforeModels:=f.domain.f.model.requests.Load()
	pinned:=phase31Request("report",state.ID,"p31-consent-old")
	oldRun,err:=f.service.Run(t.Context(),f.domain.execute,pinned)
	if err!=nil || oldRun.Target.Revision!=1 || f.domain.f.model.requests.Load()!=beforeModels { t.Fatal("publication race widened dynamic execution",oldRun,err) }
	pinned.Target.Revision=2
	pinned.Key="p31-consent-required"
	before=f.domain.attemptCount(t)
	if _,err:=f.service.Run(t.Context(),f.domain.execute,pinned); !errors.Is(err,reporting.ErrInvalid) || f.domain.attemptCount(t)!=before || f.domain.f.model.requests.Load()!=beforeModels { t.Fatal("dynamic model generation lacked explicit consent",err) }
	pinned.Dynamic=true
	pinned.Key="p31-consent-granted"
	dynamic,err:=f.service.Run(t.Context(),f.domain.execute,pinned)
	if err!=nil || dynamic.State!="completed" || f.domain.f.model.requests.Load()<=beforeModels { t.Fatal("authorized dynamic lane was not actually executed",dynamic,err) }
	beforeSource,beforeModels:=f.domain.f.f.lookups.Load(),f.domain.f.model.requests.Load()
	view,err:=f.service.View(t.Context(),f.reader(t,"",state.ID),reporting.ReportingViewRequest{Kind:"report",Run:dynamic.Run,Page:"main",Widget:"dynamic",Limit:1})
	if err!=nil || view.Output==nil || view.Output.Table==nil || view.Trust!=nil || view.Observed==nil { t.Fatal("dynamic projection lost values/provenance or inherited certification",view,err) }
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("view regenerated a dynamic question") }
}

func phase31HTTP[Out any](t testing.TB,f *phase31Fixture,token,operation string,input any) (out Out) {
	t.Helper()
	response:=callProtected(t,f.handler,http.MethodPost,"/v1/reporting/"+operation,token,phase31Wire(t,input),nil)
	if response.Code!=http.StatusOK { t.Fatalf("HTTP %s: %d %s",operation,response.Code,response.Body.String()) }
	if err:=json.Unmarshal(response.Body.Bytes(),&out); err!=nil { t.Fatal(err) }
	return
}

func phase31Equivalent(t testing.TB,first,second any,label string) {
	t.Helper()
	var a,b any
	if err:=json.Unmarshal(phase31Wire(t,first),&a); err!=nil { t.Fatal(err) }
	if err:=json.Unmarshal(phase31Wire(t,second),&b); err!=nil { t.Fatal(err) }
	if !reflect.DeepEqual(a,b) { t.Fatalf("%s provider contract diverged\n%s\n%s",label,phase31Wire(t,a),phase31Wire(t,b)) }
}

func testPhase31ProviderParity(t *testing.T) {
	f:=newPhase31Fixture(t,false)
	f.domain.block(t,"p31-parity",f.domain.base)
	mcpClient,httpClient:=f.client(t,f.scopes,true),f.client(t,f.scopes,false)
	bearer:=f.token(t,f.domain.execute.User(),f.domain.execute.Tenant(),f.scopes,false)
	search:=reporting.ReportingSearchRequest{Kind:"block",Query:"",Locale:"en",Limit:20}
	hSearch:=phase31HTTP[reporting.ReportingSearchResult](t,f,bearer,"search",search)
	mSearch:=phase22Call[reporting.ReportingSearchResult](t,mcpClient,"reporting_search",search)
	sSearch,err:=httpClient.SearchReporting(t.Context(),search)
	if err!=nil { t.Fatal(err) }
	phase31Equivalent(t,hSearch,mSearch,"search HTTP/MCP")
	phase31Equivalent(t,hSearch,sSearch,"search HTTP/SDK")
	description:=reporting.ReportingDescribeRequest{Target:reporting.DeliveryTarget{Kind:"block",ID:"p31-parity",Revision:1},Locale:"en",Outputs:[]string{"table-second","table-main"}}
	hDescription:=phase31HTTP[reporting.ReportingDescription](t,f,bearer,"describe",description)
	mDescription:=phase22Call[reporting.ReportingDescription](t,mcpClient,"reporting_describe",description)
	sDescription,err:=httpClient.DescribeReporting(t.Context(),description)
	if err!=nil { t.Fatal(err) }
	phase31Equivalent(t,hDescription,mDescription,"describe HTTP/MCP")
	phase31Equivalent(t,hDescription,sDescription,"describe HTTP/SDK")
	request:=phase31Request("block","p31-parity","p31-cross-provider-key")
	request.Outputs=[]string{"table-second","table-main"}
	before:=f.domain.attemptCount(t)
	mRun:=phase22Call[reporting.ReportingRunResult](t,mcpClient,"reporting_run",request)
	hRun:=phase31HTTP[reporting.ReportingRunResult](t,f,bearer,"run",request)
	sRun,err:=httpClient.RunReporting(t.Context(),request)
	if err!=nil { t.Fatal(err) }
	phase31Equivalent(t,mRun,hRun,"idempotent run MCP/HTTP")
	phase31Equivalent(t,mRun,sRun,"idempotent run MCP/SDK")
	if f.domain.attemptCount(t)!=before+1 { t.Fatal("provider parity/replay duplicated execution") }
	beforeSource,beforeModels:=f.domain.f.f.lookups.Load(),f.domain.f.model.requests.Load()
	selection:=reporting.ReportingViewRequest{Kind:"block",Run:mRun.Run,Output:"table-second",Offset:1,Limit:1}
	hView:=phase31HTTP[reporting.ReportingViewResult](t,f,bearer,"view",selection)
	mView:=phase22Call[reporting.ReportingViewResult](t,mcpClient,"reporting_view",selection)
	sView,err:=httpClient.ViewReporting(t.Context(),selection)
	if err!=nil { t.Fatal(err) }
	phase31Equivalent(t,hView,mView,"selected view HTTP/MCP")
	phase31Equivalent(t,hView,sView,"selected view HTTP/SDK")
	var fallback struct { Structured json.RawMessage `json:"structuredContent"`; Content []struct { Type string `json:"type"`; Text string `json:"text"` } `json:"content"` }
	if err:=json.Unmarshal(phase22RPC(t,mcpClient,"tools/call",map[string]any{"name":"reporting_view","arguments":selection}),&fallback); err!=nil { t.Fatal(err) }
	if len(fallback.Content)!=1 || fallback.Content[0].Type!="text" { t.Fatal("text-only host fallback absent") }
	var structured,textual any
	if json.Unmarshal(fallback.Structured,&structured)!=nil || json.Unmarshal([]byte(fallback.Content[0].Text),&textual)!=nil || !reflect.DeepEqual(structured,textual) { t.Fatal("structured and text fallbacks disagree") }
	list:=reporting.ReportingRunsRequest{Kind:"block",Resource:"p31-parity",Limit:20}
	hRuns:=phase31HTTP[reporting.ReportingRunsResult](t,f,bearer,"runs",list)
	mRuns:=phase22Call[reporting.ReportingRunsResult](t,mcpClient,"reporting_runs",list)
	sRuns,err:=httpClient.ListReportingRuns(t.Context(),list)
	if err!=nil { t.Fatal(err) }
	phase31Equivalent(t,hRuns,mRuns,"catalog HTTP/MCP")
	phase31Equivalent(t,hRuns,sRuns,"catalog HTTP/SDK")
	selection.Output="does-not-exist"
	denied:=callProtected(t,f.handler,http.MethodPost,"/v1/reporting/view",bearer,phase31Wire(t,selection),nil)
	if denied.Code!=http.StatusNotFound || !strings.Contains(denied.Body.String(),"not_found") { t.Fatal("HTTP missing-selection code",denied.Code,denied.Body.String()) }
	failure:=phase22RawTool(t,mcpClient,"reporting_view",selection)
	if !failure.IsError || !bytes.Contains(failure.Structured,[]byte(`"code":"not_found"`)) { t.Fatal("MCP missing-selection code",string(failure.Structured)) }
	if _,err=httpClient.ViewReporting(t.Context(),selection); err==nil { t.Fatal("SDK missing selection unexpectedly succeeded") } else {
		var status *cw.StatusError
		if !errors.As(err,&status) || status.Status!=http.StatusNotFound { t.Fatal("SDK missing-selection class",err) }
	}
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("portable view/list/error reads invoked a source or model") }
}

func testPhase31ProviderBounds(t *testing.T) {
	f:=newPhase31Fixture(t,false)
	f.domain.block(t,"p31-bounds",f.domain.base)
	run:=f.run(t,"block","p31-bounds","p31-bounds-run")
	reader:=f.reader(t,"p31-bounds","")
	beforeSource,beforeModels:=f.domain.f.f.lookups.Load(),f.domain.f.model.requests.Load()
	for _, selection:=range []reporting.ReportingViewRequest{
		{Kind:"block",Run:run.Run,Limit:1001},
		{Kind:"block",Run:run.Run,Offset:-1,Limit:1},
		{Kind:"block",Run:run.Run,Offset:3,Limit:1},
		{Kind:"block",Run:run.Run,Page:"invented",Limit:1},
		{Kind:"block",Run:run.Run,Output:"../table-main",Limit:1},
	} {
		view,err:=f.service.View(t.Context(),reader,selection)
		if !errors.Is(err,reporting.ErrInvalid) || view.Output!=nil || view.Text!=nil { t.Fatal("invalid selection produced data",view,err) }
	}
	limits:=config.DefaultReportingViewer()
	limits.MaxOutputs=1
	limited,err:=reporting.NewDelivery(f.domain.blocks,f.domain.runs,f.domain.documents,f.domain.compositions,f.domain.f.f.db,limits)
	if err!=nil { t.Fatal(err) }
	view,err:=limited.View(t.Context(),reader,reporting.ReportingViewRequest{Kind:"block",Run:run.Run,Limit:1})
	if !errors.Is(err,reporting.ErrBudget) || view.Output!=nil || view.Summary.Run!="" { t.Fatal("provider output cap was a hint instead of an enforced bound",view,err) }
	readScopes:=[]string{"reporting.read","mcp.use","cw.block.read:p31-bounds","cw.execution_context.use:"+f.domain.base.Context}
	bearer:=f.token(t,f.domain.execute.User(),f.domain.execute.Tenant(),readScopes,false)
	for _, body:=range []string{
		`{"kind":"block","run":"`+run.Run+`","output":"table-main","offset":0,"limit":1,"sql":"SELECT secret"}`,
		`{"kind":"block","run":"`+run.Run+`","output":"table-main","offset":0,"limit":1,"authorization":"Bearer forged"}`,
		`{"kind":"block","run":"`+run.Run+`","output":"table-main","offset":0,"limit":1,"_meta":{"ui":{"visibility":["app"]}}}`,
	} {
		response:=callProtected(t,f.handler,http.MethodPost,"/v1/reporting/view",bearer,[]byte(body),nil)
		if response.Code!=http.StatusBadRequest || !strings.Contains(response.Body.String(),"invalid_request") { t.Fatal("closed request accepted executable/authority extensions",response.Code,response.Body.String()) }
	}
	response:=callProtected(t,f.handler,http.MethodPost,"/v1/reporting/view",bearer,[]byte(strings.Repeat(" ",17<<20)+"{}"),nil)
	if response.Code!=http.StatusRequestEntityTooLarge { t.Fatal("request byte ceiling",response.Code,response.Body.String()) }
	foreign:=f.token(t,"foreign-reader","foreign-tenant",readScopes,false)
	response=callProtected(t,f.handler,http.MethodPost,"/v1/reporting/view",foreign,phase31Wire(t,reporting.ReportingViewRequest{Kind:"block",Run:run.Run,Limit:1}),nil)
	if response.Code!=http.StatusNotFound || strings.Contains(response.Body.String(),run.Run) { t.Fatal("cross-tenant artifact disclosure",response.Code,response.Body.String()) }
	request:=phase31Request("block","p31-bounds","p31-hint-not-authority")
	response=callProtected(t,f.handler,http.MethodPost,"/v1/reporting/run",bearer,phase31Wire(t,request),nil)
	if response.Code!=http.StatusForbidden { t.Fatal("app visibility replaced signed execution action",response.Code,response.Body.String()) }
	cancelled,cancel:=context.WithCancel(t.Context());cancel()
	if _,err=f.service.View(cancelled,reader,reporting.ReportingViewRequest{Kind:"block",Run:run.Run,Limit:1}); !errors.Is(err,context.Canceled) { t.Fatal("cancelled provider read was not cancelled",err) }
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("provider failure path executed a source/model") }
}
