package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store/postgres"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestEngineeringRegisteredSurfaces(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	handler := sourceapi.EngineeringHandler(f.token.verifier, f.service, http.NotFoundHandler())
	registry := sourceapi.EngineeringRegistry(true, true)
	if len(registry) != 14 {
		t.Fatal("unexpected implemented engineering operation inventory", registry)
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-engineering-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var published []sourceapi.Operation
	if json.Unmarshal(raw, &published) != nil || !reflect.DeepEqual(published, registry) {
		t.Fatal("published engineering operation manifest drifted from executable registry")
	}
	seen := map[string]bool{}
	for _, operation := range registry {
		key := operation.Method + " " + operation.Path
		if seen[key] || operation.Action == "" || operation.Effect == "" {
			t.Fatal("duplicate or unclassified operation", operation)
		}
		seen[key] = true
		request := httptest.NewRequest(operation.Method, strings.ReplaceAll(operation.Path, "{id}", "hidden"), nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("implemented operation bypassed verification", operation, response.Code)
		}
		request = httptest.NewRequest(operation.Method, strings.ReplaceAll(operation.Path, "{id}", "hidden"), nil)
		request.Header.Set("Authorization", "Bearer "+f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), []string{"ops.read", "cw.tenant.read:source-a"}), nil))
		response = httptest.NewRecorder()
		before := f.lookups.Load()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || f.lookups.Load() != before {
			t.Fatal("operation action denied too late", operation, response.Code)
		}
	}
	// Mutation availability is not metadata authority. Retained reads/cancellation
	// stay protected and present after new upload/profile generation is disabled.
	values := f.values
	values.Uploads.Enabled, values.Profiling.Enabled = false, false
	disabled, err := engineering.New(f.db, f.s, f.validator, f.executor, nil, values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer disabled.Close()
	off := sourceapi.EngineeringHandler(f.token.verifier, disabled, http.NotFoundHandler())
	retained := sourceapi.EngineeringRegistry(false, false)
	if len(retained) != 7 {
		t.Fatal("retained operation inventory changed", retained)
	}
	for _, operation := range retained {
		response := httptest.NewRecorder()
		off.ServeHTTP(response, httptest.NewRequest(operation.Method, strings.ReplaceAll(operation.Path, "{id}", "hidden"), nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatal("disabled generation removed a retained control", operation, response.Code)
		}
	}
	for _, path := range []string{"/v1/uploads", "/v1/uploads/hidden/content", "/v1/uploads/hidden/load", "/v1/profiles"} {
		response := httptest.NewRecorder()
		off.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatal("disabled mutation was still registered", path, response.Code)
		}
	}
}

func TestEngineeringRejectsAmbiguousBodiesBeforeSourceAccess(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	handler := sourceapi.EngineeringHandler(f.token.verifier, f.service, http.NotFoundHandler())
	token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil)
	for _, body := range []string{"null", `{}`, `[]`, `{"id":"one","id":"two"}`, `{"filename":"../../PRIVATE_CANARY"}`, `{} {}`, strings.Repeat(" ", 65537)} {
		request := httptest.NewRequest(http.MethodPost, "/v1/uploads", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		before := f.lookups.Load()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "PRIVATE_CANARY") || f.lookups.Load() != before {
			t.Fatal("invalid upload envelope touched source or leaked content", response.Code, response.Body.String())
		}
	}
	for _, media := range []string{"", "application/json", "application/octet-stream; charset=utf-8", "application/octet-stream; x=1"} {
		body := &unreadUploadBody{}
		request := httptest.NewRequest(http.MethodPut, "/v1/uploads/missing/content", body)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", media)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || body.reads.Load() != 0 {
			t.Fatal("ambiguous binary body was consumed", media, response.Code)
		}
	}
	for _, target := range []string{"/v1/uploads/missing?tenant=other", "/v1/uploads/%6dissing"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatal("alternate coordinates were accepted", target, response.Code)
		}
	}
	body := &unreadUploadBody{}
	request := httptest.NewRequest(http.MethodPut, "/v1/uploads/missing/content", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/octet-stream")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || body.reads.Load() != 0 {
		t.Fatal("unreserved upload consumed file bytes", response.Code)
	}
}

func TestEngineeringControlNeedsItsActionAndOriginalReach(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	raw, columns := engineeringCSV()
	loaded := f.load(t, engineeringSpec("operation-authority", "csv", raw, columns), raw)
	var scopes []string
	for _, scope := range f.e.Scopes() {
		if scope != "jobs.read" && scope != "jobs.cancel" {
			scopes = append(scopes, scope)
		}
	}
	withoutControl := f.token.envelope(t, f.e.Tenant(), f.e.User(), scopes...)
	if _, err := f.service.RequestOperation(context.Background(), withoutControl, loaded.Operation.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("core operation inspection omitted its action check", err)
	}
	if _, err := f.service.CancelOperation(context.Background(), withoutControl, loaded.Operation.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("core cancellation omitted its action check", err)
	}
	controlOnly := f.token.envelope(t, f.e.Tenant(), f.e.User(), "jobs.read", "jobs.cancel", "cw.tenant.read:source-a")
	if _, err := f.service.RequestOperation(context.Background(), controlOnly, loaded.Operation.ID); err == nil {
		t.Fatal("task identifier substituted for original source reach")
	}
	if _, err := f.service.CancelOperation(context.Background(), controlOnly, loaded.Operation.ID); err == nil {
		t.Fatal("cancellation bypassed original domain authority")
	}
}

func TestEngineeringHTTPCheckpointCancellationAndResume(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) { v.Jobs.Heartbeat = config.Duration(100 * time.Millisecond) }, nil)
	repo := &waitForUploadCancellation{DB: f.db, entered: make(chan struct{})}
	repo.once.Store(true)
	owner, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	server := httptest.NewServer(sourceapi.EngineeringHandler(f.token.verifier, owner, http.NotFoundHandler()))
	defer func() { owner.Close(); server.Close() }()
	token := f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil)
	client, err := sdk.New(server.URL, &http.Client{Timeout: 10 * time.Second}, func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	raw, columns := engineeringCSV()
	spec := engineeringSpec("http-cancel", "csv", raw, columns)
	f.stage(t, spec, raw)
	type outcome struct {
		run sdk.UploadRun
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		run, err := client.LoadUpload(context.Background(), spec.ID, "http-cancel-key", false)
		done <- outcome{run, err}
	}()
	select {
	case <-repo.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("real workspace load did not reach its metadata publication boundary")
	}
	status, err := client.Upload(context.Background(), spec.ID)
	if err != nil || status.Operation == "" || status.Source != nil {
		t.Fatal("pending operation unavailable or falsely active", err, status)
	}
	cancelled, err := client.CancelEngineeringOperation(context.Background(), status.Operation)
	if err != nil || cancelled.ID != status.Operation {
		t.Fatal("HTTP cancellation did not retain the actual task", err, cancelled)
	}
	select {
	case result := <-done:
		if result.err != nil || result.run.Upload.State == "active" || result.run.Operation.State != "cancelled" {
			t.Fatal("cancelled commit was published", result.err, result.run)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation failed to stop the active request owner")
	}
	resumed, err := client.LoadUpload(context.Background(), spec.ID, "http-cancel-key", true)
	if err != nil || resumed.Upload.State != "active" || resumed.Operation.ID != status.Operation || resumed.Operation.Attempts != 2 {
		t.Fatal("explicit HTTP recovery did not reconcile the original external commit", err, resumed)
	}
	operation, err := client.EngineeringOperation(context.Background(), status.Operation)
	if err != nil || operation.ID != status.Operation || operation.Attempts != 2 {
		t.Fatal("retained HTTP operation receipt diverged", err, operation)
	}
	// An empty SDK column selection is the same canonical manifest as an omitted
	// Go slice. It must neither conflict on first use nor drift during a retry.
	source := resumed.Upload.Source
	binding, err := f.s.Binding(context.Background(), f.e, source.ID, source.ContextID)
	if err != nil || len(binding.Relations) != 1 {
		t.Fatal(err)
	}
	profile, err := client.BuildProfile(context.Background(), sdk.ProfileSpec{ID: "http-profile", Source: source.ID, Context: source.ContextID, Dataset: binding.Relations[0].ID, SkipLLM: true}, "http-profile-key", false)
	if err != nil || profile.Profile.Profile == nil || len(profile.Profile.Profile.Columns) != 7 {
		t.Fatal("SDK empty-column manifest was not canonical", err, profile)
	}
	dependency := sdk.ProfileDependency{Kind: "report", ID: "http-report", Version: readexec.Hash("fixed-definition"), Source: source.ID, Context: source.ContextID, Dataset: profile.Profile.Dataset, Columns: []string{"id", "amount"}}
	if err = client.RegisterProfileDependency(context.Background(), profile.Profile.Version, dependency); err != nil {
		t.Fatal("typed dependency registration", err)
	}
	events, err := client.ProfileDependencyHealth(context.Background(), dependency)
	if err != nil || len(events) != 0 {
		t.Fatal("unchanged dependency acquired invented health events", err, events)
	}
	encoded, err := json.Marshal(profile.Profile.Profile)
	if err != nil || strings.Contains(string(encoded), "SYNTHETIC_WRITER_PASSWORD") {
		t.Fatal("public profile escaped its bounded evidence contract", err)
	}
}

type waitForUploadCancellation struct {
	*postgres.DB
	once    atomic.Bool
	entered chan struct{}
}

func (r *waitForUploadCancellation) ActivateUpload(ctx context.Context, inv jobs.Invocation, upload engineering.UploadRecord, receipt engineering.WorkspaceReceipt, source sources.Record) error {
	if r.once.Swap(false) {
		close(r.entered)
		<-ctx.Done()
		return ctx.Err()
	}
	return r.DB.ActivateUpload(ctx, inv, upload, receipt, source)
}
