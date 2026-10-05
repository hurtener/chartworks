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

var _ reporting.WriteDependencyRepository = (*DB)(nil)

// DiscoverReportWriteDependencies resolves native pins without loading their
// definitions. The BFF must authorize every returned requirement independently.
// This never grants content access or replaces native write-time validation.
func (d *DB) DiscoverReportWriteDependencies(ctx context.Context, e identity.Envelope, in reporting.WriteDependencyRequest) (out reporting.WriteDependencyManifest, err error) {
	if err = reporting.RequireWriteDependencyDiscovery(e, in); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	out = reporting.WriteDependencyManifest{Version: "report-write-dependencies-v1", Operation: in.Operation, ID: in.ID, ExpectedVersion: in.ExpectedVersion, References: []reporting.ResourceReference{}, Blocks: []reporting.DependencyBlock{}}
	if in.Operation == "save" {
		base, err := d.discoverReportDependencies(ctx, e, reporting.DependencyRequest{Kind: "report", ID: in.ID, Revision: in.Revision}, true)
		if err != nil {
			return reporting.WriteDependencyManifest{}, err
		}
		out.BaseRevision, out.BaseDigest = base.Revision, base.Digest
		out.References, out.Blocks = base.References, base.Blocks
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if in.Operation == "save" {
			var version int64
			if err := tx.QueryRow(ctx, `SELECT version FROM chartworks.document_heads WHERE tenant_id=$1 AND kind='report' AND document_id=$2 AND NOT deleted AND NOT archived`, e.Tenant(), in.ID).Scan(&version); err != nil {
				return err
			}
			if version != in.ExpectedVersion {
				return store.ErrConflict
			}
		}
		for _, canvas := range reporting.ReportCanvases(in.Definition) {
			for _, w := range canvas.Definition.Widgets {
				if w.Kind == "query" || w.Query != nil || w.Block != nil && w.Block.Narrative {
					return store.ErrInvalid
				}
				if w.Block == nil {
					continue
				}
				pin := w.Block
				if !identity.Identifier(pin.Block) || pin.Revision < 0 || pin.Revision > 256 {
					return store.ErrInvalid
				}
				private := pin.Policy == "private_preview"
				var b reporting.DependencyBlock
				// Private pins remain private even after later publication. Native
				// actor and digest custody applies before any metadata is returned.
				if err := tx.QueryRow(ctx, `SELECT h.block_id,h.topic_id,r.revision,r.digest,$4::boolean
 FROM chartworks.block_heads h JOIN chartworks.block_revisions r USING(tenant_id,block_id)
 LEFT JOIN chartworks.block_publications p USING(tenant_id,block_id,revision)
 WHERE h.tenant_id=$1 AND h.block_id=$2 AND NOT h.archived
 AND r.revision=CASE WHEN $3::bigint>0 THEN $3 ELSE h.published_revision END
 AND (($4::boolean AND $3>0 AND r.actor_id=$5 AND r.digest=$6) OR (NOT $4::boolean AND p.revision IS NOT NULL AND $6=''))`, e.Tenant(), pin.Block, pin.Revision, private, e.User(), pin.Digest).Scan(&b.ID, &b.Topic, &b.Revision, &b.Digest, &b.Private); err != nil {
					return err
				}
				if !slices.Contains(out.Blocks, b) {
					out.Blocks = append(out.Blocks, b)
				}
				if len(out.Blocks) > 128 {
					return store.ErrInvalid
				}
			}
		}
		for _, b := range out.Blocks {
			rows, err := tx.Query(ctx, `SELECT kind,permission,resource_id FROM chartworks.block_revision_references WHERE tenant_id=$1 AND block_id=$2 AND revision=$3 ORDER BY kind,permission,resource_id LIMIT 129`, e.Tenant(), b.ID, b.Revision)
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
			out.References = append(out.References, reporting.ResourceReference{Kind: "block", Permission: "read", ID: b.ID}, reporting.ResourceReference{Kind: "topic", Permission: "read", ID: b.Topic})
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
	})
	if err != nil {
		return reporting.WriteDependencyManifest{}, err
	}
	return out, nil
}
