package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/test/support"
	reportviewer "github.com/hurtener/chartworks/web/report-viewer"
)

func testPhase31AppsDiscovery(t *testing.T) {
	f := newPhase31Fixture(t, false)
	client := f.client(t, f.scopes, true)
	beforeSource, beforeModels := f.domain.f.f.lookups.Load(), f.domain.f.model.requests.Load()
	var tools struct { Tools []struct {
		Name string `json:"name"`
		Meta map[string]json.RawMessage `json:"_meta"`
		Annotations struct { ReadOnly bool `json:"readOnlyHint"`; Idempotent bool `json:"idempotentHint"` } `json:"annotations"`
	} `json:"tools"` }
	if err := json.Unmarshal(phase22RPC(t,client,"tools/list",map[string]any{}),&tools); err != nil { t.Fatal(err) }
	if len(tools.Tools) != 5 { t.Fatalf("actual reporting tools: %+v",tools.Tools) }
	seen := map[string]bool{}
	for _, tool := range tools.Tools {
		seen[tool.Name] = true
		if tool.Name == "reporting_run" && tool.Annotations.ReadOnly { t.Fatal("paid run advertised as read-only") }
		if tool.Name != "reporting_run" && !tool.Annotations.ReadOnly { t.Fatal("retained/catalog read advertised as an execution") }
		if tool.Name == "reporting_run" || tool.Name == "reporting_view" {
			var ui struct { URI string `json:"resourceUri"`; Visibility []string `json:"visibility"` }
			if err := json.Unmarshal(tool.Meta["ui"],&ui); err != nil || ui.URI != reportviewer.URI || !reflect.DeepEqual(ui.Visibility,[]string{"model","app"}) { t.Fatalf("established Apps metadata: %+v %v",ui,err) }
		}
	}
	for _, name := range []string{"reporting_search","reporting_describe","reporting_run","reporting_runs","reporting_view"} { if !seen[name] { t.Fatal("missing actual tool",name) } }
	var resources struct { Resources []struct { URI string `json:"uri"`; MIME string `json:"mimeType"` } `json:"resources"` }
	if err := json.Unmarshal(phase22RPC(t,client,"resources/list",map[string]any{}),&resources); err != nil { t.Fatal(err) }
	found := 0
	for _, resource := range resources.Resources { if resource.URI == reportviewer.URI { found++; if resource.MIME != mcpserver.AppMIME { t.Fatal("wrong Apps profile") } } }
	if found != 1 { t.Fatal("bundled app was missing or duplicated",found) }
	var resource struct { Contents []struct {
		URI string `json:"uri"`; MIME string `json:"mimeType"`; Text string `json:"text"`
		Meta struct { UI struct {
			CSP map[string][]string `json:"csp"`
			Permissions map[string]any `json:"permissions"`
		} `json:"ui"` } `json:"_meta"`
	} `json:"contents"` }
	if err := json.Unmarshal(phase22RPC(t,client,"resources/read",map[string]any{"uri":reportviewer.URI}),&resource); err != nil { t.Fatal(err) }
	if len(resource.Contents)!=1 || resource.Contents[0].Text!=reportviewer.HTML() || resource.Contents[0].MIME!=mcpserver.AppMIME || resource.Contents[0].URI!=reportviewer.URI { t.Fatal("provider did not serve the actual versioned component") }
	for _, domains := range resource.Contents[0].Meta.UI.CSP { if len(domains)!=0 { t.Fatal("bundled component unexpectedly requests network origins") } }
	if len(resource.Contents[0].Meta.UI.CSP)!=4 || len(resource.Contents[0].Meta.UI.Permissions)!=0 { t.Fatal("CSP or permissions metadata missing") }
	unprivileged := f.client(t,[]string{"mcp.use"},true)
	if err := json.Unmarshal(phase22RPC(t,unprivileged,"tools/list",map[string]any{}),&tools); err != nil || len(tools.Tools)!=0 { t.Fatal("tool visibility bypassed current actions",err) }
	for _, uri := range []string{reportviewer.URI,reportviewer.URI+"?token=ignored", "ui://chartworks/report-viewer/../../secret"} {
		raw,err := unprivileged.MCP(t.Context(),phase31Wire(t,map[string]any{"jsonrpc":"2.0","id":1,"method":"resources/read","params":map[string]any{"uri":uri}}))
		if err!=nil { t.Fatal(err) }
		var denial struct { Error *struct{Message string `json:"message"`} `json:"error"`; Result json.RawMessage `json:"result"` }
		if json.Unmarshal(raw,&denial)!=nil || denial.Error==nil || denial.Result!=nil || !strings.Contains(denial.Error.Message,"not_found") { t.Fatalf("unauthorized/aliased resource read: %s",raw) }
	}
	bindings,err := reportingapi.DeliveryMCPBindings(f.service,false)
	if err!=nil || len(bindings)!=4 { t.Fatal("metadata-only capability registration",len(bindings),err) }
	registry,err := reportingapi.DeliveryRegistry(false)
	if err!=nil || len(registry.Definitions())!=4 { t.Fatal("run advertised without configured executor",err) }
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("tool/resource discovery executed data or model work") }
}

func testPhase31CatalogAndRetainedReads(t *testing.T) {
	f := newPhase31Fixture(t,false)
	d := phase27Copy(t,f.domain.base)
	d.SQL += " /* P31_PRIVATE_SQL_CANARY */"
	f.domain.block(t,"p31-visible",d)
	f.domain.block(t,"p31-hidden",d)
	report := phase29Text("Authorized reusable report")
	report.Widgets = append(report.Widgets,phase29BlockWidget("first","p31-visible",1,"table-second"),phase29BlockWidget("second","p31-visible",2,"table-main"))
	f.domain.report(t,"p31-report",report,true)
	reader := f.reader(t,"p31-visible","p31-report")
	beforeSource,beforeModels := f.domain.f.f.lookups.Load(),f.domain.f.model.requests.Load()
	search,err := f.service.Search(t.Context(),reader,reporting.ReportingSearchRequest{Kind:"block",Limit:100})
	if err!=nil || len(search.Items)!=1 || search.Items[0].Target.ID!="p31-visible" { t.Fatalf("signed metadata search: %+v %v",search,err) }
	description,err := f.service.Describe(t.Context(),reader,reporting.ReportingDescribeRequest{Target:reporting.DeliveryTarget{Kind:"block",ID:"p31-visible"},Outputs:[]string{"table-second","table-main"}})
	if err!=nil || len(description.Outputs)!=2 || description.Outputs[0].ID!="table-second" || description.Resource.Target.Revision!=1 { t.Fatalf("selected published description: %+v %v",description,err) }
	for _, payload := range []any{search,description} {
		wire:=string(phase31Wire(t,payload))
		for _, forbidden := range []string{"P31_PRIVATE_SQL_CANARY","SELECT id",`"sql":`,`"credentials":`,`"prompt":`} { if strings.Contains(wire,forbidden) { t.Fatal("catalog leaked executable/private data",forbidden) } }
	}
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("catalog reads resolved a source/model") }
	beforeAttempts := f.domain.attemptCount(t)
	blockRun := f.run(t,"block","p31-visible","p31-block-run")
	if f.domain.attemptCount(t)!=beforeAttempts+1 { t.Fatal("block run did not execute exactly one actual bounded query") }
	beforeSource,beforeModels = f.domain.f.f.lookups.Load(),f.domain.f.model.requests.Load()
	for _, output := range []string{"table-second","table-main"} {
		first,err := f.service.View(t.Context(),reader,reporting.ReportingViewRequest{Kind:"block",Run:blockRun.Run,Output:output,Limit:1})
		if err!=nil || first.Output==nil || first.Output.Table==nil || len(first.Output.Table.Rows)!=1 || first.PageBounds.Total!=2 || first.PageBounds.Next==nil || *first.PageBounds.Next!=1 { t.Fatalf("exact first table page: %+v %v",first,err) }
		second,err := f.service.View(t.Context(),reader,reporting.ReportingViewRequest{Kind:"block",Run:blockRun.Run,Output:output,Offset:1,Limit:1})
		if err!=nil || second.Output==nil || second.Output.Table==nil || second.Output.Table.Rows[0][0].Value==first.Output.Table.Rows[0][0].Value || second.PageBounds.Next!=nil || second.Output.RetainedDigest!=first.Output.RetainedDigest { t.Fatalf("exact last page/full-artifact identity: %+v %v",second,err) }
		if first.Trust==nil || first.Observed==nil || first.Timezone!="UTC" { t.Fatal("retained provenance omitted") }
	}
	runs,err := f.service.Runs(t.Context(),reader,reporting.ReportingRunsRequest{Kind:"block",Resource:"p31-visible",Limit:20})
	if err!=nil || len(runs.Items)!=1 || runs.Items[0].Run!=blockRun.Run { t.Fatal("ordinary retained catalog",runs,err) }
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("paging or catalog read re-executed source/model work") }
	composed := f.run(t,"report","p31-report","p31-composed")
	beforeSource,beforeModels = f.domain.f.f.lookups.Load(),f.domain.f.model.requests.Load()
	view,err := f.service.View(t.Context(),reader,reporting.ReportingViewRequest{Kind:"report",Run:composed.Run,Page:"main",Widget:"first",Limit:1})
	if err!=nil || view.Output==nil || view.Output.Table==nil || view.Selection.Output!="table-second" || len(view.Outputs)!=1 { t.Fatalf("selected composed output received another union member or no values: %+v %v",view,err) }
	if _,err:=f.service.View(t.Context(),reader,reporting.ReportingViewRequest{Kind:"report",Run:composed.Run,Page:"main",Widget:"first",Output:"table-main",Limit:1}); !errors.Is(err,access.ErrNotFound) { t.Fatal("output union crossed widget selection",err) }
	runs,err = f.service.Runs(t.Context(),reader,reporting.ReportingRunsRequest{Kind:"report",Resource:"p31-report",Limit:20})
	if err!=nil || len(runs.Items)!=1 || runs.Items[0].Run!=composed.Run { t.Fatal("composition catalog did not use ordinary artifact entry",runs,err) }
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("composition projection called source/model") }
	for _, scopes := range [][]string{{"reporting.read","cw.block.read:p31-visible"},{"reporting.read","cw.execution_context.use:"+d.Context}} {
		denied:=phase27Actor(t,f.domain.f,"revoked-viewer",scopes)
		got,err:=f.service.View(t.Context(),denied,reporting.ReportingViewRequest{Kind:"block",Run:blockRun.Run,Output:"table-main",Limit:1})
		if err==nil || got.Output!=nil || got.Trust!=nil || got.Summary.Run!="" { t.Fatal("current artifact/context authority denial returned values or provenance",got,err) }
	}
}

func testPhase31ArtifactStates(t *testing.T) {
	f:=newPhase31Fixture(t,false)
	f.domain.block(t,"p31-state-block",f.domain.base)
	d:=phase29Text("Private retained report")
	d.Widgets=append(d.Widgets,phase29BlockWidget("block","p31-state-block",1,"table-main"))
	state:=f.domain.report(t,"p31-private-report",d,false)
	preview,err:=f.domain.compositions.Admit(t.Context(),f.domain.execute,"report",state.ID,reporting.CompositionRequest{Key:"p31-preview",Preview:true,Reference:reporting.DocumentReference{Revision:1}})
	if err!=nil { t.Fatal(err) }
	pending,err:=f.service.View(t.Context(),f.domain.execute,reporting.ReportingViewRequest{Kind:"report",Run:preview.ID,Page:"main",Widget:"block",Limit:1})
	if err!=nil || !pending.Summary.Private || pending.Output!=nil || pending.Summary.State=="completed" { t.Fatal("pending/private state fabricated completion",pending,err) }
	preview,err=f.domain.compositions.Run(t.Context(),f.domain.execute,preview.ID,false)
	if err!=nil || !preview.Complete { t.Fatal(preview,err) }
	owner:=phase27Actor(t,f.domain.f,f.domain.execute.User(),[]string{"reporting.read","reporting.preview","cw.report.preview:*","cw.run.read:*","cw.execution_context.use:*"})
	beforeSource,beforeModels:=f.domain.f.f.lookups.Load(),f.domain.f.model.requests.Load()
	view,err:=f.service.View(t.Context(),owner,reporting.ReportingViewRequest{Kind:"report",Run:preview.ID,Page:"main",Widget:"block",Limit:1})
	if err!=nil || !view.Summary.Private || view.Output==nil || view.Output.Table==nil { t.Fatal("authorized private retained read",view,err) }
	foreign:=phase27Actor(t,f.domain.f,"not-preview-owner",[]string{"reporting.read","reporting.preview","cw.report.preview:*","cw.run.read:*","cw.execution_context.use:*"})
	denied,err:=f.service.View(t.Context(),foreign,reporting.ReportingViewRequest{Kind:"report",Run:preview.ID,Page:"main",Widget:"block",Limit:1})
	if err==nil || denied.Output!=nil || denied.Summary.Run!="" { t.Fatal("private actor/session boundary waived",denied,err) }
	// Move only the test fixture's retention deadline into the past. The real
	// metadata-only reader must return a tombstone without loading old payloads.
	pool:=support.Raw(t,f.domain.f.f.dsn)
	if _,err:=pool.Exec(context.Background(),`UPDATE chartworks.composition_runs SET expires_at=clock_timestamp()-interval '1 second' WHERE tenant_id=$1 AND operation_id=$2`,f.domain.execute.Tenant(),preview.ID); err!=nil { t.Fatal(err) }
	expired,err:=f.service.View(t.Context(),owner,reporting.ReportingViewRequest{Kind:"report",Run:preview.ID,Limit:1})
	if err!=nil || expired.Summary.State!="expired" || expired.Output!=nil || expired.Text!=nil || len(expired.Pages)!=0 { t.Fatal("expired retained values were exposed or regenerated",expired,err) }
	if beforeSource!=f.domain.f.f.lookups.Load() || beforeModels!=f.domain.f.model.requests.Load() { t.Fatal("private/expired state reads executed work") }
}
