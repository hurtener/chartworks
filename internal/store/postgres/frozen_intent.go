package postgres

import (
	"context"
	"encoding/json"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// frozenIntentTx runs AFTER the existing tenant/target/context/private gate.
// Only bounded metadata keys are selected, never values, SQL or narrative text.
func frozenIntentTx(ctx context.Context, tx pgx.Tx, tenant, id string, view *reporting.RunView) error {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT jsonb_build_object(
 'accepted_limits',m->'accepted_limits','selection',m->'selection',
 'selection_mode',m->'selection_mode','expected_schema',m->'revision'->'definition'->'expected_schema',
 'result_policy',m->'result_policy')
 FROM (SELECT convert_from(manifest,'UTF8')::jsonb m FROM chartworks.frozen_run_payloads
 WHERE tenant_id=$1 AND operation_id=$2) p`, tenant, id).Scan(&raw)
	if err != nil {
		return err
	}
	var metadata reporting.RunView
	if len(raw) > 1<<20 || json.Unmarshal(raw, &metadata) != nil {
		return store.ErrInvalid
	}
	view.AcceptedLimits, view.Selection, view.SelectionMode = metadata.AcceptedLimits, metadata.Selection, metadata.SelectionMode
	view.ExpectedSchema, view.ResultPolicy = metadata.ExpectedSchema, metadata.ResultPolicy
	for i := range view.ExpectedSchema {
		view.ExpectedSchema[i].Sensitivity = ""
		if i < len(view.ResultPolicy) && view.ResultPolicy[i].Name == view.ExpectedSchema[i].Name {
			view.ExpectedSchema[i].Sensitivity = view.ResultPolicy[i].Sensitivity
		}
	}
	return nil
}
