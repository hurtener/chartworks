package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/jackc/pgx/v5"
)

var _ reporting.RunCandidateRepository = (*DB)(nil)

// Only original parent, tenant and private custody are disclosed to the trusted
// BFF. No names, timings, payload, current definition or query work is accessed.
func (d *DB) DiscoverRunCandidates(ctx context.Context, e identity.Envelope, in reporting.RunCandidateRequest) (out reporting.RunCandidatePage, err error) {
	if err = reporting.RequireRunCandidateDiscovery(e, in); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	preview := access.Require(e, "reporting.preview", access.Resource{Tenant: e.Tenant(), Kind: in.Kind, ID: in.Resource, Permission: "preview"}) == nil
	query := `SELECT operation_id FROM chartworks.composition_runs WHERE tenant_id=$1 AND kind='report' AND document_id=$2 AND operation_id>$3 AND (NOT private OR ($4 AND actor_id=$5 AND session_id=$6)) ORDER BY operation_id LIMIT $7`
	if in.Kind == "block" {
		query = `SELECT operation_id FROM chartworks.frozen_runs WHERE tenant_id=$1 AND block_id=$2 AND operation_id>$3 AND (NOT private OR ($4 AND actor_id=$5 AND session_id=$6)) ORDER BY operation_id LIMIT $7`
	}
	err = d.transactionOptions(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, e.Tenant(), in.Resource, in.After, preview, e.User(), e.Session(), in.Limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = reporting.RunCandidatePage{Version: "report-run-candidates-v1", Kind: in.Kind, Resource: in.Resource, Runs: []string{}}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return err
			}
			out.Runs = append(out.Runs, id)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		if len(out.Runs) > in.Limit {
			out.Runs = out.Runs[:in.Limit]
			out.Next = out.Runs[len(out.Runs)-1]
		}
		return nil
	})
	if err != nil {
		return reporting.RunCandidatePage{}, err
	}
	if !e.Valid() {
		return reporting.RunCandidatePage{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}
