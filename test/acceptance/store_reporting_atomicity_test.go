package acceptance

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

type reportingStoreFixture struct {
	f               *phase17Fixture
	blocks          *reporting.Service
	runs            *reporting.Runs
	author, execute identity.Envelope
	definition      reporting.Definition
	raw             *pgx.Conn
}

func newReportingStoreFixture(t *testing.T) *reportingStoreFixture {
	t.Helper()
	f := newPhase18Fixture(t)
	query, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	d := phase27Definition(t, f, author, "SELECT id, amount FROM analytics.sales ORDER BY id")
	d.Outputs = d.Outputs[:1]
	return &reportingStoreFixture{f: f, blocks: blocks, runs: phase28RunService(t, f, blocks, f.f.db, nil, config.DefaultReportingExecution()), author: author,
		execute: phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant())), definition: d, raw: support.Raw(t, f.f.dsn)}
}

func (f *reportingStoreFixture) create(t *testing.T, id string, publish bool) reporting.View {
	t.Helper()
	out, err := f.blocks.Create(context.Background(), f.author, reporting.CreateRequest{ID: id, Definition: f.definition})
	if err != nil {
		t.Fatal(err)
	}
	if publish {
		phase27ValidatePublish(t, f.blocks, f.author, out)
	}
	return out
}

func (f *reportingStoreFixture) admit(t *testing.T, service *reporting.Runs, block, key string, reuse int) reporting.RunView {
	t.Helper()
	out, err := service.Admit(context.Background(), f.execute, block, reporting.RunRequest{Key: key, ReuseMaxAgeSeconds: reuse})
	if err != nil {
		t.Fatal("admit test run", err)
	}
	return out
}

func (f *reportingStoreFixture) blockSnapshot(t *testing.T) string {
	t.Helper()
	return storeTableSnapshot(t, f.raw, "block_heads", "block_revisions", "block_revision_references", "block_topic_pins", "block_source_pins", "block_validations", "block_publications", "block_attestations", "block_withdrawals", "block_health", "block_events")
}

func (f *reportingStoreFixture) frozenSnapshot(t *testing.T) string {
	t.Helper()
	return storeTableSnapshot(t, f.raw, "frozen_runs", "frozen_run_payloads", "frozen_run_outputs", "frozen_run_attempts")
}

// Real write faults exercise the transaction branches that ordinary successful
// acceptance cannot reach. No immutable-history trigger is disabled.
func TestStoreBlockPublicationRollback(t *testing.T) {
	f := newReportingStoreFixture(t)
	ctx := context.Background()
	t.Run("create", func(t *testing.T) {
		for _, table := range []string{"block_heads", "block_revisions", "block_revision_references", "block_topic_pins", "block_source_pins", "block_events", "audit_events"} {
			t.Run(table, func(t *testing.T) {
				before := f.blockSnapshot(t)
				remove := storeWriteFault(t, f.raw, table, "INSERT", "")
				out, err := f.blocks.Create(ctx, f.author, reporting.CreateRequest{ID: "store-create-rollback", Definition: f.definition})
				remove()
				if !errors.Is(err, store.ErrUnavailable) || out.State.ID != "" {
					t.Fatal("failed creation returned success or unsanitized error", err)
				}
				if after := f.blockSnapshot(t); after != before {
					t.Fatal("failed creation left a head, revision, reference or audit event")
				}
			})
		}
	})
	created := f.create(t, "store-lifecycle-rollback", false)
	for _, table := range []string{"block_validations", "block_health", "block_events", "audit_events"} {
		t.Run("validate/"+table, func(t *testing.T) {
			before := f.blockSnapshot(t)
			condition := ""
			if table == "audit_events" {
				condition = "NEW.action='block.validated'"
			}
			remove := storeWriteFault(t, f.raw, table, "INSERT", condition)
			_, err := f.blocks.Validate(ctx, f.author, created.State.ID, reporting.ValidateRequest{ExpectedVersion: created.State.Version})
			remove()
			if !errors.Is(err, store.ErrUnavailable) {
				t.Fatal("failed validation checkpoint was accepted", err)
			}
			if f.blockSnapshot(t) != before {
				t.Fatal("validation checkpoint partially committed")
			}
		})
	}
	validated, err := f.blocks.Validate(ctx, f.author, created.State.ID, reporting.ValidateRequest{ExpectedVersion: created.State.Version})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ table, operation string }{{"block_publications", "INSERT"}, {"block_heads", "UPDATE"}, {"block_events", "INSERT"}, {"audit_events", "INSERT"}} {
		t.Run("publish/"+tc.table, func(t *testing.T) {
			before := f.blockSnapshot(t)
			remove := storeWriteFault(t, f.raw, tc.table, tc.operation, "")
			_, err := f.blocks.Publish(ctx, f.author, created.State.ID, reporting.PublishRequest{ExpectedVersion: validated.State.Version, Evidence: validated.Evidence.ID})
			remove()
			if !errors.Is(err, store.ErrUnavailable) || f.blockSnapshot(t) != before {
				t.Fatal("failed publication changed pointer or immutable history", err)
			}
		})
	}
	state, err := f.blocks.Publish(ctx, f.author, created.State.ID, reporting.PublishRequest{ExpectedVersion: validated.State.Version, Evidence: validated.Evidence.ID})
	if err != nil {
		t.Fatal("publish after fault removal", err)
	}
	for _, table := range []string{"block_attestations", "block_events", "audit_events"} {
		t.Run("certify/"+table, func(t *testing.T) {
			before := f.blockSnapshot(t)
			remove := storeWriteFault(t, f.raw, table, "INSERT", "")
			_, err := f.blocks.Certify(ctx, f.author, state.ID, reporting.CertifyRequest{ExpectedVersion: state.Version, Revision: state.PublishedRevision, Evidence: validated.Evidence.ID, Note: "Synthetic reviewed evidence"})
			remove()
			if !errors.Is(err, store.ErrUnavailable) || f.blockSnapshot(t) != before {
				t.Fatal("failed certification mutated business approval", err)
			}
		})
	}
}

type storeFrozenHooks struct {
	reporting.RunRepository
	seal       func(context.Context, identity.Envelope, jobs.RequestTask, reporting.PreparedRun) (reporting.RunRecord, error)
	checkpoint func(context.Context, jobs.Invocation, reporting.PreparedRunWrite) (reporting.RunRecord, error)
	reuse      func(context.Context, jobs.Invocation, string) (reporting.RunRecord, bool, error)
}

func (r *storeFrozenHooks) SealFrozenRun(ctx context.Context, e identity.Envelope, task jobs.RequestTask, p reporting.PreparedRun) (reporting.RunRecord, error) {
	if r.seal != nil {
		return r.seal(ctx, e, task, p)
	}
	return r.RunRepository.SealFrozenRun(ctx, e, task, p)
}
func (r *storeFrozenHooks) CheckpointFrozenRun(ctx context.Context, inv jobs.Invocation, p reporting.PreparedRunWrite) (reporting.RunRecord, error) {
	if r.checkpoint != nil {
		return r.checkpoint(ctx, inv, p)
	}
	return r.RunRepository.CheckpointFrozenRun(ctx, inv, p)
}
func (r *storeFrozenHooks) ReuseFrozenRun(ctx context.Context, inv jobs.Invocation, id string) (reporting.RunRecord, bool, error) {
	if r.reuse != nil {
		return r.reuse(ctx, inv, id)
	}
	return r.RunRepository.ReuseFrozenRun(ctx, inv, id)
}

func TestStoreFrozenCheckpointRollback(t *testing.T) {
	f := newReportingStoreFixture(t)
	block := f.create(t, "store-frozen-rollback", true)
	ctx := context.Background()
	for _, table := range []string{"frozen_runs", "frozen_run_payloads", "audit_events"} {
		t.Run("seal/"+table, func(t *testing.T) {
			before := f.frozenSnapshot(t)
			r := &storeFrozenHooks{RunRepository: f.f.f.db}
			r.seal = func(ctx context.Context, e identity.Envelope, task jobs.RequestTask, p reporting.PreparedRun) (reporting.RunRecord, error) {
				remove := storeWriteFault(t, f.raw, table, "INSERT", "")
				out, err := f.f.f.db.SealFrozenRun(ctx, e, task, p)
				remove()
				return out, err
			}
			service := phase28RunService(t, f.f, f.blocks, r, nil, config.DefaultReportingExecution())
			_, err := service.Admit(ctx, f.execute, block.State.ID, reporting.RunRequest{Key: "seal-fault-" + table})
			if !errors.Is(err, store.ErrUnavailable) || f.frozenSnapshot(t) != before {
				t.Fatal("seal failure leaked manifest/quota or returned success", err)
			}
		})
	}
	for index, tc := range []struct{ kind, table, operation, condition string }{
		{"attempt", "frozen_run_attempts", "INSERT", ""},
		{"result", "frozen_runs", "UPDATE", ""},
		{"result", "frozen_run_payloads", "UPDATE", ""},
		{"result", "audit_events", "INSERT", "NEW.action='reporting.query_checkpoint'"},
		{"output", "frozen_run_outputs", "INSERT", ""},
		{"output", "audit_events", "INSERT", "NEW.action='reporting.output_checkpoint'"},
		{"complete", "frozen_runs", "UPDATE", "NEW.state='succeeded'"},
		{"complete", "operations", "UPDATE", "NEW.status='succeeded'"},
		{"complete", "audit_events", "INSERT", "NEW.action='reporting.run_completed'"},
	} {
		t.Run(tc.kind+"/"+tc.table, func(t *testing.T) {
			called := false
			r := &storeFrozenHooks{RunRepository: f.f.f.db}
			r.checkpoint = func(ctx context.Context, inv jobs.Invocation, p reporting.PreparedRunWrite) (reporting.RunRecord, error) {
				w, err := p.Checked(inv)
				if err != nil {
					return reporting.RunRecord{}, err
				}
				if called || w.Kind != tc.kind {
					return f.f.f.db.CheckpointFrozenRun(ctx, inv, p)
				}
				called = true
				before := f.frozenSnapshot(t)
				remove := storeWriteFault(t, f.raw, tc.table, tc.operation, tc.condition)
				out, err := f.f.f.db.CheckpointFrozenRun(ctx, inv, p)
				remove()
				if f.frozenSnapshot(t) != before {
					t.Error("failed checkpoint changed values, usage or completion")
				}
				return out, err
			}
			service := phase28RunService(t, f.f, f.blocks, r, nil, config.DefaultReportingExecution())
			v := f.admit(t, service, block.State.ID, fmt.Sprintf("checkpoint-fault-%d", index), 0)
			out, err := service.Run(ctx, f.execute, v.ID, false)
			if !called || !errors.Is(err, store.ErrUnavailable) || out.State == "succeeded" {
				t.Fatal("checkpoint failure not exercised or falsely completed", called, out.State, err)
			}
			if tc.kind == "output" || tc.kind == "complete" {
				resumed, err := f.runs.Run(ctx, f.execute, v.ID, true)
				if err != nil || resumed.State != "succeeded" || len(resumed.QueryAttempts) != 1 {
					t.Fatal("failed output/completion could not resume the one retained query", resumed.State, err)
				}
			}
		})
	}
}

func TestStoreFrozenReuseRollback(t *testing.T) {
	f := newReportingStoreFixture(t)
	block := f.create(t, "store-reuse-rollback", true)
	ctx := context.Background()
	original := f.admit(t, f.runs, block.State.ID, "reuse-original", 0)
	if _, err := f.runs.Run(ctx, f.execute, original.ID, false); err != nil {
		t.Fatal(err)
	}
	for index, tc := range []struct{ table, operation, condition string }{
		{"frozen_runs", "UPDATE", "NEW.retained_bytes<>OLD.retained_bytes"},
		{"frozen_run_payloads", "UPDATE", ""},
		{"frozen_run_outputs", "INSERT", ""},
		{"frozen_runs", "UPDATE", "NEW.state='succeeded'"},
		{"operations", "UPDATE", "NEW.status='succeeded'"},
		{"audit_events", "INSERT", "NEW.action='reporting.run_reused'"},
	} {
		t.Run(fmt.Sprintf("%d-%s", index, tc.table), func(t *testing.T) {
			r := &storeFrozenHooks{RunRepository: f.f.f.db}
			called := false
			r.reuse = func(ctx context.Context, inv jobs.Invocation, id string) (reporting.RunRecord, bool, error) {
				called = true
				before := f.frozenSnapshot(t)
				remove := storeWriteFault(t, f.raw, tc.table, tc.operation, tc.condition)
				out, reused, err := f.f.f.db.ReuseFrozenRun(ctx, inv, id)
				remove()
				if reused || f.frozenSnapshot(t) != before {
					t.Error("failed reuse committed a partial clone or reported reuse")
				}
				return out, reused, err
			}
			service := phase28RunService(t, f.f, f.blocks, r, nil, config.DefaultReportingExecution())
			v := f.admit(t, service, block.State.ID, fmt.Sprintf("reuse-fault-%d", index), 3600)
			_, err := service.Run(ctx, f.execute, v.ID, false)
			if !called || !errors.Is(err, store.ErrUnavailable) {
				t.Fatal("reuse fault not observed", called, err)
			}
			resumed, err := f.runs.Run(ctx, f.execute, v.ID, true)
			if err != nil || resumed.State != "succeeded" || len(resumed.QueryAttempts) != 0 || resumed.ReusedFrom == "" {
				t.Fatal("reuse recovery unexpectedly executed a source query", resumed.State, err)
			}
		})
	}
}
