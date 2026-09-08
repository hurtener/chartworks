package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/workapi"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
)

// These tests cross the real SDK, HTTP, JWT, service, PostgreSQL and broker seams.
// Neither the HTTP router nor the job repository is mocked.
func TestWorkAPIAndSDK(t *testing.T) {
	q := newQueueFixture(t, nil)
	f := newGatewayFixture(t, nil)
	ctx := context.Background()
	scopes := append(q.actor.Scopes(), "ops.model", "cw.tenant.use:queue-tenant")
	actor := q.token.envelope(t, q.actor.Tenant(), q.actor.User(), scopes...)
	tokenFor := func(e identity.Envelope) string {
		return q.token.sign(t, q.token.claims(e.Tenant(), e.User(), e.Scopes()), nil)
	}
	token := tokenFor(actor)
	h := workapi.Handler(q.token.verifier, f.engine, q.service, http.NotFoundHandler())
	server := httptest.NewServer(h)
	defer server.Close()
	client, err := cw.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := callProtected(t, h, "POST", "/v1/jobs", token, `{"kind":"retention.sweep","binding_id":"maintenance"}`, map[string]string{"Idempotency-Key": strings.Repeat("a", 129)}); got.Code != http.StatusBadRequest {
		t.Fatalf("otherwise-valid oversized idempotency key accepted: %d", got.Code)
	}
	for _, role := range config.RoleNames() {
		p, err := client.ProbeGateway(ctx, role)
		if err != nil || !p.OK || p.Role != role {
			t.Fatalf("probe %s: %+v %v", role, p, err)
		}
	}
	target := cw.JobTarget{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}
	job, err := client.SubmitJob(ctx, "sdk-admit", target)
	if err != nil || job.State != "pending" {
		t.Fatalf("admission: %v %+v", err, job)
	}
	replay, err := client.SubmitJob(ctx, "sdk-admit", target)
	if err != nil || job.ID != replay.ID || job.ManifestHash != replay.ManifestHash {
		t.Fatal("HTTP replay changed manifest")
	}
	list, err := client.Jobs(ctx)
	if err != nil || len(list) != 1 {
		t.Fatal("SDK list", err)
	}
	read, err := client.Job(ctx, job.ID)
	if err != nil || read.ID != job.ID {
		t.Fatal("SDK get", err)
	}
	if _, err := client.SubmitJob(ctx, "sdk-admit", cw.JobTarget{Kind: jobs.MaintenanceKind, BindingID: "other"}); err == nil {
		t.Fatal("conflicting request admitted")
	}
	q.oldAudit(t)
	if err := q.service.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	done, err := client.Job(ctx, job.ID)
	if err != nil || done.State != "succeeded" || done.Executor != "svc:chartworks:maintenance" || done.Initiator != actor.User() {
		t.Fatal("SDK truthful completion", err)
	}
	pending, err := client.SubmitJob(ctx, "sdk-cancel", target)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := client.CancelJob(ctx, pending.ID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatal("SDK cancellation", err)
	}
	if _, err := client.CancelJob(ctx, pending.ID); err != nil {
		t.Fatal("cancel replay", err)
	}
	schedule, err := client.CreateSchedule(ctx, "sdk-schedule", cw.ScheduleRequest{Target: target, Spec: cw.Recurrence{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "skip"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := client.Schedule(ctx, schedule.ID); err != nil || got.Revision != 1 {
		t.Fatal("SDK schedule", err)
	}
	fired, err := client.FireSchedule(ctx, schedule.ID, "fire-once")
	if err != nil {
		t.Fatal(err)
	}
	if again, err := client.FireSchedule(ctx, schedule.ID, "fire-once"); err != nil || again.ID != fired.ID {
		t.Fatal("SDK fire replay", err)
	}
	if _, err := client.FireSchedule(ctx, schedule.ID, "overlap"); err == nil {
		t.Fatal("manual fire ignored overlap skip")
	}
	paused, err := client.SetScheduleState(ctx, schedule.ID, 1, false)
	if err != nil || paused.Revision != 2 || paused.Enabled {
		t.Fatal("SDK pause", err)
	}
	if _, err := client.SetScheduleState(ctx, schedule.ID, 1, true); err == nil {
		t.Fatal("stale schedule revision accepted")
	}
	if _, err := client.FireSchedule(ctx, schedule.ID, "paused"); err == nil {
		t.Fatal("paused schedule fired")
	}
	if _, err := client.SetScheduleState(ctx, schedule.ID, 2, true); err != nil {
		t.Fatal("SDK resume", err)
	}
	// Exhaustive action registry gate: no missing scope reaches a provider or persistent mutation.
	bare := q.token.envelope(t, actor.Tenant(), actor.User())
	beforeModels, beforeBroker := f.requests.Load(), q.calls.Load()
	for _, op := range workapi.Registry(true, true) {
		path := strings.ReplaceAll(op.Path, "{id}", job.ID)
		for _, test := range []struct {
			token  string
			status int
		}{{"", 401}, {tokenFor(bare), 403}} {
			got := callProtected(t, h, op.Method, path, test.token, "{}", nil)
			if got.Code != test.status {
				t.Fatalf("%s %s: %d", op.Method, path, got.Code)
			}
		}
	}
	if f.requests.Load() != beforeModels || q.calls.Load() != beforeBroker {
		t.Fatal("denied action performed remote work")
	}
	for _, body := range []string{`{}`, `null`, `{"kind":"retention.sweep","binding_id":"maintenance","tenant":"foreign"}`, `{"kind":"retention.sweep","kind":"retention.sweep","binding_id":"maintenance"}`, `{"kind":"retention.sweep","BINDING_ID":"maintenance"}`, `{"kind":"retention.sweep","binding_id":null}`, strings.Repeat("x", 9000)} {
		got := callProtected(t, h, "POST", "/v1/jobs", token, body, map[string]string{"Idempotency-Key": "bad-body"})
		if got.Code != 400 {
			t.Fatalf("body admitted %d", got.Code)
		}
	}
	for _, input := range []struct {
		method, path, body string
		headers            map[string]string
	}{{"POST", "/v1/jobs", `{}`, nil}, {"GET", "/v1/jobs?tenant=foreign", "", nil}, {"GET", "/v1/jobs", "{}", nil}, {"POST", "/v1/jobs", "{}", map[string]string{"Content-Type": "text/plain"}}, {"POST", "/v1/gateway/probes", "{}", map[string]string{"Content-Encoding": "gzip"}}} {
		got := callProtected(t, h, input.method, input.path, token, input.body, input.headers)
		if got.Code != 400 {
			t.Fatalf("ambiguous request %s: %d", input.path, got.Code)
		}
	}
	if got := callProtected(t, h, "DELETE", "/v1/jobs", token, "", nil); got.Code != 405 {
		t.Fatal("unsupported method")
	}
	if got := callProtected(t, h, "GET", "/not-a-route", token, "", nil); got.Code != 404 {
		t.Fatal("fallthrough")
	}
	foreign := q.token.envelope(t, "foreign", "operator", "scheduling.read", "cw.run.read:*")
	if got := callProtected(t, h, "GET", "/v1/jobs/"+job.ID, tokenFor(foreign), "", nil); got.Code != 404 {
		t.Fatalf("cross-tenant read: %d", got.Code)
	}
	limited := q.token.envelope(t, actor.Tenant(), actor.User(), "scheduling.read", "cw.run.read:"+pending.ID)
	got := callProtected(t, h, "GET", "/v1/jobs", tokenFor(limited), "", nil)
	var selected []cw.Job
	if json.Unmarshal(got.Body.Bytes(), &selected) != nil || len(selected) != 1 || selected[0].ID != pending.ID {
		t.Fatal("list expanded signed reach")
	}
	// Reuse the same HTTP client concurrently with independently verified tenant contexts.
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if j, err := client.Job(ctx, job.ID); err != nil || j.ID != job.ID {
				t.Error("concurrent SDK read", err)
			}
		}()
	}
	wg.Wait()
	// No new calls are needed to read or cancel retained jobs with both inference and dispatch disabled.
	metadata, err := jobs.NewMetadata(q.db, q.limits)
	if err != nil {
		t.Fatal(err)
	}
	readOnly := workapi.Handler(q.token.verifier, nil, metadata, http.NotFoundHandler())
	beforeModels, beforeBroker = f.requests.Load(), q.calls.Load()
	for _, path := range []string{"/v1/jobs", "/v1/jobs/" + job.ID, "/v1/schedules/" + schedule.ID} {
		if got := callProtected(t, readOnly, "GET", path, token, "", nil); got.Code != 200 {
			t.Fatalf("offline metadata %s: %d", path, got.Code)
		}
	}
	if got := callProtected(t, readOnly, "POST", "/v1/jobs/"+fired.ID+"/cancel", token, "{}", nil); got.Code != 200 {
		t.Fatal("offline cancellation", got.Code)
	}
	if got := callProtected(t, readOnly, "POST", "/v1/jobs", token, "{}", nil); got.Code != 405 {
		t.Fatal("offline admission", got.Code)
	}
	if got := callProtected(t, readOnly, "PUT", "/v1/schedules/"+schedule.ID+"/state", token, `{"expected_revision":3,"enabled":false}`, nil); got.Code != 200 {
		t.Fatal("offline pause", got.Code)
	}
	if got := callProtected(t, readOnly, "PUT", "/v1/schedules/"+schedule.ID+"/state", token, `{"expected_revision":4,"enabled":true}`, nil); got.Code != 503 {
		t.Fatal("offline resume admitted", got.Code)
	}
	if err := metadata.RunOnce(ctx); !errors.Is(err, jobs.ErrTransient) {
		t.Fatal("metadata started an executor")
	}
	if f.requests.Load() != beforeModels || q.calls.Load() != beforeBroker {
		t.Fatal("retained metadata used remote inference/issuer")
	}
	f.mode.Store("error")
	out := callProtected(t, h, "POST", "/v1/gateway/probes", token, `{"role":"enhance"}`, nil)
	var failedProbe struct {
		Error   string           `json:"error"`
		Receipt *json.RawMessage `json:"receipt"`
	}
	if json.Unmarshal(out.Body.Bytes(), &failedProbe) != nil || out.Code != 503 || failedProbe.Error != "unavailable" || failedProbe.Receipt == nil || strings.Contains(out.Body.String(), "CANARY") {
		t.Fatal("unsafe error projection")
	}
	f.mode.Store("schema")
	if got := callProtected(t, h, "POST", "/v1/gateway/probes", token, `{"role":"enhance"}`, nil); got.Code != 502 {
		t.Fatal("invalid output status", got.Code)
	}
	if got := callProtected(t, h, "POST", "/v1/gateway/probes", token, `{"role":"arbitrary"}`, nil); got.Code != 400 {
		t.Fatal("unknown probe role")
	}
}
