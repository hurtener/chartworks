package acceptance

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/reporting"
	"strings"
	"testing"
)

func testPhase30CatalogProvenance(t *testing.T) {
	for _, kind := range []string{"saved_sql", "report"} {
		t.Run(kind, func(t *testing.T) {
			f := newPhase30Fixture(t, false)
			f.domain.block(t, "p30-provenance-block", f.domain.base)
			id := "p30-provenance-block"
			resource := "block"
			if kind == "report" {
				doc := phase29Text("Public occurrence provenance")
				doc.Widgets = append(doc.Widgets, phase29BlockWidget("frozen", id, 1, "table-main"))
				id = "p30-provenance-report"
				f.domain.report(t, id, doc, true)
				resource = "report"
			}
			target := phase30Target(kind, id)
			target.Recipients = []string{"PRIVATE_RECIPIENT_CANARY"}
			schedule := f.schedule(t, "provenance-schedule", phase30Manual(target))
			job, err := f.queue.TestSchedule(t.Context(), f.manager(t), schedule.ID, "provenance-occurrence", schedule.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.queue.RunOnce(t.Context()); err != nil {
				t.Fatal(err)
			}
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			selection := reporting.ReportingViewRequest{Kind: resource, Run: job.ID, Output: "table-main", Limit: 10}
			if resource == "report" {
				selection.Page, selection.Widget = "main", "frozen"
			}
			view, err := f.delivery.View(t.Context(), f.domain.execute, selection)
			if err != nil || view.Output == nil || view.Summary.Scheduled == nil {
				t.Fatal("ordinary viewer lost scheduled provenance", view, err)
			}
			p := view.Summary.Scheduled
			if p.ScheduleID != schedule.ID || p.ScheduleRevision != schedule.Revision || !p.DueAt.Equal(job.DueAt) || !p.WindowStart.Equal(job.WindowStart) || !p.WindowEnd.Equal(job.WindowEnd) ||
				p.Execution != "succeeded" || p.Query != "succeeded" || p.Artifact != "retained" || p.Catalog != "available" || p.Notification != "not_requested" || p.PublishedAt == nil {
				t.Fatal("occurrence/delivery evidence changed", p)
			}
			page, err := f.delivery.Runs(t.Context(), f.domain.execute, reporting.ReportingRunsRequest{Kind: resource, Resource: id, Limit: 10})
			if err != nil || len(page.Items) != 1 || page.Items[0].Run != job.ID || page.Items[0].Scheduled == nil {
				t.Fatal("ordinary catalog lost scheduled provenance", page, err)
			}
			want, _ := json.Marshal(p)
			got, _ := json.Marshal(page.Items[0].Scheduled)
			if string(want) != string(got) {
				t.Fatal("viewer/catalog occurrence evidence diverged")
			}
			wire, _ := json.Marshal(view)
			for _, secret := range []string{"PRIVATE_RECIPIENT_CANARY", `"binding_id"`, `"executor"`, `"authorization"`} {
				if strings.Contains(string(wire), secret) {
					t.Fatal("scheduling authority/recipient details entered the viewer")
				}
			}
			reader := phase27Actor(t, f.domain.f, "without-source-context", []string{"reporting.read", "cw." + resource + ".read:" + id})
			hidden, err := f.delivery.View(t.Context(), reader, selection)
			if err == nil && (hidden.Output != nil || hidden.Summary.Scheduled != nil) {
				t.Fatal("hidden context exposed values or occurrence-derived status", hidden)
			}
			if beforeQueries != f.domain.attemptCount(t) || beforeModels != f.domain.f.model.requests.Load() {
				t.Fatal("provenance read executed source/model")
			}
		})
	}
}
