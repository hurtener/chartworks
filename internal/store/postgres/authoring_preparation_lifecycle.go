package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

type preparationLifecycle struct {
	guarded    bool
	admitted   string
	manifest   []byte
	settlement []byte
}

// Caller holds custody FOR UPDATE before this metadata is used. Native admission
// takes the same lock; missing journals alone never prove absence of dispatch.
func preparationLifecycleTx(ctx context.Context, tx pgx.Tx, tenant, id string) (m preparationLifecycle, err error) {
	err = tx.QueryRow(ctx, `SELECT admission_guarded,COALESCE(admitted_attempt_id,''),admitted_manifest,settlement FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, tenant, id).Scan(&m.guarded, &m.admitted, &m.manifest, &m.settlement)
	return
}

// The custody row is already locked. Read-only validation of the shared
// predicate adds no reverse native lock acquisition.
func assertPreparationSettledTx(ctx context.Context, tx pgx.Tx, tenant, id string) error {
	var unresolved bool
	if err := tx.QueryRow(ctx, `SELECT chartworks.preparation_has_liability(p) FROM chartworks.authoring_preparations p WHERE tenant_id=$1 AND preparation_id=$2`, tenant, id).Scan(&unresolved); err != nil {
		return err
	}
	if unresolved {
		return readexec.ErrUncertain
	}
	return nil
}

// SealAuthoringPreparation pins the validator receipt before native admission.
func (d *DB) SealAuthoringPreparation(ctx context.Context, e identity.Envelope, r reporting.AuthoringPreparationRecord, receipt readexec.Receipt) error {
	if err := reporting.RequireAuthoringPreparation(e, r, true); err != nil {
		return err
	}
	if !receipt.Validated || receipt.Source != r.Binding.Source || receipt.Context != r.Binding.Context || receipt.Dialect != "postgres" || len(receipt.Manifest) != 64 || len(receipt.Dependencies) != 1 || receipt.Dependencies[0] != r.Request.Intent.Dataset {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		prior, err := authoringPreparationRecordTx(ctx, tx, e, r.ID, false, true, true)
		if err != nil {
			return err
		}
		if prior.InputDigest != r.InputDigest || prior.Status != "accepted" || !time.Now().Before(prior.Deadline) {
			return store.ErrConflict
		}
		if err := authoringPreparationFence(ctx, tx, e, prior); err != nil {
			return err
		}
		raw, _ := json.Marshal(receipt)
		tag, err := tx.Exec(ctx, `UPDATE chartworks.authoring_preparations SET read_receipt=$3 WHERE tenant_id=$1 AND preparation_id=$2 AND admission_guarded AND settlement IS NULL AND read_receipt IS NULL AND admitted_attempt_id IS NULL AND deadline>clock_timestamp()`, e.Tenant(), r.ID, raw)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		return reporting.RequireAuthoringPreparation(e, prior, true)
	})
}

// settlementTx never locks the native journal after a witness already exists.
// Native retention excludes every linked row without a matching witness. This
// prevents custody->journal and retention->custody lock inversion.
func preparationSettlementTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, r reporting.AuthoringPreparationRecord, m preparationLifecycle, allowNoDispatch bool) ([]byte, error) {
	if m.settlement != nil {
		return m.settlement, nil
	}
	a, err := scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND attempt_number=1 FOR SHARE`, e.Tenant(), e.User(), r.SourceOperation))
	if errors.Is(err, pgx.ErrNoRows) {
		if !allowNoDispatch || !m.guarded || m.admitted != "" {
			return nil, nil
		}
		proof := reporting.PreparationSettlement{Kind: "not_issued", Status: "failed", RemoteState: "not_issued", Finished: time.Now().UTC()}
		return json.Marshal(proof)
	}
	if err != nil {
		return nil, err
	}
	if a.Number != 1 || a.Manifest.Operation != r.SourceOperation || a.Manifest.Session != r.Session || a.Manifest.Receipt.Source != r.Binding.Source || a.Manifest.Receipt.Context != r.Binding.Context || a.Finished == nil || !slices.Contains([]string{"succeeded", "empty", "truncated", "cancelled", "timed_out", "failed", "interrupted"}, a.Status) || !slices.Contains([]string{"stopped", "not_issued"}, a.RemoteState) || a.RemoteState == "not_issued" && a.Remote != nil {
		return nil, nil
	}
	expected := m.manifest
	if expected == nil && r.Attempt != nil {
		expected, _ = json.Marshal(r.Attempt.Manifest)
		if r.Attempt.ID != a.ID {
			return nil, nil
		}
	}
	var manifest readexec.Manifest
	if expected == nil || json.Unmarshal(expected, &manifest) != nil || readexec.Hash(manifest) != readexec.Hash(a.Manifest) || m.admitted != "" && m.admitted != a.ID {
		return nil, nil
	}
	proof := reporting.PreparationSettlement{Kind: "attempt", Attempt: a.ID, Manifest: readexec.Hash(a.Manifest), Status: a.Status, RemoteState: a.RemoteState, Finished: *a.Finished}
	return json.Marshal(proof)
}

// SettleAuthoringPreparation is consumed only by explicit existing controls. It
// can close guarded no-dispatch work or persist a positively terminal original
// attempt. It never reconstructs rows, schema, a prepared revision or validation.
func (d *DB) SettleAuthoringPreparation(ctx context.Context, e identity.Envelope, id string, cancel bool) (out reporting.AuthoringPreparationRecord, err error) {
	err = d.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = authoringPreparationRecordTx(ctx, tx, e, id, false, true, true)
		if err != nil {
			return err
		}
		m, err := preparationLifecycleTx(ctx, tx, e.Tenant(), id)
		if err != nil {
			return err
		}
		proof, err := preparationSettlementTx(ctx, tx, e, out, m, cancel || time.Now().After(out.Deadline))
		if err != nil {
			return err
		}
		out.Settled = proof != nil
		if proof == nil {
			return reporting.RequireAuthoringPreparation(e, out, true)
		}
		if m.settlement != nil {
			if err := assertPreparationSettledTx(ctx, tx, e.Tenant(), id); err != nil {
				return err
			}
			if out.Status != "accepted" && out.Status != "uncertain" {
				return reporting.RequireAuthoringPreparation(e, out, true)
			}
		}
		if out.Status == "accepted" || out.Status == "uncertain" {
			out.Status = "failed"
			out.Code = "result_not_retained"
			if cancel {
				out.Code = "cancelled"
			}
		}
		raw, _ := json.Marshal(out)
		_, err = tx.Exec(ctx, `UPDATE chartworks.authoring_preparations SET status=$3,record=$4,settlement=$5 WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), id, out.Status, raw, proof)
		if err != nil {
			return err
		}
		if err := assertPreparationSettledTx(ctx, tx, e.Tenant(), id); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "authoring.preparation_settled", id); err != nil {
			return err
		}
		return reporting.RequireAuthoringPreparation(e, out, true)
	})
	if err != nil {
		out = reporting.AuthoringPreparationRecord{}
	}
	return
}

func readConsumedPreparationTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, key string, operation, execute bool, grants []byte) (reporting.AuthoringPreparationRecord, error) {
	coordinate := "preparation_id"
	if operation {
		coordinate = "operation_id"
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT p.record FROM chartworks.authoring_preparation_consumed p WHERE p.tenant_id=$1 AND p.`+coordinate+`=$2 AND p.actor_id=$3 AND p.session_id=$4 AND `+authoringPreparationEligibility, e.Tenant(), key, e.User(), e.Session(), grants, execute).Scan(&raw)
	if err != nil {
		return reporting.AuthoringPreparationRecord{}, err
	}
	var receipt reporting.AuthoringPreparationConsumed
	if json.Unmarshal(raw, &receipt) != nil || receipt.Revision != 1 {
		return reporting.AuthoringPreparationRecord{}, store.ErrInvalid
	}
	out := receipt.PreparationRecord()
	if err := reporting.RequireAuthoringPreparation(e, out, execute); err != nil {
		return reporting.AuthoringPreparationRecord{}, err
	}
	return out, nil
}

// One actor-scoped, committed batch per explicit fresh admission. Quota or pin
// rejection in the later reservation transaction cannot roll this progress back.
func (d *DB) pruneAuthoringPreparations(ctx context.Context, e identity.Envelope) error {
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		rows, err := tx.Query(ctx, `SELECT p.preparation_id,p.status,CASE WHEN p.status='consumed' THEN chartworks.preparation_consumed_record(p) END FROM chartworks.authoring_preparations p WHERE p.tenant_id=$1 AND p.actor_id=$2 AND p.status IN('prepared','failed','consumed') AND p.settlement IS NOT NULL AND p.settled_at<=statement_timestamp()-interval '24 hours' AND chartworks.preparation_cleanup_eligible(p) AND (p.status<>'consumed' OR octet_length(chartworks.preparation_consumed_record(p)::text)<=65536) ORDER BY p.settled_at,p.preparation_id LIMIT 100 FOR UPDATE OF p SKIP LOCKED`, e.Tenant(), e.User())
		if err != nil {
			return err
		}
		type candidate struct {
			id, status string
			receipt    []byte
		}
		var selected []candidate
		for rows.Next() {
			var c candidate
			if err = rows.Scan(&c.id, &c.status, &c.receipt); err != nil {
				rows.Close()
				return err
			}
			selected = append(selected, c)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		for _, c := range selected {
			// Oversize legacy references stay in full custody, still charged. They do
			// not get silently discarded or replaced by an incomplete receipt.
			if len(c.receipt) > 64<<10 {
				continue
			}
			if c.status == "consumed" {
				_, err = tx.Exec(ctx, `INSERT INTO chartworks.authoring_preparation_consumed(tenant_id,preparation_id,actor_id,session_id,target_id,operation_id,input_digest,source_id,topic_id,revision,record) SELECT tenant_id,preparation_id,actor_id,session_id,target_id,operation_id,input_digest,source_id,topic_id,1,$3 FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), c.id, c.receipt)
				if err != nil {
					return err
				}
			}
			if _, err = tx.Exec(ctx, `DELETE FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND preparation_id=$2`, e.Tenant(), c.id); err != nil {
				return err
			}
			if err = auditJob(ctx, tx, scope, "authoring.preparation_pruned", c.id); err != nil {
				return err
			}
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return nil
	})
}
