package nlqexec

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
	"github.com/hurtener/chartworks/internal/semantics"
)

// learnedParameterSchema extracts kinds only. Query-owned scalar bindings never
// enter reusable examples. The question is a value-free demonstration label,
// not an executable interpretation or a source of parameter values for reuse.
func learnedParameterSchema(ctx context.Context, q QueryRecord) (string, *exampleparams.Schema, error) {
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
	if x.ParameterSchema.Validate() != nil {
		return false
	}
	if x.ParameterSchema == nil {
		return true
	}
	return x.Digest == parameterExampleDigest(x.Topic, x.Question, x.SQL, x.ParameterSchema) && x.Question != "" && x.SQL != "" && len(x.Question) <= 16384 && len(x.SQL) <= 32768 && utf8.ValidString(x.Question) && utf8.ValidString(x.SQL) && !strings.ContainsRune(x.Question+x.SQL, 0)
}

func parameterExampleDigest(topic, question, sql string, schema *exampleparams.Schema) string {
	if schema == nil {
		return exampleDigest(topic, question, sql)
	}
	return exec.Hash([]any{"parameterized-example-v1", topic, question, sql, schema})
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
	text := "question:" + x.Question + " sql:" + x.SQL
	if x.ParameterSchema == nil {
		return text
	}
	raw, err := json.Marshal(x.ParameterSchema)
	if err != nil {
		return ""
	} // bounded validated schema
	return text + " parameter_schema:" + string(raw) + "\nThis reviewed SQL demonstration has abstract positional parameters, not reusable values. Resolve the current question's values and produce the current candidate's parameter bindings. Do not copy a previous question's constants, invent validation-probe values, or treat redaction markers as values."
}

func portableExampleVersion(schema *exampleparams.Schema) int {
	if schema != nil {
		return 2
	}
	return 1
}
func portableExampleValid(row PortableExample) bool {
	if row.SchemaVersion != portableExampleVersion(row.ParameterSchema) {
		return false
	}
	return row.ParameterSchema.Validate() == nil
}
