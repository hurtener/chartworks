package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
	"github.com/hurtener/chartworks/internal/semantics"
)

// OwnedExamplePolicy marks a reviewed unbound template. It carries no predicate
// values, defaults or source permission. The current route owns all rebinding.
const OwnedExamplePolicy = "current-owned-predicates-v1"

func validExampleBindingPolicy(policy string) bool {
	return policy == "" || ownedLearningPolicy(policy)
}

func originExampleDigest(topic, question, sql string, parameters *exampleparams.Schema, origin ExampleOrigin) string {
	if origin.Requalification != nil {
		return exec.Hash([]any{ExampleRequalificationPolicy, topic, question, sql, parameters, origin})
	}
	if origin.BindingPolicy == "" {
		return parameterExampleDigest(topic, question, sql, parameters)
	}
	return exec.Hash([]any{origin.BindingPolicy, topic, question, sql, parameters})
}

// reusableLearningBase never removes predicates by parsing a bound query. Only
// an exact, authenticated binder reconstruction can establish the stored base.
// A manually corrected bound statement lacks that base proof; its feedback is
// recorded without auto-learning. A reviewer can import a new unbound template.
func (s *Service) reusableLearningBase(ctx context.Context, e identity.Envelope, q QueryRecord, a admission, correction string, provedBase ...*exec.Plan) (QueryRecord, string, bool, error) {
	if ctx == nil {
		return QueryRecord{}, "", false, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return QueryRecord{}, "", false, err
	}
	if !hasActiveBusinessEvidence(q.Route) && (q.Clarification == nil || q.Clarification.Binding.SchemaVersion == 0) {
		if correction != "" {
			q.SQL = correction
		}
		return q, "", true, nil
	}
	if err := s.verifyQueryClarificationBinding(ctx, e, q, a); err != nil {
		return QueryRecord{}, "", false, err
	}
	evidence := q.Clarification
	if correction != "" || evidence == nil || !clarificationBindingSchemaValid(q) || evidence.BaseSQL == "" {
		return QueryRecord{}, "", false, nil
	}
	// Known values in an allegedly unbound base are grounds not to learn, even
	// when executable SQL has passed its different native-safety requirements.
	if len(q.Parameters) == 0 && evidence.Binding.SchemaVersion == 1 {
		return QueryRecord{}, "", false, exec.ErrBinding
	}
	private, complete, err := ownedLearningPrivateValues(ctx, q)
	if err != nil {
		return QueryRecord{}, "", false, err
	}
	if !complete {
		return QueryRecord{}, "", false, nil
	}
	if err := exec.CheckLearningParameterContent(ctx, evidence.BaseSQL, private); err != nil {
		if errors.Is(err, exec.ErrUnsupported) {
			return QueryRecord{}, "", false, nil
		}
		return QueryRecord{}, "", false, err
	}
	base := q
	base.SQL, base.Parameters = evidence.BaseSQL, append([]exec.Parameter(nil), evidence.BaseParameters...)
	// Derive a value-free label from independent reviewed metric identities. Do
	// not carry relative date words, free-text answers, old names or result prose.
	var roots []string
	if q.Route.Selection != nil {
		for _, topic := range q.Route.Selection.Topics {
			for _, root := range topic.Roots {
				if root.Reason == "required_rule" {
					continue
				}
				if root.Reference.Kind == semantics.KindMeasure || root.Reference.Kind == semantics.KindKPI {
					roots = append(roots, string(root.Reference.Kind)+":"+root.Reference.ID)
				}
			}
		}
	}
	sort.Strings(roots)
	encoded, _ := json.Marshal(roots)
	if len(roots) > 128 || len(encoded) > 8192 {
		return QueryRecord{}, "", false, exec.ErrLimit
	}
	// Only the catalog-derived suffix needs value redaction. Fixed server
	// labels cannot disclose a private value merely because it is a common
	// word such as "current"; redact that spelling in the supplied IDs.
	redactions := make([]semantics.ClarificationResolution, 0, len(private))
	for _, p := range private {
		if p.Value != "" {
			redactions = append(redactions, semantics.ClarificationResolution{Value: p.Value, Sensitivity: semantics.LiteralSensitive})
		}
	}
	suffix := semantics.RedactClarificationText(string(encoded), nil, redactions)
	base.Question = "Reviewed unbound query template for current service-owned filters. Metric identities: " + suffix
	if string(q.Locale) == "es" {
		base.Question = "Plantilla revisada sin valores anteriores, para los filtros actuales del servicio. Métricas: " + suffix
	}
	if len(base.Question) > 16384 {
		return QueryRecord{}, "", false, exec.ErrLimit
	}
	// This is a dry check, not execution. The base must stand on its own for
	// native structure, while live parameter values remain private to this call.
	plan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: base.SQL, Parameters: base.Parameters}, a.relationScope)
	if err != nil {
		return QueryRecord{}, "", false, err
	}
	if len(provedBase) == 1 && provedBase[0] != nil {
		*provedBase[0] = plan
	}
	return base, learningPolicyForBinding(evidence.Binding.SchemaVersion), true, nil
}

// Only a freshly sealed routed context can supply current owned predicates.
// A stored origin marker or imported JSON does not establish active constraints.
func ownedExampleApplicable(example ExampleRecord, a admission) bool {
	if example.Origin.BindingPolicy == "" {
		return true
	}
	if scopedLearningPolicy(example.Origin.BindingPolicy) {
		return scopedExampleApplicable(example.Origin.BindingPolicy, a)
	}
	if example.Origin.BindingPolicy != OwnedExamplePolicy || !hasActiveBusinessEvidence(a.route) {
		return false
	}
	constraints, err := a.route.ResolvedBusinessConstraints()
	return err == nil && len(constraints) > 0 && a.route.SourceBindingDigest == exec.Hash(a.binding)
}

func ownedExampleInstruction(x ExampleRecord) string {
	if !ExampleParametersValid(x) || !ownedLearningPolicy(x.Origin.BindingPolicy) {
		return ""
	}
	raw, err := json.Marshal(struct {
		Question      string                `json:"question"`
		SQL           string                `json:"sql"`
		Parameters    *exampleparams.Schema `json:"parameter_schema,omitempty"`
		BindingPolicy string                `json:"binding_policy"`
	}{x.Question, x.SQL, x.ParameterSchema, x.Origin.BindingPolicy})
	if err != nil {
		return ""
	}
	return "Reviewed unbound SQL demonstration: " + string(raw) + " This is a base template only. Do not reproduce earlier filter values or add predicates owned by the service. The service binds only this request's freshly resolved reviewed predicates after generation. Any abstract model parameters must use the current question, never a historical value or validation probe."
}

// Labels are deliberately neutral; changing a binding never changes or exposes
// a reusable example's historical scalar/time values through its question.
func neutralOwnedExampleQuestion(text string) bool {
	return strings.HasPrefix(text, "Reviewed unbound query template for current service-owned filters. Metric identities: ") || strings.HasPrefix(text, "Plantilla revisada sin valores anteriores, para los filtros actuales del servicio. Métricas: ")
}

// Apply the same known-literal boundary to the active value's aliases, not just
// its normalized SQL parameter. If all reviewed spellings cannot fit the bounded
// disclosure checker, record feedback but do not create an automatic example.
func ownedLearningPrivateValues(ctx context.Context, q QueryRecord) ([]exec.Parameter, bool, error) {
	values := make([]exec.Parameter, 0, len(q.Parameters))
	seen := map[exec.Parameter]bool{}
	add := func(p exec.Parameter) bool {
		if !p.Valid() {
			return false
		}
		if seen[p] {
			return true
		}
		if len(values) >= exampleparams.MaxSlots {
			return false
		}
		seen[p] = true
		values = append(values, p)
		return true
	}
	for _, p := range q.Parameters {
		if !add(p) {
			return nil, false, nil
		}
	}
	for _, r := range q.Route.Resolutions {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		// A proved NULL selection has no SQL position, but still needs the
		// disclosure scanner's syntax checks. Represent that actual resolved
		// value as null only for scanning; it never enters model/source slots.
		if r.Null && !add(exec.Parameter{Kind: "null"}) {
			return nil, false, nil
		}
		if r.Sensitivity != semantics.LiteralSensitive {
			continue
		}
		if r.Value != "" && !add(exec.Parameter{Kind: "text", Value: r.Value}) {
			return nil, false, nil
		}
		if r.Effect == nil {
			continue
		}
		for _, v := range r.Effect.Values {
			if strings.Join(strings.Fields(v.Canonical), " ") != r.Value {
				continue
			}
			for _, alias := range v.Aliases {
				if !add(exec.Parameter{Kind: "text", Value: alias}) {
					return nil, false, nil
				}
			}
		}
	}
	return values, true, nil
}
