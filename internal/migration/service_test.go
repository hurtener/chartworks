package migration

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

type adapterTest struct {
	mu    sync.Mutex
	calls []string
	fail  string
}

func (a *adapterTest) Validate(_ context.Context, _ identity.Envelope, o Object, _ Mapping) error {
	if o.ExternalRef == a.fail {
		return ErrUnsupported
	}
	return nil
}
func (a *adapterTest) Apply(_ context.Context, _ identity.Envelope, o Object, m Mapping, _ string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, o.ExternalRef)
	if o.ExternalRef == a.fail {
		return "", errors.New("apply")
	}
	if m.Destination != "" {
		return m.Destination, nil
	}
	return "dst-" + o.ExternalRef, nil
}

func actorTest(t *testing.T, scopes ...string) identity.Envelope {
	t.Helper()
	base := []string{"cw.tenant.read:t", "cw.tenant.write:t", "cw.tenant.erase:t"}
	e, err := identity.FromVerified("t", "a", "s", append(base, scopes...), time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func evidenceTest() []Evidence {
	out := []Evidence{}
	for _, p := range []struct {
		s string
		n int
	}{{"B", 20}, {"R", 16}, {"Q", 10}, {"N", 16}} {
		for i := 1; i <= p.n; i++ {
			f := fmt.Sprintf("%s%02d", p.s, i)
			out = append(out, Evidence{Feature: f, Disposition: "required", Outcome: "passed", EvidenceType: "runtime", Reference: "ref-" + f, Source: "synthetic", SourceVersion: "v1"})
		}
	}
	return append(out, Evidence{Feature: "Q11", Disposition: "excluded", Outcome: "unsupported", EvidenceType: "operator", Reference: "discard", Source: "synthetic", SourceVersion: "v1"})
}
func manifestTest(id string) Manifest {
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	return Manifest{Version: ManifestVersion, Batch: "batch-" + id, Cohort: "cohort-" + id, SourceSnapshot: "snapshot-" + id, Engine: "postgres", Dialect: "postgres", Mappings: []Mapping{{Kind: KindSource, ExternalRef: "src-" + id, Destination: "source", Revision: 1}}, Objects: []Object{{Kind: KindSource, ExternalRef: "src-" + id, Revision: 1, PayloadVersion: "v1", Payload: `{"name":"source"}`, Lifecycle: "private_draft", Private: true, Origin: "synthetic", Retention: Retention{ExpiresAt: &at}}, {Kind: KindTopic, ExternalRef: "topic-" + id, Parents: []string{"src-" + id}, Revision: 1, PayloadVersion: "v1", Payload: `{"name":"topic"}`, Lifecycle: "private_draft", Private: true, Origin: "synthetic", Retention: Retention{ExpiresAt: &at}}, {Kind: KindCertificate, ExternalRef: "cert-" + id, Parents: []string{"topic-" + id}, Revision: 1, PayloadVersion: "v1", Payload: `{"name":"certificate"}`, Lifecycle: "historical", Private: true, Origin: "synthetic", Retention: Retention{ExpiresAt: &at}}}, Fields: []FieldDisposition{{Path: "src-" + id + ".name", Status: "retained"}, {Path: "topic-" + id + ".name", Status: "transformed", Reason: "coordinate remap"}, {Path: "cert-" + id + ".name", Status: "retained"}}, Evidence: evidenceTest(), Calibration: &Calibration{Revision: "c1", ModelVersion: "m1", EmbeddingSpace: "e1", BudgetVersion: "b1", Payload: `{"prompt_pack":"pack-one","optimization_revision":"opt-one","locale":"en-US","temperature":0.2,"max_output_tokens":2048,"example_policy_revision":"examples-one","template_thresholds":[{"template":"sales","threshold":0.72}]}`, State: "review_candidate"}, Boundary: &OccurrenceBoundary{Stream: "stream-" + id, ResumeAfter: at, ScheduleVersion: 1}}
}

func serviceTest(t *testing.T, a *adapterTest) (*Service, *MemoryRepository, identity.Envelope) {
	t.Helper()
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository(func() time.Time { return now })
	s, err := New(repo, map[Kind]Adapter{KindSource: a, KindTopic: a}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return s, repo, actorTest(t, "migration.read", "migration.write", "migration.cutover", "migration.erase")
}

func TestLifecycle(t *testing.T) {
	a := &adapterTest{}
	s, _, e := serviceTest(t, a)
	m := manifestTest("life")
	plan, err := s.DryRun(t.Context(), e, DryRunRequest{Manifest: m})
	if err != nil || !plan.Ready || len(plan.Objects) != 3 || plan.Objects[2].Action != "historical_quarantine" {
		t.Fatal(err, plan)
	}
	b, err := s.Import(t.Context(), e, ImportRequest{Manifest: m})
	if err != nil || b.State != "complete" || b.Applied != 2 || b.Quarantined != 1 {
		t.Fatal(err, b)
	}
	if len(a.calls) != 2 {
		t.Fatal(a.calls)
	}
	same, err := s.Import(t.Context(), e, ImportRequest{Manifest: m, Expected: b.Revision})
	if err != nil || same != b || len(a.calls) != 2 {
		t.Fatal("replay", err, same, a.calls)
	}
	x, err := s.Export(t.Context(), e, ExportRequest{Batch: b.ID, Limit: 2})
	if err != nil || len(x.Manifest.Objects) != 2 {
		t.Fatal(err, x)
	}
	x, err = s.Export(t.Context(), e, ExportRequest{Batch: b.ID, After: x.Manifest.Objects[1].ExternalRef, Limit: 2})
	if err != nil || len(x.Manifest.Objects) != 1 {
		t.Fatal(err, x)
	}
	cut, err := s.Cutover(t.Context(), e, CutoverRequest{Batch: b.ID, Route: "new", Expected: 0, OperatorRef: "runbook"})
	if err != nil || cut.Generation != 1 {
		t.Fatal(err, cut)
	}
	replayedCutover, err := s.Cutover(t.Context(), e, CutoverRequest{Batch: b.ID, Route: "new", Expected: 0, OperatorRef: "runbook"})
	if err != nil || !reflect.DeepEqual(replayedCutover, cut) {
		t.Fatal("cutover replay", err, replayedCutover)
	}
	cut, err = s.Rollback(t.Context(), e, RollbackRequest{Cohort: m.Cohort, Expected: 1, OperatorRef: "rollback", Effects: []string{"sent"}})
	if err != nil || cut.State != "rolled_back" {
		t.Fatal(err, cut)
	}
	replayedRollback, err := s.Rollback(t.Context(), e, RollbackRequest{Cohort: m.Cohort, Expected: 1, OperatorRef: "rollback", Effects: []string{"sent"}})
	if err != nil || !reflect.DeepEqual(replayedRollback, cut) {
		t.Fatal("rollback replay", err, replayedRollback)
	}
	if _, err := s.Rollback(t.Context(), e, RollbackRequest{Cohort: m.Cohort, Expected: 1, OperatorRef: "other", Effects: []string{"sent"}}); !errors.Is(err, ErrConflict) {
		t.Fatal("changed rollback replay", err)
	}
	erased, err := s.Erase(t.Context(), e, EraseRequest{Batch: b.ID, Limit: 10})
	if err != nil || erased.Remaining != 0 {
		t.Fatal(err, erased)
	}
}

func TestAdapterFunctionsAndCurrentCutover(t *testing.T) {
	object := Object{ExternalRef: "source"}
	mapping := Mapping{Destination: "target"}
	empty := AdapterFuncs{}
	if !errors.Is(empty.Validate(t.Context(), identity.Envelope{}, object, mapping), ErrUnsupported) {
		t.Fatal("empty validator accepted")
	}
	if _, err := empty.Apply(t.Context(), identity.Envelope{}, object, mapping, "digest"); !errors.Is(err, ErrUnsupported) {
		t.Fatal("empty apply accepted")
	}
	adapter := AdapterFuncs{ValidateFunc: func(context.Context, identity.Envelope, Object, Mapping) error { return nil }, ApplyFunc: func(context.Context, identity.Envelope, Object, Mapping, string) (string, error) {
		return "target", nil
	}}
	if adapter.Validate(t.Context(), identity.Envelope{}, object, mapping) != nil {
		t.Fatal("validator function not called")
	}
	if got, err := adapter.Apply(t.Context(), identity.Envelope{}, object, mapping, "digest"); err != nil || got != "target" {
		t.Fatal("apply function not called", got, err)
	}

	s, repo, e := serviceTest(t, &adapterTest{})
	m := manifestTest("current")
	if _, err := s.Import(t.Context(), e, ImportRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cutover(t.Context(), e, CutoverRequest{Batch: m.Batch, Route: "route", OperatorRef: "drill"}); err != nil {
		t.Fatal(err)
	}
	current, err := repo.CurrentCutover(t.Context(), e, m.Cohort)
	if err != nil || current.Route != "route" {
		t.Fatal(current, err)
	}
}

func TestValidationAndAuthority(t *testing.T) {
	a := &adapterTest{}
	s, _, e := serviceTest(t, a)
	m := manifestTest("bad")
	cases := []func(*Manifest){func(x *Manifest) { x.Version = "bad" }, func(x *Manifest) {
		x.Objects[0].Payload = `{"token":"x"}`
		x.Fields = append(x.Fields, FieldDisposition{Path: "src-bad.token", Status: "dropped", Reason: "secret"})
	}, func(x *Manifest) {
		x.Objects[0].Payload = `{"connection":{"authorization":"x"}}`
		x.Fields[0].Path = "src-bad.connection"
	}, func(x *Manifest) { x.Objects[1].Parents = []string{"cert-bad"} }, func(x *Manifest) { x.Fields = x.Fields[:1] }, func(x *Manifest) {
		x.Fields = append(x.Fields, FieldDisposition{Path: "unknown.field", Status: "retained"})
	}, func(x *Manifest) {
		x.Mappings = append(x.Mappings, Mapping{Kind: KindProfile, ExternalRef: "unknown", Destination: "profile", Revision: 1})
	}, func(x *Manifest) { x.Evidence = x.Evidence[:1] }, func(x *Manifest) { x.Calibration.State = "active" }, func(x *Manifest) { x.Calibration.Payload = `{"prompt_pack":"pack","unknown":true}` }, func(x *Manifest) { x.Boundary.ResumeAfter = time.Time{} }}
	for i, change := range cases {
		x := m
		x.Objects = append([]Object(nil), m.Objects...)
		x.Fields = append([]FieldDisposition(nil), m.Fields...)
		x.Evidence = append([]Evidence(nil), m.Evidence...)
		c := *m.Calibration
		x.Calibration = &c
		b := *m.Boundary
		x.Boundary = &b
		change(&x)
		if _, err := s.DryRun(t.Context(), e, DryRunRequest{Manifest: x}); err == nil {
			t.Fatal("accepted", i)
		}
	}
	zero := identity.Envelope{}
	if _, err := s.DryRun(t.Context(), zero, DryRunRequest{Manifest: m}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	read := actorTest(t, "migration.read")
	if _, err := s.Import(t.Context(), read, ImportRequest{Manifest: m}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	expired := manifestTest("expired")
	past := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	expired.Objects[0].Retention.ExpiresAt = &past
	plan, err := s.DryRun(t.Context(), e, DryRunRequest{Manifest: expired})
	if err != nil || plan.Objects[0].Action != "retention_quarantine" || len(plan.Limitations) == 0 {
		t.Fatal("expired source reached owner adapter", err, plan)
	}
}

func TestUnsupportedAndResume(t *testing.T) {
	a := &adapterTest{fail: "topic-u"}
	s, repo, e := serviceTest(t, a)
	m := manifestTest("u")
	plan, err := s.DryRun(t.Context(), e, DryRunRequest{Manifest: m})
	if err != nil || plan.Ready || plan.Objects[1].Action != "unsupported_quarantine" {
		t.Fatal(err, plan)
	}
	b, err := s.Import(t.Context(), e, ImportRequest{Manifest: m})
	if err != nil || b.State != "complete" || b.Applied != 1 || b.Quarantined != 2 {
		t.Fatal(err, b)
	}
	if _, err = s.Cutover(t.Context(), e, CutoverRequest{Batch: b.ID, Route: "x", Expected: 0, OperatorRef: "x"}); !errors.Is(err, ErrNotReady) {
		t.Fatal(err)
	}
	stored, _, _, _ := repo.Batch(t.Context(), e, b.ID)
	if _, err = s.Resume(t.Context(), e, ResumeRequest{Batch: b.ID, Expected: stored.Revision - 1}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestConcurrentCAS(t *testing.T) {
	a := &adapterTest{}
	s, _, e := serviceTest(t, a)
	m := manifestTest("cas")
	b, err := s.Import(t.Context(), e, ImportRequest{Manifest: m})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, x := s.Cutover(context.Background(), e, CutoverRequest{Batch: b.ID, Route: "r", Expected: 0, OperatorRef: "op"})
			errs <- x
		}()
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for x := range errs {
		if x == nil {
			success++
		} else if errors.Is(x, ErrConflict) {
			conflict++
		}
	}
	if success != 2 || conflict != 0 {
		t.Fatal(success, conflict)
	}
	if _, err := s.Cutover(t.Context(), e, CutoverRequest{Batch: b.ID, Route: "different", Expected: 0, OperatorRef: "op"}); !errors.Is(err, ErrConflict) {
		t.Fatal("divergent stale cutover", err)
	}
}
