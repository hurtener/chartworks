package nlqexec

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
)

// reviewLegacyAnalyticalContinuation establishes whether a retained analytical
// query can become the edit base of a current-policy child. Old SQL is evidence
// to check, never a new source of predicate or truncation authority.
func (s *Service) reviewLegacyAnalyticalContinuation(ctx context.Context, e identity.Envelope, old QueryRecord, a admission, constraints []exec.BusinessConstraint) error {
	if old.SQL == "" || old.AnalyticalVersion < 1 || old.AnalyticalVersion >= analyticalRecordVersion || old.Analytical == nil {
		return nil
	}
	legacy, err := expectedAnalytical(ctx, old, a, constraints)
	if err != nil {
		return err
	}
	if legacy == nil {
		return exec.ErrBinding
	}
	plan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: old.SQL, Parameters: old.Parameters}, a.relationScope)
	if err != nil {
		return err
	}
	if _, err = exec.CheckAnalyticalPlan(ctx, plan, *legacy); err != nil {
		return err
	}
	current, err := compileAnalyticalVersion(ctx, a, analyticalRecordVersion, constraints)
	if err != nil {
		return err
	}
	if current == nil {
		return exec.ErrBinding
	}
	_, err = exec.CheckAnalyticalPlan(ctx, plan, *current)
	if legacyCurrentIntentReviewNeeded(old.AnalyticalVersion, current, err) {
		question := "Please review the complete current intent in a new preflight, then submit its query ID, answer context and typed answers as intent_review. This replaces the old intent; the original query remains available for replay."
		if old.Locale == nlq.LanguageSpanish {
			question = "Revisa la intención actual completa en una nueva consulta previa; envía su identificador, contexto y respuestas tipadas en intent_review. Esto reemplaza la intención anterior; la consulta original sigue disponible para reproducirla."
		}
		return &generationDecisionError{problem: generationdecision.Problem{Version: generationdecision.Version, Outcome: generationdecision.Insufficient, Questions: []string{question}}}
	}
	return err
}

// This classification is used only after the exact retained proof succeeds.
// It requests a complete, independently reviewed current intent; it never
// approves the old SQL under the current contract or copies its predicates.
func legacyCurrentIntentReviewNeeded(version int, current *exec.AnalyticalContract, err error) bool {
	var failure *exec.AnalyticalError
	if !errors.As(err, &failure) {
		return false
	}
	switch failure.Code {
	case "analytical_query_population_mismatch", "analytical_limit_mismatch", "analytical_order_mismatch":
		return true
	case "analytical_population_mismatch":
		// V8 proves ordinary group existence before checking query-owned WHERE.
		// An unowned legacy WHERE can therefore surface at this earlier gate.
		// Keep the dedicated retained-v7 group-policy review path distinct.
		return version >= 1 && version <= 6 && current != nil && current.Version == exec.AnalyticalGroupedProgramsVersion && current.GroupedPopulations == nil && len(current.Populations) == 0 && current.Grain != nil && len(current.Grain.Columns)+len(current.Grain.Buckets) > 0
	}
	return false
}

func groupedDomainReviewGuidance(old QueryRecord) error {
	question := "Review which groups should exist in a current published grouping policy, create a new preflight, then submit its query ID and catalog selection digest as intent_review. The retained query is not approved for replay by this review."
	if old.Locale == nlq.LanguageSpanish {
		question = "Revisa qué grupos deben existir en una política publicada actual, crea una nueva consulta previa y envía su identificador y resumen de selección en intent_review. Esta revisión no aprueba la reproducción de la consulta conservada."
	}
	return &generationDecisionError{problem: generationdecision.Problem{Version: generationdecision.Version, Outcome: generationdecision.Insufficient, Questions: []string{question}}}
}
