package postgres

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/jackc/pgx/v5"
	"time"
)

// scheduledProvenanceTx is called only after the existing artifact/read-context
// checks in frozenReadTx or the non-redacted compositionViewTx. A schedule ID is
// never accepted as a substitute for artifact permission. No payload is read.
func scheduledProvenanceTx(ctx context.Context, tx pgx.Tx, tenant, id, artifactState string, expires time.Time) (*reporting.ScheduledProvenance, error) {
	out := new(reporting.ScheduledProvenance)
	err := tx.QueryRow(ctx, `SELECT COALESCE(o.schedule_id,''),COALESCE(o.schedule_revision,0),o.due_at,o.window_start,o.window_end,o.status,
      d.query_state,d.artifact_state,d.catalog_state,d.notification_state,d.published_at
      FROM chartworks.operations o JOIN chartworks.reporting_occurrence_delivery d USING(tenant_id,operation_id)
      WHERE o.tenant_id=$1 AND o.operation_id=$2 AND o.kind='reporting.scheduled' AND o.dispatch_mode='queued'`, tenant, id).Scan(
		&out.ScheduleID, &out.ScheduleRevision, &out.DueAt, &out.WindowStart, &out.WindowEnd, &out.Execution,
		&out.Query, &out.Artifact, &out.Catalog, &out.Notification, &out.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	switch artifactState {
	case "normalized":
		out.Query = "succeeded"
	case "succeeded", "completed":
		out.Query, out.Artifact = "succeeded", "retained"
	case "partial":
		out.Query, out.Artifact = "partial", "retained"
	}
	if out.Catalog == "pending" && (out.Execution == "blocked" || out.Execution == "failed" || out.Execution == "cancelled" || out.Execution == "expired") {
		out.Catalog = "unavailable"
	}
	if artifactState == "expired" || !time.Now().Before(expires) {
		out.Artifact, out.Catalog = "expired", "expired"
	}
	return out, nil
}
