package nlqexec

import (
	"context"
	"reflect"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func clarificationOriginError(locale nlq.Language, code string) error {
	message := "Use the query ID and answer context from the same current preflight. To change the question, use session refinement."
	if locale == nlq.LanguageSpanish {
		message = "Usá el identificador y el contexto de la misma consulta previa actual. Para cambiar la pregunta, usá el refinamiento de la sesión."
	}
	return &nlqroute.Clarification{Reason: code, Outcome: semantics.ClarificationInvalid, Errors: []semantics.ClarificationFieldError{{Field: "clarification_query", Code: code, Message: message}}}
}

// validateClarificationOrigin verifies provenance before any provider work.
// Caller-supplied query IDs are references, not permission. Stateless routing
// remains a fresh interpretation; execution submissions must address the
// actual retained pending question instead of borrowing another form's pin.
func (s *Service) validateClarificationOrigin(ctx context.Context, e identity.Envelope, question QuestionRequest, action string) error {
	if ctx == nil || !e.Valid() {
		return access.ErrUnauthenticated
	}
	if err := requireQuestionAction(e, action, question); err != nil {
		return err
	}
	if err := validateQuestion(question); err != nil {
		return err
	}
	if question.ClarificationQuery == "" && len(question.Answers) == 0 {
		return nil
	}
	if !identity.Identifier(question.ClarificationQuery) {
		return clarificationOriginError(question.Locale, "clarification_query_required")
	}
	old, err := s.repo.ReadQuery(ctx, mustScope(e), question.ClarificationQuery)
	if err != nil {
		return err
	}
	if old.Session != e.Session() {
		return ErrForeignSession
	}
	if old.Status != "preflight" || old.SQL != "" {
		return clarificationOriginError(question.Locale, "clarification_query_not_pending")
	}
	topics := append([]string(nil), question.Topics...)
	if len(topics) == 0 && question.Topic != "" {
		topics = []string{question.Topic}
	}
	request := old.Route.Request
	if question.Question != request.Question || question.Locale != request.Locale || question.Context != old.Context || !reflect.DeepEqual(topics, old.Topics) || !reflect.DeepEqual(question.References, request.References) || !reflect.DeepEqual(question.MetricIDs, request.MetricIDs) || !reflect.DeepEqual(question.Joins, request.JoinChoices) || question.AnswerContext == "" || question.AnswerContext != old.Route.AnswerContext {
		return clarificationOriginError(question.Locale, "clarification_question_mismatch")
	}
	return nil
}
