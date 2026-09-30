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
	var failure *exec.AnalyticalError
	if errors.As(err, &failure) && (failure.Code == "analytical_query_population_mismatch" || failure.Code == "analytical_limit_mismatch" || failure.Code == "analytical_order_mismatch") {
		question := "Please review the retained query's filters and result limits as governed intent before refining it. The original query remains available for replay."
		if old.Locale == nlq.LanguageSpanish {
			question = "Revisa los filtros y límites de resultados de la consulta conservada como intención gobernada antes de modificarla. La consulta original sigue disponible para reproducirla."
		}
		return &generationDecisionError{problem: generationdecision.Problem{Version: generationdecision.Version, Outcome: generationdecision.Insufficient, Questions: []string{question}}}
	}
	return err
}
