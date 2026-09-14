package acceptance

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

// Every criterion runs through real domain consumers, PostgreSQL and the actual
// Pengui HTTP verifier. Missing criteria must remain failures in the phase runner.
func TestPhase30(t *testing.T) {
	t.Run("AC01", testPhase30Targets)
	t.Run("AC02", testPhase30Authority)
}

func testPhase30Targets(t *testing.T) {
	for _, kind := range []string{"saved_sql", "saved_question", "block", "report"} {
		t.Run(kind, func(t *testing.T) {
			f := newPhase30Fixture(t, kind == "saved_question")
			id := "p30-target"
			switch kind {
			case "saved_sql":
				f.domain.block(t, id, f.domain.base)
			case "block":
				f.certify(t, id)
			case "report":
				f.domain.block(t, "p30-child", f.domain.base)
				d := phase29Text("Scheduled governed output")
				d.Widgets = append(d.Widgets, phase29BlockWidget("frozen", "p30-child", 1, "table-second", "table-main"))
				f.domain.report(t, id, d, true)
			case "saved_question":
				d := phase29Text("Explicitly dynamic saved question")
				d.Widgets = append(d.Widgets, f.domain.queryWidget())
				f.domain.report(t, id, d, true)
			}
			target := phase30Target(kind, id)
			if kind == "saved_question" {
				invalid := target
				invalid.Dynamic = false
				before := f.domain.attemptCount(t)
				if _, err := f.queue.Submit(t.Context(), f.actor, "implicit-dynamic", jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &invalid}); err == nil || f.domain.attemptCount(t) != before {
					t.Fatal("dynamic execution did not require explicit opt-in", err)
				}
			}
			raw := support.Raw(t, f.domain.f.f.dsn)
			var documents int
			if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.document_heads WHERE tenant_id=$1`, f.actor.Tenant()).Scan(&documents); err != nil {
				t.Fatal(err)
			}
			j := f.submit(t, "one-real-occurrence", target)
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			if err := f.queue.RunOnce(t.Context()); err != nil {
				t.Fatal("real queue target execution", err, f.get(t, j.ID))
			}
			done := f.get(t, j.ID)
			if done.State != "succeeded" || done.Attempts != 1 || done.ManifestHash != j.ManifestHash || done.Delivery == nil || done.Delivery.Catalog != "available" || done.Delivery.Artifact != "retained" || done.Delivery.Notification != "not_requested" {
				t.Fatalf("missing truthful persisted delivery: %+v", done)
			}
			if f.domain.attemptCount(t) != beforeQueries+1 {
				t.Fatal("target bypassed execution or duplicated selected-output query")
			}
			if kind == "saved_question" {
				if f.domain.f.model.requests.Load() <= beforeModels || done.Delivery.Reserved.ModelCalls < 1 {
					t.Fatal("dynamic lane did not execute/reserve actual inference")
				}
			} else if f.domain.f.model.requests.Load() != beforeModels || done.Delivery.Reserved.ModelCalls != 0 {
				t.Fatal("deterministic target invoked inference")
			}
			if done.Delivery.Reserved.Queries != 1 {
				t.Fatal("physical query attempt was not durably reserved", done.Delivery)
			}
			var afterDocuments int
			if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.document_heads WHERE tenant_id=$1`, f.actor.Tenant()).Scan(&afterDocuments); err != nil || afterDocuments != documents {
				t.Fatal("scheduling created a hidden report", err)
			}
			beforeQueries, beforeModels = f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			selection := reporting.ReportingViewRequest{Kind: target.ResourceKind(), Run: j.ID, Limit: 1}
			view, err := f.delivery.View(t.Context(), f.domain.execute, selection)
			if err != nil || view.Summary.Run != j.ID || view.Output == nil && view.Text == nil {
				t.Fatal("scheduled result not consumable by existing viewer", view, err)
			}
			if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
				t.Fatal("catalog/view consumed a fresh query or model")
			}
		})
	}
}

func testPhase30Authority(t *testing.T) {
	f := newPhase30Fixture(t, false)
	f.domain.block(t, "p30-authority", f.domain.base)
	target := phase30Target("saved_sql", "p30-authority")
	before := f.domain.attemptCount(t)
	if _, err := f.queue.Submit(t.Context(), f.actor, "higher-binding", jobs.Submission{Kind: jobs.ReportingKind, BindingID: "higher", Reporting: &target}); !errors.Is(err, access.ErrNotFound) || f.domain.attemptCount(t) != before {
		t.Fatal("creator selected a stronger unsigned binding", err)
	}
	narrowScopes := slices.DeleteFunc(slices.Clone(f.admissionScopes), func(s string) bool { return strings.HasPrefix(s, "cw.execution_context.use:") })
	narrow := phase27Actor(t, f.domain.f, f.actor.User(), narrowScopes)
	if _, err := f.queue.Submit(t.Context(), narrow, "missing-dependency", jobs.Submission{Kind: jobs.ReportingKind, BindingID: "reporting", Reporting: &target}); err == nil || f.domain.attemptCount(t) != before {
		t.Fatal("admission lacked dependency reach", err)
	}
	for _, mode := range []int64{1, 3, 4, 5, 6, 7, 8} {
		t.Run(fmt.Sprintf("renewal-%d", mode), func(t *testing.T) {
			j := f.submit(t, fmt.Sprintf("denied-renewal-%d", mode), target)
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			f.mode.Store(mode)
			if err := f.queue.RunOnce(t.Context()); err == nil {
				t.Fatal("invalid renewed authority executed")
			}
			done := f.get(t, j.ID)
			if done.State != "blocked" || f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
				t.Fatal("refusal had protected effects", done)
			}
			if done.Delivery == nil || done.Delivery.Catalog == "available" {
				t.Fatal("refused execution was advertised as delivery", done)
			}
		})
	}
	j := f.submit(t, "retry-authority", target)
	f.mode.Store(2)
	if err := f.queue.RunOnce(t.Context()); err == nil {
		t.Fatal("temporary issuer failure hidden")
	}
	if retry := f.get(t, j.ID); retry.State != "retry" || retry.ManifestHash != j.ManifestHash {
		t.Fatal("retry changed accepted manifest", retry)
	}
	f.retryNow(t, j.ID)
	f.mode.Store(0)
	if err := f.queue.RunOnce(t.Context()); err != nil {
		t.Fatal("fresh retry", err)
	}
	done := f.get(t, j.ID)
	if done.State != "succeeded" || done.Attempts != 2 || !done.DueAt.Equal(j.DueAt) || !done.WindowStart.Equal(j.WindowStart) {
		t.Fatal("renewal changed temporal or retry identity", done)
	}
	var stored string
	if err := support.Raw(t, f.domain.f.f.dsn).QueryRow(t.Context(), `SELECT row_to_json(o)::text FROM chartworks.operations o WHERE tenant_id=$1 AND operation_id=$2`, f.actor.Tenant(), j.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, token := range f.tokens {
		if strings.Contains(stored, token) {
			t.Fatal("execution JWT persisted")
		}
	}
	if strings.Contains(stored, "SYNTHETIC_REPORTING_BROKER_SECRET") || strings.Contains(stored, f.domain.base.SQL) {
		t.Fatal("dispatch persisted bearer credential or SQL")
	}
	seen := 0
	for _, r := range f.requests {
		if r["job"] == j.ID {
			seen++
			if r["manifest"] != j.ManifestHash || r["binding"] != j.BindingID {
				t.Fatal("broker request repinned on retry")
			}
		}
	}
	if seen != 2 {
		t.Fatal("retry did not acquire fresh Pengui authority", seen)
	}
}
