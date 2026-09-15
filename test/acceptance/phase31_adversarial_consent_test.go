package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
)

// The real repository remains responsible for storage and authorization. The
// hook only places a real publication between two otherwise ordinary reads.
type phase31PublicationRace struct {
	reporting.DocumentRepository
	rootReads int
	onSecond  func()
}

func (r *phase31PublicationRace) ReadDocument(ctx context.Context, e identity.Envelope, kind, id string, ref reporting.DocumentReference, access reporting.Access, redact bool) (reporting.DocumentSnapshot, error) {
	snapshot, err := r.DocumentRepository.ReadDocument(ctx, e, kind, id, ref, access, redact)
	if err == nil && kind == "dashboard" {
		r.rootReads++
		if r.rootReads == 2 {
			r.onSecond()
		}
	}
	return snapshot, err
}

// Dashboard pages are exact by construction. Retain a regression for that
// safety property rather than changing the domain to support a hypothetical
// floating page. Publishing a dynamic child cannot change the accepted page.
func TestPhase31AdversarialPublicationConsent(t *testing.T) {
	f := newPhase31Fixture(t, true)
	d := phase29Text("Originally deterministic")
	report := f.domain.report(t, "p31-consent-report", d, true)
	dashboard := phase29Text("Explicit dynamic consent must survive publication")
	dashboard.Widgets = nil
	dashboard.Pages = []reporting.DocumentPage{{ID: "main", Title: "Exact report", Report: report.ID, Revision: 0}}
	if _, err := f.domain.documents.Create(t.Context(), f.domain.author, "dashboard", "p31-floating-dashboard", dashboard); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("floating dashboard page bypassed its exact-revision contract", err)
	}
	dashboard.Pages[0].Revision = 1
	state, err := f.domain.documents.Create(t.Context(), f.domain.author, "dashboard", "p31-consent-dashboard", dashboard)
	if err != nil {
		t.Fatal("exact dashboard fixture admission", err)
	}
	state = phase29Publish(t, f.domain.documents, f.domain.author, state)
	race := &phase31PublicationRace{DocumentRepository: f.domain.f.f.db}
	race.onSecond = func() {
		d.Widgets = append(d.Widgets, f.domain.queryWidget())
		updated, err := f.domain.documents.Edit(t.Context(), f.domain.author, "report", report.ID, report.Version, reporting.DocumentReference{}, d)
		if err != nil {
			t.Fatal("concurrent report edit", err)
		}
		phase29Publish(t, f.domain.documents, f.domain.author, updated)
	}
	queries := reporting.DocumentsFromQueries(f.domain.query)
	documents, err := reporting.NewDocuments(race, f.domain.blocks, queries, f.domain.limits)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.domain.f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, f.domain.f.f.db, f.domain.runs, queries, runner)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(f.domain.blocks, f.domain.runs, documents, compositions, f.domain.f.f.db, f.domain.limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
	request := phase31Request("dashboard", state.ID, "publication-race-no-consent")
	result, err := delivery.Run(t.Context(), f.domain.execute, request)
	if race.rootReads < 2 || err != nil || result.State != "completed" {
		t.Fatal("exact admitted dashboard did not survive concurrent publication", race.rootReads, result, err)
	}
	if f.domain.attemptCount(t) != beforeQueries || f.domain.f.model.requests.Load() != beforeModels {
		t.Fatal("new child publication caused unconsented execution")
	}
	retained, err := f.domain.f.f.db.ReadComposition(t.Context(), f.domain.execute, result.Run)
	if err != nil || len(retained.Manifest.Pages) != 1 || retained.Manifest.Pages[0].Revision != 1 || len(retained.Manifest.Groups) != 0 {
		t.Fatal("concurrent publication changed the sealed exact page", retained.Manifest, err)
	}
}
