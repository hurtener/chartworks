package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func compositionGroupByID(m reporting.CompositionManifest, id string) (reporting.CompositionGroup, bool) {
	for _, group := range m.Groups {
		if group.ID == id {
			return group, true
		}
	}
	return reporting.CompositionGroup{}, false
}

func compositionDefinitionsTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.CompositionManifest) error {
	// Locks protect archive eligibility until the manifest is sealed. Exact
	// revisions remain usable when a different revision is published meanwhile.
	keys := map[string][2]string{m.Kind + ":" + m.Document: {m.Kind, m.Document}}
	for _, page := range m.Pages {
		keys["report:"+page.Report] = [2]string{"report", page.Report}
	}
	order := make([]string, 0, len(keys))
	for key := range keys {
		order = append(order, key)
	}
	slices.Sort(order)
	for _, key := range order {
		value := keys[key]
		var archived bool
		if err := tx.QueryRow(ctx, `SELECT archived FROM chartworks.document_heads WHERE tenant_id=$1 AND kind=$2 AND document_id=$3 FOR SHARE`, e.Tenant(), value[0], value[1]).Scan(&archived); err != nil {
			return err
		}
		if archived {
			return reporting.ErrStale
		}
	}
	root, err := documentTx(ctx, tx, e, m.Kind, m.Document, reporting.DocumentReference{Revision: m.Revision}, reporting.Execute, m.Redacted)
	if err != nil {
		return err
	}
	if root.Revision.Digest != m.Digest || root.PublishedAt == nil && !m.Private {
		return reporting.ErrStale
	}
	for _, page := range m.Pages {
		snapshot, err := documentTx(ctx, tx, e, "report", page.Report, reporting.DocumentReference{Revision: page.Revision}, reporting.Execute, false)
		if err != nil {
			return err
		}
		if snapshot.Revision.Digest != page.Digest || snapshot.PublishedAt == nil && !m.Private {
			return reporting.ErrStale
		}
		if m.Kind == "dashboard" {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.document_page_refs WHERE tenant_id=$1 AND kind='dashboard' AND document_id=$2 AND revision=$3 AND page_id=$4 AND report_id=$5 AND report_revision=$6)`, e.Tenant(), m.Document, m.Revision, page.ID, page.Report, page.Revision).Scan(&exists); err != nil {
				return err
			}
			if !exists || snapshot.PublishedAt == nil {
				return store.ErrInvalid
			}
		}
	}
	for _, group := range m.Groups {
		if group.Kind != "block" {
			continue
		}
		var archived bool
		if err := tx.QueryRow(ctx, `SELECT archived FROM chartworks.block_heads WHERE tenant_id=$1 AND block_id=$2 FOR SHARE`, e.Tenant(), group.Block).Scan(&archived); err != nil {
			return err
		}
		if archived {
			return reporting.ErrStale
		}
		block, err := blockTx(ctx, tx, e, group.Block, reporting.Reference{Revision: group.Revision}, reporting.Execute)
		if err != nil {
			return err
		}
		if err := reporting.CheckCompositionBlock(e, group, block); err != nil {
			return err
		}
	}
	return nil
}

func insertCompositionIndexes(ctx context.Context, tx pgx.Tx, e identity.Envelope, record reporting.CompositionRecord) error {
	m := record.Manifest
	for ordinal, group := range m.Groups {
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.composition_run_groups(tenant_id,operation_id,group_id,ordinal,kind) VALUES($1,$2,$3,$4,$5)`, e.Tenant(), m.ID, group.ID, ordinal, group.Kind); err != nil {
			return err
		}
	}
	view := reporting.SummarizeComposition(record)
	for ordinal, page := range m.Pages {
		body, err := json.Marshal(view.Pages[ordinal])
		if err != nil {
			return store.ErrInvalid
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.composition_run_pages(tenant_id,operation_id,page_id,ordinal,report_id,revision,private,summary) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.Tenant(), m.ID, page.ID, ordinal, page.Report, page.Revision, page.Private, body); err != nil {
			return err
		}
		for _, widget := range page.Widgets {
			var static []byte
			if payload := reporting.CompositionStaticPayload(page.ID, widget); payload != nil {
				static, err = json.Marshal(payload)
				if err != nil {
					return store.ErrInvalid
				}
			}
			outputs := []string{}
			if widget.Definition.Block != nil {
				outputs = append(outputs, widget.Definition.Block.Outputs...)
			}
			selected, err := json.Marshal(outputs)
			if err != nil {
				return store.ErrInvalid
			}
			if _, err := tx.Exec(ctx, `INSERT INTO chartworks.composition_run_widgets(tenant_id,operation_id,page_id,widget_id,group_id,selected_outputs,static_payload) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), m.ID, page.ID, widget.Definition.ID, nullableString(widget.Group), selected, static); err != nil {
				return err
			}
		}
	}
	for _, ref := range reporting.CompositionReferences(m) {
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.composition_run_references(tenant_id,operation_id,page_id,action,kind,permission,resource_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, e.Tenant(), m.ID, ref.Page, ref.Action, ref.Kind, ref.Permission, ref.ID); err != nil {
			return err
		}
	}
	return nil
}

// SealComposition reserves retention and seals all floating inputs atomically
// under the existing request-key lock. Replays keep the first successful seal.
func (d *DB) SealComposition(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedComposition) (out reporting.CompositionRecord, err error) {
	m, err := proof.Checked(e)
	if err != nil {
		return out, err
	}
	if err := task.Require(e); err != nil {
		return out, err
	}
	if task.ID != m.ID || task.Input.Kind != m.Kind+".run" || task.Input.Target != m.Document || task.Input.InputHash != m.RequestHash || task.ManifestHash != m.TaskHash || !task.Created.Equal(m.Created) {
		return out, store.ErrInvalid
	}
	body, err := json.Marshal(m)
	if err != nil {
		return out, store.ErrInvalid
	}
	out = reporting.CompositionRecord{Manifest: m, State: "sealed", Results: []reporting.GroupResult{}}
	charged := reporting.CompositionRetainedBytes(out)
	// Preserve room for bounded failure receipts before making external calls.
	if charged+int64(len(m.Groups))*1024 > int64(m.Limits.MaxRetainedBytes) {
		return reporting.CompositionRecord{}, reporting.ErrBudget
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		actual, err := readRequestTx(ctx, tx, e, task.ID, true)
		if err != nil {
			return err
		}
		if actual.ManifestHash != m.TaskHash {
			return store.ErrConflict
		}
		previous, err := compositionHeadTx(ctx, tx, e, m.ID, true, false)
		if err == nil {
			if previous.requestHash != m.RequestHash {
				return store.ErrConflict
			}
			out, err = compositionReadTx(ctx, tx, e, previous)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if actual.State != "pending" || !time.Now().Before(actual.Expires) {
			return store.ErrExpired
		}
		if err := compositionDefinitionsTx(ctx, tx, e, m); err != nil {
			return err
		}
		if err := frozenQuotaLock(ctx, tx, e.Tenant()); err != nil {
			return err
		}
		var count int
		var used int64
		if err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(GREATEST(retained_bytes,reserved_bytes)),0) FROM (
 SELECT retained_bytes,reserved_bytes FROM chartworks.frozen_runs WHERE tenant_id=$1
 UNION ALL SELECT retained_bytes,reserved_bytes FROM chartworks.composition_runs WHERE tenant_id=$1) usage`, e.Tenant()).Scan(&count, &used); err != nil {
			return err
		}
		if count >= m.ArtifactLimits.MaxRequests || used > m.ArtifactLimits.MaxTenantBytes-int64(m.Limits.MaxRetainedBytes) {
			return reporting.ErrBudget
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.composition_runs(tenant_id,operation_id,actor_id,session_id,kind,document_id,revision,definition_digest,request_hash,task_hash,manifest_digest,private,partial_policy,redacted,total_pages,query_groups,created_at,expires_at,retained_bytes,reserved_bytes,max_bytes)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$20)`, e.Tenant(), m.ID, m.Actor, m.Session, m.Kind, m.Document, m.Revision, m.Digest, m.RequestHash, m.TaskHash, m.ManifestDigest(), m.Private, m.Policy, m.Redacted, len(m.Pages), len(m.Groups), m.Created, m.Expires, charged, m.Limits.MaxRetainedBytes)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.composition_run_payloads(tenant_id,operation_id,manifest) VALUES($1,$2,$3)`, e.Tenant(), m.ID, body); err != nil {
			return err
		}
		if err := insertCompositionIndexes(ctx, tx, e, out); err != nil {
			return err
		}
		if err := frozenAudit(ctx, tx, e, "composition.sealed", m.ID); err != nil {
			return err
		}
		if _, err := proof.Checked(e); err != nil {
			return err
		}
		h, err := compositionHeadTx(ctx, tx, e, m.ID, true, false)
		if err != nil {
			return err
		}
		out, err = compositionReadTx(ctx, tx, e, h)
		return err
	})
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	return out, nil
}
