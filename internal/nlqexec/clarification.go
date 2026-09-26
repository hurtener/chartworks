package nlqexec

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

// ClarificationChange is value-free same-session replacement evidence. Old
// resolutions remain historical evidence on the parent, never active filters on
// the child. This is business provenance, not a grant or execution proof.
type ClarificationChange struct {
	Topic    string `json:"topic"`
	Pattern  string `json:"pattern"`
	Slot     string `json:"slot"`
	Action   string `json:"action"`
	Previous string `json:"previous,omitempty"`
	Current  string `json:"current,omitempty"`
}

// ClarificationEvidence is protected query storage. Keeping the unbound base
// separate makes a replacement/removal rebuild its predicates rather than reuse
// old answer filters. Public consumers receive only Binding and Changes.
type ClarificationEvidence struct {
	SchemaVersion  int                         `json:"schema_version"`
	BaseSQL        string                      `json:"base_sql"`
	BaseParameters []exec.Parameter            `json:"base_parameters"`
	Binding        exec.BusinessBindingReceipt `json:"binding"`
	Changes        []ClarificationChange       `json:"changes,omitempty"`
}

// LogValue omits protected clarification evidence from ordinary structured logs.
func (ClarificationEvidence) LogValue() slog.Value {
	return slog.StringValue("clarification-evidence(redacted)")
}
func (ClarificationEvidence) String() string { return "clarification-evidence(redacted)" }

// GoString preserves evidence redaction for Go-syntax formatting.
func (v ClarificationEvidence) GoString() string { return v.String() }

// Only the current router can reevaluate persisted business evidence. A public
// JSON RouteResult cannot manufacture the in-process route/context seal.
type clarificationReplayer interface {
	ReplayClarifications(context.Context, identity.Envelope, nlqroute.RouteResult) ([]exec.BusinessConstraint, string, error)
}

func hasActiveBusinessEvidence(route nlqroute.RouteResult) bool {
	return len(route.Resolutions) != 0 || route.Interpretation != nil && len(route.Interpretation.Values)+len(route.Interpretation.Temporal) != 0
}

func bindClarificationCandidate(ctx context.Context, a admission, candidate generatedCandidate) (generatedCandidate, error) {
	if !hasActiveBusinessEvidence(a.route) {
		return candidate, nil
	}
	constraints, err := a.route.ResolvedBusinessConstraints()
	if err != nil {
		return generatedCandidate{}, err
	}
	if len(constraints) == 0 && referenceOnlyBindingMatches(a.route, a.binding) {
		return candidate, nil
	}
	if len(constraints) == 0 || exec.Hash(a.binding) != a.route.SourceBindingDigest {
		return generatedCandidate{}, exec.ErrBinding
	}
	bound, err := exec.BindBusinessConstraints(ctx, a.binding, candidate.SQL, candidate.Parameters, constraints)
	if err != nil {
		return generatedCandidate{}, err
	}
	candidate.clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: candidate.SQL, BaseParameters: append([]exec.Parameter(nil), candidate.Parameters...), Binding: bound.Receipt}
	candidate.SQL, candidate.Parameters = bound.SQL, bound.Parameters
	return candidate, nil
}

func sealClarificationCandidate(candidate *generatedCandidate, plan exec.Plan, previous, current []semantics.ClarificationResolution) error {
	if candidate.clarification == nil {
		if len(previous) != 0 {
			candidate.clarification = &ClarificationEvidence{SchemaVersion: 1, Changes: clarificationChanges(previous, current)}
		}
		return nil
	}
	receipt := plan.Receipt()
	if !receipt.Validated {
		return exec.ErrBinding
	}
	candidate.clarification.Binding.Validation = &receipt
	candidate.clarification.Changes = clarificationChanges(previous, current)
	return nil
}

func (s *Service) replayQueryClarifications(ctx context.Context, e identity.Envelope, record QueryRecord) ([]exec.BusinessConstraint, error) {
	if record.Route.AnswerContext == "" && len(record.Route.Resolutions) == 0 && len(record.Route.Request.Answers) == 0 && (record.Route.Interpretation == nil || len(record.Route.Interpretation.Values)+len(record.Route.Interpretation.Temporal) == 0) {
		return nil, nil
	}
	replayer, ok := s.router.(clarificationReplayer)
	if !ok {
		return nil, exec.ErrBinding
	}
	constraints, binding, err := replayer.ReplayClarifications(ctx, e, record.Route)
	if err != nil {
		return nil, err
	}
	if len(constraints) != 0 && binding != record.Route.SourceBindingDigest {
		return nil, exec.ErrBinding
	}
	return constraints, nil
}

// verifyQueryClarificationBinding deterministically reconstructs the candidate
// before a new execution. It does not trust stored SQL, a digest, or a previous
// answer as validator-issued proof; Run still performs normal fresh validation.
func (s *Service) verifyQueryClarificationBinding(ctx context.Context, e identity.Envelope, record QueryRecord, a admission) error {
	if !hasActiveBusinessEvidence(record.Route) {
		if record.Clarification != nil {
			evidence := record.Clarification
			if evidence.SchemaVersion != 1 || evidence.BaseSQL != "" || len(evidence.BaseParameters) != 0 || evidence.Binding.SchemaVersion != 0 {
				return exec.ErrBinding
			}
			for _, change := range evidence.Changes {
				if change.Action != "removed" || change.Current != "" {
					return exec.ErrBinding
				}
			}
		}
		return nil
	}
	constraints, err := s.replayQueryClarifications(ctx, e, record)
	if err != nil {
		return err
	}
	if len(constraints) == 0 {
		return validateReferenceOnlyEvidence(record, a.binding)
	}
	evidence := record.Clarification
	if evidence == nil || evidence.SchemaVersion != 1 || evidence.BaseSQL == "" || evidence.Binding.SchemaVersion != 1 || evidence.Binding.Validation == nil || !evidence.Binding.Validation.Validated || evidence.Binding.Validation.Source != a.binding.Source || evidence.Binding.Validation.Context != a.binding.Context || evidence.Binding.Validation.Dialect != a.binding.Dialect || evidence.Binding.Validation.Contract != a.binding.Contract || evidence.Binding.SourceBinding != exec.Hash(a.binding) {
		return exec.ErrBinding
	}
	bound, err := exec.BindBusinessConstraints(ctx, a.binding, evidence.BaseSQL, evidence.BaseParameters, constraints)
	if err != nil {
		return err
	}
	expected := evidence.Binding
	expected.Validation = nil
	if bound.SQL != record.SQL || !parametersEqual(bound.Parameters, record.Parameters) || exec.Hash(bound.Receipt) != exec.Hash(expected) {
		return exec.ErrBinding
	}
	return nil
}

func parametersEqual(a, b []exec.Parameter) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func publicClarificationBinding(evidence *ClarificationEvidence) *exec.BusinessBindingReceipt {
	if evidence == nil || evidence.Binding.SchemaVersion == 0 {
		return nil
	}
	raw, err := json.Marshal(evidence.Binding)
	if err != nil {
		return nil
	}
	var out exec.BusinessBindingReceipt
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return &out
}

func clarificationChanges(previous, current []semantics.ClarificationResolution) []ClarificationChange {
	type key struct{ topic, pattern, slot string }
	old := map[key]semantics.ClarificationResolution{}
	for _, r := range previous {
		old[key{r.Topic, r.Pattern, r.Slot}] = r
	}
	var out []ClarificationChange
	for _, r := range current {
		k := key{r.Topic, r.Pattern, r.Slot}
		change := ClarificationChange{Topic: r.Topic, Pattern: r.Pattern, Slot: r.Slot, Action: "resolved", Current: r.ID}
		if prior, found := old[k]; found {
			change.Previous = prior.ID
			change.Action = "superseded"
			delete(old, k)
		}
		out = append(out, change)
	}
	for _, r := range old {
		out = append(out, ClarificationChange{Topic: r.Topic, Pattern: r.Pattern, Slot: r.Slot, Action: "removed", Previous: r.ID})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Topic != b.Topic {
			return a.Topic < b.Topic
		}
		if a.Pattern != b.Pattern {
			return a.Pattern < b.Pattern
		}
		return a.Slot < b.Slot
	})
	return out
}

func mergeRefinementClarifications(old QueryRecord, delta QuestionRequest, out *QuestionRequest) error {
	if delta.AnswerContext != "" && delta.AnswerContext != old.Route.AnswerContext {
		return &nlqroute.Clarification{Outcome: semantics.ClarificationInvalid, Reason: "stale_answer", Errors: []semantics.ClarificationFieldError{{Field: "answer_context", Code: "stale_answer", Message: clarificationResumeMessage(string(out.Locale))}}}
	}
	answers := semantics.CloneClarificationAnswers(delta.Answers)
	choices := make([]nlqroute.ChoiceSelection, 0, len(delta.Choices))
	// Preserve the legacy reference-choice surface while normalizing corrections
	// to the exact publication already retained by this same session.
	for _, choice := range delta.Choices {
		index := -1
		for i, prior := range old.Route.Request.Answers {
			if prior.Slot == choice.Slot && (choice.Pattern == "" || prior.Pattern == choice.Pattern) {
				if index != -1 {
					return ErrInvalid
				}
				index = i
			}
		}
		if index < 0 {
			choices = append(choices, choice)
			continue
		}
		prior := old.Route.Request.Answers[index]
		if prior.Value == nil || prior.Value.OptionID == "" {
			return ErrInvalid
		}
		prior.Value = &semantics.ClarificationValue{OptionID: choice.Value}
		answers = append(answers, prior)
	}
	merged, err := semantics.MergeClarificationAnswers(old.Route.Request.Answers, answers)
	if err != nil {
		return err
	}
	out.Answers, out.Choices = merged, choices
	out.AnswerContext = ""
	// Refine has already reauthorized and replayed the parent. Only carried
	// answers/choices need the old form's pin. An inferred-only continuation
	// can remove its last predicate, changing the optional binding component
	// of the freshly computed context without changing source authority.
	// Explicit caller pins are still checked above, and answered forms retain
	// their exact original pin so policy/source changes cannot be bypassed.
	if len(merged) > 0 || len(choices) > 0 {
		out.AnswerContext = old.Route.AnswerContext
	}
	return nil
}

func clarificationResumeMessage(locale string) string {
	if locale == "es" {
		return "La publicación o el contexto cambió. Volvé a evaluar la pregunta antes de confirmar respuestas."
	}
	return "The publication or context changed. Reevaluate the question before confirming answers."
}

// Instructions are classified with the same reviewed sensitivity as answers
// before they enter precedence resolution, protected prompt storage or providers.
func redactClarificationInstructions(in QuestionRequest, route nlqroute.RouteResult) QuestionRequest {
	redact := func(items []nlq.Instruction) []nlq.Instruction {
		out := append([]nlq.Instruction(nil), items...)
		for i := range out {
			out[i].Text = semantics.RedactClarificationText(out[i].Text, in.Answers, route.Resolutions)
		}
		return out
	}
	in.Question = route.Request.Question
	in.EditBase = redact(in.EditBase)
	in.Hints = redact(in.Hints)
	in.ExampleInput = redact(in.ExampleInput)
	in.Default = redact(in.Default)
	return in
}

func publicClarificationChanges(evidence *ClarificationEvidence) []ClarificationChange {
	if evidence == nil {
		return nil
	}
	return append([]ClarificationChange(nil), evidence.Changes...)
}
