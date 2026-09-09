package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// ReadDatasetCatalog selects authorized relation JSON in SQL, never fetches a
// broad binding to redact later, and never opens a warehouse connection.
func (d *DB) ReadDatasetCatalog(ctx context.Context, e identity.Envelope, in sources.DatasetQuery) (out []sources.Dataset, err error) {
	if err = sources.CheckDatasetQuery(e, in); err != nil {
		return nil, err
	}
	selection, err := access.Constrain(e, "sources.read", "dataset", "query")
	if err != nil {
		return nil, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer cancel()
	out = []sources.Dataset{}
	err = d.transactionOptions(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT r.source_id,r.context_id,r.revision,r.binding->>'dialect',relation
   FROM chartworks.sources s JOIN chartworks.source_revisions r
   ON(r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision)
   CROSS JOIN LATERAL jsonb_array_elements(r.binding->'relations') relation
   WHERE NOT s.deleted AND s.tenant_id=$1 AND s.source_id=$2 AND r.context_id=$3
   AND ($4 OR relation->>'id'=ANY($5::text[])) AND ($6::text='' OR relation->>'id'=$6)
   AND ($7::text='' OR (relation->>'id') COLLATE "C">$7 COLLATE "C")
   ORDER BY (relation->>'id') COLLATE "C" LIMIT $8`, e.Tenant(), in.Source, in.Context, selection.All(), selection.IDs(), in.Dataset, in.After, in.Limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item sources.Dataset
			var raw []byte
			if err = rows.Scan(&item.Source, &item.Context, &item.Revision, &item.Dialect, &raw); err != nil {
				return err
			}
			if json.Unmarshal(raw, &item.Relation) != nil {
				return store.ErrInvalid
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
