package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSafeErrors(t *testing.T) {
	for _, tc := range []struct{ in, want error }{{nlqbyo.ErrBudget, nlqbyo.ErrBudget}, {nil, nil}, {context.Canceled, context.Canceled}, {context.DeadlineExceeded, context.DeadlineExceeded}, {pgx.ErrNoRows, store.ErrNotFound}, {store.ErrExpired, store.ErrExpired}, {errors.New("credential-canary"), store.ErrUnavailable}} {
		if !errors.Is(safe(tc.in), tc.want) {
			t.Errorf("wrong safe category")
		}
	}
	for _, code := range []string{"23505", "40001", "40P01", "23503", "23502", "23514", "22003", "55000", "57014"} {
		e := safe(&pgconn.PgError{Code: code, Message: "credential-canary"})
		if e == nil || e.Error() == "credential-canary" {
			t.Fatal("unsafe database error")
		}
	}
	manifest, err := Migrations()
	if err != nil || SchemaVersion() != "54" || len(manifest) != 54 {
		t.Fatal("schema version", err, SchemaVersion(), len(manifest))
	}
	reviewedSchedule := manifest[27]
	if reviewedSchedule.Version != 28 || reviewedSchedule.Name != "migrations/028_reviewed_schedule_changes.sql" || len(reviewedSchedule.Checksum) != 64 || !strings.Contains(reviewedSchedule.SQL, "schedule_change_receipt_shape") || !strings.Contains(reviewedSchedule.SQL, "change_revision") {
		t.Fatal("preserved reviewed-schedule migration identity", reviewedSchedule.Version, reviewedSchedule.Name, reviewedSchedule.Checksum)
	}
	for _, m := range []struct {
		index  int
		name   string
		marker string
	}{
		{28, "migrations/029_reports_dashboards.sql", "document_revisions"},
		{29, "migrations/030_report_composition_runs.sql", "composition_group_guard"},
		{30, "migrations/031_nested_report_leases.sql", "nested_child_shape"},
		{31, "migrations/032_timezone_database.sql", "queue_timezone_version"},
		{32, "migrations/033_reporting_occurrences.sql", "reporting_occurrence_delivery"},
		{33, "migrations/034_orphaned_child_capacity.sql", "pending_execution_roots"},
		{34, "migrations/035_reporting_output_intent.sql", "block_output_intents_check"},
		{35, "migrations/036_nlq_clarification.sql", "nlq_clarification_immutable"},
		{36, "migrations/037_clarification_comparison.sql", "baseline_clarification_bounded"},
		{37, "migrations/038_reporting_rule_snapshots.sql", "block_rule_pins"},
		{38, "migrations/039_template_selection_evidence.sql", "template_selections"},
		{39, "migrations/040_reporting_template_selections.sql", "block_template_selections_shape"},
		{40, "migrations/041_reporting_output_locale_bounds.sql", "octet_length(label->>'locale') NOT BETWEEN 2 AND 64"},
		{41, "migrations/042_learning_templates.sql", "example_selection"},
		{42, "migrations/043_reporting_display_intent.sql", "block_display_intent_check"},
		{43, "migrations/044_reporting_question_assessments.sql", "block_question_assessments"},
		{44, "migrations/045_document_deletion_catalog.sql", "document_deletion_tombstones"},
		{45, "migrations/046_render_renditions.sql", "render_renditions"},
		{46, "migrations/047_guided_onboarding.sql", "onboarding_runs"},
		{47, "migrations/048_evaluation.sql", "evaluation_runs"},
		{48, "migrations/049_nlq_parent_lineage.sql", "parent_digest"},
		{49, "migrations/050_migration_cutover.sql", "migration_batches"},
		{51, "migrations/052_nlq_reviewed_scope.sql", "nlq_relation_scope_immutable"},
		{52, "migrations/053_nlq_analytical.sql", "nlq_analytical_immutable"},
		{53, "migrations/054_nlq_analytical_grain.sql", "analytical-metrics-v2"},
	} {
		added := manifest[m.index]
		if added.Version != m.index+1 || added.Name != m.name || len(added.Checksum) != 64 || !strings.Contains(added.SQL, m.marker) {
			t.Fatal("reporting migration identity", added.Version, added.Name, added.Checksum)
		}
	}
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		if !strings.Contains(manifest[9].SQL, "'"+dialect+"'") {
			t.Fatal("warehouse dialect migration missing closed variant", dialect)
		}
	}
	if !strings.Contains(manifest[45].SQL, "pg_get_expr(conbin,conrelid)") || strings.Contains(manifest[45].SQL, "pg_get_constraintdef") {
		t.Fatal("rendition migration must splice the prior audit predicate, not nest a CHECK definition")
	}
}
