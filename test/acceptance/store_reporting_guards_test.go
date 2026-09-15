package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

// The domain issues every proof and invocation in this test. A serializable task
// with a valid self-digest still cannot replace the server's original admission.
func TestStoreFrozenAdmissionAndCheckpointGuards(t *testing.T) {
	// An absent caller context is invalid input, not a background authority fallback.
	var missingContext context.Context
	f := newReportingStoreFixture(t)
	db, ctx := f.f.f.db, context.Background()
	block := f.create(t, "store-frozen-guards", true)
	hooks := &storeFrozenHooks{RunRepository: db}
	hooks.seal = func(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedRun) (reporting.RunRecord, error) {
		before := f.frozenSnapshot(t)
		t.Run("seal/requires_context", func(t *testing.T) {
			_, err := db.SealFrozenRun(missingContext, e, task, proof)
			if !errors.Is(err, access.ErrUnauthenticated) {
				t.Fatal("nil context admitted", err)
			}
		})
		t.Run("seal/rejects_untrusted_task", func(t *testing.T) {
			bad := task
			bad.ManifestHash = ""
			_, err := db.SealFrozenRun(ctx, e, bad, proof)
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatal("tampered operation accepted", err)
			}
		})
		t.Run("seal/self_digest_is_not_authority", func(t *testing.T) {
			bad := task
			bad.ID = "different-operation"
			bad.ManifestHash = bad.Digest()
			if !bad.Valid() {
				t.Fatal("fixture must pass task self-validation")
			}
			_, err := db.SealFrozenRun(ctx, e, bad, proof)
			if !errors.Is(err, store.ErrInvalid) {
				t.Fatal("another operation bound to original proof", err)
			}
		})
		if before != f.frozenSnapshot(t) {
			t.Fatal("denied seal wrote values or quota")
		}
		out, err := db.SealFrozenRun(ctx, e, task, proof)
		if err != nil {
			return out, err
		}
		before = f.frozenSnapshot(t)
		replay, err := db.SealFrozenRun(ctx, e, task, proof)
		if err != nil || replay.View.ManifestDigest != out.View.ManifestDigest || f.frozenSnapshot(t) != before {
			t.Fatal("same seal was not idempotent", err)
		}
		return out, nil
	}
	var completedInv jobs.Invocation
	var completedProof reporting.PreparedRunWrite
	hooks.checkpoint = func(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedRunWrite) (reporting.RunRecord, error) {
		w, err := proof.Checked(inv)
		if err != nil {
			return reporting.RunRecord{}, err
		}
		t.Run("checkpoint/"+w.Kind+"_requires_context", func(t *testing.T) {
			_, err := db.CheckpointFrozenRun(missingContext, inv, proof)
			if !errors.Is(err, access.ErrUnauthenticated) {
				t.Fatal("nil context checkpoint accepted", err)
			}
		})
		if w.Kind == "result" {
			t.Run("reuse/cannot_redirect_operation", func(t *testing.T) {
				_, reused, err := db.ReuseFrozenRun(ctx, inv, "another-operation")
				if !errors.Is(err, store.ErrInvalid) || reused {
					t.Fatal("redirected reuse accepted", err)
				}
				_, reused, err = db.ReuseFrozenRun(missingContext, inv, w.Manifest.ID)
				if !errors.Is(err, access.ErrUnauthenticated) || reused {
					t.Fatal("reuse without context accepted", err)
				}
			})
		}
		out, err := db.CheckpointFrozenRun(ctx, inv, proof)
		if err != nil {
			return out, err
		}
		if w.Kind == "result" || w.Kind == "output" {
			before := f.frozenSnapshot(t)
			replay, repeatErr := db.CheckpointFrozenRun(ctx, inv, proof)
			if repeatErr != nil || replay.View.RetainedBytes != out.View.RetainedBytes || f.frozenSnapshot(t) != before {
				t.Fatal("duplicate checkpoint changed values or double-charged bytes", w.Kind, repeatErr)
			}
			if w.Kind == "result" {
				ignored, reused, reuseErr := db.ReuseFrozenRun(ctx, inv, w.Manifest.ID)
				if reuseErr != nil || reused || ignored.Result == nil {
					t.Fatal("reuse replaced an already normalized result", reuseErr)
				}
			}
		}
		if w.Kind == "complete" {
			completedInv, completedProof = inv, proof
		}
		return out, nil
	}
	service := phase28RunService(t, f.f, f.blocks, hooks, nil, config.DefaultReportingExecution())
	admitted := f.admit(t, service, block.State.ID, "store-guard-key", 0)
	result, err := service.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || result.State != "succeeded" {
		t.Fatal("valid work after guard failures", result.State, err)
	}
	before := f.frozenSnapshot(t)
	if _, err = db.CheckpointFrozenRun(ctx, completedInv, completedProof); !errors.Is(err, store.ErrConflict) {
		t.Fatal("completed invocation was still a live commit fence", err)
	}
	if before != f.frozenSnapshot(t) {
		t.Fatal("stale invocation mutated the artifact")
	}
	if _, err = db.ReadFrozenRun(missingContext, f.execute, admitted.ID, true); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = db.ListFrozenArtifacts(missingContext, f.execute, "", 10); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = db.ExpireFrozenArtifacts(missingContext, f.execute, 10); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	// A completed task may be cancelled idempotently, but that must not claim a
	// new native cancellation or discard the already published result.
	cancelled, err := db.CancelFrozenRun(ctx, f.execute, admitted.ID)
	if err != nil || cancelled.State != "completed" || f.frozenSnapshot(t) != before {
		t.Fatal("completed cancellation changed artifact", cancelled.State, err)
	}
}

func TestStoreFrozenCancellationReach(t *testing.T) {
	// An absent caller context is invalid input, not a background authority fallback.
	var missingContext context.Context
	f := newReportingStoreFixture(t)
	db, ctx := f.f.f.db, context.Background()
	block := f.create(t, "store-private-cancel", false)
	if _, err := f.blocks.Validate(ctx, f.author, block.State.ID, reporting.ValidateRequest{ExpectedVersion: block.State.Version}); err != nil {
		t.Fatal(err)
	}
	admitted, err := f.runs.Admit(ctx, f.execute, block.State.ID, reporting.RunRequest{Key: "private-cancel-key", Reference: reporting.Reference{Revision: 1}, Policy: "private_preview"})
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"jobs.cancel", "cw.block.execute:" + block.State.ID, "cw.execution_context.use:" + admitted.Context, "reporting.preview", "cw.block.preview:" + block.State.ID}
	for _, tc := range []struct {
		name   string
		scopes []string
		actor  string
		want   error
	}{
		{"no_action", scopes[1:], f.execute.User(), access.ErrForbidden},
		{"no_target", append([]string{scopes[0]}, scopes[2:]...), f.execute.User(), access.ErrNotFound},
		{"no_context", []string{scopes[0], scopes[1], scopes[3], scopes[4]}, f.execute.User(), access.ErrNotFound},
		{"no_preview", scopes[:3], f.execute.User(), access.ErrForbidden},
		{"other_actor", scopes, "other-canceller", store.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := phase27Actor(t, f.f, tc.actor, tc.scopes)
			before := storeTableSnapshot(t, f.raw, "operations", "frozen_runs", "frozen_run_payloads")
			_, err := db.CancelFrozenRun(ctx, e, admitted.ID)
			if !errors.Is(err, tc.want) || before != storeTableSnapshot(t, f.raw, "operations", "frozen_runs", "frozen_run_payloads") {
				t.Fatal("unauthorized cancellation changed state", err)
			}
		})
	}
	canceller := phase27Actor(t, f.f, f.execute.User(), scopes)
	if _, err := db.CancelFrozenRun(missingContext, canceller, admitted.ID); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := db.CancelFrozenRun(ctx, canceller, "missing-run"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	// Cancel must be available with control/preview reach alone: no execution or
	// source-query action may be necessary to stop already admitted work.
	out, err := db.CancelFrozenRun(ctx, canceller, admitted.ID)
	if err != nil || out.State != "cancelled" {
		t.Fatal("control-only private cancellation", err)
	}
	runner, err := jobs.NewRequestRunner(db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := runner.Admit(ctx, f.execute, "unsealed-cancel-key", jobs.RequestInput{Kind: "reporting.run", Target: block.State.ID, InputHash: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CancelFrozenRun(ctx, canceller, reserved.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("unsealed operation fabricated artifact metadata", err)
	}
}

// Corrupt only the mutable copied receipt; the original durable read journal
// and all immutable definitions remain untouched. Reads must discard the whole
// response, not return rows or a partial list alongside an integrity error.
func TestStoreFrozenAttemptIntegrity(t *testing.T) {
	f := newReportingStoreFixture(t)
	db, ctx := f.f.f.db, context.Background()
	b := f.create(t, "store-receipt-integrity", true)
	v := f.admit(t, f.runs, b.State.ID, "store-receipt-integrity-key", 0)
	v, err := f.runs.Run(ctx, f.execute, v.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	var original []byte
	if err = f.raw.QueryRow(ctx, `SELECT receipt FROM chartworks.frozen_run_attempts WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), v.ID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"wrong_source", func(m map[string]any) {
			m["manifest"].(map[string]any)["validation"].(map[string]any)["source"] = "foreign-source"
		}},
		{"conflicting_attempt_id", func(m map[string]any) { m["id"] = strings.Repeat("b", 32) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(original, &value); err != nil {
				t.Fatal(err)
			}
			tc.change(value)
			bad, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.raw.Exec(ctx, `UPDATE chartworks.frozen_run_attempts SET receipt=$3 WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), v.ID, bad); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := f.raw.Exec(ctx, `UPDATE chartworks.frozen_run_attempts SET receipt=$3 WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), v.ID, original); err != nil {
					t.Error(err)
				}
			}()
			out, readErr := db.ReadFrozenRun(ctx, f.execute, v.ID, false)
			if !errors.Is(readErr, store.ErrInvalid) || out.Result != nil || out.Manifest != nil {
				t.Fatal("corrupted receipt returned retained values", readErr)
			}
			page, listErr := db.ListFrozenArtifacts(ctx, f.execute, "", 10)
			if !errors.Is(listErr, store.ErrInvalid) || len(page.Items) != 0 || page.Next != "" {
				t.Fatal("corrupted receipt leaked a partial catalog", listErr)
			}
		})
	}
	if _, err = db.ReadFrozenRun(ctx, f.execute, v.ID, false); err != nil {
		t.Fatal("valid receipt restoration", err)
	}
}
