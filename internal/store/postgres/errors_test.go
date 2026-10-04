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
	if err != nil || SchemaVersion() != "84" || len(manifest) != 84 {
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
		{54, "migrations/055_nlq_analytical_calendar.sql", "analytical-metrics-v3"},
		{55, "migrations/056_nlq_query_population.sql", "owned-query-predicates-v1"},
		{56, "migrations/057_nlq_example_parameters.sql", "nlq_example_parameters_immutable"},
		{57, "migrations/058_read_query_error.sql", "'query_error'"},
		{58, "migrations/059_nlq_grouping_continuity.sql", "analytical-metrics-v5"},
		{59, "migrations/060_read_query_diagnostics.sql", "read_query_diagnostic_outcome"},
		{60, "migrations/061_nlq_generation_pending.sql", "nlq_generation_pending_immutable"},
		{61, "migrations/062_nlq_analytical_intent.sql", "analytical-metrics-v6"},
		{62, "migrations/063_nlq_grouped_populations.sql", "analytical-metrics-v7"},
		{63, "migrations/064_nlq_example_parameter_domains.sql", "example-parameters-v2"},
		{64, "migrations/065_nlq_grouped_programs.sql", "analytical-metrics-v8"},
		{65, "migrations/066_nlq_intent_review.sql", "nlq_intent_review_immutable"},
		{66, "migrations/067_topic_authoring_quality.sql", "topic_authoring_quality_complete"},
		{67, "migrations/068_generation_example_retrieval.sql", "nlq_examples_generation_origin"},
		{68, "migrations/069_topic_feedback_proposals.sql", "topic_feedback_proposals"},
		{69, "migrations/070_topic_authoring_vocabulary.sql", "vocabulary"},
		{70, "migrations/071_example_requalification.sql", "nlq_example_requalification_shape"},
		{71, "migrations/072_nlq_scoped_populations.sql", "analytical-metrics-v9"},
		{72, "migrations/073_nlq_plan_submission.sql", "nlq_plan_submission_immutable"},
		{73, "migrations/074_nlq_query_retention.sql", "nlq_query_origins"},
		{74, "migrations/075_png_renditions.sql", "render_renditions_format_check"},
		{75, "migrations/076_saved_query_derivation.sql", "nlq_saved_derivation_shape"},
		{76, "migrations/077_nlq_grouped_owned_populations.sql", "analytical-metrics-v10"},
		{77, "migrations/078_nlq_group_selection.sql", "analytical-metrics-v11"},
		{78, "migrations/079_frozen_reuse_owners.sql", "frozen_reuse_custody"},
		{79, "migrations/080_nlq_grouped_fact_predicates.sql", "independent_filtered_grouped_populations"},
		{80, "migrations/081_nlq_scalar_predicate_entailment.sql", "nlq_scalar_entailment_binding_shape"},
		{81, "migrations/082_protected_clarification_origin.sql", "protected_saved_route_equal"},
		{82, "migrations/083_private_document_block_refs.sql", "document_private_publication_guard"},
		{83, "migrations/084_authoring_preparations.sql", "immutable_authoring_preparation"},
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
