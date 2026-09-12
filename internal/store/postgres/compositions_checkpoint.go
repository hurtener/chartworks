package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func verifyCompositionGroupTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.CompositionManifest, g reporting.CompositionGroup, r reporting.GroupResult) error {
	if err := reporting.CheckCompositionResult(m, g, r); err != nil {
		return err
	}
	if r.State == "failed" {
		return nil
	}
	if g.Kind == "block" {
		child, err := frozenReadTx(ctx, tx, e, r.ChildRun, true, true)
		if err != nil {
			return err
		}
		if child.Manifest == nil || child.Result == nil || r.Block == nil || child.View.ManifestDigest != r.Block.ManifestDigest || child.View.RevisionDigest != g.Definition || child.View.Private != m.Private || len(child.Outputs) != len(r.Outputs) {
			return store.ErrInvalid
		}
		for i, output := range child.Outputs {
			if readexec.Hash(output) != readexec.Hash(r.Outputs[i]) {
				return store.ErrInvalid
			}
		}
		return nil
	}
	q := r.Query
	if q == nil || r.QueryPlan == nil || r.QueryPlan.Operation != "composition:"+m.ID+":"+g.ID || q.Execution.Attempt.Manifest.Operation != r.QueryPlan.Operation || q.Execution.Attempt.Manifest.Session != e.Session() {
		return store.ErrInvalid
	}
	actual, err := scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND attempt_id=$4 FOR SHARE`, e.Tenant(), e.User(), r.QueryPlan.Operation, q.Execution.Attempt.ID))
	if err != nil {
		return err
	}
	if readexec.Hash(actual) != readexec.Hash(q.Execution.Attempt) || actual.RemoteState != "stopped" || actual.Finished == nil || !actual.Finished.Equal(*r.Observed) || actual.Manifest.Receipt.Source != g.Binding.Source || actual.Manifest.Receipt.Context != g.Binding.Context {
		return store.ErrInvalid
	}
	var retained []byte
	if err := tx.QueryRow(ctx, `SELECT result FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4 AND operation=$5 AND status IN('succeeded','empty','truncated')`, e.Tenant(), e.User(), e.Session(), q.Query, r.QueryPlan.Operation).Scan(&retained); err != nil {
		return err
	}
	var result readexec.Result
	if json.Unmarshal(retained, &result) != nil || readexec.Hash(result) != readexec.Hash(q.Execution.Result) {
		return store.ErrInvalid
	}
	return nil
}

func updateCompositionSummaries(ctx context.Context, tx pgx.Tx, tenant string, r reporting.CompositionRecord) error {
	view := reporting.SummarizeComposition(r)
	for _, page := range view.Pages {
		body, err := json.Marshal(page)
		if err != nil {
			return store.ErrInvalid
		}
		if _, err := tx.Exec(ctx, `UPDATE chartworks.composition_run_pages SET summary=$4 WHERE tenant_id=$1 AND operation_id=$2 AND page_id=$3`, tenant, r.Manifest.ID, page.ID, body); err != nil {
			return err
		}
	}
	return nil
}

// CheckpointComposition retains values only under the live common operation
// fence. Successful values are checked against actual child and read journals.
func (d *DB) CheckpointComposition(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedCompositionWrite) (out reporting.CompositionRecord, err error) {
	w, err := proof.Checked(inv)
	if err != nil {
		return out, err
	}
	m := w.Manifest
	e, err := inv.Current(m.Kind+".run", m.Document, m.RequestHash)
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := requestFenceTx(ctx, tx, inv); err != nil {
			return err
		}
		h, err := compositionHeadTx(ctx, tx, e, m.ID, true, true)
		if err != nil {
			return err
		}
		if h.view.Manifest != m.ManifestDigest() || h.taskHash != inv.Lease().Task.ManifestHash || h.view.State != "sealed" {
			return store.ErrConflict
		}
		out, err = compositionReadTx(ctx, tx, e, h)
		if err != nil {
			return err
		}
		action := ""
		switch w.Kind {
		case "query_start":
			if out.Started[w.Group] {
				return store.ErrConflict
			}
			if _, err := tx.Exec(ctx, `UPDATE chartworks.composition_run_groups SET started=true WHERE tenant_id=$1 AND operation_id=$2 AND group_id=$3 AND kind='query' AND NOT started`, e.Tenant(), m.ID, w.Group); err != nil {
				return err
			}
			out.Started[w.Group] = true
			action = "composition.query_started"
		case "plan":
			if w.Plan == nil || !out.Started[w.Group] || w.Plan.Operation != "composition:"+m.ID+":"+w.Group {
				return store.ErrInvalid
			}
			if old, exists := out.Plans[w.Group]; exists {
				if old != *w.Plan {
					return store.ErrConflict
				}
				return nil
			}
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4 AND operation=$5)`, e.Tenant(), e.User(), e.Session(), w.Plan.Query, w.Plan.Operation).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return store.ErrInvalid
			}
			body, err := json.Marshal(w.Plan)
			if err != nil || len(body) > 4096 {
				return store.ErrInvalid
			}
			out.Plans[w.Group] = *w.Plan
			if _, err := tx.Exec(ctx, `UPDATE chartworks.composition_run_groups SET plan=$4 WHERE tenant_id=$1 AND operation_id=$2 AND group_id=$3 AND plan IS NULL`, e.Tenant(), m.ID, w.Group, body); err != nil {
				return err
			}
			action = "composition.plan_checkpoint"
		case "group":
			if w.Result == nil {
				return store.ErrInvalid
			}
			for _, old := range out.Results {
				if old.Group == w.Group {
					if old.Digest != w.Result.Digest {
						return store.ErrConflict
					}
					return nil
				}
			}
			group, exists := compositionGroupByID(m, w.Group)
			if !exists {
				return store.ErrInvalid
			}
			if w.Result.QueryPlan != nil {
				if plan, exists := out.Plans[w.Group]; !exists || plan != *w.Result.QueryPlan {
					return store.ErrInvalid
				}
			}
			if err := verifyCompositionGroupTx(ctx, tx, e, m, group, *w.Result); err != nil {
				return err
			}
			body, err := reporting.CompositionResultJSON(*w.Result)
			if err != nil {
				return err
			}
			out.Results = append(out.Results, *w.Result)
			if _, err := tx.Exec(ctx, `UPDATE chartworks.composition_run_groups SET result=$4,result_digest=$5 WHERE tenant_id=$1 AND operation_id=$2 AND group_id=$3 AND result IS NULL`, e.Tenant(), m.ID, w.Group, body, w.Result.Digest); err != nil {
				return err
			}
			action = "composition.group_checkpoint"
		case "complete":
			state, code, mixed, err := reporting.CompositionCompletion(m, out.Results)
			if err != nil {
				return err
			}
			if w.Outcome != state || w.Code != code {
				return store.ErrInvalid
			}
			if _, err := tx.Exec(ctx, `UPDATE chartworks.composition_runs SET state=$3,code=$4,complete=$5,mixed_freshness=$6,reserved_bytes=0,finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), m.ID, state, code, state == "completed", mixed); err != nil {
				return err
			}
			if err := completeRequestTx(ctx, tx, inv); err != nil {
				return err
			}
			out.State, out.Code = state, code
			action = "composition.completed"
		default:
			return store.ErrInvalid
		}
		charged := reporting.CompositionRetainedBytes(out)
		remaining := len(m.Groups) - len(out.Results)
		if charged+int64(remaining)*1024 > h.maximum {
			return reporting.ErrBudget
		}
		if _, err := tx.Exec(ctx, `UPDATE chartworks.composition_runs SET retained_bytes=$3 WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), m.ID, charged); err != nil {
			return err
		}
		if err := updateCompositionSummaries(ctx, tx, e.Tenant(), out); err != nil {
			return err
		}
		if err := frozenAudit(ctx, tx, e, action, m.ID); err != nil {
			return err
		}
		if _, err := proof.Checked(inv); err != nil {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	if !e.Valid() {
		return reporting.CompositionRecord{}, access.ErrUnauthenticated
	}
	return out, nil
}
