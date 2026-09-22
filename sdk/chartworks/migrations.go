//nolint:revive // Aliases and methods intentionally expose the versioned migration SDK contract.
package chartworks

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
)

var ErrMigrationRequest = errors.New("chartworks: invalid migration request")

type MigrationManifest = migration.Manifest
type MigrationObject = migration.Object
type MigrationTombstoneTarget = migration.TombstoneTarget
type MigrationMapping = migration.Mapping
type MigrationFieldDisposition = migration.FieldDisposition
type MigrationEvidence = migration.Evidence
type MigrationOccurrenceBoundary = migration.OccurrenceBoundary
type MigrationPlan = migration.Plan
type MigrationBatch = migration.Batch
type MigrationExport = migration.Export
type MigrationCutover = migration.Cutover
type MigrationEraseResult = migration.EraseResult
type MigrationDryRunRequest = migration.DryRunRequest
type MigrationImportRequest = migration.ImportRequest
type MigrationResumeRequest = migration.ResumeRequest
type MigrationExportRequest = migration.ExportRequest
type MigrationCutoverRequest = migration.CutoverRequest
type MigrationRollbackRequest = migration.RollbackRequest
type MigrationEraseRequest = migration.EraseRequest

func (c *Client) DryRunMigration(ctx context.Context, in MigrationDryRunRequest) (out MigrationPlan, err error) {
	if !identity.Identifier(in.Manifest.Batch) {
		return out, ErrMigrationRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/migrations/dry-runs", "", in, &out, 10<<20)
	return
}
func (c *Client) ImportMigration(ctx context.Context, in MigrationImportRequest) (out MigrationBatch, err error) {
	if !identity.Identifier(in.Manifest.Batch) || in.Expected < 0 {
		return out, ErrMigrationRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/migrations/imports", "", in, &out, 10<<20)
	return
}
func (c *Client) ResumeMigration(ctx context.Context, in MigrationResumeRequest) (out MigrationBatch, err error) {
	if !identity.Identifier(in.Batch) || in.Expected < 1 {
		return out, ErrMigrationRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/migrations/resume", "", in, &out, 4<<20)
	return
}
func (c *Client) ExportMigration(ctx context.Context, in MigrationExportRequest) (out MigrationExport, err error) {
	if !identity.Identifier(in.Batch) || in.Limit < 1 || in.Limit > 1000 {
		return out, ErrMigrationRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/migrations/exports", "", in, &out, 10<<20)
	return
}
func (c *Client) CutoverMigration(ctx context.Context, in MigrationCutoverRequest) (out MigrationCutover, err error) {
	if !identity.Identifier(in.Batch) || !identity.Identifier(in.Route) || !identity.Identifier(in.OperatorRef) || in.Expected < 0 {
		return out, ErrMigrationRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/migrations/cutovers", "", in, &out, 4<<20)
	return
}
func (c *Client) RollbackMigration(ctx context.Context, in MigrationRollbackRequest) (out MigrationCutover, err error) {
	if !identity.Identifier(in.Cohort) || !identity.Identifier(in.OperatorRef) || in.Expected < 1 {
		return out, ErrMigrationRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/migrations/rollbacks", "", in, &out, 4<<20)
	return
}
func (c *Client) EraseMigration(ctx context.Context, in MigrationEraseRequest) (out MigrationEraseResult, err error) {
	if !identity.Identifier(in.Batch) || in.Limit < 1 || in.Limit > 1000 {
		return out, ErrMigrationRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/migrations/erasures", "", in, &out, 4<<20)
	return
}
