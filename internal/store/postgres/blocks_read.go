package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.Repository = (*DB)(nil)

// Reach remains immutable Pengui input. JSON is a bounded SQL bind value, not a
// dynamically constructed predicate or a persisted local permissions registry.
func blockGrants(e identity.Envelope) ([]byte, error) {
	if !e.Valid() {
		return nil, access.ErrUnauthenticated
	}
	grants := []reporting.ResourceReference{}
	for _, r := range e.Reach() {
		grants = append(grants, reporting.ResourceReference{Kind: r.Kind, Permission: r.Permission, ID: r.ID})
	}
	return json.Marshal(grants)
}

const blockHeadColumns = `h.block_id,h.topic_id,h.version,h.draft_revision,COALESCE(h.published_revision,0),h.draft_state,h.archived,h.created_at,h.updated_at`

func scanBlockHead(row pgx.Row) (out reporting.State, err error) {
	err = row.Scan(&out.ID, &out.Topic, &out.Version, &out.DraftRevision, &out.PublishedRevision, &out.DraftState, &out.Archived, &out.CreatedAt, &out.UpdatedAt)
	return
}

// Metadata filters are applied by PostgreSQL before definition/evidence payloads
// are projected. A missing grant never triggers a fetch-wide/filter-later path.
const blockReferenceEligibility = `EXISTS(SELECT 1 FROM chartworks.block_revision_references present WHERE (present.tenant_id,present.block_id,present.revision)=(r.tenant_id,r.block_id,r.revision)) AND NOT EXISTS (
 SELECT 1 FROM chartworks.block_revision_references rr
 WHERE (rr.tenant_id,rr.block_id,rr.revision)=(r.tenant_id,r.block_id,r.revision)
 AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements($6::jsonb) g
  WHERE g->>'kind'=rr.kind AND g->>'permission'=rr.permission
    AND g->>'id' IN(rr.resource_id,'*')))`

const blockCurrent = `NOT EXISTS (
 SELECT 1 FROM chartworks.block_source_pins bp
 LEFT JOIN chartworks.sources src ON (src.tenant_id,src.source_id)=(bp.tenant_id,bp.source_id)
 LEFT JOIN chartworks.source_revisions sr ON (sr.tenant_id,sr.source_id,sr.revision)=(src.tenant_id,src.source_id,src.current_revision)
 WHERE (bp.tenant_id,bp.block_id,bp.revision)=(r.tenant_id,r.block_id,r.revision)
 AND (src.source_id IS NULL OR src.deleted OR src.current_revision<>bp.source_revision OR sr.context_id<>bp.context_id))
 AND NOT EXISTS (
 SELECT 1 FROM chartworks.block_topic_pins bp
 LEFT JOIN chartworks.topic_publication_heads th ON (th.tenant_id,th.topic_id)=(bp.tenant_id,bp.topic_id)
 LEFT JOIN chartworks.topic_published_versions tv ON (tv.tenant_id,tv.topic_id,tv.version_id)=(th.tenant_id,th.topic_id,th.active_version)
 WHERE (bp.tenant_id,bp.block_id,bp.revision)=(r.tenant_id,r.block_id,r.revision)
 AND (th.topic_id IS NULL OR th.archived OR th.active_version IS DISTINCT FROM bp.version_id OR tv.digest IS DISTINCT FROM bp.digest))`

func blockReadArgs(e identity.Envelope, id string, ref reporting.Reference, a reporting.Access) ([]any, error) {
	if ref.Revision < 0 || ref.Revision > 256 || ref.Draft && ref.Revision != 0 {
		return nil, store.ErrInvalid
	}
	if err := reporting.Require(e, id, a); err != nil {
		return nil, err
	}
	if a == reporting.SQLRead {
		if err := reporting.Require(e, id, reporting.Read); err != nil {
			return nil, err
		}
	}
	grants, err := blockGrants(e)
	if err != nil {
		return nil, err
	}
	private := a == reporting.Write || a == reporting.Validate || a == reporting.Publish
	if !private {
		private = reporting.Require(e, id, reporting.Preview) == nil
	}
	parent := "read"
	if a == reporting.Write {
		parent = "write"
	}
	full := a == reporting.Write || a == reporting.Validate || a == reporting.Preview || a == reporting.SQLRead
	return []any{e.Tenant(), id, ref.Revision, ref.Draft, e.User(), grants, private, parent, full}, nil
}

func blockTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, ref reporting.Reference, a reporting.Access) (out reporting.Snapshot, err error) {
	args, err := blockReadArgs(e, id, ref, a)
	if err != nil {
		return out, err
	}
	var definition, provenance, references, validation, attestation, withdrawal, health []byte
	err = tx.QueryRow(ctx, `SELECT `+blockHeadColumns+`,r.revision,r.revision_id,
 CASE WHEN $9 THEN r.definition ELSE r.definition-'sql' END,
 r.digest,r.execution_digest,r.actor_id,r.created_at,
 CASE WHEN $9 THEN r.provenance ELSE '{}'::jsonb END,
 (SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',rr.kind,'permission',rr.permission,'id',rr.resource_id) ORDER BY rr.kind,rr.permission,rr.resource_id),'[]'::jsonb)
  FROM chartworks.block_revision_references rr WHERE (rr.tenant_id,rr.block_id,rr.revision)=(r.tenant_id,r.block_id,r.revision)),
 v.record,a.attestation,w.withdrawal,p.created_at,(`+blockCurrent+`),bh.observation
 FROM chartworks.block_heads h
 JOIN chartworks.block_revisions r ON (r.tenant_id,r.block_id)=(h.tenant_id,h.block_id)
  AND r.revision=CASE WHEN $3::bigint>0 THEN $3 WHEN $4 THEN h.draft_revision ELSE h.published_revision END
 LEFT JOIN chartworks.block_publications p ON (p.tenant_id,p.block_id,p.revision)=(r.tenant_id,r.block_id,r.revision)
 LEFT JOIN LATERAL(SELECT record FROM chartworks.block_validations v WHERE (v.tenant_id,v.block_id,v.revision)=(r.tenant_id,r.block_id,r.revision) ORDER BY created_at DESC,evidence_id DESC LIMIT 1) v ON true
 LEFT JOIN LATERAL(SELECT attestation_id,attestation FROM chartworks.block_attestations a WHERE (a.tenant_id,a.block_id,a.revision)=(r.tenant_id,r.block_id,r.revision) ORDER BY created_at DESC,attestation_id DESC LIMIT 1) a ON true
 LEFT JOIN chartworks.block_withdrawals w ON (w.tenant_id,w.block_id,w.revision,w.attestation_id)=(r.tenant_id,r.block_id,r.revision,a.attestation_id)
 LEFT JOIN chartworks.block_health bh ON (bh.tenant_id,bh.block_id,bh.revision)=(r.tenant_id,r.block_id,r.revision)
 WHERE h.tenant_id=$1 AND h.block_id=$2
 AND (p.revision IS NOT NULL OR (r.actor_id=$5 AND $7))
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) g WHERE g->>'kind'='topic' AND g->>'permission'=$8 AND g->>'id' IN(h.topic_id,'*'))
 AND `+blockReferenceEligibility, args...).Scan(
		&out.State.ID, &out.State.Topic, &out.State.Version, &out.State.DraftRevision, &out.State.PublishedRevision, &out.State.DraftState, &out.State.Archived, &out.State.CreatedAt, &out.State.UpdatedAt,
		&out.Revision.Number, &out.Revision.ID, &definition, &out.Revision.Digest, &out.Revision.ExecutionDigest, &out.Revision.Actor, &out.Revision.CreatedAt, &provenance, &references,
		&validation, &attestation, &withdrawal, &out.PublishedAt, &out.Current, &health)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(definition, &out.Revision.Definition) != nil || json.Unmarshal(provenance, &out.Revision.Provenance) != nil || json.Unmarshal(references, &out.References) != nil {
		return reporting.Snapshot{}, store.ErrInvalid
	}
	if args[8].(bool) && (reporting.DefinitionDigest(out.Revision.Definition) != out.Revision.Digest || reporting.ExecutionDigest(out.Revision.Definition) != out.Revision.ExecutionDigest) {
		return reporting.Snapshot{}, store.ErrInvalid
	}
	if len(validation) > 0 && json.Unmarshal(validation, &out.Validation) != nil || len(attestation) > 0 && json.Unmarshal(attestation, &out.Attestation) != nil || len(withdrawal) > 0 && json.Unmarshal(withdrawal, &out.Withdrawal) != nil || len(health) > 0 && json.Unmarshal(health, &out.Health) != nil {
		return reporting.Snapshot{}, store.ErrInvalid
	}
	if out.Validation != nil && (out.Validation.Evidence.Revision != out.Revision.Number || out.Validation.Evidence.RevisionID != out.Revision.ID || out.Validation.Evidence.DefinitionDigest != out.Revision.Digest) {
		return reporting.Snapshot{}, store.ErrInvalid
	}
	if !out.Current {
		out.Health = reporting.Health{Status: "stale", Reason: "dependency_revision_changed"}
	} else if out.Validation != nil && time.Now().Before(out.Validation.Evidence.ExpiresAt) && out.Health.Status == "" {
		observed := out.Validation.Evidence.CreatedAt
		out.Health = reporting.Health{Status: "healthy", Reason: "validated_observation", ObservedAt: &observed, DependencyDigest: out.Validation.Evidence.DependencyDigest}
	} else if out.Validation == nil && out.Health.Status == "" {
		out.Health = reporting.Health{Status: "unknown", Reason: "not_validated"}
	} else if out.Health.Status == "" {
		out.Health = reporting.Health{Status: "stale", Reason: "validation_expired"}
	}
	if !e.Valid() {
		return reporting.Snapshot{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

func (d *DB) ReadBlock(ctx context.Context, e identity.Envelope, id string, ref reporting.Reference, a reporting.Access) (out reporting.Snapshot, err error) {
	if _, err = blockReadArgs(e, id, ref, a); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = blockTx(ctx, tx, e, id, ref, a)
		return err
	})
	return
}

func (d *DB) ListBlocks(ctx context.Context, e identity.Envelope, in reporting.ListRequest) (out reporting.Page, err error) {
	out.Items = []reporting.Summary{}
	if in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) {
		return out, store.ErrInvalid
	}
	selection, err := access.Constrain(e, reporting.Read.Action(), "block", "read")
	if err != nil {
		return out, err
	}
	grants, err := blockGrants(e)
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	// $6 keeps the exact dependency eligibility predicate shared with ReadBlock.
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+blockHeadColumns+`,r.revision,r.definition->'metadata',(p.revision IS NULL)
 FROM chartworks.block_heads h JOIN chartworks.block_revisions r
 ON (r.tenant_id,r.block_id)=(h.tenant_id,h.block_id)
 AND r.revision=CASE WHEN $4 AND h.published_revision IS NULL AND NOT h.archived THEN h.draft_revision ELSE h.published_revision END
 LEFT JOIN chartworks.block_publications p ON(p.tenant_id,p.block_id,p.revision)=(r.tenant_id,r.block_id,r.revision)
 WHERE h.tenant_id=$1 AND h.block_id>$2 AND NOT h.archived AND($7 OR h.block_id=ANY($8::text[]))
 AND (p.revision IS NOT NULL OR (r.actor_id=$5 AND $9 AND EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) g WHERE g->>'kind'='block' AND g->>'permission'='preview' AND g->>'id' IN(h.block_id,'*'))))
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) g WHERE g->>'kind'='topic' AND g->>'permission'='read' AND g->>'id' IN(h.topic_id,'*'))
 AND `+blockReferenceEligibility+` ORDER BY h.block_id LIMIT $3`, e.Tenant(), in.After, in.Limit+1, in.IncludeDrafts, e.User(), grants, selection.All(), selection.IDs(), e.Has(reporting.Preview.Action()))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item reporting.Summary
			var metadata []byte
			if err := rows.Scan(&item.State.ID, &item.State.Topic, &item.State.Version, &item.State.DraftRevision, &item.State.PublishedRevision, &item.State.DraftState, &item.State.Archived, &item.State.CreatedAt, &item.State.UpdatedAt, &item.Revision, &metadata, &item.Private); err != nil {
				return err
			}
			if json.Unmarshal(metadata, &item.Metadata) != nil {
				return store.ErrInvalid
			}
			if len(out.Items) == in.Limit {
				out.Next = out.Items[len(out.Items)-1].State.ID
				break
			}
			out.Items = append(out.Items, item)
		}
		return rows.Err()
	})
	if err == nil && !e.Valid() {
		err = access.ErrUnauthenticated
	}
	if err != nil {
		return reporting.Page{}, err
	}
	return out, nil
}

// BlockHistory exposes no definition, SQL, validation values or private capture
// provenance. Private events require the original actor plus preview authority.
func (d *DB) BlockHistory(ctx context.Context, e identity.Envelope, id string) (out reporting.History, err error) {
	if err = reporting.Require(e, id, reporting.Read); err != nil {
		return out, err
	}
	grants, err := blockGrants(e)
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// A private-only block is visible only through explicit preview reach.
		var count int
		args := []any{e.Tenant(), id, int64(0), false, e.User(), grants, reporting.Require(e, id, reporting.Preview) == nil}
		rows, err := tx.Query(ctx, `SELECT ev.version,ev.kind,ev.revision,ev.actor_id,ev.note,ev.created_at
 FROM chartworks.block_events ev JOIN chartworks.block_revisions r ON(r.tenant_id,r.block_id,r.revision)=(ev.tenant_id,ev.block_id,ev.revision)
 JOIN chartworks.block_heads h ON(h.tenant_id,h.block_id)=(ev.tenant_id,ev.block_id)
 LEFT JOIN chartworks.block_publications p ON(p.tenant_id,p.block_id,p.revision)=(r.tenant_id,r.block_id,r.revision)
 WHERE h.tenant_id=$1 AND h.block_id=$2 AND $3::bigint=0 AND NOT $4::boolean
 AND(p.revision IS NOT NULL OR(r.actor_id=$5 AND $7))
 AND EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) g WHERE g->>'kind'='topic' AND g->>'permission'='read' AND g->>'id' IN(h.topic_id,'*'))
 AND `+blockReferenceEligibility+` ORDER BY ev.version LIMIT 4096`, args...)
		if err != nil {
			return err
		}
		out.Events = []reporting.Event{}
		for rows.Next() {
			var event reporting.Event
			if err := rows.Scan(&event.Version, &event.Kind, &event.Revision, &event.Actor, &event.Note, &event.CreatedAt); err != nil {
				rows.Close()
				return err
			}
			out.Events = append(out.Events, event)
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if count == 0 {
			return store.ErrNotFound
		}
		out.State, err = scanBlockHead(tx.QueryRow(ctx, `SELECT `+blockHeadColumns+` FROM chartworks.block_heads h WHERE h.tenant_id=$1 AND h.block_id=$2`, e.Tenant(), id))
		if err != nil {
			return err
		}
		if reporting.Require(e, id, reporting.Preview) != nil {
			out.State.DraftRevision = 0
			out.State.DraftState = ""
		}
		return nil
	})
	return
}

// blockConsistency is used in tests and diagnostics without exporting raw data.
func blockConsistency(s reporting.Snapshot) error {
	if s.State.ID == "" || s.Revision.Number < 1 || s.Revision.ID == "" || s.State.DraftRevision < s.Revision.Number {
		return fmt.Errorf("%w: block coordinate", store.ErrInvalid)
	}
	return nil
}
