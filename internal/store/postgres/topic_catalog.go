package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/jackc/pgx/v5"
)

// ListPublishedTopics applies signed topic AND all dependency reach in SQL before
// any public name/description is selected. Private drafts are never queried.
func (d *DB) ListPublishedTopics(ctx context.Context, e identity.Envelope, in topics.ListRequest) (out []topics.Summary, err error) {
	if err = topics.CheckList(e, in); err != nil {
		return nil, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer cancel()
	args := []any{e.Tenant()}
	for _, p := range [][2]string{{"topic", "read"}, {"source", "read"}, {"dataset", "query"}, {"execution_context", "use"}} {
		selection, selectErr := access.Constrain(e, "topics.read", p[0], p[1])
		if selectErr != nil {
			return nil, selectErr
		}
		args = append(args, selection.All(), selection.IDs())
	}
	args = append(args, in.After, in.Limit)
	out = []topics.Summary{}
	err = d.transactionOptions(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT h.topic_id,v.version_id,h.revision,v.definition->>'name',v.definition->>'description',v.digest,v.created_at
   FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v
   ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version)
   WHERE h.tenant_id=$1 AND NOT h.archived AND ($2 OR h.topic_id=ANY($3::text[]))
   AND NOT EXISTS(SELECT 1 FROM chartworks.topic_published_dependencies d
    WHERE(d.tenant_id,d.topic_id,d.version_id)=(v.tenant_id,v.topic_id,v.version_id)
    AND(NOT($4 OR d.source_id=ANY($5::text[])) OR NOT($6 OR d.dataset_id=ANY($7::text[]))
    OR NOT($8 OR d.context_id=ANY($9::text[]))))
   AND ($10::text='' OR h.topic_id COLLATE "C">$10 COLLATE "C") ORDER BY h.topic_id COLLATE "C" LIMIT $11`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item topics.Summary
			if err = rows.Scan(&item.Topic, &item.Version, &item.Revision, &item.Name, &item.Description, &item.Digest, &item.PublishedAt); err != nil {
				return err
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
