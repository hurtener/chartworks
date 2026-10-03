package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ nlqroute.ApplicabilityReader = (*DB)(nil)

// ReadClarificationApplicability is an authenticated projection of one immutable
// query route. It issues no source grant and reads neither SQL nor result rows.
// The router must independently re-admit current topics/rules/source bindings.
func (d *DB) ReadClarificationApplicability(ctx context.Context, e identity.Envelope, id, action string) (out nlqroute.ApplicabilityRecord, err error) {
	if !identity.Identifier(id) || action != "query.preflight" && action != "query.plan" && action != "query.execute" {
		return out, store.ErrInvalid
	}
	selection, err := access.Constrain(e, action, "execution_context", "use")
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT query_id,tenant_id,actor_id,session_id,context_id,status,revision,route
FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4
AND ($5::boolean OR context_id=ANY($6::text[]))`, e.Tenant(), e.User(), e.Session(), id, selection.All(), selection.IDs()).Scan(&out.Query, &out.Tenant, &out.Actor, &out.Session, &out.Context, &out.Status, &out.Revision, &raw)
		if err != nil {
			return err
		}
		if json.Unmarshal(raw, &out.Route) != nil {
			return store.ErrMigration
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		return ctx.Err()
	})
	if err != nil {
		return nlqroute.ApplicabilityRecord{}, err
	}
	return out, nil
}

func validateApplicabilityInsert(ctx context.Context, tx pgx.Tx, scope store.Scope, q nlqexec.QueryRecord) error {
	if q.Route.Applicability != nil && q.Context != q.Route.Request.Context || !q.Route.ApplicabilityWriteValid(scope.Tenant(), scope.Actor(), q.Session, q.ID) {
		return store.ErrInvalid
	}
	p := q.Route.Applicability
	if p == nil || p.PriorQuery == "" {
		return nil
	}
	var parent nlqexec.QueryRecord
	if q.Parent != p.PriorQuery {
		origin, ok := q.ReviewedApplicabilityOrigin()
		if !ok || origin.QueryID != p.PriorQuery {
			return store.ErrInvalid
		}
		if err := scanNLQQuery(tx.QueryRow(ctx, `SELECT `+nlqQueryColumns+` FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4 FOR UPDATE`, scope.Tenant(), scope.Actor(), q.Session, origin.QueryID), &parent); err != nil {
			return err
		}
		if parent.Context != q.Context || parent.Revision != origin.Revision || nlqexec.QueryLineageDigest(parent) != origin.Digest {
			return store.ErrInvalid
		}
	} else {
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT context_id,route FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4`, scope.Tenant(), scope.Actor(), q.Session, p.PriorQuery).Scan(&parent.Context, &raw); err != nil {
			return err
		}
		if parent.Context != q.Context || json.Unmarshal(raw, &parent.Route) != nil {
			return store.ErrInvalid
		}
	}
	if parent.Route.Applicability == nil || p.PriorDigest != exec.Hash(parent.Route.Applicability) {
		return store.ErrInvalid
	}
	return nil
}
