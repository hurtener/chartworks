package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestPNGRenditionRetainedTransportsAndExpiry(t *testing.T) {
	f := newPhase31Fixture(t, false)
	d := f.domain
	ctx := t.Context()
	d.block(t, "png-retained", d.base)
	run, err := d.runs.Admit(ctx, d.execute, "png-retained", reporting.RunRequest{Key: "png-retained", Outputs: []string{"table-main"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = d.runs.Run(ctx, d.execute, run.ID, false); err != nil {
		t.Fatal(err)
	}
	request := phase32Request("png")
	request.View.Run, request.View.Output = run.ID, "table-main"
	scopes := []string{"reporting.read", "reporting.export", "reporting.retention", "cw.run.read:*", "cw.run.export:*", "cw.execution_context.use:" + run.Context, "cw.tenant.erase:" + d.execute.Tenant()}
	actor := phase27Actor(t, d.f, d.execute.User(), scopes)
	view, err := f.service.View(ctx, actor, request.View)
	if err != nil {
		t.Fatal(err)
	}
	clock := &phase32ExpiryClock{Repository: d.f.f.db, at: time.Now()}
	service, err := rendering.NewManaged(f.service, clock, rendering.LocalProcessor{MaxBytes: 4 << 20}, 4<<20, phase32Options())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(reportingapi.DeliveryHandler(d.f.model.token.verifier, f.service, true, http.NotFoundHandler(), service))
	defer server.Close()
	clientFor := func(user, tenant string, scopes []string) *sdk.Client {
		token := f.token(t, user, tenant, scopes, false)
		c, e := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	client := clientFor(actor.User(), actor.Tenant(), scopes)
	first, err := client.CreateReportingRendition(ctx, request)
	if err != nil {
		t.Fatal("HTTP PNG create", err)
	}
	raw, err := sdk.ReportingRenditionBytes(ctx, first, 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(raw))
	if err != nil || image.Bounds().Dx() != request.Width || image.Bounds().Dy() != request.Height {
		t.Fatal("decoded HTTP raster", err)
	}
	repeated, err := client.CreateReportingRendition(ctx, request)
	if err != nil || repeated.ID != first.ID || repeated.Content != first.Content {
		t.Fatal("durable replay", err)
	}
	read, err := client.ReadReportingRendition(ctx, first.ID)
	if err != nil || read.Digest != first.Digest || read.ContentEncoding != "base64" {
		t.Fatal("retained HTTP binary", err)
	}
	foreign := clientFor("other", "other", scopes)
	if _, err = foreign.ReadReportingRendition(ctx, first.ID); err == nil {
		t.Fatal("foreign tenant artifact readable")
	}
	wrongContext := clientFor(actor.User(), actor.Tenant(), []string{"reporting.read", "reporting.export", "cw.run.read:*", "cw.run.export:*", "cw.execution_context.use:other-context"})
	if _, err = wrongContext.CreateReportingRendition(ctx, request); err == nil {
		t.Fatal("same-tenant foreign context exported")
	}
	denied := clientFor(actor.User(), actor.Tenant(), []string{"reporting.read", "cw.run.read:*"})
	if _, err = denied.CreateReportingRendition(ctx, request); err == nil {
		t.Fatal("read-only export accepted")
	}
	bindings, err := reportingapi.DeliveryMCPBindings(f.service, true, service)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := mcpserver.New(d.f.model.token.verifier, registry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	token := f.token(t, actor.User(), actor.Tenant(), append(append([]string{}, scopes...), "mcp.use"), true)
	mcpClient, err := mcp.Client(func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(rendering.ReadRequest{ID: first.ID})
	result, err := mcpClient.CallTool(ctx, "reporting_rendition_read", input)
	if err != nil || result == nil || result.IsError {
		t.Fatal("MCP retained PNG", err)
	}
	full := request
	full.Full = true
	if _, err = client.CreateReportingRendition(ctx, full); err == nil {
		t.Fatal("full report PNG silently admitted")
	}
	clock.at = view.Summary.Expires.Add(time.Microsecond)
	expired, err := client.ExpireReportingRenditions(ctx, 10)
	if err != nil || expired.Count != 1 {
		t.Fatal("actual expiry", expired, err)
	}
	if _, err = client.ReadReportingRendition(ctx, first.ID); err == nil {
		t.Fatal("expired PNG remained readable")
	}
}
