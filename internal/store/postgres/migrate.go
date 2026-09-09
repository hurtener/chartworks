package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migration is a copy of compiled, forward-only migration data for operational inspection.
type Migration struct {
	Version             int
	Name, SQL, Checksum string
}

// Migrations returns the exact build's ordered schema manifest.
func Migrations() ([]Migration, error) {
	names, e := fs.Glob(migrationFiles, "migrations/*.sql")
	if e != nil {
		return nil, store.ErrMigration
	}
	sort.Strings(names)
	out := make([]Migration, 0, len(names))
	for i, name := range names {
		b, err := migrationFiles.ReadFile(name)
		if err != nil {
			return nil, store.ErrMigration
		}
		sum := sha256.Sum256(b)
		out = append(out, Migration{Version: i + 1, Name: name, SQL: string(b), Checksum: hex.EncodeToString(sum[:])})
	}
	return out, nil
}
func history(ctx context.Context, tx pgx.Tx, manifest []Migration) (int, error) {
	rows, e := tx.Query(ctx, `SELECT version,name,checksum FROM chartworks.schema_migrations ORDER BY version`)
	if e != nil {
		return 0, store.ErrMigration
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var v int
		var name, sum string
		if e = rows.Scan(&v, &name, &sum); e != nil {
			return 0, store.ErrMigration
		}
		if n >= len(manifest) || v != manifest[n].Version || name != manifest[n].Name || sum != manifest[n].Checksum {
			return 0, store.ErrMigration
		}
		n++
	}
	if rows.Err() != nil {
		return 0, store.ErrMigration
	}
	return n, nil
}
func (d *DB) migrate(ctx context.Context) error {
	manifest, e := Migrations()
	if e != nil {
		return e
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Transaction-scoped lock: rollback/cancellation cannot strand a pooled session lock.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7214060201)`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS chartworks; CREATE TABLE IF NOT EXISTS chartworks.schema_migrations(version integer PRIMARY KEY CHECK(version>0),name text NOT NULL,checksum text NOT NULL CHECK(length(checksum)=64),applied_at timestamptz NOT NULL DEFAULT clock_timestamp())`); err != nil {
			return err
		}
		n, err := history(ctx, tx, manifest)
		if err != nil {
			return err
		}
		for _, m := range manifest[n:] {
			if _, err = tx.Exec(ctx, m.SQL); err != nil {
				return store.ErrMigration
			}
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, m.Version, m.Name, m.Checksum); err != nil {
				return store.ErrMigration
			}
		}
		return requiredRelations(ctx, tx)
	})
}

// Check probes connectivity and exact migration history, without changing the database.
func (d *DB) Check(ctx context.Context) error {
	manifest, e := Migrations()
	if e != nil {
		return e
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		n, err := history(ctx, tx, manifest)
		if err != nil {
			return err
		}
		if n != len(manifest) {
			return store.ErrMigration
		}
		return requiredRelations(ctx, tx)
	})
}

// SchemaVersion identifies this build's expected schema without querying a customer warehouse.
func SchemaVersion() string {
	m, e := Migrations()
	if e != nil {
		return "unavailable"
	}
	return fmt.Sprint(len(m))
}

// Required relation presence complements history checks, without claiming a superuser-tamper sandbox.
func requiredRelations(ctx context.Context, tx pgx.Tx) error {
	relations := []string{
		"audit_events", "canonical_entity_heads", "canonical_entity_revisions", "canonical_entity_terms",
		"job_occurrences", "job_schedules", "operation_attempts", "operations", "pipeline_heads", "pipeline_outputs",
		"pipeline_runs", "pipeline_stages", "pipeline_versions", "policies", "policy_revisions", "profile_dependencies",
		"profile_heads", "profile_health_events", "profile_versions", "queue_limits", "read_attempts", "schema_migrations",
		"source_revisions", "sources", "topic_draft_dependencies", "topic_draft_heads", "topic_draft_versions",
		"topic_publication_events", "topic_publication_heads", "topic_published_canonical_refs", "topic_published_dependencies",
		"topic_generation_checkpoints", "topic_health", "topic_published_generations", "topic_published_versions", "topic_reviews", "topic_rule_draft_heads",
		"topic_rule_draft_versions", "topic_rule_publication_events", "topic_rule_publication_heads",
		"topic_rule_published_versions", "topic_rule_reviews", "topic_rule_comparison_evidence", "topic_rule_evidence_invalidations",
		"uploads", "vector_facets", "vector_generations", "vector_heads",
		"nlq_examples", "nlq_feedback", "nlq_queries", "nlq_sessions",
		"byo_context_bundles", "block_heads", "block_revisions", "block_revision_references", "block_topic_pins", "block_source_pins", "block_validations", "block_publications", "block_attestations", "block_withdrawals", "block_events", "block_health", "byo_steps",
	}
	var count int
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='chartworks' AND c.relkind='r' AND c.relname=ANY($1::text[])`, relations).Scan(&count); e != nil {
		return e
	}
	if count != len(relations) {
		return store.ErrMigration
	}
	return nil
}
