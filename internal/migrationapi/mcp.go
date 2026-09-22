//nolint:revive // MCPBindings is the public composition seam for the optional group.
package migrationapi

import (
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/migration"
)

func MCPBindings(service *migration.Service) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, nil
	}
	registry, err := Registry()
	if err != nil {
		return nil, err
	}
	mapper := func(err error) mcpserver.Fault { _, code := classify(err); return mcpserver.Fault{Code: code} }
	type bindingSpec struct {
		id, name, description string
		build                 func() (mcpserver.Binding, error)
	}
	specs := []bindingSpec{
		{id: "migrationDryRun", name: "migration_dry_run", description: "Validate a neutral cohort bundle, dependency order, evidence and exhaustive loss ledger without writing domain state.", build: func() (mcpserver.Binding, error) {
			return mcpserver.Bind(registry, "migrationDryRun", "migration_dry_run", "migration", "Validate a neutral cohort bundle, dependency order, evidence and exhaustive loss ledger without writing domain state.", service.DryRun, mapper)
		}},
		{id: "migrationImport", name: "migration_import", description: "Import a neutral cohort through current-authority domain adapters with resumable exact checkpoints and quarantine.", build: func() (mcpserver.Binding, error) {
			return mcpserver.Bind(registry, "migrationImport", "migration_import", "migration", "Import a neutral cohort through current-authority domain adapters with resumable exact checkpoints and quarantine.", service.Import, mapper)
		}},
		{id: "migrationResume", name: "migration_resume", description: "Resume one exact migration batch revision without repeating completed domain checkpoints.", build: func() (mcpserver.Binding, error) {
			return mcpserver.Bind(registry, "migrationResume", "migration_resume", "migration", "Resume one exact migration batch revision without repeating completed domain checkpoints.", service.Resume, mapper)
		}},
		{id: "migrationExport", name: "migration_export", description: "Export a bounded neutral manifest page stripped of credentials and current authority claims.", build: func() (mcpserver.Binding, error) {
			return mcpserver.Bind(registry, "migrationExport", "migration_export", "migration", "Export a bounded neutral manifest page stripped of credentials and current authority claims.", service.Export, mapper)
		}},
		{id: "migrationCutover", name: "migration_cutover", description: "Activate one migrated cohort route at a fenced schedule occurrence boundary after complete evidence.", build: func() (mcpserver.Binding, error) {
			return mcpserver.Bind(registry, "migrationCutover", "migration_cutover", "migration", "Activate one migrated cohort route at a fenced schedule occurrence boundary after complete evidence.", service.Cutover, mapper)
		}},
		{id: "migrationRollback", name: "migration_rollback", description: "Restore the prior route while retaining an honest ledger of irreversible external effects.", build: func() (mcpserver.Binding, error) {
			return mcpserver.Bind(registry, "migrationRollback", "migration_rollback", "migration", "Restore the prior route while retaining an honest ledger of irreversible external effects.", service.Rollback, mapper)
		}},
		{id: "migrationErase", name: "migration_erase", description: "Erase bounded online migration journal records under current signed retention authority.", build: func() (mcpserver.Binding, error) {
			return mcpserver.Bind(registry, "migrationErase", "migration_erase", "migration", "Erase bounded online migration journal records under current signed retention authority.", service.Erase, mapper)
		}},
	}
	out := make([]mcpserver.Binding, 0, len(specs))
	for _, s := range specs {
		b, err := s.build()
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}
