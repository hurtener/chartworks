package nlqexec

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
	"github.com/hurtener/chartworks/internal/semantics"
)

// learnedParameterSchema extracts kinds only. Query-owned scalar bindings never
// enter reusable examples. The question is a value-free demonstration label,
// not an executable interpretation or a source of parameter values for reuse.
func learnedParameterSchema(ctx context.Context, q QueryRecord, bindingPolicy ...string) (string, *exampleparams.Schema, error) {
	if ctx == nil {
		return "", nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if len(q.Parameters) == 0 {
		return q.Question, nil, nil
	}
	if len(q.Parameters) > exampleparams.MaxSlots {
		return "", nil, ErrInvalid
	}
	kinds := make([]string, len(q.Parameters))
	redactions := make([]semantics.ClarificationResolution, 0, len(q.Parameters))
	for i, p := range q.Parameters {
		if !p.Valid() || !utf8.ValidString(p.Value) {
			return "", nil, exec.ErrBinding
		}
		kinds[i] = p.Kind
		if p.Value != "" {
			redactions = append(redactions, semantics.ClarificationResolution{Value: p.Value, Sensitivity: semantics.LiteralSensitive})
		}
	}
	schema, err := exampleparams.New(kinds)
	if err != nil {
		return "", nil, ErrInvalid
	}
	question := semantics.RedactClarificationText(q.Question, nil, redactions)
	// The owned-learning producer already redacted the catalog suffix. The
	// fixed server-authored prefix cannot reveal a coincidentally equal value.
	if len(bindingPolicy) == 1 && bindingPolicy[0] == OwnedExamplePolicy && neutralOwnedExampleQuestion(q.Question) {
		question = q.Question
	}
	if strings.TrimSpace(question) == "" || len(question) > 16384 || !utf8.ValidString(question) || strings.ContainsRune(question, 0) {
		return "", nil, ErrInvalid
	}
	return question, schema, nil
}

// ExampleParametersValid protects typed evidence at storage and retrieval seams.
// Legacy nil keeps its existing validation behavior. A present schema must match
// the new digest domain. Storage immutability and portable row versions separately
// prevent dropping a new template schema and presenting it as a legacy example.
func ExampleParametersValid(x ExampleRecord) bool {
	if !validExampleBindingPolicy(x.Origin.BindingPolicy) || x.ParameterSchema.Validate() != nil {
		return false
	}
	if x.ParameterSchema == nil && x.Origin.BindingPolicy == "" {
		return true
	}
	return (x.Origin.BindingPolicy == "" || neutralOwnedExampleQuestion(x.Question)) && x.Digest == originExampleDigest(x.Topic, x.Question, x.SQL, x.ParameterSchema, x.Origin) && x.Question != "" && x.SQL != "" && len(x.Question) <= 16384 && len(x.SQL) <= 32768 && utf8.ValidString(x.Question) && utf8.ValidString(x.SQL) && !strings.ContainsRune(x.Question+x.SQL, 0)
}

func parameterExampleDigest(topic, question, sql string, schema *exampleparams.Schema) string {
	if schema == nil {
		return exampleDigest(topic, question, sql)
	}
	domain := "parameterized-example-v1"
	if schema.Version == exampleparams.DomainVersion {
		domain = "parameterized-example-v2"
	}
	return exec.Hash([]any{domain, topic, question, sql, schema})
}

// Probes are for current-source native dry validation only, never execution,
// stored bindings or the model request. Failed type-dependent casts fail review.
func exampleValidationParameters(x ExampleRecord) ([]exec.Parameter, error) {
	if !ExampleParametersValid(x) {
		return nil, exec.ErrBinding
	}
	values, err := x.ParameterSchema.ProbeValues()
	if err != nil {
		return nil, exec.ErrBinding
	}
	if x.ParameterSchema == nil {
		return nil, nil
	}
	out := make([]exec.Parameter, len(values))
	for i, value := range values {
		out[i] = exec.Parameter{Kind: x.ParameterSchema.Slots[i].Kind, Value: value}
		if !out[i].Valid() {
			return nil, exec.ErrBinding
		}
	}
	return out, nil
}

func learnedExampleText(x ExampleRecord) string {
	if x.Origin.BindingPolicy != "" {
		return ownedExampleInstruction(x)
	}
	text := "question:" + x.Question + " sql:" + x.SQL
	if x.ParameterSchema == nil {
		return text
	}
	// The context owner accepts a single-line instruction. Encode the full
	// typed demonstration as data so multiline SQL and literal whitespace are
	// preserved exactly, not normalized or treated as instruction boundaries.
	raw, err := json.Marshal(struct {
		Question        string                `json:"question"`
		SQL             string                `json:"sql"`
		ParameterSchema *exampleparams.Schema `json:"parameter_schema"`
	}{x.Question, x.SQL, x.ParameterSchema})
	if err != nil {
		return ""
	} // bounded validated schema
	return "Reviewed parameterized SQL demonstration: " + string(raw) + " This reviewed SQL demonstration has abstract positional parameters, not reusable values. Resolve the current question's values and produce the current candidate's parameter bindings. Do not copy a previous question's constants, invent validation-probe values, or treat redaction markers as values."
}

func portableExampleVersion(schema *exampleparams.Schema, policy ...string) int {
	if schema != nil && schema.Version == exampleparams.DomainVersion {
		return 4
	}
	if len(policy) == 1 && policy[0] == OwnedExamplePolicy {
		return 3
	}
	if schema != nil {
		return 2
	}
	return 1
}
func portableExampleValid(row PortableExample) bool {
	if row.SchemaVersion != portableExampleVersion(row.ParameterSchema, row.Origin.BindingPolicy) {
		return false
	}
	return validExampleBindingPolicy(row.Origin.BindingPolicy) && row.ParameterSchema.Validate() == nil
}

// Keep legacy records unchanged. Domain metadata is added only by a live sealed
// native plan; test/legacy validators returning zero plans can produce v1 only.
func learnedParameterSchemaFromPlan(ctx context.Context, e identity.Envelope, q QueryRecord, plan exec.Plan, binding exec.Binding, policy string) (string, *exampleparams.Schema, error) {
	question, schema, err := learnedParameterSchema(ctx, q, policy)
	if err != nil || schema == nil || !plan.Receipt().Validated {
		return question, schema, err
	}
	// Preserve non-text schemas. No specialized domain
	// is asserted and their original public-probe review remains mandatory.
	hasText := false
	for _, slot := range schema.Slots {
		hasText = hasText || slot.Kind == "text"
	}
	if !hasText {
		return question, schema, nil
	}
	domains, err := plan.LearningParameterDomains(ctx, e, binding)
	if err != nil {
		return "", nil, err
	}
	schema, err = schema.WithDomains(domains)
	if err != nil {
		return "", nil, exec.ErrBinding
	}
	return question, schema, nil
}

func verifyExampleParameterDomains(ctx context.Context, e identity.Envelope, schema *exampleparams.Schema, plan exec.Plan, binding exec.Binding) error {
	if schema == nil || schema.Version == exampleparams.Version {
		return nil
	}
	if schema.Validate() != nil {
		return exec.ErrBinding
	}
	domains, err := plan.LearningParameterDomains(ctx, e, binding)
	if err != nil {
		return err
	}
	if len(domains) != len(schema.Slots) {
		return exec.ErrBinding
	}
	for i, domain := range domains {
		if domain != schema.Slots[i].Domain {
			return exec.ErrBinding
		}
	}
	return nil
}
