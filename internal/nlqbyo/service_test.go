package nlqbyo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type fixture struct {
	record                                                       Record
	publication                                                  topics.Contract
	rules                                                        rulesets.Published
	ruleErr, readErr, finishErr, reserveErr, topicErr, sourceErr error
	step                                                         Step
	replay                                                       bool
	calls, validates, executes, routes                           int
	result                                                       nlqroute.RouteResult
	request                                                      nlqroute.RouteRequest
}

func (f *fixture) Route(_ context.Context, _ identity.Envelope, r nlqroute.RouteRequest) (nlqroute.RouteResult, error) {
	f.routes++
	f.request = r
	return f.result, nil
}
func (f *fixture) Contract(context.Context, identity.Envelope, string) (topics.Contract, error) {
	f.calls++
	return f.publication, f.topicErr
}
func (f *fixture) Read(context.Context, identity.Envelope, string, string) (rulesets.Published, error) {
	return f.rules, f.ruleErr
}
func (f *fixture) Binding(context.Context, identity.Envelope, string, string) (exec.Binding, error) {
	f.calls++
	return f.record.Binding.Clone(), f.sourceErr
}
func (f *fixture) ValidateWithin(context.Context, identity.Envelope, exec.Request, []exec.RelationScope) (exec.Plan, error) {
	f.validates++
	return exec.Plan{}, exec.ErrUnsafe
}
func (f *fixture) Execute(context.Context, identity.Envelope, exec.Plan, exec.Options) (exec.ExecutionReport, error) {
	f.executes++
	return exec.ExecutionReport{}, exec.ErrBinding
}
func (f *fixture) CreateBYOBundle(context.Context, store.Scope, Record, config.QueryBundles) error {
	return store.ErrUnavailable
}
func (f *fixture) ReadBYOBundle(context.Context, store.Scope, Reference, string, time.Time) (Record, error) {
	return f.record, f.readErr
}
func (f *fixture) ReserveBYOStep(_ context.Context, _ store.Scope, _ Reference, _ string, s Step, _ time.Time) (Step, bool, error) {
	if f.replay {
		return f.step, false, f.reserveErr
	}
	s.Number = 1
	f.step = s
	return s, true, f.reserveErr
}
func (f *fixture) FinishBYOStep(_ context.Context, _ store.Scope, _ Reference, _ string, s Step) error {
	f.step = s
	return f.finishErr
}
func (f *fixture) ReadBYOSteps(context.Context, store.Scope, Reference, string) ([]Step, error) {
	return []Step{}, f.readErr
}

func setup(t *testing.T) (*Service, *fixture, identity.Envelope) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"query.context", "query.submit", "sources.query", "cw.topic.read:topic", "cw.source.read:source", "cw.source.query:source", "cw.execution_context.use:context", "cw.dataset.query:sales"}, now.Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	binding := exec.Binding{Tenant: "tenant", Source: "source", Context: "context", Revision: 1, Dialect: "postgres", Contract: "contract", Fingerprint: exec.Hash("fixture"), Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "id", NativeType: "integer", Safe: true}}}}}
	pin := SemanticPin{Topic: "topic", Version: "v1", Digest: exec.Hash("topic")}
	b := Bundle{Reference: Reference{1, strings.Repeat("a", 64), "context"}, CreatedAt: now, ExpiresAt: now.Add(time.Minute), Source: "source", Semantics: []SemanticPin{pin}, MaxSteps: 8, Requirements: SQLRequirements{Dialect: "postgres", Relations: binding.Clone().Relations, ReadOnly: true, Rows: 100, Bytes: 4096, TimeoutMillis: 1000, MaxSQLBytes: 4096, MaxParameters: 8}}
	f := &fixture{record: Record{Bundle: b, Session: "session", Binding: binding, DataReach: dataReach(e), Digest: exec.Hash(b), RetainUntil: now.Add(time.Hour)}, ruleErr: store.ErrNotFound}
	f.publication = topics.Contract{Publication: topics.Published{State: topics.State{Active: true, Version: "v1"}, Digest: pin.Digest, Definition: topics.Definition{Topic: "topic", Version: "v1", Datasets: []topics.Dataset{{ID: "sales", Source: topics.Binding{Source: "source", Context: "context", SourceRevision: 1, Dataset: "sales"}, Columns: []semantics.Column{{SourceName: "id"}}}}}}}
	s, err := New(f, f, f, f, f, f, f, config.DefaultQueryBundles(), config.DefaultReadValidation(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, f, e
}
func TestConstructorAndAdmission(t *testing.T) {
	s, f, e := setup(t)
	if !s.CanCreate() {
		t.Fatal("router missing")
	}
	var absent *fixture
	for _, dep := range []any{nil, absent} {
		if !nilValue(dep) {
			t.Fatal("nil dependency")
		}
	}
	if _, err := New(nil, nil, f, f, f, f, f, s.limits, s.read, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	bad := s.limits
	bad.MaxSteps = 0
	if _, err := New(nil, f, f, f, f, f, f, bad, s.read, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	read := s.read
	read.MaxSQLBytes = 0
	if _, err := New(nil, f, f, f, f, f, f, s.limits, read, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	offline, err := New(absent, f, f, f, f, f, f, s.limits, s.read, nil)
	if err != nil || offline.CanCreate() {
		t.Fatal(err)
	}
	ctx := context.Background()
	in := CreateRequest{SchemaVersion: 1, Route: nlqroute.RouteRequest{Context: "context"}}
	if _, err = offline.Create(ctx, e, in); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err = s.Create(nil, e, in); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, identity.Envelope{}, in); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	noAction, _ := identity.FromVerified("tenant", "actor", "session", nil, time.Now().Add(time.Hour), time.Now)
	if _, err = s.Create(ctx, noAction, in); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.Create(cancelled, e, in); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	in.SchemaVersion = 2
	if _, err = s.Create(ctx, e, in); !errors.Is(err, ErrInvalid) || f.routes != 0 {
		t.Fatal("invalid create did I/O", err)
	}
	in.SchemaVersion = 1
	f.result.Outcome = nlq.StrategyNoRoute
	in.Route.Examples = []nlq.OptionalItem{{Source: "claimed-reviewed"}}
	out, err := s.Create(ctx, e, in)
	if err != nil || out.Bundle != nil || f.request.Examples[0].Source != "external_input_unreviewed" || in.Route.Examples[0].Source != "claimed-reviewed" {
		t.Fatal(out, err)
	}
	f.result.Context = &nlqroute.ContextView{}
	f.result.Outcome = nlq.StrategySingleTopic
	if _, err = s.Create(ctx, e, in); err == nil {
		t.Fatal("forged serialized context became sealed")
	}
}
func TestLookupRevalidatesExactCapturedState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fixture)
	}{
		{"missing", func(f *fixture) { f.readErr = store.ErrNotFound }},
		{"changed-reach", func(f *fixture) { f.record.DataReach = exec.Hash("different") }},
		{"session", func(f *fixture) { f.record.Session = "other" }},
		{"expired", func(f *fixture) {
			f.record.Bundle.ExpiresAt = f.record.Bundle.CreatedAt
			f.record.Digest = exec.Hash(f.record.Bundle)
		}},
		{"archived", func(f *fixture) { f.publication.Publication.State.Archived = true }},
		{"version", func(f *fixture) { f.publication.Publication.State.Version = "v2" }},
		{"topic-digest", func(f *fixture) { f.publication.Publication.Digest = exec.Hash("changed") }},
		{"topic-not-found", func(f *fixture) { f.topicErr = store.ErrNotFound }},
		{"source-conflict", func(f *fixture) { f.sourceErr = exec.ErrBinding }},
		{"rules-retired", func(f *fixture) { f.ruleErr = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, f, e := setup(t)
			tc.mutate(f)
			if _, err := s.Lookup(context.Background(), e, f.record.Bundle.Reference); !errors.Is(err, ErrReplan) {
				t.Fatal(err)
			}
			if f.validates != 0 || f.executes != 0 {
				t.Fatal("lookup executed")
			}
		})
	}
	s, f, e := setup(t)
	if out, err := s.Lookup(context.Background(), e, f.record.Bundle.Reference); err != nil || out.Bundle.ID == "" {
		t.Fatal(err)
	}
	ref := f.record.Bundle.Reference
	ref.SchemaVersion = 2
	if _, err := s.Lookup(context.Background(), e, ref); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	ref.SchemaVersion = 1
	ref.ID = "bad"
	if _, err := s.Lookup(context.Background(), e, ref); !errors.Is(err, ErrReplan) {
		t.Fatal(err)
	}
	f.readErr = store.ErrUnavailable
	if _, err := s.Lookup(context.Background(), e, f.record.Bundle.Reference); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestExplicitRejectedStepsAndReplay(t *testing.T) {
	s, f, e := setup(t)
	ctx := context.Background()
	in := SubmitRequest{Reference: f.record.Bundle.Reference, Operation: "one", SQL: "UPDATE analytics.sales SET id=0"}
	out, err := s.Submit(ctx, e, in)
	if err != nil || out.Step.Status != "rejected" || out.Step.Code != "sql_unsafe" || !StepValid(out.Step) || out.ValuesAvailable || f.validates != 1 || f.executes != 0 {
		t.Fatal(out, err)
	}
	f.replay = true
	if out, err = s.Submit(ctx, e, in); err != nil || !out.Replayed || out.Result != nil || f.validates != 1 {
		t.Fatal("replay did work", err)
	}
	f.step.Status = "accepted"
	f.step.Deadline = time.Now().Add(-time.Second)
	if out, err = s.Submit(ctx, e, in); err != nil || out.Step.Status != "uncertain" || f.validates != 1 {
		t.Fatal(err)
	}
	f.replay = false
	f.reserveErr = ErrBudget
	if _, err = s.Submit(ctx, e, in); !errors.Is(err, ErrBudget) || f.validates != 1 {
		t.Fatal(err)
	}
	f.reserveErr = nil
	f.finishErr = store.ErrUnavailable
	if _, err = s.Submit(ctx, e, in); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal(err)
	}
	for _, mutate := range []func(*SubmitRequest){func(r *SubmitRequest) { r.Operation = "bad/op" }, func(r *SubmitRequest) { r.SQL = "" }, func(r *SubmitRequest) { r.SQL = string([]byte{255}) }, func(r *SubmitRequest) { r.SQL = strings.Repeat("x", s.read.MaxSQLBytes+1) }, func(r *SubmitRequest) { r.Parameters = make([]exec.Parameter, s.read.MaxParameters+1) }, func(r *SubmitRequest) { r.Parameters = []exec.Parameter{{Kind: "sql", Value: "SELECT 1"}} }} {
		bad := in
		mutate(&bad)
		if _, err = s.Submit(ctx, e, bad); err == nil {
			t.Fatal("bad request admitted")
		}
	}
}
func TestSemanticProjectionAndStorageValidation(t *testing.T) {
	s, f, _ := setup(t)
	scope, _ := store.NewScope("tenant", "actor")
	if !RecordValid(f.record, scope, s.limits) {
		t.Fatal("fixture invalid")
	}
	defs := []topics.Definition{f.publication.Publication.Definition}
	r, err := semanticRelations(f.record.Binding, defs)
	if err != nil || len(r) != 1 || len(relationScope(r)[0].Columns) != 1 || datasetIDs(r)[0] != "sales" {
		t.Fatal(err)
	}
	for _, bad := range []exec.Binding{{}, func() exec.Binding { b := f.record.Binding.Clone(); b.Revision++; return b }(), func() exec.Binding { b := f.record.Binding.Clone(); b.Relations[0].Columns[0].Safe = false; return b }()} {
		if _, err = semanticRelations(bad, defs); !errors.Is(err, ErrReplan) {
			t.Fatal(err)
		}
	}
	f.record.Bundle.Semantics = append(f.record.Bundle.Semantics, f.record.Bundle.Semantics[0])
	f.record.Digest = exec.Hash(f.record.Bundle)
	if RecordValid(f.record, scope, s.limits) {
		t.Fatal("duplicate pin")
	}
	for _, dialect := range []string{"postgres", "sqlserver", "mysql", "bigquery", "snowflake", "databricks"} {
		if parameterStyle(dialect) == "" {
			t.Fatal(dialect)
		}
	}
	for _, cause := range []error{exec.ErrUnsafe, exec.ErrUnsupported, exec.ErrType, exec.ErrLimit, exec.ErrBinding, ErrReplan, access.ErrForbidden, access.ErrUnauthenticated, context.Canceled, context.DeadlineExceeded, exec.ErrCancelled, exec.ErrTimeout, store.ErrUnavailable} {
		if errorCode(cause) == "" {
			t.Fatal(cause)
		}
	}
	if replanError(store.ErrUnavailable) != store.ErrUnavailable || replanError(store.ErrConflict) != ErrReplan {
		t.Fatal("failure class lost")
	}
	if _, _, err = s.current(context.Background(), identity.Envelope{}, nil); !errors.Is(err, ErrReplan) {
		t.Fatal(err)
	}
	id, err := opaqueID()
	if err != nil || !ReferenceValid(Reference{1, id, "context"}) {
		t.Fatal(err)
	}
}
func FuzzReferenceAndReceipt(f *testing.F) {
	f.Add(`{"schema_version":1,"bundle_id":"`+strings.Repeat("a", 64)+`","context":"context"}`, "accepted")
	f.Add(`{"schema_version":2}`, "succeeded|empty")
	f.Fuzz(func(t *testing.T, raw, status string) {
		if len(raw) > 1<<20 || len(status) > 1000 {
			return
		}
		var ref Reference
		_ = json.Unmarshal([]byte(raw), &ref)
		if ReferenceValid(ref) {
			if ref.SchemaVersion != 1 || len(ref.ID) != 64 || strings.ToLower(ref.ID) != ref.ID {
				t.Fatal("invalid reference accepted")
			}
		}
		s := Step{Operation: "op", InputDigest: exec.Hash("i"), BundleDigest: exec.Hash("b"), Semantics: []SemanticPin{{Topic: "topic", Version: "v1", Digest: exec.Hash("p")}}, Status: status, Code: "ok", CreatedAt: time.Unix(10, 0), Deadline: time.Unix(11, 0)}
		if StepValid(s) && strings.Contains(status, "|") {
			t.Fatal("composed status admitted")
		}
	})
}
