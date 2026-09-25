package nlqexec

import (
	"errors"
	"log/slog"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq/generationdecision"
	"github.com/hurtener/chartworks/internal/semantics"
)

var (
	// ErrGenerationClarification signals a non-executable response requiring user
	// choices, not a malformed SQL candidate to feed into an automatic repair.
	ErrGenerationClarification = errors.New("nlqexec: generation requires clarification")
	// ErrGenerationContext signals missing reviewed evidence, not permission to
	// invent columns, relationships, metric meaning or source authority.
	ErrGenerationContext = errors.New("nlqexec: generation context is insufficient")
)

// generationDecisionError keeps model-authored questions out of ordinary error
// formatting/logging. Only GenerationProblem exposes the already-redacted view.
type generationDecisionError struct{ problem generationdecision.Problem }

func (e *generationDecisionError) Error() string {
	return generationdecision.ErrorCode(e.problem.Outcome)
}
func (e *generationDecisionError) GoString() string     { return e.Error() }
func (e *generationDecisionError) LogValue() slog.Value { return slog.StringValue(e.Error()) }
func (e *generationDecisionError) Unwrap() error {
	if e.problem.Outcome == generationdecision.Clarify {
		return ErrGenerationClarification
	}
	return ErrGenerationContext
}

// GenerationProblem returns detached non-executable metadata. It cannot be
// constructed by wrapping arbitrary native/provider error text.
func GenerationProblem(err error) *generationdecision.Problem {
	var e *generationDecisionError
	if !errors.As(err, &e) {
		return nil
	}
	return generationdecision.Public(e.problem)
}

const generationDecisionInstruction = " Decide readiness before proposing SQL. The required decision is ready, clarify, or insufficient_context. Use clarify when unresolved user choices could materially change the metric, rows, grain, time window or requested result; ask bounded questions instead of guessing. Use insufficient_context when reviewed evidence cannot establish the answer; do not invent schema, relationships or authority. For either blocked decision, sql must be the empty string, parameters/assumptions/ambiguities must be empty arrays, and questions must contain 1-8 short questions in the question's language without private values or SQL. For ready, questions must be empty and sql must contain one candidate. The ambiguities array under ready is only nonblocking descriptive caveats; material uncertainty must use clarify or insufficient_context. A previous SQL candidate or correction request never requires you to fabricate an executable answer."

func candidateDecision(a admission, c generatedCandidate) error {
	if generationdecision.Check(c.Decision, c.Questions, c.SQL != "", len(c.Parameters), len(c.Assumptions), len(c.Ambiguities)) != nil {
		return ErrGeneration
	}
	if c.Decision == generationdecision.Ready {
		return nil
	}
	// The same owner that projects accepted explanations performs redaction of
	// blocked questions. Old/refinement/private binding values are never repeated
	// to the model or exposed through the error projection.
	q := QueryRecord{Route: a.route, Parameters: append([]exec.Parameter(nil), a.decisionParameters...)}
	if a.refinementParameters != nil {
		q.Parameters = append(q.Parameters, a.refinementParameters.values...)
	}
	notes := generatedCandidate{Assumptions: append([]string(nil), c.Questions...)}
	retainGenerationExplanations(&q, notes, semantics.CloneClarificationAnswers(a.decisionAnswers), a.decisionParent)
	for i, text := range q.Assumptions {
		if len(text) > generationdecision.MaxQuestionBytes {
			q.Assumptions[i] = "[question withheld: redaction exceeds limit]"
		}
	}
	problem := generationdecision.Problem{Version: generationdecision.Version, Outcome: c.Decision, Questions: q.Assumptions}
	if generationdecision.Public(problem) == nil {
		return ErrGeneration
	}
	return &generationDecisionError{problem: problem}
}
