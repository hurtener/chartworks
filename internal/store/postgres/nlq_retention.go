package postgres

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// queryRetentionFence orders origin deletion against private query, reference,
// composition checkpoint and read admission. It grants no authority.
func queryRetentionFence(ctx context.Context, tx pgx.Tx, tenant string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,721415))`, tenant)
	return err
}

// markOwnedQueryErasure preserves only identities. Its input roots were selected
// under the authorized document deletion fence. Saved-copy propagation is stored
// at creation, never guessed from arbitrary conversational parentage.
func markOwnedQueryErasure(ctx context.Context, tx pgx.Tx, tenant string, roots []string) ([]string, error) {
	if _, err := tx.Exec(ctx, `UPDATE chartworks.nlq_query_origins o SET erased_at=COALESCE(erased_at,clock_timestamp())
 WHERE o.tenant_id=$1 AND EXISTS(SELECT 1 FROM chartworks.nlq_query_owners k WHERE (k.tenant_id,k.actor_id,k.session_id,k.query_id)=(o.tenant_id,o.actor_id,o.session_id,o.query_id) AND k.composition_root=ANY($2::text[]))`, tenant, roots); err != nil {
		return nil, err
	}
	// A saved report is independently authored; only its retained derivative run
	// is invalidated. Its definition and unavailable borrowed reference survive.
	rows, err := tx.Query(ctx, `SELECT DISTINCT c.operation_id FROM chartworks.composition_runs c
 LEFT JOIN chartworks.composition_run_groups g ON(g.tenant_id,g.operation_id)=(c.tenant_id,c.operation_id)
 LEFT JOIN chartworks.composition_run_payloads p ON(p.tenant_id,p.operation_id)=(c.tenant_id,c.operation_id)
 WHERE c.tenant_id=$1 AND (c.operation_id=ANY($2::text[]) OR EXISTS(
 SELECT 1 FROM chartworks.nlq_query_origins o WHERE (o.tenant_id,o.actor_id,o.session_id)=(c.tenant_id,c.actor_id,c.session_id) AND o.erased_at IS NOT NULL
 AND (o.query_id IN(convert_from(g.plan,'UTF8')::jsonb->>'query',convert_from(g.result,'UTF8')::jsonb->'query'->>'query') OR EXISTS(SELECT 1 FROM jsonb_array_elements(convert_from(p.manifest,'UTF8')::jsonb->'groups') x WHERE x->'origin'->>'query'=o.query_id)))) ORDER BY c.operation_id`, tenant, roots)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO chartworks.composition_origin_erasure(tenant_id,operation_id) SELECT $1,unnest($2::text[]) ON CONFLICT DO NOTHING`, tenant, out); err != nil {
		return nil, err
	}
	return out, nil
}

func eraseOwnedQueryJournals(ctx context.Context, tx pgx.Tx, tenant string) error {
	for _, sql := range []string{
		`UPDATE chartworks.read_attempts a SET cancel_requested=true WHERE a.tenant_id=$1 AND a.status IN('accepted','dispatching','running','uncertain') AND EXISTS(SELECT 1 FROM chartworks.nlq_query_operations k JOIN chartworks.nlq_query_origins o USING(tenant_id,actor_id,session_id,query_id) WHERE (k.tenant_id,k.actor_id,k.operation)=(a.tenant_id,a.actor_id,a.operation_id) AND o.erased_at IS NOT NULL)`,
		`DELETE FROM chartworks.read_attempts a WHERE a.tenant_id=$1 AND a.status NOT IN('accepted','dispatching','running','uncertain') AND EXISTS(SELECT 1 FROM chartworks.nlq_query_operations k JOIN chartworks.nlq_query_origins o USING(tenant_id,actor_id,session_id,query_id) WHERE (k.tenant_id,k.actor_id,k.operation)=(a.tenant_id,a.actor_id,a.operation_id) AND o.erased_at IS NOT NULL)`,
	} {
		if _, err := tx.Exec(ctx, sql, tenant); err != nil {
			return err
		}
	}
	return nil
}

// erasePrivateLearning deletes only complete, solely erased unreviewed copies.
// Shared/incomplete candidates are excluded from serving until explicit review;
// their retained payload limitation is exposed in the deletion receipt.
func erasePrivateLearning(ctx context.Context, tx pgx.Tx, tenant string) (erased, quarantined int, err error) {
	tag, err := tx.Exec(ctx, `INSERT INTO chartworks.nlq_example_erasures(tenant_id,example_id)
 SELECT x.tenant_id,x.example_id FROM chartworks.nlq_examples x WHERE x.tenant_id=$1 AND x.reviewed_at IS NULL
 AND x.evidence_count=(SELECT count(*) FROM chartworks.nlq_example_contributions c WHERE(c.tenant_id,c.example_id)=(x.tenant_id,x.example_id))
 AND EXISTS(SELECT 1 FROM chartworks.nlq_example_contributions c JOIN chartworks.nlq_query_origins o USING(tenant_id,actor_id,session_id,query_id) WHERE(c.tenant_id,c.example_id)=(x.tenant_id,x.example_id) AND o.erased_at IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM chartworks.nlq_example_contributions c LEFT JOIN chartworks.nlq_query_origins o USING(tenant_id,actor_id,session_id,query_id) WHERE(c.tenant_id,c.example_id)=(x.tenant_id,x.example_id) AND o.erased_at IS NULL)
 ON CONFLICT DO NOTHING`, tenant)
	if err != nil {
		return 0, 0, err
	}
	erased = int(tag.RowsAffected())
	tag, err = tx.Exec(ctx, `INSERT INTO chartworks.nlq_example_quarantine(tenant_id,example_id,reason)
 SELECT x.tenant_id,x.example_id,'erased_contributor' FROM chartworks.nlq_examples x WHERE x.tenant_id=$1 AND x.reviewed_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM chartworks.nlq_example_erasures z WHERE(z.tenant_id,z.example_id)=(x.tenant_id,x.example_id))
 AND EXISTS(SELECT 1 FROM chartworks.nlq_example_contributions c JOIN chartworks.nlq_query_origins o USING(tenant_id,actor_id,session_id,query_id) WHERE(c.tenant_id,c.example_id)=(x.tenant_id,x.example_id) AND o.erased_at IS NOT NULL)
 ON CONFLICT DO NOTHING`, tenant)
	if err != nil {
		return 0, 0, err
	}
	quarantined = int(tag.RowsAffected())
	_, err = tx.Exec(ctx, `DELETE FROM chartworks.nlq_examples x WHERE x.tenant_id=$1 AND x.reviewed_at IS NULL AND EXISTS(SELECT 1 FROM chartworks.nlq_example_erasures z WHERE(z.tenant_id,z.example_id)=(x.tenant_id,x.example_id))`, tenant)
	return erased, quarantined, err
}
