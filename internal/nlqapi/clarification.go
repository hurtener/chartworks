package nlqapi

import (
	"errors"

	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

// clarificationProblem exposes only the reviewed, bounded repair projection.
// Error messages and submitted scalar values never become transport diagnostics.
func clarificationProblem(err error) *semantics.ClarificationProblem {
	var failure *nlqroute.Clarification
	if !errors.As(err, &failure) {
		return nil
	}
	return semantics.PublicClarificationProblem(semantics.ClarificationProblem{
		Outcome: failure.Outcome, Reason: failure.Reason,
		Questions: failure.Questions, Fields: failure.Errors,
	})
}
