package postgres

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.CompositionRepository = (*DB)(nil)
var _ reporting.CompositionLifecycleRepository = (*DB)(nil)

type compositionHead struct {
	view        reporting.CompositionView
	actor       string
	session     string
	definition  string
	requestHash string
	taskHash    string
	policy      string
	maximum     int64
	totalPages  int
}

const compositionHeadColumns = `c.operation_id,c.kind,c.document_id,c.revision,c.manifest_digest,c.state,c.code,c.private,c.complete,c.mixed_freshness,c.redacted,c.created_at,c.expires_at,c.finished_at,c.query_groups,c.retained_bytes,c.actor_id,c.session_id,c.definition_digest,c.request_hash,c.task_hash,c.partial_policy,c.max_bytes,c.total_pages`

func compositionHeadTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, execution, lock bool) (h compositionHead, err error) {
	if !e.Valid() || !identity.Identifier(id) {
		return h, access.ErrUnauthenticated
	}
	query := `SELECT ` + compositionHeadColumns + ` FROM chartworks.composition_runs c WHERE c.tenant_id=$1 AND c.operation_id=$2`
	args := []any{e.Tenant(), id}
	if execution {
		query += ` AND c.actor_id=$3 AND c.session_id=$4`
		args = append(args, e.User(), e.Session())
	}
	if lock {
		query += ` FOR UPDATE OF c`
	}
	v := &h.view
	v.Pages = []reporting.CompositionPageSummary{}
	err = tx.QueryRow(ctx, query, args...).Scan(&v.ID, &v.Kind, &v.Document, &v.Revision, &v.Manifest, &v.State, &v.Code, &v.Private, &v.Complete, &v.MixedFreshness, &v.Redacted, &v.Created, &v.Expires, &v.Finished, &v.QueryGroups, &v.RetainedBytes, &h.actor, &h.session, &h.definition, &h.requestHash, &h.taskHash, &h.policy, &h.maximum, &h.totalPages)
	if err != nil {
		return compositionHead{}, err
	}
	if execution {
		err = reporting.RequireDocument(e, v.Kind, v.Document, reporting.Execute)
		if err == nil && v.Private {
			err = reporting.RequireDocument(e, v.Kind, v.Document, reporting.Preview)
		}
	} else {
		err = reporting.RequireCompositionArtifact(e, v.Kind, v.Document, id, h.actor, h.session, v.Private)
	}
	if err != nil {
		return compositionHead{}, err
	}
	return h, nil
}

// Required execution reach is checked in SQL before the manifest (which can
// include source-bound definitions and typed parameters) is fetched.
func compositionExecutionReachTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string) error {
	grants, err := blockGrants(e)
	if err != nil {
		return err
	}
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.composition_run_references WHERE tenant_id=$1 AND operation_id=$2)
 AND NOT EXISTS(SELECT 1 FROM chartworks.composition_run_references dep
 WHERE dep.tenant_id=$1 AND dep.operation_id=$2 AND dep.action<>'reporting.read'
 AND (NOT dep.action=ANY($3::text[]) OR NOT EXISTS(SELECT 1 FROM jsonb_array_elements($4::jsonb) g
 WHERE g->>'kind'=dep.kind AND g->>'permission'=dep.permission AND g->>'id' IN(dep.resource_id,'*'))))`, e.Tenant(), id, e.Scopes(), grants).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return access.ErrNotFound
	}
	return nil
}

func compositionReadTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, h compositionHead) (out reporting.CompositionRecord, err error) {
	if !time.Now().Before(h.view.Expires) || h.view.State == "expired" {
		return out, reporting.ErrExpired
	}
	if err := compositionExecutionReachTx(ctx, tx, e, h.view.ID); err != nil {
		return out, err
	}
	var body []byte
	if err := tx.QueryRow(ctx, `SELECT manifest FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`, e.Tenant(), h.view.ID).Scan(&body); err != nil {
		return out, err
	}
	out.Manifest, err = reporting.DecodeCompositionManifest(body, h.view.Manifest)
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	m := out.Manifest
	if m.ID != h.view.ID || m.Tenant != e.Tenant() || m.Actor != h.actor || m.Session != h.session || m.Kind != h.view.Kind || m.Document != h.view.Document || m.Revision != h.view.Revision || m.Digest != h.definition || m.RequestHash != h.requestHash || m.TaskHash != h.taskHash || m.Private != h.view.Private || m.Policy != h.policy || !m.Expires.Equal(h.view.Expires) || !m.Created.Equal(h.view.Created) {
		return reporting.CompositionRecord{}, store.ErrInvalid
	}
	if err := reporting.RequireComposition(e, m); err != nil {
		return reporting.CompositionRecord{}, err
	}
	out.State, out.Code = h.view.State, h.view.Code
	out.Results = []reporting.GroupResult{}
	out.Plans = map[string]nlqexec.SavedPlan{}
	out.Started = map[string]bool{}
	rows, err := tx.Query(ctx, `SELECT group_id,started,plan,result,result_digest FROM chartworks.composition_run_groups WHERE tenant_id=$1 AND operation_id=$2 ORDER BY ordinal`, e.Tenant(), h.view.ID)
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id string
		var started bool
		var plan, result []byte
		var hash *string
		if err := rows.Scan(&id, &started, &plan, &result, &hash); err != nil {
			return reporting.CompositionRecord{}, err
		}
		if count >= len(m.Groups) || m.Groups[count].ID != id {
			return reporting.CompositionRecord{}, store.ErrInvalid
		}
		count++
		out.Started[id] = started
		if len(plan) != 0 {
			var p nlqexec.SavedPlan
			if json.Unmarshal(plan, &p) != nil || !started || !identity.Identifier(p.Query) || p.Operation != "composition:"+m.ID+":"+id {
				return reporting.CompositionRecord{}, store.ErrInvalid
			}
			out.Plans[id] = p
		}
		if len(result) != 0 {
			r, err := reporting.DecodeCompositionResult(result, m, id)
			if err != nil || hash == nil || *hash != r.Digest {
				return reporting.CompositionRecord{}, store.ErrInvalid
			}
			out.Results = append(out.Results, r)
		}
	}
	if err := rows.Err(); err != nil {
		return reporting.CompositionRecord{}, err
	}
	if count != len(m.Groups) || reporting.CompositionRetainedBytes(out) != h.view.RetainedBytes {
		return reporting.CompositionRecord{}, store.ErrInvalid
	}
	if !e.Valid() {
		return reporting.CompositionRecord{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

// ReadComposition is actor/session-private execution recovery, not artifact read.
func (d *DB) ReadComposition(ctx context.Context, e identity.Envelope, id string) (out reporting.CompositionRecord, err error) {
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		h, err := compositionHeadTx(ctx, tx, e, id, true, false)
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

// $3 contains only the current verified resource set. Dashboard page names and
// actual context membership are filtered before summaries or payloads leave SQL.
const compositionPageEligibility = `(c.kind='report' OR EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g
 WHERE g->>'kind'='report' AND g->>'permission'='read' AND g->>'id' IN(p.report_id,'*')))
 AND (NOT p.private OR ($4::boolean AND EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g
 WHERE g->>'kind'='report' AND g->>'permission'='preview' AND g->>'id' IN(p.report_id,'*'))))
 AND NOT EXISTS(SELECT 1 FROM chartworks.composition_run_references dep
 WHERE dep.tenant_id=p.tenant_id AND dep.operation_id=p.operation_id AND dep.page_id=p.page_id AND dep.action='reporting.read'
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($3::jsonb) g WHERE g->>'kind'=dep.kind AND g->>'permission'=dep.permission AND g->>'id' IN(dep.resource_id,'*')))`

func compositionViewTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, h compositionHead) (reporting.CompositionView, error) {
	v := h.view
	if !time.Now().Before(v.Expires) || v.State == "expired" {
		v.State, v.Code, v.Complete, v.RetainedBytes = "expired", "retention_expired", false, 0
		v.QueryGroups, v.MixedFreshness = 0, false
		return v, nil
	}
	grants, err := blockGrants(e)
	if err != nil {
		return reporting.CompositionView{}, err
	}
	rows, err := tx.Query(ctx, `SELECT p.summary FROM chartworks.composition_run_pages p JOIN chartworks.composition_runs c USING(tenant_id,operation_id)
 WHERE p.tenant_id=$1 AND p.operation_id=$2 AND `+compositionPageEligibility+` ORDER BY p.ordinal`, e.Tenant(), v.ID, grants, e.Has("reporting.preview"))
	if err != nil {
		return reporting.CompositionView{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var body []byte
		var page reporting.CompositionPageSummary
		if err := rows.Scan(&body); err != nil {
			return reporting.CompositionView{}, err
		}
		if json.Unmarshal(body, &page) != nil || !identity.Identifier(page.ID) || !identity.Identifier(page.Report) || len(page.Widgets) > 100 {
			return reporting.CompositionView{}, store.ErrInvalid
		}
		v.Pages = append(v.Pages, page)
	}
	if err := rows.Err(); err != nil {
		return reporting.CompositionView{}, err
	}
	if len(v.Pages) != h.totalPages {
		v.Redacted, v.Complete = true, false
		// Counts and timing derived from invisible pages are not projected.
		v.QueryGroups, v.RetainedBytes, v.MixedFreshness = 0, 0, false
	}
	if !e.Valid() {
		return reporting.CompositionView{}, access.ErrUnauthenticated
	}
	return v, ctx.Err()
}

// ViewComposition reads only protected metadata indexes, never result payloads.
func (d *DB) ViewComposition(ctx context.Context, e identity.Envelope, id string) (out reporting.CompositionView, err error) {
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	if !e.Has("reporting.read") {
		return out, access.ErrForbidden
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		h, err := compositionHeadTx(ctx, tx, e, id, false, false)
		if err != nil {
			return err
		}
		out, err = compositionViewTx(ctx, tx, e, h)
		return err
	})
	if err != nil {
		return reporting.CompositionView{}, err
	}
	return out, nil
}

// CompositionWidget requires current retained-read and actual context reach,
// without reporting.execute, query.execute, sources.query or a model provider.
func (d *DB) CompositionWidget(ctx context.Context, e identity.Envelope, id, page, widget string) (out reporting.CompositionPayload, err error) {
	if !identity.Identifier(page) || !identity.Identifier(widget) {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	if !e.Has("reporting.read") {
		return out, access.ErrForbidden
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		h, err := compositionHeadTx(ctx, tx, e, id, false, false)
		if err != nil {
			return err
		}
		if !time.Now().Before(h.view.Expires) || h.view.State == "expired" {
			return reporting.ErrExpired
		}
		if !slices.Contains([]string{"completed", "partial"}, h.view.State) {
			return reporting.ErrIncomplete
		}
		grants, err := blockGrants(e)
		if err != nil {
			return err
		}
		var selected, static, body []byte
		var hash *string
		err = tx.QueryRow(ctx, `SELECT w.selected_outputs,w.static_payload,g.result,g.result_digest
 FROM chartworks.composition_run_pages p JOIN chartworks.composition_runs c USING(tenant_id,operation_id)
 JOIN chartworks.composition_run_widgets w USING(tenant_id,operation_id,page_id)
 LEFT JOIN chartworks.composition_run_groups g ON(g.tenant_id,g.operation_id,g.group_id)=(w.tenant_id,w.operation_id,w.group_id)
 WHERE p.tenant_id=$1 AND p.operation_id=$2 AND `+compositionPageEligibility+` AND p.page_id=$5 AND w.widget_id=$6`, e.Tenant(), id, grants, e.Has("reporting.preview"), page, widget).Scan(&selected, &static, &body, &hash)
		if err != nil {
			return err
		}
		if len(static) != 0 {
			if json.Unmarshal(static, &out) != nil || out.Page != page || out.Widget != widget {
				return store.ErrInvalid
			}
		} else {
			var result reporting.GroupResult
			var outputs []string
			if len(body) == 0 || json.Unmarshal(body, &result) != nil || json.Unmarshal(selected, &outputs) != nil || hash == nil || *hash != result.Digest || result.Digest != reporting.GroupResultDigest(result) {
				return store.ErrInvalid
			}
			out, err = reporting.CompositionPayloadFromResult(page, widget, outputs, result)
			if err != nil {
				return err
			}
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.CompositionPayload{}, err
	}
	return out, nil
}
