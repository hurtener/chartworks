package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ nlqexec.SavedQueryReader = (*DB)(nil)

// ReadSavedQuery omits the result column in SQL, including when looking up a
// replay operation. Current actor, session and signed context reach constrain
// the projection before any protected definition is returned to the service.
func (d *DB) ReadSavedQuery(ctx context.Context, e identity.Envelope, id string, operation bool) (out nlqexec.QueryRecord, err error) {
	if !identity.Identifier(id) {
		return out, store.ErrInvalid
	}
	selection, err := access.Constrain(e, "query.execute", "execution_context", "use")
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT tenant_id,actor_id,session_id,query_id,parent_id,operation,topic_id,topics,topic_versions,rule_versions,context_id,locale,question,route,generation,sql_text,parameters,receipt,status,NULL::jsonb,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision,created_at,updated_at
 FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3
 AND (($4::boolean AND operation=$5) OR (NOT $4::boolean AND query_id=$5))
 AND ($6::boolean OR context_id=ANY($7::text[]))`, e.Tenant(), e.User(), e.Session(), operation, id, selection.All(), selection.IDs())
		if err := scanNLQQuery(row, &out); err != nil {
			return err
		}
		if err := markRuleEvidenceStale(ctx, tx, e.Tenant(), &out); err != nil {
			return err
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return nlqexec.QueryRecord{}, err
	}
	return out, nil
}
