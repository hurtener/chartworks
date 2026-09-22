package acceptance

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5/pgconn"
)

// Ordinary writes cannot replace retained values. Simulate storage corruption
// using only the isolated fixture owner's repair access, then restore the
// immutable trigger before reading through the real authorized repository.
func TestCW03RetainedPayloadIntegrity(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	d, err := reporting.MigrateDefinition(f.base)
	if err != nil {
		t.Fatal(err)
	}
	f.block(t, "cw03-retained-integrity", d)
	accepted, err := f.runs.Admit(ctx, f.execute, "cw03-retained-integrity", reporting.RunRequest{Key: "cw03-integrity", Outputs: []string{"table-main"}})
	if err != nil {
		t.Fatal(err)
	}
	done, err := f.runs.Run(ctx, f.execute, accepted.ID, false)
	if err != nil || done.State != "succeeded" {
		t.Fatal("integrity fixture did not retain a real result", done, err)
	}
	reader := phase28Reader(t, f.f, "cw03-integrity-reader", "cw03-retained-integrity", d.Context)
	otherContext := phase28Reader(t, f.f, "cw03-integrity-isolated", "cw03-retained-integrity", "other-context")
	before, err := f.f.f.db.ReadFrozenRun(ctx, reader, done.ID, false)
	if err != nil || before.Manifest == nil || before.Result == nil || len(before.Outputs) != 1 {
		t.Fatal("valid retained evidence unavailable", err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	queries, models := f.attemptCount(t), f.f.model.requests.Load()
	for _, tc := range []struct{ name, table, trigger, column, filter, damaged string }{
		{"result-digest-mismatch", "frozen_run_payloads", "frozen_payload_immutable", "result", "", `{"fields":[],"rows":[]}`},
		{"output-identity-mismatch", "frozen_run_outputs", "frozen_output_immutable", "payload", " AND output_id='table-main'", `{"id":"other-output","kind":"table","state":"succeeded"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// SQL identifiers/filters are fixed synthetic constants; retained
			// identities and bytes are parameters, never interpolated input.
			where := " WHERE tenant_id=$1 AND operation_id=$2" + tc.filter
			update := "UPDATE chartworks." + tc.table + " SET " + tc.column + "=$3" + where
			var original []byte
			if err := raw.QueryRow(t.Context(), "SELECT "+tc.column+" FROM chartworks."+tc.table+where, reader.Tenant(), done.ID).Scan(&original); err != nil {
				t.Fatal(err)
			}
			_, err := raw.Exec(t.Context(), update, reader.Tenant(), done.ID, []byte(tc.damaged))
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatal("ordinary retained mutation was not rejected by immutability", err)
			}
			repair := func(ctx context.Context, value []byte) error {
				if _, err := raw.Exec(ctx, "ALTER TABLE chartworks."+tc.table+" DISABLE TRIGGER "+tc.trigger); err != nil {
					return err
				}
				_, writeErr := raw.Exec(ctx, update, reader.Tenant(), done.ID, value)
				_, enableErr := raw.Exec(ctx, "ALTER TABLE chartworks."+tc.table+" ENABLE TRIGGER "+tc.trigger)
				return errors.Join(writeErr, enableErr)
			}
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := repair(cleanup, original); err != nil {
					t.Error("restore retained evidence and immutable trigger", err)
				}
			})
			if err := repair(t.Context(), []byte(tc.damaged)); err != nil {
				t.Fatal("prepare synthetic damaged storage", err)
			}
			out, readErr := f.f.f.db.ReadFrozenRun(t.Context(), reader, done.ID, false)
			if !errors.Is(readErr, store.ErrInvalid) || !reflect.DeepEqual(out, reporting.RunRecord{}) {
				t.Fatal("damaged retained evidence exposed a partial result", readErr)
			}
			isolated, isolatedErr := f.f.f.db.ReadFrozenRun(t.Context(), otherContext, done.ID, false)
			if !errors.Is(isolatedErr, store.ErrNotFound) || !reflect.DeepEqual(isolated, reporting.RunRecord{}) {
				t.Fatal("integrity failure bypassed current context eligibility", isolatedErr)
			}
			// Metadata listing is independent of protected payload decoding.
			page, listErr := f.f.f.db.ListFrozenArtifacts(t.Context(), reader, "", 10)
			if listErr != nil || len(page.Items) != 1 || page.Items[0].ID != done.ID {
				t.Fatal("metadata-only catalog depended on retained payload values", listErr)
			}
		})
	}
	after, err := f.f.f.db.ReadFrozenRun(ctx, reader, done.ID, false)
	if err != nil || !reflect.DeepEqual(before.Manifest, after.Manifest) || !reflect.DeepEqual(before.Result, after.Result) || !reflect.DeepEqual(before.Outputs, after.Outputs) {
		t.Fatal("repair changed the accepted manifest or retained evidence", err)
	}
	if f.attemptCount(t) != queries || f.f.model.requests.Load() != models {
		t.Fatal("integrity reads regenerated warehouse/model work")
	}
}
