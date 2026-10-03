package postgres

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func (d *DB) ReadPlanOperation(ctx context.Context, scope store.Scope, operation string) (out nlqexec.QueryRecord, err error) {
	if err = checkScope(scope); err != nil || operation == "" {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := scanNLQQuery(tx.QueryRow(ctx, `SELECT `+nlqQueryColumns+` FROM chartworks.nlq_queries WHERE tenant_id=$1 AND actor_id=$2 AND plan_operation=$3`, scope.Tenant(), scope.Actor(), operation), &out); err != nil {
			return err
		}
		return markRuleEvidenceStale(ctx, tx, scope.Tenant(), &out)
	})
	if errors.Is(err, store.ErrNotFound) {
		return d.ReadOperation(ctx, scope, operation)
	}
	return out, err
}
