package acceptance

import (
	"testing"

	"github.com/hurtener/chartworks/test/support"
)

// Withdraw the actual topic and its facet heads through the existing lifecycle
// owner. Source removal models the real persisted unavailable-source state; it
// does not alter accepted manifests, results, scopes or authority responses.
func phase30WithdrawDependency(t *testing.T, f *phase30Fixture, change string) {
	t.Helper()
	switch change {
	case "archive-topic":
		_, published := newPhase18Service(t, f.domain.f)
		id := f.domain.base.Topics[0].Topic
		current, err := published.Read(t.Context(), f.domain.f.e, id, "")
		if err != nil {
			t.Fatal("read current topic for withdrawal", err)
		}
		if _, err := published.Archive(t.Context(), f.domain.f.e, id, current.State.Revision, "Withdraw accepted semantic dependency before publication"); err != nil {
			t.Fatal("actual topic and facet withdrawal", err)
		}
	case "delete-source":
		tag, err := support.Raw(t, f.domain.f.f.dsn).Exec(t.Context(), `UPDATE chartworks.sources SET deleted=true WHERE tenant_id=$1 AND source_id=$2`, f.actor.Tenant(), f.domain.base.Source)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatal("source withdrawal fixture", tag.RowsAffected(), err)
		}
	default:
		t.Fatal("unsupported dependency withdrawal fixture", change)
	}
}

// Dynamic groups have semantic/source pins but no block certificate. Successful
// query evidence must not hide a withdrawn dependency from retry eligibility.
func TestPhase30AdversarialDynamicDependencies(t *testing.T) {
	for _, change := range []string{"unchanged", "archive-topic", "delete-source"} {
		t.Run(change, func(t *testing.T) {
			f := newPhase30Fixture(t, true)
			d := phase29Text("Dynamic dependency eligibility on retry")
			d.Widgets = append(d.Widgets, f.domain.queryWidget())
			state := f.domain.report(t, "p30-dynamic-retry", d, true)
			job := f.submit(t, "dynamic-eligibility", phase30Target("saved_question", state.ID))
			raw := support.Raw(t, f.domain.f.f.dsn)
			if _, err := raw.Exec(t.Context(), `CREATE FUNCTION chartworks.p30_dynamic_delivery() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.catalog_state='available' THEN RAISE EXCEPTION 'synthetic catalog outage' USING ERRCODE='58000'; END IF; RETURN NEW; END; $$;
 CREATE TRIGGER p30_dynamic_delivery BEFORE UPDATE ON chartworks.reporting_occurrence_delivery FOR EACH ROW EXECUTE FUNCTION chartworks.p30_dynamic_delivery();`); err != nil {
				t.Fatal(err)
			}
			beforeQueries, beforeModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			if err := f.queue.RunOnce(t.Context()); err == nil {
				t.Fatal("publication outage was not exercised")
			}
			current := f.get(t, job.ID)
			afterQueries, afterModels := f.domain.attemptCount(t), f.domain.f.model.requests.Load()
			if current.State != "retry" || current.Delivery == nil || current.Delivery.Catalog != "pending" || afterQueries <= beforeQueries || afterModels <= beforeModels {
				t.Fatal("dynamic fixture did not reach the post-query publication boundary", current)
			}
			var completed int
			if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2 AND kind='query' AND convert_from(result,'UTF8')::jsonb->>'state'='completed'`, f.actor.Tenant(), job.ID).Scan(&completed); err != nil || completed != 1 {
				t.Fatal("missing durable dynamic query result", completed, err)
			}
			if _, err := raw.Exec(t.Context(), `DROP TRIGGER p30_dynamic_delivery ON chartworks.reporting_occurrence_delivery; DROP FUNCTION chartworks.p30_dynamic_delivery();`); err != nil {
				t.Fatal(err)
			}
			if change != "unchanged" {
				phase30WithdrawDependency(t, f, change)
			}
			f.retryNow(t, job.ID)
			err := f.queue.RunOnce(t.Context())
			current = f.get(t, job.ID)
			if current.ManifestHash != job.ManifestHash || current.Attempts != 2 || f.domain.attemptCount(t) != afterQueries || f.domain.f.model.requests.Load() != afterModels {
				t.Fatal("retry changed pins or regenerated query/model work", current, err)
			}
			if change == "unchanged" {
				if err != nil || current.State != "succeeded" || current.Delivery.Catalog != "available" {
					t.Fatal("eligible dynamic delivery retry refused", current, err)
				}
			} else if err == nil || current.State != "blocked" || current.Delivery.Catalog == "available" {
				t.Fatal("P1: dynamic retry published with an unavailable accepted dependency", change, current, err)
			}
		})
	}
}
