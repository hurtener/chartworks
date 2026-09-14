package acceptance

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	broker "github.com/hurtener/chartworks/internal/jobs/pengui"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

// Like phase06, this fixture exercises the production HTTPS authority adapter,
// real asymmetric verifier and actual PostgreSQL queue. Only the remote Pengui
// issuer is a synthetic fixture; no fake jobs.Authority or executor is installed.
type phase30Fixture struct {
	domain          *phase29ExecutionFixture
	delivery        *reporting.Delivery
	scheduled       *reporting.Scheduled
	queue           *jobs.Service
	provider        *broker.Provider
	actor           identity.Envelope
	limits          jobs.Limits
	runtimeScopes   []string
	admissionScopes []string
	mode            atomic.Int64
	calls           atomic.Int64
	mu              sync.Mutex
	tokens          []string
	requests        []map[string]string
}

func phase30ExecutionScopes(tenant string) []string {
	return slices.DeleteFunc(phase29RuntimeScopes(tenant), func(s string) bool {
		return s == "reporting.preview" || s == "reporting.retention" || strings.HasPrefix(s, "jobs.") ||
			strings.HasPrefix(s, "cw.dashboard.") || strings.Contains(s, ".preview:")
	})
}

func newPhase30Fixture(t *testing.T, dynamic bool) *phase30Fixture {
	t.Helper()
	d := newPhase29Execution(t, dynamic)
	f := &phase30Fixture{domain: d, limits: jobs.Defaults()}
	f.limits.Backoff = 20 * time.Millisecond
	f.limits.Heartbeat = 100 * time.Millisecond
	f.runtimeScopes = phase30ExecutionScopes(d.execute.Tenant())
	f.admissionScopes = append(slices.Clone(f.runtimeScopes), "scheduling.write", "cw.tenant.write:"+d.execute.Tenant(), "cw.execution_binding.use:reporting")
	if len(f.runtimeScopes)+2 > 32 || len(f.admissionScopes) > 32 {
		t.Fatal("reporting fixture exceeds actual issuer scope ceiling")
	}
	f.actor = phase27Actor(t, d.f, d.execute.User(), f.admissionScopes)
	var err error
	f.delivery, err = reporting.NewDelivery(d.blocks, d.runs, d.documents, d.compositions, d.f.f.db, d.limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	f.scheduled, err = reporting.NewScheduled(f.delivery, d.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	tokens := d.f.f.token
	cfg := tokens.cfg
	cfg.Audiences.Jobs = "chartworks:execution"
	verifier, err := auth.New(cfg, tokens.server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Close)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		client, secret, ok := r.BasicAuth()
		if !ok || client != "reporting-fixture" || secret != "SYNTHETIC_REPORTING_BROKER_SECRET" || r.Method != http.MethodPost || r.URL.Path != "/exchange/execution-authority" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var in struct {
			Version  int    `json:"version"`
			Binding  string `json:"binding_id"`
			Job      string `json:"job_id"`
			Manifest string `json:"manifest_hash"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || in.Version != 1 || in.Binding != "reporting" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, map[string]string{"binding": in.Binding, "job": in.Job, "manifest": in.Manifest})
		f.mu.Unlock()
		mode := f.mode.Load()
		if mode == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if mode == 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if mode == 9 {
			<-r.Context().Done()
			return
		}
		scopes := append(slices.Clone(f.runtimeScopes), "cw.execution_binding.use:"+in.Binding, "cw.run.execute:"+in.Job)
		if mode == 4 {
			scopes = slices.DeleteFunc(scopes, func(s string) bool { return strings.HasPrefix(s, "cw.execution_context.use:") })
		}
		claims := tokens.claims(d.execute.Tenant(), jobs.Executor(in.Binding), scopes)
		now := time.Now().Unix()
		claims["iat"], claims["nbf"], claims["exp"] = now, now-1, now+30
		claims["aud"], claims["session"] = cfg.Audiences.Jobs, in.Job
		claims["execution_version"], claims["execution_binding_revision"] = 1, 1
		claims["execution_binding"], claims["execution_manifest"] = in.Binding, in.Manifest
		switch mode {
		case 3:
			claims["iat"], claims["exp"] = now-31, now-1
		case 5:
			claims["execution_manifest"] = strings.Repeat("0", 64)
		case 6:
			claims["execution_binding"] = "higher"
		case 7:
			claims["aud"] = cfg.HTTPAudience()
		case 8:
			claims["user"], claims["sub"] = "svc:other", "svc:other"
		}
		token := tokens.sign(t, claims, nil)
		f.mu.Lock()
		f.tokens = append(f.tokens, token)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "access_token": token, "token_type": "Bearer", "expires_in": 30, "binding_id": in.Binding, "binding_revision": 1})
	}))
	t.Cleanup(server.Close)
	f.provider, err = broker.New(server.URL+"/exchange/execution-authority", map[string]broker.Credential{d.execute.Tenant(): {ClientID: "reporting-fixture", Secret: "SYNTHETIC_REPORTING_BROKER_SECRET"}}, verifier, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.provider.Close)
	f.queue, err = jobs.NewWithReporting(d.f.f.db, f.provider, f.limits, nil, f.scheduled)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *phase30Fixture) controls(t *testing.T) identity.Envelope {
	t.Helper()
	return phase27Actor(t, f.domain.f, f.actor.User(), []string{
		"scheduling.read", "scheduling.write", "scheduling.cancel", "scheduling.execute",
		"cw.run.read:*", "cw.run.write:*", "cw.schedule.read:*", "cw.schedule.write:*", "cw.schedule.execute:*",
	})
}

func phase30Target(kind, id string) jobs.ReportingTarget {
	t := jobs.ReportingTarget{Type: kind, ID: id, Revision: 1, Locale: "en", Timezone: "UTC", Outputs: []string{}, Arguments: []jobs.ReportingArgument{},
		Budget: jobs.ReportingBudget{TimeoutMillis: 10000, MaxRows: 1000, MaxBytes: 1 << 20, QueryAttempts: 8}}
	if t.ResourceKind() == "block" {
		t.Outputs = []string{"table-second", "table-main"}
	}
	if t.ResourceKind() == "report" {
		t.Locale = "en-US"
	}
	if kind == "saved_question" {
		t.Dynamic, t.Widget, t.Budget.ModelCalls, t.Budget.ModelTokens = true, "dynamic", 16, 1<<20
	}
	return t
}

func (f *phase30Fixture) submit(t *testing.T, key string, target jobs.ReportingTarget) jobs.Job {
	t.Helper()
	j, err := f.queue.Submit(t.Context(), f.actor, key, jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target})
	if err != nil || !j.Valid() || j.Reporting == nil {
		t.Fatalf("reporting admission: %+v %v", j, err)
	}
	return j
}

func (f *phase30Fixture) get(t *testing.T, id string) jobs.Job {
	t.Helper()
	j, err := f.queue.Get(t.Context(), f.controls(t), id)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func (f *phase30Fixture) retryNow(t *testing.T, id string) {
	t.Helper()
	_, err := support.Raw(t, f.domain.f.f.dsn).Exec(t.Context(), `UPDATE chartworks.operations SET next_attempt_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2`, f.actor.Tenant(), id)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *phase30Fixture) certify(t *testing.T, id string) {
	t.Helper()
	created, err := f.domain.blocks.Create(t.Context(), f.domain.blockAuthor, reporting.CreateRequest{ID: id, Definition: f.domain.base})
	if err != nil {
		t.Fatal(err)
	}
	state, evidence := phase27ValidatePublish(t, f.domain.blocks, f.domain.blockAuthor, created)
	_, err = f.domain.blocks.Certify(t.Context(), f.domain.blockAuthor, id, reporting.CertifyRequest{ExpectedVersion: state.Version, Revision: created.Revision, Evidence: evidence.ID, Note: "Synthetic exact reviewed evidence"})
	if err != nil {
		t.Fatal(err)
	}
}
