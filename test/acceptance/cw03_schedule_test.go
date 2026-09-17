package acceptance

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

// Exercise the real queue, fresh signed execution authority, immutable parent
// and child manifests, query checkpoints and failing catalog transaction.
func TestCW03ScheduledCompositionRetryPins(t *testing.T) {
	f := newPhase30Fixture(t, false)
	block := cw03Definition(t, f.domain.base)
	f.domain.block(t, "cw03-scheduled-block", block)
	report := phase29Text("Versioned retained output selection")
	report.Widgets = []reporting.Widget{phase29BlockWidget("selected", "cw03-scheduled-block", 0, "table-main", "second")}
	report.Widgets[0].Block.Limits = &reporting.QueryLimits{MaxRows: 2, MaxBytes: 65536, TimeoutMillis: 10000, QueryAttempts: 1}
	state := f.domain.report(t, "cw03-scheduled-report", report, true)
	target := phase30Target("report", state.ID)
	target.Budget.MaxRows = 2
	target.Budget.MaxBytes = 65536
	target.Budget.QueryAttempts = 1
	job := f.submit(t, "cw03-scheduled-retry", target)
	raw := support.Raw(t, f.domain.f.f.dsn)
	_, err := raw.Exec(t.Context(), `CREATE FUNCTION chartworks.cw03_delivery_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.catalog_state='available' THEN RAISE EXCEPTION 'synthetic catalog outage' USING ERRCODE='58000'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER cw03_delivery_outage BEFORE UPDATE ON chartworks.reporting_occurrence_delivery FOR EACH ROW EXECUTE FUNCTION chartworks.cw03_delivery_outage();`)
	if err != nil {
		t.Fatal(err)
	}
	beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
	if err = f.queue.RunOnce(t.Context()); err == nil {
		t.Fatal("did not reach real catalog failure")
	}
	current := f.get(t, job.ID)
	if current.State != "retry" || current.Delivery == nil || current.Delivery.Catalog != "pending" || f.domain.attemptCount(t) != beforeQueries+1 {
		t.Fatal("not a post-query retry", current)
	}
	query := `SELECT manifest FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`
	var before, after []byte
	if err = raw.QueryRow(t.Context(), query, f.actor.Tenant(), job.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var manifest reporting.CompositionManifest
	if err = json.Unmarshal(before, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Revision != 1 || len(manifest.Groups) != 1 || manifest.Groups[0].Revision != 1 || manifest.Groups[0].QueryLimits == nil || manifest.Groups[0].QueryLimits.MaxRows != 2 || manifest.Groups[0].QueryLimits.QueryAttempts != 1 || !slices.Equal(manifest.Groups[0].Outputs, []string{"table-main", "second"}) {
		t.Fatal("accepted pins lost", manifest.Groups)
	}
	if _, err = raw.Exec(t.Context(), `DROP TRIGGER cw03_delivery_outage ON chartworks.reporting_occurrence_delivery; DROP FUNCTION chartworks.cw03_delivery_outage();`); err != nil {
		t.Fatal(err)
	}
	// Later publications change metadata, defaults, selection, and ceilings, but
	// do not withdraw the authority/lifecycle eligibility of the accepted revisions.
	changed := phase27Copy(t, block)
	changed.Outputs[0].Intent.Metadata[0].DisplayName = "Later output name"
	changed.Outputs[0].Intent.DefaultSelected = false
	changed.QueryLimits.MaxRows = 1
	f.republishBlock(t, "cw03-scheduled-block", changed)
	report.Widgets[0].Block.Outputs = []string{"second"}
	report.Widgets[0].Block.Limits.MaxRows = 1
	state, err = f.domain.documents.Edit(t.Context(), f.domain.author, "report", state.ID, state.Version, reporting.DocumentReference{}, report)
	if err != nil {
		t.Fatal(err)
	}
	phase29Publish(t, f.domain.documents, f.domain.author, state)
	// Ignore the explicitly requested validation reads for those new publications.
	afterPublication := f.domain.attemptCount(t)
	f.retryNow(t, job.ID)
	if err = f.queue.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	done := f.get(t, job.ID)
	if done.State != "succeeded" || done.Attempts != 2 || done.ManifestHash != job.ManifestHash || !reflect.DeepEqual(done.Reporting, job.Reporting) || done.Delivery.Catalog != "available" || f.domain.attemptCount(t) != afterPublication || f.domain.f.model.requests.Load() != beforeModels {
		t.Fatal("retry lost pins or regenerated accepted work", done)
	}
	if err = raw.QueryRow(t.Context(), query, f.actor.Tenant(), job.ID).Scan(&after); err != nil || string(before) != string(after) {
		t.Fatal("retry rewrote accepted composition manifest", err)
	}
	view, err := f.delivery.View(t.Context(), f.domain.execute, reporting.DeliveryViewRequest{Kind: "report", Run: job.ID, Page: "main", Widget: "selected", Output: "table-main", Limit: 1})
	if err != nil || view.Output == nil || view.AcceptedSelection == nil || !slices.Equal(view.AcceptedSelection.Selected, []string{"table-main", "second"}) || view.QueryLimits == nil || view.QueryLimits.MaxRows != 2 || view.QueryLimits.QueryAttempts != 1 || view.Output.Intent.Metadata[0].DisplayName != "Output table-main" {
		t.Fatal("retained scheduled projection drifted", view, err)
	}
	if f.domain.attemptCount(t) != afterPublication || f.domain.f.model.requests.Load() != beforeModels {
		t.Fatal("scheduled retained read performed work")
	}
}
