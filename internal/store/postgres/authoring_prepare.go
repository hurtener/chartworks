package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.AuthoringPreparationRepository = (*DB)(nil)

// Full payload plus bounded receipt, admitted manifest and settlement growth.
const preparationReservationBytes = (2 << 20) + (64 << 10) + (64 << 10) + (4 << 10)

func (d *DB) AuthoringRulesActive(ctx context.Context, e identity.Envelope, pin reporting.TopicPin) (active bool, err error) {
	if err = access.Require(e, "topics.read", access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: pin.Topic}); err != nil {
		return
	}
	p, readErr := d.ReadPublishedTopic(ctx, e, pin.Topic, pin.Version, drafts.Read)
	if readErr != nil {
		return false, readErr
	}
	if !p.State.Active || p.State.Archived || p.Digest != pin.Digest {
		return false, reporting.ErrStale
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2 AND active_version IS NOT NULL)`, e.Tenant(), pin.Topic).Scan(&active)
	})
	return
}
func scanAuthoringPreparation(row pgx.Row) (r reporting.AuthoringPreparationRecord, err error) {
	var raw []byte
	if err = row.Scan(&raw); err != nil {
		return
	}
	if json.Unmarshal(raw, &r) != nil {
		err = store.ErrInvalid
	}
	return
}

// Eligibility is applied before SQL-bearing custody is selected. The same
// predicate protects retained reads, operation replay and transactional consume.
const authoringPreparationEligibility = `jsonb_array_length(p.record->'references') BETWEEN 1 AND 4096
 AND NOT EXISTS (SELECT 1 FROM (VALUES ('block','read',p.target_id),('block','write',p.target_id),('block','preview',p.target_id),('topic','write',p.topic_id)) required(kind,permission,id)
 WHERE NOT EXISTS (SELECT 1 FROM jsonb_array_elements($5::jsonb) g WHERE g->>'kind'=required.kind AND g->>'permission'=required.permission AND g->>'id'=required.id))
 AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(p.record->'references') rr
 WHERE NOT EXISTS (SELECT 1 FROM jsonb_array_elements($5::jsonb) g WHERE g->>'kind'=rr->>'kind' AND g->>'permission'=rr->>'permission' AND g->>'id'=rr->>'id'))
 AND (NOT $6::boolean OR EXISTS (SELECT 1 FROM jsonb_array_elements($5::jsonb) g WHERE g->>'kind'='source' AND g->>'permission'='query' AND g->>'id'=p.source_id))`

func authoringPreparationGrants(e identity.Envelope, execute bool) ([]byte, error) {
	if !e.Valid() {
		return nil, access.ErrUnauthenticated
	}
	for _, reach := range e.Reach() {
		if reach.ID == "*" {
			return nil, access.ErrForbidden
		}
	}
	if err := access.Require(e, "charts.bind", access.Tenant(e, "read")); err != nil {
		return nil, err
	}
	if err := access.Require(e, "reporting.write", access.Tenant(e, "write")); err != nil {
		return nil, err
	}
	for _, action := range []string{"reporting.read", "reporting.write", "reporting.preview"} {
		if !e.Has(action) {
			return nil, access.ErrForbidden
		}
	}
	if execute && (!e.Has("reporting.validate") || !e.Has("sources.query")) {
		return nil, access.ErrForbidden
	}
	return blockGrants(e)
}

func authoringPreparationRecordTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, key string, operation, execute, lock bool) (reporting.AuthoringPreparationRecord, error) {
	grants, err := authoringPreparationGrants(e, execute)
	if err != nil {
		return reporting.AuthoringPreparationRecord{}, err
	}
	coordinate := "preparation_id"
	if operation {
		coordinate = "operation_id"
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	r, err := scanAuthoringPreparation(tx.QueryRow(ctx, `SELECT p.record FROM chartworks.authoring_preparations p WHERE p.tenant_id=$1 AND p.`+coordinate+`=$2 AND p.actor_id=$3 AND p.session_id=$4 AND `+authoringPreparationEligibility+suffix, e.Tenant(), key, e.User(), e.Session(), grants, execute))
	if errors.Is(err, pgx.ErrNoRows) && !lock {
		return readConsumedPreparationTx(ctx, tx, e, key, operation, execute, grants)
	}
	if err == nil {
		err = reporting.RequireAuthoringPreparation(e, r, execute)
	}
	if err != nil {
		return reporting.AuthoringPreparationRecord{}, err
	}
	return r, nil
}

// Exclusive topic-head lock fences absence of rules as well as current pins:
// existing first/replacement rule publication takes this head FOR SHARE.
func authoringPreparationFence(ctx context.Context, tx pgx.Tx, e identity.Envelope, r reporting.AuthoringPreparationRecord) error {
	if len(r.Topics) != 1 || !r.Binding.Valid() || r.Binding.Tenant != e.Tenant() {
		return store.ErrInvalid
	}
	pin := r.Topics[0]
	var version, digest string
	var archived bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(h.active_version,''),h.archived,COALESCE(v.digest,'') FROM chartworks.topic_publication_heads h LEFT JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version) WHERE h.tenant_id=$1 AND h.topic_id=$2 FOR UPDATE OF h`, e.Tenant(), pin.Topic).Scan(&version, &archived, &digest); err != nil {
		return err
	}
	if archived || version != pin.Version || digest != pin.Digest {
		return reporting.ErrStale
	}
	var active bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.topic_rule_publication_heads WHERE tenant_id=$1 AND topic_id=$2 AND active_version IS NOT NULL)`, e.Tenant(), pin.Topic).Scan(&active); err != nil {
		return err
	}
	if active {
		return reporting.ErrStale
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT r.binding FROM chartworks.sources h JOIN chartworks.source_revisions r ON(r.tenant_id,r.source_id,r.revision)=(h.tenant_id,h.source_id,h.current_revision) WHERE h.tenant_id=$1 AND h.source_id=$2 AND NOT h.deleted FOR SHARE OF h`, e.Tenant(), r.Binding.Source).Scan(&raw); err != nil {
		return err
	}
	var current readexec.Binding
	if json.Unmarshal(raw, &current) != nil || readexec.Hash(current) != readexec.Hash(r.Binding) {
		return reporting.ErrStale
	}
	return nil
}
func authoringPreparationShape(r reporting.AuthoringPreparationRecord) error {
	if !identity.Identifier(r.ID) || !identity.Identifier(r.Operation) || !identity.Identifier(r.Target) || r.Compiler != reporting.AuthoringCompilerForIntent(r.Request.Intent) || r.InputDigest != readexec.Hash(r.Request) || r.Request.NewBlock != r.Target || r.Request.Operation != r.Operation || r.SourceOperation != "chart-prepare:"+readexec.Hash([]string{r.Binding.Tenant, r.Actor, r.Session, r.Target, r.Operation}) || len(r.Statement) < 1 || len(r.Statement) > 64<<10 || len(r.Scope) != 1 || len(r.Dependencies) != 1 || r.Scope[0].Dataset != r.Request.Intent.Dataset || r.Dependencies[0].Dataset != r.Request.Intent.Dataset || len(r.Topics) != 1 || r.Topics[0] != r.Request.Intent.Topic {
		return store.ErrInvalid
	}
	return nil
}
func (d *DB) ReserveAuthoringPreparation(ctx context.Context, e identity.Envelope, r reporting.AuthoringPreparationRecord) (out reporting.AuthoringPreparationRecord, fresh bool, err error) {
	defer func() {
		if err != nil {
			out = reporting.AuthoringPreparationRecord{}
			fresh = false
		}
	}()
	if err = reporting.RequireAuthoringPreparation(e, r, true); err != nil {
		return
	}
	if err = authoringPreparationShape(r); err != nil {
		return
	}
	if r.Status != "accepted" || r.Revision != nil || r.Attempt != nil || r.Digest != "" || r.Code != "" || !r.Deadline.After(r.CreatedAt) || r.Deadline.Sub(r.CreatedAt) > time.Minute || r.ExpiresAt.Sub(r.CreatedAt) > 15*time.Minute {
		return out, false, store.ErrInvalid
	}
	raw, marshalErr := json.Marshal(r)
	if marshalErr != nil || len(raw) > 2<<20 {
		return out, false, store.ErrInvalid
	}
	// Authenticated retained lookup precedes all new-contract checks and cleanup.
	if prior, priorErr := d.ReadAuthoringPreparationOperation(ctx, e, r.Operation); priorErr == nil {
		if err := reporting.RequireAuthoringPreparation(e, prior, true); err != nil {
			return out, false, err
		}
		if prior.InputDigest != r.InputDigest || prior.Target != r.Target {
			return out, false, store.ErrConflict
		}
		return prior, false, nil
	} else if !errors.Is(priorErr, store.ErrNotFound) {
		return out, false, priorErr
	}
	if err = reporting.AuthoringPreparationAdmission(r.Request, time.Now()); err != nil {
		return
	}
	if err = d.pruneAuthoringPreparations(ctx, e); err != nil {
		return
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "chart-prepare:"+e.Tenant()); err != nil {
			return err
		}
		var priorExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND operation_id=$4 UNION ALL SELECT 1 FROM chartworks.authoring_preparation_consumed WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND operation_id=$4)`, e.Tenant(), e.User(), e.Session(), r.Operation).Scan(&priorExists); err != nil {
			return err
		}
		if priorExists {
			prior, readErr := authoringPreparationRecordTx(ctx, tx, e, r.Operation, true, true, false)
			if readErr != nil {
				return readErr
			}
			if prior.InputDigest != r.InputDigest || prior.Target != r.Target {
				return store.ErrConflict
			}
			if err := reporting.RequireAuthoringPreparation(e, prior, true); err != nil {
				return err
			}
			out = prior
			return nil
		}
		if err := reporting.AuthoringPreparationAdmission(r.Request, time.Now()); err != nil {
			return err
		}
		if err := authoringPreparationFence(ctx, tx, e, r); err != nil {
			return err
		}
		var count, actorCount int
		var bytes, actorBytes int64
		var exists bool
		// Unsettled records reserve worst-case outcome bytes, including uncertain
		// or logical failed custody. Compact receipts remain byte-charged.
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE full_payload),count(*) FILTER(WHERE full_payload AND actor_id=$2),COALESCE(sum(charged),0),COALESCE(sum(charged) FILTER(WHERE actor_id=$2),0) FROM (SELECT actor_id,true AS full_payload,CASE WHEN chartworks.preparation_has_liability(p) OR status IN('accepted','uncertain') THEN GREATEST($3,octet_length(record::text)+COALESCE(octet_length(read_receipt::text),0)+COALESCE(octet_length(admitted_manifest::text),0)+COALESCE(octet_length(settlement::text),0)) ELSE octet_length(record::text)+COALESCE(octet_length(read_receipt::text),0)+COALESCE(octet_length(admitted_manifest::text),0)+COALESCE(octet_length(settlement::text),0) END AS charged FROM chartworks.authoring_preparations p WHERE tenant_id=$1 UNION ALL SELECT actor_id,false,octet_length(record::text) FROM chartworks.authoring_preparation_consumed WHERE tenant_id=$1) charges`, e.Tenant(), e.User(), preparationReservationBytes).Scan(&count, &actorCount, &bytes, &actorBytes); err != nil {
			return err
		}
		if count >= 10000 || actorCount >= 128 || bytes+preparationReservationBytes > 256<<20 || actorBytes+preparationReservationBytes > 16<<20 {
			return readexec.ErrLimit
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.block_heads WHERE tenant_id=$1 AND block_id=$2)`, e.Tenant(), r.Target).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return store.ErrConflict
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.authoring_preparations p WHERE p.tenant_id=$1 AND p.actor_id=$2 AND p.target_id=$3 AND chartworks.preparation_has_liability(p))`, e.Tenant(), e.User(), r.Target).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return readexec.ErrUncertain
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.authoring_preparations(tenant_id,preparation_id,actor_id,session_id,target_id,operation_id,input_digest,source_operation,status,record,created_at,deadline,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'accepted',$9,$10,$11,$12)`, e.Tenant(), r.ID, e.User(), e.Session(), r.Target, r.Operation, r.InputDigest, r.SourceOperation, raw, r.CreatedAt, r.Deadline, r.ExpiresAt); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "authoring.preparation_reserved", r.ID); err != nil {
			return err
		}
		out = r
		fresh = true
		return reporting.RequireAuthoringPreparation(e, r, true)
	})
	return
}
func (d *DB) ReadAuthoringPreparation(ctx context.Context, e identity.Envelope, id string) (out reporting.AuthoringPreparationRecord, err error) {
	defer func() {
		if err != nil {
			out = reporting.AuthoringPreparationRecord{}
		}
	}()
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	if !identity.Identifier(id) {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = authoringPreparationRecordTx(ctx, tx, e, id, false, false, false)
		if err != nil {
			return err
		}
		return reporting.RequireAuthoringPreparation(e, out, false)
	})
	return
}
func (d *DB) ReadAuthoringPreparationOperation(ctx context.Context, e identity.Envelope, operation string) (out reporting.AuthoringPreparationRecord, err error) {
	defer func() {
		if err != nil {
			out = reporting.AuthoringPreparationRecord{}
		}
	}()
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	if !identity.Identifier(operation) {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = authoringPreparationRecordTx(ctx, tx, e, operation, true, false, false)
		if err != nil {
			return err
		}
		return reporting.RequireAuthoringPreparation(e, out, false)
	})
	return
}
func preparationAttempt(ctx context.Context, tx pgx.Tx, e identity.Envelope, r reporting.AuthoringPreparationRecord) error {
	if r.Attempt == nil || r.Revision == nil || r.Attempt.Manifest.Operation != r.SourceOperation || r.Attempt.Manifest.Session != e.Session() || r.Revision.Definition.SQL != r.Statement || reporting.DefinitionDigest(r.Revision.Definition) != r.Revision.Digest || reporting.ExecutionDigest(r.Revision.Definition) != r.Revision.ExecutionDigest || r.Revision.Number != 1 || r.Revision.Actor != e.User() {
		return store.ErrInvalid
	}
	manifest, err := json.Marshal(r.Attempt.Manifest)
	if err != nil {
		return store.ErrInvalid
	}
	var id string
	return tx.QueryRow(ctx, `SELECT attempt_id FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3 AND operation_id=$4 AND source_id=$5 AND context_id=$6 AND manifest=$7::jsonb AND status=$8 AND status IN('succeeded','empty') AND remote_state='stopped' AND finished_at IS NOT NULL AND NOT cancel_requested FOR SHARE`, e.Tenant(), e.User(), r.Attempt.ID, r.SourceOperation, r.Binding.Source, r.Binding.Context, manifest, r.Attempt.Status).Scan(&id)
}
func (d *DB) FinishAuthoringPreparation(ctx context.Context, e identity.Envelope, r reporting.AuthoringPreparationRecord) error {
	if err := reporting.RequireAuthoringPreparation(e, r, true); err != nil {
		return err
	}
	if err := authoringPreparationShape(r); err != nil {
		return err
	}
	if r.Status != "prepared" && r.Status != "failed" && r.Status != "uncertain" {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		prior, err := authoringPreparationRecordTx(ctx, tx, e, r.ID, false, true, true)
		if err != nil {
			return err
		}
		if prior.Status != "accepted" || prior.InputDigest != r.InputDigest {
			return store.ErrConflict
		}
		meta, err := preparationLifecycleTx(ctx, tx, e.Tenant(), r.ID)
		if err != nil {
			return err
		}
		// Verify against accepted custody, not caller-supplied attempt coordinates.
		proof, err := preparationSettlementTx(ctx, tx, e, prior, meta, r.Status == "failed")
		if err != nil {
			return err
		}
		if r.Status == "prepared" {
			if err := authoringPreparationFence(ctx, tx, e, r); err != nil {
				return err
			}
			if err := preparationAttempt(ctx, tx, e, r); err != nil {
				return err
			}
		}
		raw, err := json.Marshal(r)
		if err != nil || len(raw) > 2<<20 {
			return store.ErrInvalid
		}
		tag, err := tx.Exec(ctx, `UPDATE chartworks.authoring_preparations SET status=$5,record=$6,settlement=$7 WHERE tenant_id=$1 AND preparation_id=$2 AND actor_id=$3 AND session_id=$4 AND status='accepted' AND expires_at>clock_timestamp()`, e.Tenant(), r.ID, e.User(), e.Session(), r.Status, raw, proof)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		if proof != nil {
			if err := assertPreparationSettledTx(ctx, tx, e.Tenant(), r.ID); err != nil {
				return err
			}
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "authoring.preparation_"+r.Status, r.ID); err != nil {
			return err
		}
		return reporting.RequireAuthoringPreparation(e, r, true)
	})
}

// Runs inside CommitBlock's existing creation transaction. Native validation,
// publication and certification are never transferred from preparation.
func consumeAuthoringPreparation(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.Mutation) error {
	if m.Preparation == nil {
		return nil
	}
	r, err := authoringPreparationRecordTx(ctx, tx, e, m.Preparation.ID, false, true, true)
	if err != nil {
		return err
	}
	if err := reporting.RequireAuthoringPreparation(e, r, true); err != nil {
		return err
	}
	if r.Status != "prepared" || !time.Now().Before(r.ExpiresAt) || r.Digest != m.Preparation.Digest || r.Target != m.ID || r.Revision == nil || m.Revision == nil || readexec.Hash(r.Revision) != readexec.Hash(m.Revision) || readexec.Hash(r.References) != readexec.Hash(m.References) || m.Topic != r.Topics[0].Topic {
		return store.ErrConflict
	}
	if err := authoringPreparationFence(ctx, tx, e, r); err != nil {
		return err
	}
	if err := preparationAttempt(ctx, tx, e, r); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE chartworks.authoring_preparations SET status='consumed',record=jsonb_set(record,'{status}','"consumed"') WHERE tenant_id=$1 AND preparation_id=$2 AND status='prepared' AND expires_at>clock_timestamp()`, e.Tenant(), r.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return reporting.ErrStale
	}
	return nil
}
