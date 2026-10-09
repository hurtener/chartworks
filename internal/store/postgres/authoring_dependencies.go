package postgres

import (
	"context"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.DependencyRepository = (*DB)(nil)

// DiscoverReportDependencies is a separate metadata projection. It never returns
// or decodes a definition, SQL, artifact or validation payload and never constructs an
// envelope. Native content/execute paths still demand every signed dependency.
func (d *DB) DiscoverReportDependencies(ctx context.Context, e identity.Envelope, in reporting.DependencyRequest) (out reporting.DependencyManifest, err error) {
	if err = reporting.RequireDependencyDiscovery(e, in); err != nil {
		return out, err
	}
	return d.discoverReportDependencies(ctx, e, in, false)
}

// write is used only after exact report-write discovery admission. A native edit
// can read its private baseline under write authority; block custody is unchanged.
func (d *DB) discoverReportDependencies(ctx context.Context, e identity.Envelope, in reporting.DependencyRequest, write bool) (out reporting.DependencyManifest, err error) {
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		out, err = reportDependenciesTx(ctx, tx, e, in, write)
		return err
	})
	if err != nil {
		return reporting.DependencyManifest{}, err
	}
	return out, nil
}

func reportDependenciesTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, in reporting.DependencyRequest, write bool) (out reporting.DependencyManifest, err error) {
	out = reporting.DependencyManifest{Version: "report-dependencies-v1", Kind: in.Kind, ID: in.ID, References: []reporting.ResourceReference{}, Blocks: []reporting.DependencyBlock{}}
	preview := write || access.Require(e, "reporting.preview", access.Resource{Tenant: e.Tenant(), Kind: in.Kind, ID: in.ID, Permission: "preview"}) == nil
	err = func() error {
		if in.Kind == "block" {
			var b reporting.DependencyBlock
			err := tx.QueryRow(ctx, `SELECT h.block_id,COALESCE(h.topic_id,''),COALESCE(h.source_parent,''),r.revision,r.digest,p.revision IS NULL
 FROM chartworks.block_heads h JOIN chartworks.block_revisions r USING(tenant_id,block_id)
 LEFT JOIN chartworks.block_publications p USING(tenant_id,block_id,revision)
 WHERE h.tenant_id=$1 AND h.block_id=$2 AND NOT h.archived
 AND r.revision=CASE WHEN $3::bigint>0 THEN $3 WHEN $4='draft' THEN h.draft_revision ELSE h.published_revision END
 AND (p.revision IS NOT NULL OR ($5::boolean AND r.actor_id=$6))`, e.Tenant(), in.ID, in.Revision, in.Stage, preview, e.User()).Scan(&b.ID, &b.Topic, &b.Source, &b.Revision, &b.Digest, &b.Private)
			if err != nil {
				return err
			}
			out.Revision, out.Digest, out.Private = b.Revision, b.Digest, b.Private
			out.Blocks = append(out.Blocks, b)
		} else {
			err := tx.QueryRow(ctx, `SELECT r.revision,r.digest,p.revision IS NULL
 FROM chartworks.document_heads h JOIN chartworks.document_revisions r USING(tenant_id,kind,document_id)
 LEFT JOIN chartworks.document_publications p USING(tenant_id,kind,document_id,revision)
 WHERE h.tenant_id=$1 AND h.kind='report' AND h.document_id=$2 AND NOT h.deleted AND NOT h.archived
 AND r.revision=CASE WHEN $3::bigint>0 THEN $3 WHEN $4='draft' THEN h.draft_revision WHEN $4='review' THEN h.review_revision WHEN $4='editing' THEN COALESCE(h.draft_revision,h.review_revision) ELSE h.published_revision END
 AND (p.revision IS NOT NULL OR $5::boolean)
 AND NOT jsonb_path_exists(r.definition,'$.** ? (@.kind == "query")')
 AND NOT EXISTS(SELECT 1 FROM chartworks.document_private_block_refs b
 WHERE (b.tenant_id,b.kind,b.document_id,b.revision)=(r.tenant_id,r.kind,r.document_id,r.revision) AND b.block_actor_id<>$6)
 AND NOT EXISTS(SELECT 1 FROM chartworks.document_query_refs q
 WHERE (q.tenant_id,q.kind,q.document_id,q.revision)=(r.tenant_id,r.kind,r.document_id,r.revision))`, e.Tenant(), in.ID, in.Revision, in.Stage, preview, e.User()).Scan(&out.Revision, &out.Digest, &out.Private)
			if err != nil {
				return err
			}
			// A latest-publication pin follows the native current head. Include
			// both stored report requirements and the newly selected block's
			// requirements; old coordinates never stand in for current ones.
			rows, err := tx.Query(ctx, `SELECT DISTINCT b.block_id,COALESCE(h.topic_id,''),COALESCE(h.source_parent,''),b.selected_revision,r.digest,b.private
 FROM (SELECT block_id,COALESCE(pinned_revision,h.published_revision) selected_revision,false private
 FROM chartworks.document_block_refs b JOIN chartworks.block_heads h USING(tenant_id,block_id)
 WHERE b.tenant_id=$1 AND b.kind='report' AND b.document_id=$2 AND b.revision=$3
 UNION ALL SELECT block_id,block_revision,true FROM chartworks.document_private_block_refs
 WHERE tenant_id=$1 AND kind='report' AND document_id=$2 AND revision=$3) b
 JOIN chartworks.block_heads h ON h.tenant_id=$1 AND h.block_id=b.block_id
 LEFT JOIN chartworks.block_revisions r ON r.tenant_id=$1 AND r.block_id=b.block_id AND r.revision=b.selected_revision
 ORDER BY b.block_id,b.selected_revision LIMIT 129`, e.Tenant(), in.ID, out.Revision)
			if err != nil {
				return err
			}
			out.Blocks, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (b reporting.DependencyBlock, err error) {
				err = row.Scan(&b.ID, &b.Topic, &b.Source, &b.Revision, &b.Digest, &b.Private)
				return
			})
			if err != nil || len(out.Blocks) > 128 {
				if err == nil {
					err = store.ErrInvalid
				}
				return err
			}
			rows, err = tx.Query(ctx, `SELECT resource_kind,permission,resource_id FROM chartworks.document_references
 WHERE tenant_id=$1 AND kind='report' AND document_id=$2 AND revision=$3 ORDER BY resource_kind,permission,resource_id LIMIT 129`, e.Tenant(), in.ID, out.Revision)
			if err != nil {
				return err
			}
			out.References, err = dependencyReferences(rows)
			if err != nil {
				return err
			}
		}
		for _, b := range out.Blocks {
			rows, err := tx.Query(ctx, `SELECT kind,permission,resource_id FROM chartworks.block_revision_references
 WHERE tenant_id=$1 AND block_id=$2 AND revision=$3 ORDER BY kind,permission,resource_id LIMIT 129`, e.Tenant(), b.ID, b.Revision)
			if err != nil {
				return err
			}
			refs, err := dependencyReferences(rows)
			if err != nil {
				return err
			}
			if len(refs) == 0 {
				return store.ErrInvalid
			}
			out.References = append(out.References, refs...)
			out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "read", ID: b.ID})
			if b.Topic != "" {
				out.References = append(out.References, reporting.ResourceReference{Kind: "topic", Permission: "read", ID: b.Topic})
			} else if b.Source != "" {
				out.References = append(out.References, reporting.ResourceReference{Kind: "source", Permission: "read", ID: b.Source})
			} else {
				return store.ErrInvalid
			}
			if b.Private {
				out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "preview", ID: b.ID})
			}
		}
		slices.SortFunc(out.References, func(a, b reporting.ResourceReference) int {
			return slices.Compare([]string{a.Kind, a.Permission, a.ID}, []string{b.Kind, b.Permission, b.ID})
		})
		out.References = slices.Compact(out.References)
		if len(out.References) > 128 {
			return store.ErrInvalid
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	}()
	if err != nil {
		return reporting.DependencyManifest{}, err
	}
	return out, nil
}

func dependencyReferences(rows pgx.Rows) ([]reporting.ResourceReference, error) {
	refs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (r reporting.ResourceReference, err error) {
		err = row.Scan(&r.Kind, &r.Permission, &r.ID)
		return
	})
	if err == nil && len(refs) > 128 {
		err = store.ErrInvalid
	}
	return refs, err
}
