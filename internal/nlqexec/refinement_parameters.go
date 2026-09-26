package nlqexec

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
)

// This request-local state is built only after the protected parent is loaded,
// reauthorized and its service-owned predicates are replayed. It is not accepted
// from JSON, never serialized into model input, and is not an execution proof.
type refinementParameters struct {
	parent, sql string
	revision    int64
	digest      string
	values      []exec.Parameter
}

func (*refinementParameters) String() string         { return "refinement-parameters(redacted)" }
func (p *refinementParameters) GoString() string     { return p.String() }
func (p *refinementParameters) LogValue() slog.Value { return slog.StringValue(p.String()) }

// refinementSQLBase separates model parameters from business-owned parameters.
// A reference-only change receipt does not replace a valid SQL base with "".
// The caller must verifyQueryClarificationBinding before invoking this helper.
func refinementSQLBase(old QueryRecord) (string, []exec.Parameter, error) {
	sql, values := old.SQL, old.Parameters
	if old.Clarification != nil && old.Clarification.Binding.SchemaVersion != 0 {
		evidence := old.Clarification
		if evidence.SchemaVersion != 1 || evidence.Binding.SchemaVersion != 1 || evidence.BaseSQL == "" {
			return "", nil, exec.ErrBinding
		}
		sql, values = evidence.BaseSQL, evidence.BaseParameters
	}
	if len(values) > 64 || sql == "" && len(values) > 0 {
		return "", nil, exec.ErrBinding
	}
	for _, v := range values {
		if !v.Valid() {
			return "", nil, exec.ErrBinding
		}
	}
	return sql, append([]exec.Parameter(nil), values...), nil
}

func retainRefinementParameters(ctx context.Context, question *QuestionRequest, old QueryRecord, sql string, values []exec.Parameter, edits ...ParameterEdit) (context.Context, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selected, err := replaceRefinementParameters(ctx, values, edits)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return ctx, nil
	}
	if old.ID == "" || old.Revision < 1 || sql == "" || question == nil || len(values) > 64 {
		return nil, exec.ErrBinding
	}
	for _, value := range values {
		if !value.Valid() {
			return nil, exec.ErrBinding
		}
	}
	// Typed slot replacements express only value changes, not a new free-text
	// interpretation. A different question could also request new predicates or
	// slot roles, which this bounded contract does not infer. Require fresh Plan.
	// Structured reference/metric edits and reviewed answer edits remain usable.
	if question.Question != old.Question {
		return nil, exec.ErrUnsupported
	}
	// Only kinds/positions travel to the model. Placeholder scalar output is
	// ignored and replaced server-side with retained or explicitly replaced
	// private values; neither old nor replacement values are added to the prompt.
	slots := make([]validationRepairSlot, len(values))
	for i, p := range values {
		slots[i] = validationRepairSlot{Position: i + 1, Kind: p.Kind}
	}
	raw, err := json.Marshal(slots)
	if err != nil {
		return nil, ErrGeneration
	}
	question.EditBase = replaceInstruction(question.EditBase, nlq.Instruction{Key: "retained_parameter_slots", Text: "The previous_sql positional bindings are selected privately by the service, including any explicit typed replacements. Preserve every parameter position and kind, every complete clause containing a parameter, and the full source/CTE/join/derived-table/window/set-operation input graph including aliases. When a parameter-bearing GROUP/ORDER/DISTINCT expression can refer to output aliases or ordinals, preserve the full output target list. Named-window parameters also preserve their output and ORDER consumers. Do not substitute literals, drop/add slots, reorder predicates or relocate slots. Return valid placeholder scalar values of the same kinds; the service ignores those values and applies the private selected bindings before validation. Output/group/order/limit edits outside these protected clauses and namespaces are permitted; do not rewrite inner queries. If a nested SELECT output or named window exports a parameter-dependent value, preserve all outer clauses as well: the service does not infer alias dataflow. Slot metadata: " + string(raw)})
	state := &refinementParameters{parent: old.ID, revision: old.Revision, digest: QueryLineageDigest(old), sql: sql, values: selected}
	return context.WithValue(ctx, refinementParameterKey{}, state), nil
}

type refinementParameterKey struct{}

func refinementParameterState(ctx context.Context) *refinementParameters {
	state, _ := ctx.Value(refinementParameterKey{}).(*refinementParameters)
	return state
}

func (p *refinementParameters) verifyParent(old *QueryRecord) error {
	if p == nil {
		return nil
	}
	if old == nil || p.parent != old.ID || p.revision != old.Revision || p.digest != QueryLineageDigest(*old) {
		return exec.ErrBinding
	}
	return nil
}

func restoreRefinementParameters(ctx context.Context, a admission, c generatedCandidate) (generatedCandidate, error) {
	if a.refinementParameters == nil {
		return c, nil
	}
	previous := a.refinementParameters
	restored, err := restoreValidationRepairParameters(c, previous.values)
	if err != nil {
		return generatedCandidate{}, err
	}
	if err = exec.CheckParameterContinuity(ctx, a.binding.Dialect, previous.sql, restored.SQL, len(previous.values)); err != nil {
		return generatedCandidate{}, err
	}
	return restored, nil
}
