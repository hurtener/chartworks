package postgres

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// sweepRows is the single retention effect shared by inline and queued consumers.
// The queued caller supplies the accepted occurrence's observation time, not retry time.
func sweepRows(ctx context.Context, tx pgx.Tx, scope store.Scope, out *store.Operation, asOf time.Time) error {
	tag, err := tx.Exec(ctx, `DELETE FROM chartworks.audit_events WHERE (tenant_id,event_id) IN (
 SELECT tenant_id,event_id FROM chartworks.audit_events WHERE tenant_id=$1 AND created_at<$2 ORDER BY created_at,event_id LIMIT $3 FOR UPDATE SKIP LOCKED)`, scope.Tenant(), out.Cutoff, out.Limit)
	if err != nil {
		return err
	}
	out.DeletedEvents = tag.RowsAffected()
	tag, err = tx.Exec(ctx, `UPDATE chartworks.operations SET status='expired',deleted_events=0,deleted_operations=0 WHERE (tenant_id,operation_id) IN (
 SELECT o.tenant_id,o.operation_id FROM chartworks.operations o WHERE o.tenant_id=$1 AND o.operation_id<>$2 AND o.status='succeeded' AND o.expires_at<$4 AND NOT EXISTS(SELECT 1 FROM chartworks.audit_events a WHERE (a.tenant_id,a.operation_id)=(o.tenant_id,o.operation_id)) ORDER BY o.expires_at,o.operation_id LIMIT $3 FOR UPDATE OF o SKIP LOCKED)`, scope.Tenant(), out.ID, out.Limit, asOf)
	if err != nil {
		return err
	}
	out.DeletedOperations = tag.RowsAffected()
	// Bundle values expire at lookup; this existing bounded retention worker
	// removes the exact snapshot and its content-free steps after retention.
	_, err = tx.Exec(ctx, `DELETE FROM chartworks.byo_context_bundles WHERE (tenant_id,actor_id,session_id,bundle_id) IN (SELECT tenant_id,actor_id,session_id,bundle_id FROM chartworks.byo_context_bundles WHERE tenant_id=$1 AND retain_until<=$2 ORDER BY retain_until,bundle_id LIMIT $3 FOR UPDATE SKIP LOCKED)`, scope.Tenant(), asOf, out.Limit)
	return err
}
