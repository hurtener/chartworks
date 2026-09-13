package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ jobs.ReportingUsageRepository = (*DB)(nil)

// reportingDispatchTx runs under the shared admission transaction. No source
// connection, model, identity lookup, or token is involved. Immutable pins are
// resolved here once; later attempts can only use or reject this exact result.
func reportingDispatchTx(ctx context.Context, tx pgx.Tx, tenant string, target jobs.ReportingTarget) (jobs.ReportingDispatch, error) {
	out := jobs.ReportingDispatch{Target: target, Pins: []jobs.ReportingPin{}}
	blocked := func() (jobs.ReportingDispatch, error) {
		return jobs.ReportingDispatch{Target: target, Pins: []jobs.ReportingPin{}, Blocked: "dependency_unavailable"}, nil
	}
	if target.ResourceKind() == "block" {
		err := tx.QueryRow(ctx, `SELECT r.revision,r.digest FROM chartworks.block_heads h
 JOIN chartworks.block_revisions r ON(r.tenant_id,r.block_id)=(h.tenant_id,h.block_id)
 AND r.revision=CASE WHEN $3::bigint=0 THEN h.published_revision ELSE $3 END
 JOIN chartworks.block_publications p ON(p.tenant_id,p.block_id,p.revision)=(r.tenant_id,r.block_id,r.revision)
 WHERE h.tenant_id=$1 AND h.block_id=$2 AND NOT h.archived FOR SHARE OF h`, tenant, target.ID, target.Revision).Scan(&out.Revision, &out.Digest)
		if errors.Is(err, pgx.ErrNoRows) {
			return blocked()
		}
		return out, err
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT r.revision,r.digest,r.definition FROM chartworks.document_heads h
 JOIN chartworks.document_revisions r ON(r.tenant_id,r.kind,r.document_id)=(h.tenant_id,h.kind,h.document_id)
 AND r.revision=CASE WHEN $3::bigint=0 THEN h.published_revision ELSE $3 END
 JOIN chartworks.document_publications p ON(p.tenant_id,p.kind,p.document_id,p.revision)=(r.tenant_id,r.kind,r.document_id,r.revision)
 WHERE h.tenant_id=$1 AND h.kind='report' AND h.document_id=$2 AND NOT h.archived FOR SHARE OF h`, tenant, target.ID, target.Revision).Scan(&out.Revision, &out.Digest, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return blocked()
	}
	if err != nil {
		return out, err
	}
	if reporting.DocumentDigest(raw) != out.Digest {
		return out, store.ErrInvalid
	}
	definition, err := reporting.ProjectStoredDocument(raw, "report")
	if err != nil {
		return blocked()
	}
	found := target.Type != "saved_question"
	for _, widget := range definition.Widgets {
		if target.Type == "saved_question" && widget.ID != target.Widget {
			continue
		}
		found = true
		if target.Type == "saved_question" && widget.Kind != "query" {
			return blocked()
		}
		if widget.Kind == "query" && (!target.Dynamic || widget.Query == nil || widget.Query.Durability != "replayable") {
			return blocked()
		}
		if widget.Kind != "block" {
			continue
		}
		pin := jobs.ReportingPin{ID: widget.ID, Block: widget.Block.Block}
		err := tx.QueryRow(ctx, `SELECT r.revision,r.digest FROM chartworks.block_heads h
 JOIN chartworks.block_revisions r ON(r.tenant_id,r.block_id)=(h.tenant_id,h.block_id)
 AND r.revision=CASE WHEN $3::bigint=0 THEN h.published_revision ELSE $3 END
 JOIN chartworks.block_publications p ON(p.tenant_id,p.block_id,p.revision)=(r.tenant_id,r.block_id,r.revision)
 WHERE h.tenant_id=$1 AND h.block_id=$2 AND NOT h.archived FOR SHARE OF h`, tenant, pin.Block, widget.Block.Revision).Scan(&pin.Revision, &pin.Digest)
		if errors.Is(err, pgx.ErrNoRows) {
			return blocked()
		}
		if err != nil {
			return out, err
		}
		out.Pins = append(out.Pins, pin)
	}
	if !found {
		return blocked()
	}
	return out, nil
}

func insertReportingDeliveryTx(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
	if j.Kind != jobs.ReportingKind || j.Reporting == nil {
		return nil
	}
	b := j.Reporting.Target.Budget
	_, err := tx.Exec(ctx, `INSERT INTO chartworks.reporting_occurrence_delivery
 (tenant_id,operation_id,artifact_kind,query_limit,model_call_limit,model_token_limit)
 VALUES($1,$2,$3,$4,$5,$6)`, j.Tenant, j.ID, j.Reporting.Target.ResourceKind(), b.QueryAttempts, b.ModelCalls, b.ModelTokens)
	return err
}

// ReserveReportingUsage requires the original root owner and its present fence.
// Checking and incrementing are one transaction, so retries and replicas share
// the same nonrefundable budget. Refusal precedes all physical source/model I/O.
func (d *DB) ReserveReportingUsage(ctx context.Context, inv jobs.Invocation, charge jobs.ReportingCharge) error {
	if !charge.Valid() {
		return jobs.ErrInvalid
	}
	if _, nested := inv.Parent(); nested {
		return jobs.ErrAuthority
	}
	l := inv.Lease()
	j := l.Task.Dispatch
	if j == nil || j.Kind != jobs.ReportingKind || !j.Valid() || j.Reporting.Blocked != "" {
		return jobs.ErrAuthority
	}
	e, err := inv.Current(l.Task.Input.Kind, l.Task.Input.Target, l.Task.Input.InputHash)
	if err != nil {
		return err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer stop()
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := requestFenceTx(ctx, tx, inv); err != nil {
			return err
		}
		b := j.Reporting.Target.Budget
		tag, err := tx.Exec(ctx, `UPDATE chartworks.reporting_occurrence_delivery
 SET query_reservations=query_reservations+$3,model_call_reservations=model_call_reservations+$4,model_token_reservations=model_token_reservations+$5
 WHERE tenant_id=$1 AND operation_id=$2 AND query_limit=$6 AND model_call_limit=$7 AND model_token_limit=$8
 AND query_reservations+$3<=query_limit AND model_call_reservations+$4<=model_call_limit AND model_token_reservations+$5<=model_token_limit
 AND catalog_state='pending'`, j.Tenant, j.ID, charge.Queries, charge.ModelCalls, charge.ModelTokens, b.QueryAttempts, b.ModelCalls, b.ModelTokens)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return jobs.ErrReportingBudget
		}
		return nil
	})
}

func reportingReceiptTx(ctx context.Context, tx pgx.Tx, j jobs.Job) (*jobs.ReportingReceipt, error) {
	if j.Kind != jobs.ReportingKind || j.Reporting == nil {
		return nil, nil
	}
	var out jobs.ReportingReceipt
	err := tx.QueryRow(ctx, `SELECT query_state,artifact_state,catalog_state,notification_state,artifact_kind,
 query_reservations,model_call_reservations,model_token_reservations
 FROM chartworks.reporting_occurrence_delivery WHERE tenant_id=$1 AND operation_id=$2`, j.Tenant, j.ID).Scan(
		&out.Query, &out.Artifact, &out.Catalog, &out.Notification, &out.Kind, &out.Reserved.Queries, &out.Reserved.ModelCalls, &out.Reserved.ModelTokens)
	if err != nil {
		return nil, err
	}
	var state string
	var expired bool
	query := `SELECT state,expires_at<=clock_timestamp() FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2`
	if out.Kind == "report" {
		query = `SELECT state,expires_at<=clock_timestamp() FROM chartworks.composition_runs WHERE tenant_id=$1 AND operation_id=$2`
	}
	err = tx.QueryRow(ctx, query, j.Tenant, j.ID).Scan(&state, &expired)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		out.ArtifactID = j.ID
		if state == "normalized" {
			out.Query = "succeeded"
		}
		if state == "expired" || expired {
			out.Artifact, out.Catalog = "expired", "expired"
		}
	} else if j.State == "blocked" || j.State == "failed" || j.State == "cancelled" || j.State == "expired" {
		out.Query, out.Catalog = j.State, "unavailable"
	}
	return &out, nil
}

// completeReportingRequestTx publishes a truthful catalog receipt and finishes
// the SAME root fence. A failed report is blocked, not a successful delivery.
// Notification remains not_requested regardless of descriptive recipients.
func completeReportingRequestTx(ctx context.Context, tx pgx.Tx, inv jobs.Invocation) error {
	l := inv.Lease()
	j := l.Task.Dispatch
	if j == nil || j.Kind != jobs.ReportingKind || !j.Valid() {
		return jobs.ErrAuthority
	}
	e, err := inv.Current(l.Task.Input.Kind, l.Task.Input.Target, l.Task.Input.InputHash)
	if err != nil {
		return err
	}
	var state string
	var queryState string
	if j.Reporting.Target.ResourceKind() == "block" {
		var normalized bool
		err = tx.QueryRow(ctx, `SELECT f.state,p.result IS NOT NULL FROM chartworks.frozen_runs f
 JOIN chartworks.frozen_run_payloads p USING(tenant_id,operation_id) WHERE f.tenant_id=$1 AND f.operation_id=$2`, j.Tenant, j.ID).Scan(&state, &normalized)
		if err != nil {
			return err
		}
		if !normalized {
			return reporting.ErrIncomplete
		}
		queryState = "succeeded"
	} else {
		err = tx.QueryRow(ctx, `SELECT state FROM chartworks.composition_runs WHERE tenant_id=$1 AND operation_id=$2`, j.Tenant, j.ID).Scan(&state)
		queryState = "succeeded"
		if state == "partial" {
			queryState = "partial"
		}
		if state == "failed" {
			queryState = "failed"
		}
	}
	if err != nil {
		return err
	}
	if state != "succeeded" && state != "completed" && state != "partial" && state != "failed" {
		return reporting.ErrIncomplete
	}
	jobState, code, catalog, audit := "succeeded", "", "available", "reporting.delivery_completed"
	if state == "failed" {
		jobState, code, catalog, audit = "blocked", "reporting_attention", "unavailable", "reporting.delivery_blocked"
	}
	tag, err := tx.Exec(ctx, `UPDATE chartworks.operations SET status=$6,error_code=$7,finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL
 WHERE tenant_id=$1 AND operation_id=$2 AND status='running' AND lease_owner=$3 AND fence=$4 AND manifest_hash=$5
 AND lease_until>clock_timestamp() AND expires_at>clock_timestamp()`, j.Tenant, j.ID, l.Owner, l.Fence, l.Task.ManifestHash, jobState, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return store.ErrConflict
	}
	tag, err = tx.Exec(ctx, `UPDATE chartworks.reporting_occurrence_delivery SET query_state=$3,artifact_state='retained',catalog_state=$4,
 published_at=CASE WHEN $4='available' THEN clock_timestamp() ELSE NULL END WHERE tenant_id=$1 AND operation_id=$2 AND catalog_state='pending'`, j.Tenant, j.ID, queryState, catalog)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return store.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE chartworks.operation_attempts SET state=$5,error_code=$6,executor_id=$4,finished_at=clock_timestamp()
 WHERE tenant_id=$1 AND operation_id=$2 AND fence=$3 AND state='acquiring'`, j.Tenant, j.ID, l.Fence, e.User(), jobState, code); err != nil {
		return err
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return err
	}
	return auditJob(ctx, tx, scope, audit, j.ID)
}

// Keep the dispatch's closed JSON shape checked before inserting it. These
// helpers expose no raw definition or token in error text or history.
func reportingDispatchJSON(j jobs.Job) ([]byte, []byte, error) {
	if j.Kind != jobs.ReportingKind || j.Reporting == nil || j.Reporting.Input != jobs.ReportingInput(j) || !j.Valid() || !reportingWindow(j) {
		return nil, nil, jobs.ErrInvalid
	}
	manifest, err := json.Marshal(j)
	if err != nil || len(manifest) > 64<<10 {
		return nil, nil, jobs.ErrInvalid
	}
	input, err := json.Marshal(j.Reporting.Input)
	return manifest, input, err
}

// reportingWindow keeps manual first-occurrence handling explicit in the
// parameter core. A zero-width manual occurrence is never labeled a data window.
func reportingWindow(j jobs.Job) bool {
	return !j.DueAt.IsZero() && j.WindowEnd.Equal(j.DueAt) && !j.WindowStart.After(j.WindowEnd) &&
		j.DueAt.Equal(j.DueAt.UTC().Truncate(time.Microsecond)) && !strings.Contains(j.Reporting.Target.Timezone, "..")
}
