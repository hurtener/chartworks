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

var _ reporting.AuthoringOptionRepository = (*DB)(nil)

const authoringOptionEligibility = `NOT EXISTS(SELECT 1 FROM jsonb_array_elements(p.record->'references') r
 WHERE NOT EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) g WHERE g->>'kind'=r->>'kind' AND g->>'permission'=r->>'permission' AND g->>'id'=r->>'id'))
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) g WHERE g->>'kind'='source' AND g->>'permission'='query' AND g->>'id'=p.source_id)
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(p.record->'blocks') b CROSS JOIN (VALUES ('read'),('preview'),('execute')) required(permission)
 WHERE (required.permission<>'preview' OR b->>'policy'='private_preview') AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) g WHERE g->>'kind'='block' AND g->>'permission'=required.permission AND g->>'id'=b->>'block'))`

func authoringOptionReadTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, in reporting.AuthoringOptionReference, lock bool) (reporting.AuthoringOptionRecord, error) {
	if !reporting.AuthoringOptionOperationValid(in.Operation, time.Now(), false) {
		return reporting.AuthoringOptionRecord{}, store.ErrInvalid
	}
	if err := reporting.RequireAuthoringOptionTarget(e, in.Target); err != nil {
		return reporting.AuthoringOptionRecord{}, err
	}
	grants, err := blockGrants(e)
	if err != nil {
		return reporting.AuthoringOptionRecord{}, err
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF p"
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT p.record FROM chartworks.authoring_option_operations p WHERE p.tenant_id=$1 AND p.actor_id=$2 AND p.session_id=$3 AND p.operation_id=$4 AND p.target_digest=$5 AND `+authoringOptionEligibility+suffix, e.Tenant(), e.User(), e.Session(), in.Operation, readexec.Hash(in.Target), grants).Scan(&raw)
	var out reporting.AuthoringOptionRecord
	if err == nil && json.Unmarshal(raw, &out) != nil {
		err = store.ErrInvalid
	}
	if err == nil {
		err = reporting.RequireAuthoringOption(e, out)
	}
	if err != nil {
		return reporting.AuthoringOptionRecord{}, err
	}
	return out, nil
}

func (d *DB) ReadAuthoringOption(ctx context.Context, e identity.Envelope, in reporting.AuthoringOptionReference) (out reporting.AuthoringOptionRecord, err error) {
	if err = reporting.RequireAuthoringOptionTarget(e, in.Target); err != nil {
		return
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		out, err = authoringOptionReadTx(ctx, tx, e, in, false)
		return err
	})
	if err != nil {
		out = reporting.AuthoringOptionRecord{}
	}
	return
}

func optionShape(r reporting.AuthoringOptionRecord) error {
	if !reporting.AuthoringOptionOperationValid(r.Operation, time.Now(), false) || !identity.Identifier(r.Source) || !identity.Identifier(r.Context) || !identity.Identifier(r.Dataset) || !identity.Identifier(r.Dimension) || r.SourceRevision < 1 || r.Rows < 2 || r.Rows > 200 || len(r.InputDigest) != 64 || len(r.BindingDigest) != 64 || len(r.ResolutionDigest) != 64 || r.SourceOperation != "authoring-option:"+readexec.Hash([]string{r.Tenant, r.Actor, r.Session, r.Operation}) || len(r.Blocks) > 100 || !r.Deadline.After(r.CreatedAt) || r.Deadline.Sub(r.CreatedAt) > time.Minute {
		return store.ErrInvalid
	}
	if (r.Target.Dataset != nil && len(r.Blocks) != 0) || (r.Target.Report != nil && len(r.Blocks) == 0) {
		return store.ErrInvalid
	}
	return nil
}

// Lock order follows report -> sorted blocks -> topic -> source. The exclusive
// topic head lock also fences first rule publication, whose lock is shared.
func authoringOptionFence(ctx context.Context, tx pgx.Tx, e identity.Envelope, r reporting.AuthoringOptionRecord) error {
	if err := reporting.RequireAuthoringOption(e, r); err != nil {
		return err
	}
	if x := r.Target.Report; x != nil {
		var draft, published int64
		var archived, deleted bool
		if err := tx.QueryRow(ctx, `SELECT COALESCE(draft_revision,0),COALESCE(published_revision,0),archived,deleted FROM chartworks.document_heads WHERE tenant_id=$1 AND kind='report' AND document_id=$2 FOR SHARE`, e.Tenant(), x.Report).Scan(&draft, &published, &archived, &deleted); err != nil {
			return err
		}
		if archived || deleted || x.Policy == "private_preview" && draft != x.Revision || x.Policy == "published" && published != x.Revision {
			return reporting.ErrStale
		}
		snapshot, err := documentTx(ctx, tx, e, "report", x.Report, reporting.DocumentReference{Revision: x.Revision}, reporting.Read, false)
		if err != nil {
			return err
		}
		if snapshot.Revision.Digest != x.Digest || x.Policy == "private_preview" && snapshot.Revision.Actor != e.User() {
			return reporting.ErrStale
		}
	}
	for _, b := range r.Blocks {
		var draft int64
		var archived bool
		if err := tx.QueryRow(ctx, `SELECT COALESCE(draft_revision,0),archived FROM chartworks.block_heads WHERE tenant_id=$1 AND block_id=$2 FOR SHARE`, e.Tenant(), b.Block).Scan(&draft, &archived); err != nil {
			return err
		}
		if archived {
			return reporting.ErrStale
		}
		snapshot, err := blockTx(ctx, tx, e, b.Block, reporting.Reference{Revision: b.Revision}, reporting.Read)
		if err != nil {
			return err
		}
		if snapshot.Revision.Digest != b.Digest || b.Policy == "private_preview" && snapshot.Revision.Actor != e.User() || b.Policy == "published" && snapshot.PublishedAt == nil {
			return reporting.ErrStale
		}
		pins, err := reporting.AuthoringRuleAbsence(snapshot.Revision)
		if err != nil || len(pins) != 1 || pins[0] != r.Topic {
			return reporting.ErrStale
		}
	}
	var version, digest string
	var archived bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(h.active_version,''),h.archived,COALESCE(v.digest,'') FROM chartworks.topic_publication_heads h LEFT JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version) WHERE h.tenant_id=$1 AND h.topic_id=$2 FOR UPDATE OF h`, e.Tenant(), r.Topic.Topic).Scan(&version, &archived, &digest); err != nil {
		return err
	}
	if archived || version != r.Topic.Version || digest != r.Topic.Digest {
		return reporting.ErrStale
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2 AND active_version IS NOT NULL)`, e.Tenant(), r.Topic.Topic).Scan(&active); err != nil {
		return err
	}
	if active {
		return reporting.ErrStale
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT r.binding FROM chartworks.sources h JOIN chartworks.source_revisions r ON(r.tenant_id,r.source_id,r.revision)=(h.tenant_id,h.source_id,h.current_revision) WHERE h.tenant_id=$1 AND h.source_id=$2 AND NOT h.deleted FOR SHARE OF h`, e.Tenant(), r.Source).Scan(&raw); err != nil {
		return err
	}
	var binding readexec.Binding
	if json.Unmarshal(raw, &binding) != nil || binding.Context != r.Context || binding.Revision != r.SourceRevision || readexec.Hash(binding) != r.BindingDigest {
		return reporting.ErrStale
	}
	if r.Target.Dataset != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.block_heads WHERE tenant_id=$1 AND block_id=$2)`, e.Tenant(), r.Target.Dataset.NewBlock).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return store.ErrConflict
		}
	}
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	return nil
}

func (d *DB) ReserveAuthoringOption(ctx context.Context, e identity.Envelope, r reporting.AuthoringOptionRecord) (out reporting.AuthoringOptionRecord, fresh bool, err error) {
	defer func() {
		if err != nil {
			out = reporting.AuthoringOptionRecord{}
			fresh = false
		}
	}()
	if err = reporting.RequireAuthoringOption(e, r); err != nil {
		return
	}
	if err = optionShape(r); err != nil {
		return
	}
	if r.Status != "accepted" || r.Receipt != nil || r.CancelRequested || r.Code != "" || r.ExecutionStatus != "" || r.RemoteState != "not_issued" {
		return out, false, store.ErrInvalid
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > 256<<10 {
		return out, false, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "authoring-option:"+e.Tenant()); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.authoring_option_operations WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3)`, e.Tenant(), e.User(), r.Operation).Scan(&exists); err != nil {
			return err
		}
		if exists {
			prior, err := authoringOptionReadTx(ctx, tx, e, reporting.AuthoringOptionReference{Target: r.Target, Operation: r.Operation}, false)
			if err != nil {
				return err
			}
			if prior.InputDigest != r.InputDigest {
				return store.ErrConflict
			}
			out = prior
			return nil
		}
		if !reporting.AuthoringOptionOperationValid(r.Operation, time.Now(), true) {
			return store.ErrExpired
		}
		// Bounded terminal pruning leaves every active/unknown admission intact.
		// An expired key cannot become fresh even after both journals are pruned.
		if _, err := tx.Exec(ctx, `DELETE FROM chartworks.authoring_option_operations p WHERE (p.tenant_id,p.actor_id,p.session_id,p.operation_id) IN (SELECT tenant_id,actor_id,session_id,operation_id FROM chartworks.authoring_option_operations WHERE tenant_id=$1 AND status IN('completed','failed') AND record->>'remote_state' IN('stopped','not_issued') AND created_at<clock_timestamp()-interval '24 hours' ORDER BY created_at LIMIT 100)`, e.Tenant()); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.authoring_option_operations p WHERE p.tenant_id=$1 AND p.actor_id=$2 AND p.namespace_digest=$3 AND p.status IN('accepted','uncertain') AND NOT EXISTS(SELECT 1 FROM chartworks.read_attempts a WHERE a.tenant_id=p.tenant_id AND a.actor_id=p.actor_id AND a.operation_id=p.source_operation AND a.finished_at IS NOT NULL AND a.status<>'uncertain' AND a.remote_state IN('stopped','not_issued')))`, e.Tenant(), e.User(), reporting.AuthoringOptionNamespace(r.Target)).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return readexec.ErrUncertain
		}
		var count, actorCount int
		var bytes, actorBytes int64
		if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE actor_id=$2),COALESCE(sum(CASE WHEN status IN('accepted','uncertain') THEN 262144 ELSE octet_length(record::text) END),0),COALESCE(sum(CASE WHEN status IN('accepted','uncertain') THEN 262144 ELSE octet_length(record::text) END) FILTER(WHERE actor_id=$2),0) FROM chartworks.authoring_option_operations WHERE tenant_id=$1`, e.Tenant(), e.User()).Scan(&count, &actorCount, &bytes, &actorBytes); err != nil {
			return err
		}
		if count >= 10000 || actorCount >= 512 || bytes+(256<<10) > 128<<20 || actorBytes+(256<<10) > 16<<20 {
			return readexec.ErrLimit
		}
		if err := authoringOptionFence(ctx, tx, e, r); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.authoring_option_operations(tenant_id,actor_id,session_id,operation_id,target_digest,namespace_digest,input_digest,source_operation,record,status,created_at,deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'accepted',$10,$11)`, e.Tenant(), e.User(), e.Session(), r.Operation, readexec.Hash(r.Target), reporting.AuthoringOptionNamespace(r.Target), r.InputDigest, r.SourceOperation, raw, r.CreatedAt, r.Deadline); err != nil {
			return err
		}
		for _, b := range r.Blocks {
			if _, err := tx.Exec(ctx, `INSERT INTO chartworks.authoring_option_block_refs(tenant_id,actor_id,session_id,operation_id,block_id,block_revision,policy) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), e.User(), e.Session(), r.Operation, b.Block, b.Revision, b.Policy); err != nil {
				return err
			}
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "authoring.option_reserved", r.Operation); err != nil {
			return err
		}
		out = r
		fresh = true
		return nil
	})
	return
}

func (d *DB) CheckAuthoringOption(ctx context.Context, e identity.Envelope, r reporting.AuthoringOptionRecord) error {
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		prior, err := authoringOptionReadTx(ctx, tx, e, reporting.AuthoringOptionReference{Target: r.Target, Operation: r.Operation}, false)
		if err != nil {
			return err
		}
		if prior.InputDigest != r.InputDigest || prior.Status != "accepted" || prior.CancelRequested || !time.Now().Before(prior.Deadline) || !reporting.AuthoringOptionOperationValid(r.Operation, time.Now(), true) {
			return store.ErrConflict
		}
		return authoringOptionFence(ctx, tx, e, prior)
	})
}

func (d *DB) SealAuthoringOption(ctx context.Context, e identity.Envelope, r reporting.AuthoringOptionRecord, receipt readexec.Receipt) error {
	if !receipt.Validated || receipt.Source != r.Source || receipt.Context != r.Context || receipt.Dialect != "postgres" || len(receipt.Manifest) != 64 || len(receipt.Dependencies) != 1 || receipt.Dependencies[0] != r.Dataset {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		prior, err := authoringOptionReadTx(ctx, tx, e, reporting.AuthoringOptionReference{Target: r.Target, Operation: r.Operation}, true)
		if err != nil {
			return err
		}
		if prior.InputDigest != r.InputDigest || prior.Status != "accepted" || prior.CancelRequested || prior.Receipt != nil || !time.Now().Before(prior.Deadline) || !reporting.AuthoringOptionOperationValid(r.Operation, time.Now(), true) {
			return store.ErrConflict
		}
		if err := authoringOptionFence(ctx, tx, e, prior); err != nil {
			return err
		}
		prior.Receipt = &receipt
		return updateOptionRecord(ctx, tx, e, prior, "authoring.option_reserved")
	})
}

func updateOptionRecord(ctx context.Context, tx pgx.Tx, e identity.Envelope, r reporting.AuthoringOptionRecord, event string) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return store.ErrInvalid
	}
	if _, err := tx.Exec(ctx, `UPDATE chartworks.authoring_option_operations SET status=$5,record=$6 WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND operation_id=$4`, e.Tenant(), e.User(), e.Session(), r.Operation, r.Status, raw); err != nil {
		return err
	}
	scope, _ := store.NewScope(e.Tenant(), e.User())
	return auditJob(ctx, tx, scope, event, r.Operation)
}

func (d *DB) FinishAuthoringOption(ctx context.Context, e identity.Envelope, r reporting.AuthoringOptionRecord) error {
	if !slices.Contains([]string{"completed", "failed", "uncertain"}, r.Status) {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		prior, err := authoringOptionReadTx(ctx, tx, e, reporting.AuthoringOptionReference{Target: r.Target, Operation: r.Operation}, true)
		if err != nil {
			return err
		}
		if prior.InputDigest != r.InputDigest {
			return store.ErrConflict
		}
		if prior.Status == "completed" || prior.Status == "failed" {
			return store.ErrConflict
		}
		// Outcomes are copied from the original journal, never caller-supplied
		// attempt coordinates. A cancel flag suppresses any successful response.
		a, attemptErr := scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 AND attempt_number=1`, e.Tenant(), e.User(), prior.SourceOperation))
		if attemptErr == nil {
			if a.Manifest.Session != e.Session() {
				return store.ErrConflict
			}
			if r.ExecutionStatus != a.Status || r.RemoteState != a.RemoteState {
				return store.ErrConflict
			}
			if r.Status != "uncertain" && (a.Finished == nil || a.Status == "uncertain" || !slices.Contains([]string{"stopped", "not_issued"}, a.RemoteState)) {
				return store.ErrConflict
			}
		} else if !errors.Is(attemptErr, pgx.ErrNoRows) {
			return attemptErr
		} else if r.Status == "completed" || r.ExecutionStatus != "" || r.RemoteState != "not_issued" {
			return store.ErrConflict
		}
		if r.Status == "completed" {
			if prior.CancelRequested || !slices.Contains([]string{"succeeded", "empty"}, a.Status) {
				return store.ErrConflict
			}
			if err := authoringOptionFence(ctx, tx, e, prior); err != nil {
				return err
			}
		}
		prior.Status, prior.Code, prior.ExecutionStatus, prior.RemoteState = r.Status, r.Code, r.ExecutionStatus, r.RemoteState
		return updateOptionRecord(ctx, tx, e, prior, "authoring.option_finished")
	})
}

func (d *DB) StopAuthoringOption(ctx context.Context, e identity.Envelope, in reporting.AuthoringOptionReference, cancel bool) (out reporting.AuthoringOptionRecord, err error) {
	err = d.requestControlTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		out, err = authoringOptionReadTx(ctx, tx, e, in, true)
		if err != nil {
			return err
		}
		if out.Status == "completed" || out.Status == "failed" {
			return nil
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3)`, e.Tenant(), e.User(), out.SourceOperation).Scan(&exists); err != nil {
			return err
		}
		if cancel {
			out.CancelRequested = true
		}
		// The same reservation row is locked by the native BeginRead trigger.
		// Cancellation therefore wins before admission or controls an existing
		// journal attempt. Expired reconciliation cannot resume source work.
		if !exists && (cancel || time.Now().After(out.Deadline.Add(3*time.Second))) {
			out.Status = "failed"
			out.Code = "result_not_retained"
			out.RemoteState = "not_issued"
			if cancel {
				out.Code = "cancelled"
			}
		}
		return updateOptionRecord(ctx, tx, e, out, "authoring.option_control")
	})
	if err != nil {
		out = reporting.AuthoringOptionRecord{}
	}
	return
}
