package postgres

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.DocumentRepository = (*DB)(nil)

const documentHeadColumns = `h.kind,h.document_id,h.version,h.latest_revision,COALESCE(h.draft_revision,0),COALESCE(h.review_revision,0),COALESCE(h.published_revision,0),h.archived,h.created_at,h.updated_at`

func scanDocumentState(row pgx.Row) (out reporting.DocumentState, err error) {
	err = row.Scan(&out.Kind, &out.ID, &out.Version, &out.LatestRevision, &out.DraftRevision, &out.ReviewRevision, &out.PublishedRevision, &out.Archived, &out.Created, &out.Updated)
	return
}

// $6 is always the verified envelope's bound, bounded resource set. The SQL
// excludes inaccessible definitions before any payload is returned to Go.
const documentReferenceEligibility = `NOT EXISTS (
 SELECT 1 FROM chartworks.document_references dep
 WHERE (dep.tenant_id,dep.kind,dep.document_id,dep.revision)=(r.tenant_id,r.kind,r.document_id,r.revision)
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) grant_ref
  WHERE grant_ref->>'kind'=dep.resource_kind AND grant_ref->>'permission'=dep.permission
  AND grant_ref->>'id' IN(dep.resource_id,'*')))`

// A dashboard never acquires a page's read authority from the enclosing label.
// This predicate is shared by full-reference authoring reads and redacted reads.
const documentPageEligibility = `($9::boolean AND EXISTS(
 SELECT 1 FROM jsonb_array_elements($6::jsonb) grant_ref
 WHERE grant_ref->>'kind'='report' AND grant_ref->>'permission'='read'
 AND grant_ref->>'id' IN(page.report_id,'*'))
 AND EXISTS(SELECT 1 FROM chartworks.document_heads report_head
  WHERE (report_head.tenant_id,report_head.kind,report_head.document_id)=(page.tenant_id,'report',page.report_id)
  AND NOT report_head.archived)
 AND NOT EXISTS(SELECT 1 FROM chartworks.document_references page_dep
  WHERE (page_dep.tenant_id,page_dep.kind,page_dep.document_id,page_dep.revision)=(page.tenant_id,'report',page.report_id,page.report_revision)
  AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements($6::jsonb) grant_ref
   WHERE grant_ref->>'kind'=page_dep.resource_kind AND grant_ref->>'permission'=page_dep.permission
   AND grant_ref->>'id' IN(page_dep.resource_id,'*'))))`

func documentReadArgs(e identity.Envelope, kind, id string, ref reporting.DocumentReference, a reporting.Access, redact bool) ([]any, error) {
	if ref.Revision < 0 || ref.Revision > 256 || !slices.Contains([]string{"", "published", "draft", "review"}, ref.Stage) || ref.Revision > 0 && ref.Stage != "" {
		return nil, store.ErrInvalid
	}
	if err := reporting.RequireDocument(e, kind, id, a); err != nil {
		return nil, err
	}
	grants, err := blockGrants(e)
	if err != nil {
		return nil, err
	}
	private := a == reporting.Write || a == reporting.Publish || reporting.RequireDocument(e, kind, id, reporting.Preview) == nil
	return []any{e.Tenant(), kind, id, ref.Revision, ref.Stage, grants, private, redact, e.Has("reporting.read")}, nil
}

func documentTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, kind, id string, ref reporting.DocumentReference, a reporting.Access, redact bool) (out reporting.DocumentSnapshot, err error) {
	args, err := documentReadArgs(e, kind, id, ref, a, redact)
	if err != nil {
		return out, err
	}
	var raw, origins []byte
	err = tx.QueryRow(ctx, `SELECT `+documentHeadColumns+`,r.revision,
 CASE WHEN r.kind='dashboard' AND $8::boolean THEN (r.definition-'pages') || jsonb_build_object('pages',
  (SELECT COALESCE(jsonb_agg(jsonb_build_object('id',page.page_id,'title',page.title,'report',page.report_id,'revision',page.report_revision) ORDER BY page.ordinal),'[]'::jsonb)
   FROM chartworks.document_page_refs page
   WHERE (page.tenant_id,page.kind,page.document_id,page.revision)=(r.tenant_id,r.kind,r.document_id,r.revision)
   AND `+documentPageEligibility+`)) ELSE r.definition END,
 r.digest,r.actor_id,r.session_id,r.created_at,r.origins,p.created_at
 FROM chartworks.document_heads h
 JOIN chartworks.document_revisions r ON(r.tenant_id,r.kind,r.document_id)=(h.tenant_id,h.kind,h.document_id)
 AND r.revision=CASE WHEN $4::bigint>0 THEN $4 WHEN $5='draft' THEN h.draft_revision WHEN $5='review' THEN h.review_revision ELSE h.published_revision END
 LEFT JOIN chartworks.document_publications p ON(p.tenant_id,p.kind,p.document_id,p.revision)=(r.tenant_id,r.kind,r.document_id,r.revision)
 WHERE h.tenant_id=$1 AND h.kind=$2 AND h.document_id=$3
 AND (p.revision IS NOT NULL OR $7::boolean)
 AND `+documentReferenceEligibility+`
 AND ($8::boolean OR r.kind<>'dashboard' OR NOT EXISTS(
  SELECT 1 FROM chartworks.document_page_refs page
  WHERE (page.tenant_id,page.kind,page.document_id,page.revision)=(r.tenant_id,r.kind,r.document_id,r.revision)
  AND NOT `+documentPageEligibility+`))`, args...).Scan(
		&out.State.Kind, &out.State.ID, &out.State.Version, &out.State.LatestRevision, &out.State.DraftRevision, &out.State.ReviewRevision, &out.State.PublishedRevision, &out.State.Archived, &out.State.Created, &out.State.Updated,
		&out.Revision.Number, &raw, &out.Revision.Digest, &out.Revision.Actor, &out.Revision.Session, &out.Revision.Created, &origins, &out.PublishedAt)
	if err != nil {
		return out, err
	}
	out.Revision.Raw = append(json.RawMessage(nil), raw...)
	if json.Unmarshal(origins, &out.Revision.Origins) != nil {
		return reporting.DocumentSnapshot{}, store.ErrInvalid
	}
	if !(kind == "dashboard" && redact) && reporting.DocumentDigest(out.Revision.Raw) != out.Revision.Digest {
		return reporting.DocumentSnapshot{}, store.ErrInvalid
	}
	if _, err := reporting.ProjectStoredDocument(out.Revision.Raw, kind); err != nil {
		return reporting.DocumentSnapshot{}, store.ErrInvalid
	}
	if !e.Valid() {
		return reporting.DocumentSnapshot{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

// ReadDocument applies tenant, target, dependency and private-preview restrictions
// in SQL. redactPages=false requires every page; it is not a redaction bypass.
func (d *DB) ReadDocument(ctx context.Context, e identity.Envelope, kind, id string, ref reporting.DocumentReference, a reporting.Access, redactPages bool) (out reporting.DocumentSnapshot, err error) {
	if _, err = documentReadArgs(e, kind, id, ref, a, redactPages); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		out, err = documentTx(ctx, tx, e, kind, id, ref, a, redactPages)
		return err
	})
	if err != nil {
		return reporting.DocumentSnapshot{}, err
	}
	return out, nil
}

// ListDocuments projects only localized metadata. It cannot load retained result
// tables, widget text, query SQL or hidden dashboard page names.
func (d *DB) ListDocuments(ctx context.Context, e identity.Envelope, kind, after string, limit int) (out reporting.DocumentList, err error) {
	out.Items = []reporting.DocumentSummary{}
	if !slices.Contains([]string{"report", "dashboard"}, kind) || limit < 1 || limit > 100 || after != "" && !identity.Identifier(after) {
		return out, store.ErrInvalid
	}
	selection, err := access.Constrain(e, "reporting.read", kind, "read")
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
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT h.kind,h.document_id,h.version,r.revision,r.definition->'metadata'
 FROM chartworks.document_heads h JOIN chartworks.document_revisions r
 ON(r.tenant_id,r.kind,r.document_id,r.revision)=(h.tenant_id,h.kind,h.document_id,h.published_revision)
 WHERE h.tenant_id=$1 AND h.kind=$2 AND h.document_id>$3 AND NOT h.archived
 AND ($4::boolean OR h.document_id=ANY($5::text[])) AND `+documentReferenceEligibility+`
 ORDER BY h.document_id LIMIT $7`, e.Tenant(), kind, after, selection.All(), selection.IDs(), grants, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item reporting.DocumentSummary
			var metadata []byte
			if err := rows.Scan(&item.Kind, &item.ID, &item.Version, &item.Revision, &metadata); err != nil {
				return err
			}
			if json.Unmarshal(metadata, &item.Metadata) != nil {
				return store.ErrInvalid
			}
			if len(out.Items) == limit {
				out.Next = out.Items[len(out.Items)-1].ID
				break
			}
			out.Items = append(out.Items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.DocumentList{}, err
	}
	return out, nil
}
