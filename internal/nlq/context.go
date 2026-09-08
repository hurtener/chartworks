// Package nlq owns deterministic query-context assembly and generation inputs.
// It does not retrieve data, evaluate rules, call a model, or execute SQL.
package nlq

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/tiktoken-go/tokenizer"
)

const (
	LowBudget         = 1500
	MediumBudget      = 3000
	HighBudget        = 6500
	MaxExamples       = 7
	MaxOmissions      = 7
	MaxConstraints    = 256
	TokenizerEncoding = "cl100k_base"
)

var (
	ErrInvalid            = errors.New("nlq: invalid context")
	ErrInsufficient       = errors.New("nlq: insufficient context budget")
	ErrConstraintConflict = errors.New("nlq: mandatory constraints are not satisfied")
)

type ValidationCode string

const (
	CodeInvalidValue ValidationCode = "invalid_value"
	CodeLimit        ValidationCode = "limit_exceeded"
	CodeDuplicateID  ValidationCode = "duplicate_id"
	CodeDuplicateKey ValidationCode = "duplicate_key"
	CodeUnsupported  ValidationCode = "unsupported"
)

type ValidationError struct {
	Code ValidationCode
	Path string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("nlq: %s at %s", e.Code, e.Path) }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

type BudgetError struct {
	Tier           Tier
	Budget         int
	RequiredTokens int
}

func (e *BudgetError) Error() string {
	return "nlq: mandatory context exceeds selected token budget"
}
func (e *BudgetError) Unwrap() error { return ErrInsufficient }

type Tier string

const (
	TierLow    Tier = "low"
	TierMedium Tier = "medium"
	TierHigh   Tier = "high"
)

func (t Tier) Budget() int {
	switch t {
	case TierLow:
		return LowBudget
	case TierMedium:
		return MediumBudget
	case TierHigh:
		return HighBudget
	default:
		return 0
	}
}

func (t Tier) valid() bool { return t.Budget() > 0 }

func TierForConfidence(confidence float64) (Tier, error) {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		return "", &ValidationError{Code: CodeInvalidValue, Path: "confidence"}
	}
	if confidence < 0.70 {
		return TierLow, nil
	}
	if confidence < 0.85 {
		return TierMedium, nil
	}
	return TierHigh, nil
}

type Language string

const (
	LanguageEnglish Language = "en"
	LanguageSpanish Language = "es"
)

func (l Language) valid() bool { return l == LanguageEnglish || l == LanguageSpanish }

type Strategy string

const (
	StrategySingleTopic Strategy = "single_topic"
	StrategyMultiTopic  Strategy = "multi_topic"
	StrategyClarify     Strategy = "clarify"
	StrategyNoRoute     Strategy = "no_route"
)

func (s Strategy) valid() bool {
	switch s {
	case StrategySingleTopic, StrategyMultiTopic, StrategyClarify, StrategyNoRoute:
		return true
	default:
		return false
	}
}

// TokenCounter is the one budget currency used by every context lane.
// Implementations must count model tokens; character estimates are not valid.
type TokenCounter interface {
	Count(string) (int, error)
}

// TiktokenCounter is a deterministic, CGo-free cl100k_base counter. The
// vocabulary is embedded by the pinned dependency and is never downloaded at
// runtime.
type TiktokenCounter struct {
	codec tokenizer.Codec
}

func NewTiktokenCounter() (*TiktokenCounter, error) {
	codec, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return nil, ErrInvalid
	}
	return &TiktokenCounter{codec: codec}, nil
}

func (c *TiktokenCounter) Count(text string) (int, error) {
	if c == nil || c.codec == nil || !utf8.ValidString(text) {
		return 0, ErrInvalid
	}
	n, err := c.codec.Count(text)
	if err != nil || n < 0 {
		return 0, ErrInvalid
	}
	return n, nil
}

// MandatoryConstraint is a detached, already-evaluated rule result. The
// phase-16 service owns subject/rule loading and hard-constraint evaluation;
// this package only carries the resulting mandatory text into generation.
type MandatoryConstraint struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// ConstraintState is an adapter boundary for phase 16's ConstraintEvaluation.
// A false Allowed value is a typed terminal result; ContextAssembler never
// evaluates references or loads rule payloads itself.
type ConstraintState struct {
	Allowed  bool                  `json:"allowed"`
	Required []MandatoryConstraint `json:"required"`
	Excluded []MandatoryConstraint `json:"excluded"`
}

type PinnedMetric struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Evidence struct {
	ID         string   `json:"id"`
	Text       string   `json:"text"`
	Priority   int      `json:"priority"`
	Source     string   `json:"source,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type OptionalItem struct {
	ID         string   `json:"id"`
	Text       string   `json:"text"`
	Priority   int      `json:"priority"`
	Source     string   `json:"source,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type ContextInput struct {
	Locale       Language         `json:"locale"`
	Strategy     Strategy         `json:"strategy"`
	Topic        string           `json:"topic,omitempty"`
	TopicVersion string           `json:"topic_version,omitempty"`
	Question     string           `json:"question"`
	Evidence     []Evidence       `json:"evidence"`
	Constraints  *ConstraintState `json:"constraints,omitempty"`
	Metrics      []PinnedMetric   `json:"metrics"`
	Advisory     []OptionalItem   `json:"advisory"`
	Examples     []OptionalItem   `json:"examples"`
}

type Lane string

const (
	LaneHeader      Lane = "header"
	LaneEvidence    Lane = "evidence"
	LaneConstraints Lane = "constraints"
	LaneMetrics     Lane = "metrics"
	LaneAdvisory    Lane = "advisory"
	LaneExamples    Lane = "examples"
)

type LaneUsage struct {
	Lane           Lane `json:"lane"`
	OriginalTokens int  `json:"original_tokens"`
	IncludedTokens int  `json:"included_tokens"`
}

type Omission struct {
	Lane   Lane   `json:"lane"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type AssembledContext struct {
	Tier         Tier             `json:"tier"`
	Budget       int              `json:"budget"`
	Tokens       int              `json:"tokens"`
	Locale       Language         `json:"locale"`
	Strategy     Strategy         `json:"strategy"`
	Topic        string           `json:"topic,omitempty"`
	TopicVersion string           `json:"topic_version,omitempty"`
	Question     string           `json:"question"`
	Prompt       string           `json:"prompt"`
	Evidence     []Evidence       `json:"evidence"`
	Constraints  *ConstraintState `json:"constraints,omitempty"`
	Metrics      []PinnedMetric   `json:"metrics"`
	Advisory     []OptionalItem   `json:"advisory"`
	Examples     []OptionalItem   `json:"examples"`
	// Audit is retained for bounded service metadata and deliberately stays out
	// of the model input wire. Callers that need audit evidence can inspect it
	// before handing the assembled context to a model adapter.
	Audit AssemblyAudit `json:"-"`
	seal  [32]byte
}

// AssemblyAudit is metadata about pruning. It deliberately carries no text
// from omitted inputs, so it can be returned alongside the model context.
type AssemblyAudit struct {
	Omitted      []Omission  `json:"omitted"`
	OmittedCount int         `json:"omitted_count"`
	Usage        []LaneUsage `json:"usage"`
}

type ContextAssembler struct {
	counter TokenCounter
}

func NewContextAssembler(counter TokenCounter) (*ContextAssembler, error) {
	if counter == nil {
		return nil, ErrInvalid
	}
	return &ContextAssembler{counter: counter}, nil
}

func NewDefaultContextAssembler() (*ContextAssembler, error) {
	counter, err := NewTiktokenCounter()
	if err != nil {
		return nil, err
	}
	return NewContextAssembler(counter)
}

func (a *ContextAssembler) Assemble(ctx context.Context, input ContextInput, tier Tier) (AssembledContext, error) {
	if ctx == nil {
		return AssembledContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context"}
	}
	if err := ctx.Err(); err != nil {
		return AssembledContext{}, err
	}
	if a == nil || a.counter == nil {
		return AssembledContext{}, ErrInvalid
	}
	if !tier.valid() {
		return AssembledContext{}, &ValidationError{Code: CodeUnsupported, Path: "tier"}
	}
	copyInput, err := cloneAndValidateInput(input)
	if err != nil {
		return AssembledContext{}, err
	}
	if copyInput.Constraints != nil && !copyInput.Constraints.Allowed {
		return AssembledContext{}, ErrConstraintConflict
	}

	constraints := flattenConstraints(copyInput.Constraints)
	base := renderBase(copyInput, constraints)
	baseTokens, err := a.counter.Count(base)
	if err != nil {
		return AssembledContext{}, err
	}
	budget := tier.Budget()
	if baseTokens > budget {
		return AssembledContext{}, &BudgetError{Tier: tier, Budget: budget, RequiredTokens: baseTokens}
	}

	selected := make([]optionalCandidate, 0, len(copyInput.Evidence)+len(copyInput.Advisory)+MaxExamples)
	for _, item := range copyInput.Evidence {
		selected = append(selected, optionalCandidate{lane: LaneEvidence, id: item.ID, priority: item.Priority, evidence: item})
	}
	for _, item := range copyInput.Advisory {
		selected = append(selected, optionalCandidate{lane: LaneAdvisory, id: item.ID, priority: item.Priority, advisory: item})
	}
	examples := append([]OptionalItem(nil), copyInput.Examples...)
	sortOptional(examples)
	for i, item := range examples {
		if i >= MaxExamples {
			continue
		}
		selected = append(selected, optionalCandidate{lane: LaneExamples, id: item.ID, priority: item.Priority, example: item})
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].priority != selected[j].priority {
			return selected[i].priority > selected[j].priority
		}
		if selected[i].lane != selected[j].lane {
			return selected[i].lane < selected[j].lane
		}
		return selected[i].id < selected[j].id
	})

	assembled := AssembledContext{
		Tier: tier, Budget: budget, Locale: copyInput.Locale, Strategy: copyInput.Strategy,
		Topic: copyInput.Topic, TopicVersion: copyInput.TopicVersion, Question: copyInput.Question,
		Constraints: cloneConstraintState(copyInput.Constraints), Metrics: cloneMetrics(copyInput.Metrics),
	}
	if len(examples) > MaxExamples {
		for _, item := range examples[MaxExamples:] {
			assembled.addOmission(Omission{Lane: LaneExamples, ID: item.ID, Reason: "max_examples"})
		}
	}
	for _, item := range selected {
		if err := ctx.Err(); err != nil {
			return AssembledContext{}, err
		}
		candidatePrompt := renderWithCandidate(base, item)
		candidateTokens, err := a.counter.Count(candidatePrompt)
		if err != nil {
			return AssembledContext{}, err
		}
		if candidateTokens > budget {
			assembled.addOmission(Omission{Lane: item.lane, ID: item.id, Reason: "budget"})
			continue
		}
		base = candidatePrompt
		assembled.add(item)
	}

	assembled.Prompt = base
	assembled.Tokens, err = a.counter.Count(base)
	if err != nil {
		return AssembledContext{}, err
	}
	assembled.Audit.Usage, err = a.usage(copyInput, assembled)
	if err != nil {
		return AssembledContext{}, err
	}
	assembled.seal = sealAssembledContext(assembled)
	return assembled, nil
}

func (a *ContextAssembler) AssembleForConfidence(ctx context.Context, input ContextInput, confidence float64) (AssembledContext, error) {
	tier, err := TierForConfidence(confidence)
	if err != nil {
		return AssembledContext{}, err
	}
	return a.Assemble(ctx, input, tier)
}

// validateAssembledContext accepts only an assembler-produced value whose
// canonical prompt, budget and tokenizer count still agree. The deep copy
// prevents later caller mutation from reaching a generation consumer.
func (a *ContextAssembler) validateAssembledContext(input AssembledContext) (AssembledContext, error) {
	if a == nil || a.counter == nil {
		return AssembledContext{}, ErrInvalid
	}
	if !input.Tier.valid() || input.Budget != input.Tier.Budget() {
		return AssembledContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context.budget"}
	}
	if input.seal != sealAssembledContext(input) {
		return AssembledContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context.seal"}
	}
	if len(input.Examples) > MaxExamples {
		return AssembledContext{}, &ValidationError{Code: CodeLimit, Path: "context.examples"}
	}
	canonicalInput, err := cloneAndValidateInput(outputInput(input))
	if err != nil {
		return AssembledContext{}, err
	}
	if canonicalInput.Constraints != nil && !canonicalInput.Constraints.Allowed {
		return AssembledContext{}, ErrConstraintConflict
	}
	canonicalPrompt := renderAssembledPrompt(canonicalInput)
	if input.Prompt != canonicalPrompt {
		return AssembledContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context.prompt"}
	}
	count, err := a.counter.Count(input.Prompt)
	if err != nil {
		return AssembledContext{}, err
	}
	if count != input.Tokens || count > input.Budget {
		return AssembledContext{}, &ValidationError{Code: CodeInvalidValue, Path: "context.tokens"}
	}
	return cloneAssembled(input), nil
}

type optionalCandidate struct {
	lane     Lane
	id       string
	priority int
	evidence Evidence
	advisory OptionalItem
	example  OptionalItem
}

func (c optionalCandidate) text() string {
	switch c.lane {
	case LaneEvidence:
		return c.evidence.Text
	case LaneAdvisory:
		return c.advisory.Text
	case LaneExamples:
		return c.example.Text
	default:
		return ""
	}
}

func (c optionalCandidate) source() string {
	switch c.lane {
	case LaneEvidence:
		return c.evidence.Source
	case LaneAdvisory:
		return c.advisory.Source
	case LaneExamples:
		return c.example.Source
	default:
		return ""
	}
}

func (a *AssembledContext) add(candidate optionalCandidate) {
	item := OptionalItem{ID: candidate.id, Text: candidate.text(), Priority: candidate.priority, Source: candidate.source()}
	switch candidate.lane {
	case LaneEvidence:
		a.Evidence = append(a.Evidence, Evidence{ID: item.ID, Text: item.Text, Priority: item.Priority, Source: item.Source, Confidence: cloneFloat(candidate.evidence.Confidence)})
	case LaneAdvisory:
		item.Confidence = cloneFloat(candidate.advisory.Confidence)
		a.Advisory = append(a.Advisory, item)
	case LaneExamples:
		item.Confidence = cloneFloat(candidate.example.Confidence)
		a.Examples = append(a.Examples, item)
	}
}

func (a *AssembledContext) addOmission(omission Omission) {
	a.Audit.OmittedCount++
	if len(a.Audit.Omitted) < MaxOmissions {
		a.Audit.Omitted = append(a.Audit.Omitted, omission)
	}
}

func (a *ContextAssembler) usage(input ContextInput, output AssembledContext) ([]LaneUsage, error) {
	values := []struct {
		lane     Lane
		original string
		included string
	}{
		{LaneHeader, renderHeader(input), renderHeader(outputInput(output))},
		{LaneEvidence, renderEvidence(input.Evidence), renderEvidence(output.Evidence)},
		{LaneConstraints, renderConstraints(flattenConstraints(input.Constraints)), renderConstraints(flattenConstraints(output.Constraints))},
		{LaneMetrics, renderMetrics(input.Metrics), renderMetrics(output.Metrics)},
		{LaneAdvisory, renderOptional(LaneAdvisory, input.Advisory), renderOptional(LaneAdvisory, output.Advisory)},
		{LaneExamples, renderOptional(LaneExamples, input.Examples), renderOptional(LaneExamples, output.Examples)},
	}
	usage := make([]LaneUsage, 0, len(values))
	for _, value := range values {
		original, err := a.counter.Count(value.original)
		if err != nil {
			return nil, err
		}
		included, err := a.counter.Count(value.included)
		if err != nil {
			return nil, err
		}
		usage = append(usage, LaneUsage{Lane: value.lane, OriginalTokens: original, IncludedTokens: included})
	}
	return usage, nil
}

func renderWithCandidate(base string, candidate optionalCandidate) string {
	return base + renderItem(candidate.lane, candidate.id, candidate.text())
}

func renderBase(input ContextInput, constraints []MandatoryConstraint) string {
	return renderHeader(input) + renderConstraints(constraints) + renderMetrics(input.Metrics)
}

func renderHeader(input ContextInput) string {
	return fmt.Sprintf("strategy:%s\nlocale:%s\ntopic:%s\nversion:%s\nquestion:%s\n", input.Strategy, input.Locale, input.Topic, input.TopicVersion, input.Question)
}

func renderConstraints(items []MandatoryConstraint) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(renderItem(LaneConstraints, item.ID, item.Kind+": "+item.Text))
	}
	return b.String()
}

func renderMetrics(items []PinnedMetric) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(renderItem(LaneMetrics, item.ID, item.Text))
	}
	return b.String()
}

func renderEvidence(items []Evidence) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(renderItem(LaneEvidence, item.ID, item.Text))
	}
	return b.String()
}

func renderOptional(lane Lane, items []OptionalItem) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(renderItem(lane, item.ID, item.Text))
	}
	return b.String()
}

func renderItem(lane Lane, id, text string) string {
	return fmt.Sprintf("%s[%s]:%s\n", lane, id, text)
}

func outputInput(output AssembledContext) ContextInput {
	return ContextInput{Locale: output.Locale, Strategy: output.Strategy, Topic: output.Topic, TopicVersion: output.TopicVersion, Question: output.Question, Evidence: cloneEvidence(output.Evidence), Constraints: cloneConstraintState(output.Constraints), Metrics: cloneMetrics(output.Metrics), Advisory: cloneOptional(output.Advisory), Examples: cloneOptional(output.Examples)}
}

func renderAssembledPrompt(input ContextInput) string {
	base := renderBase(input, flattenConstraints(input.Constraints))
	selected := optionalCandidates(input)
	for _, item := range selected {
		base = renderWithCandidate(base, item)
	}
	return base
}

func optionalCandidates(input ContextInput) []optionalCandidate {
	selected := make([]optionalCandidate, 0, len(input.Evidence)+len(input.Advisory)+len(input.Examples))
	for _, item := range input.Evidence {
		selected = append(selected, optionalCandidate{lane: LaneEvidence, id: item.ID, priority: item.Priority, evidence: item})
	}
	for _, item := range input.Advisory {
		selected = append(selected, optionalCandidate{lane: LaneAdvisory, id: item.ID, priority: item.Priority, advisory: item})
	}
	for _, item := range input.Examples {
		selected = append(selected, optionalCandidate{lane: LaneExamples, id: item.ID, priority: item.Priority, example: item})
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].priority != selected[j].priority {
			return selected[i].priority > selected[j].priority
		}
		if selected[i].lane != selected[j].lane {
			return selected[i].lane < selected[j].lane
		}
		return selected[i].id < selected[j].id
	})
	return selected
}

func flattenConstraints(state *ConstraintState) []MandatoryConstraint {
	if state == nil {
		return nil
	}
	out := append([]MandatoryConstraint(nil), state.Required...)
	out = append(out, state.Excluded...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func cloneAndValidateInput(input ContextInput) (ContextInput, error) {
	if !input.Locale.valid() {
		return ContextInput{}, &ValidationError{Code: CodeUnsupported, Path: "locale"}
	}
	if !input.Strategy.valid() {
		return ContextInput{}, &ValidationError{Code: CodeUnsupported, Path: "strategy"}
	}
	if !validText(input.Question, 16<<10) {
		return ContextInput{}, &ValidationError{Code: CodeInvalidValue, Path: "question"}
	}
	if input.Topic != "" && !identity.Identifier(input.Topic) || input.TopicVersion != "" && !identity.Identifier(input.TopicVersion) {
		return ContextInput{}, &ValidationError{Code: CodeInvalidValue, Path: "topic"}
	}
	if len(input.Evidence) > 256 || len(input.Metrics) > 128 || len(input.Advisory) > 256 || len(input.Examples) > 256 {
		return ContextInput{}, &ValidationError{Code: CodeLimit, Path: "lanes"}
	}
	if input.Constraints != nil && len(input.Constraints.Required)+len(input.Constraints.Excluded) > MaxConstraints {
		return ContextInput{}, &ValidationError{Code: CodeLimit, Path: "constraints"}
	}
	out := ContextInput{Locale: input.Locale, Strategy: input.Strategy, Topic: input.Topic, TopicVersion: input.TopicVersion, Question: input.Question}
	seen := map[string]bool{}
	var err error
	out.Evidence, err = cloneEvidenceChecked(input.Evidence, seen, "evidence")
	if err != nil {
		return ContextInput{}, err
	}
	out.Metrics, err = cloneMetricsChecked(input.Metrics, seen)
	if err != nil {
		return ContextInput{}, err
	}
	out.Advisory, err = cloneOptionalChecked(input.Advisory, seen, "advisory")
	if err != nil {
		return ContextInput{}, err
	}
	out.Examples, err = cloneOptionalChecked(input.Examples, seen, "examples")
	if err != nil {
		return ContextInput{}, err
	}
	if input.Constraints != nil {
		state := &ConstraintState{Allowed: input.Constraints.Allowed}
		state.Required, err = cloneConstraintsChecked(input.Constraints.Required, seen, "constraints.required")
		if err != nil {
			return ContextInput{}, err
		}
		state.Excluded, err = cloneConstraintsChecked(input.Constraints.Excluded, seen, "constraints.excluded")
		if err != nil {
			return ContextInput{}, err
		}
		out.Constraints = state
	}
	return out, nil
}

func validText(value string, max int) bool {
	return len(value) > 0 && len(value) <= max && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func validID(value string) bool { return identity.Identifier(value) }

func validSource(value string) bool { return value == "" || validID(value) }

func validateConfidence(value *float64) bool {
	return value == nil || (!math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= 1)
}

func cloneEvidenceChecked(items []Evidence, seen map[string]bool, path string) ([]Evidence, error) {
	out := cloneEvidence(items)
	for i := range out {
		if !validID(out[i].ID) || !validText(out[i].Text, 16<<10) || !validSource(out[i].Source) || !validateConfidence(out[i].Confidence) {
			return nil, &ValidationError{Code: CodeInvalidValue, Path: path}
		}
		if seen[out[i].ID] {
			return nil, &ValidationError{Code: CodeDuplicateID, Path: path}
		}
		seen[out[i].ID] = true
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func cloneMetricsChecked(items []PinnedMetric, seen map[string]bool) ([]PinnedMetric, error) {
	out := cloneMetrics(items)
	for i := range out {
		if !validID(out[i].ID) || !validText(out[i].Text, 16<<10) {
			return nil, &ValidationError{Code: CodeInvalidValue, Path: "metrics"}
		}
		if seen[out[i].ID] {
			return nil, &ValidationError{Code: CodeDuplicateID, Path: "metrics"}
		}
		seen[out[i].ID] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func cloneOptionalChecked(items []OptionalItem, seen map[string]bool, path string) ([]OptionalItem, error) {
	out := cloneOptional(items)
	for i := range out {
		if !validID(out[i].ID) || !validText(out[i].Text, 16<<10) || !validSource(out[i].Source) || !validateConfidence(out[i].Confidence) {
			return nil, &ValidationError{Code: CodeInvalidValue, Path: path}
		}
		if seen[out[i].ID] {
			return nil, &ValidationError{Code: CodeDuplicateID, Path: path}
		}
		seen[out[i].ID] = true
	}
	sortOptional(out)
	return out, nil
}

func cloneConstraintsChecked(items []MandatoryConstraint, seen map[string]bool, path string) ([]MandatoryConstraint, error) {
	out := cloneConstraints(items)
	for i := range out {
		if !validID(out[i].ID) || !validID(out[i].Kind) || !validText(out[i].Text, 16<<10) {
			return nil, &ValidationError{Code: CodeInvalidValue, Path: path}
		}
		if seen[out[i].ID] {
			return nil, &ValidationError{Code: CodeDuplicateID, Path: path}
		}
		seen[out[i].ID] = true
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

func sortOptional(items []OptionalItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		return items[i].ID < items[j].ID
	})
}

func cloneEvidence(items []Evidence) []Evidence {
	out := append([]Evidence(nil), items...)
	for i := range out {
		out[i].Confidence = cloneFloat(out[i].Confidence)
	}
	return out
}
func cloneMetrics(items []PinnedMetric) []PinnedMetric { return append([]PinnedMetric(nil), items...) }
func cloneOptional(items []OptionalItem) []OptionalItem {
	out := append([]OptionalItem(nil), items...)
	for i := range out {
		out[i].Confidence = cloneFloat(out[i].Confidence)
	}
	return out
}
func cloneConstraints(items []MandatoryConstraint) []MandatoryConstraint {
	return append([]MandatoryConstraint(nil), items...)
}
func cloneConstraintState(state *ConstraintState) *ConstraintState {
	if state == nil {
		return nil
	}
	return &ConstraintState{Allowed: state.Allowed, Required: cloneConstraints(state.Required), Excluded: cloneConstraints(state.Excluded)}
}
func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneAudit(a AssemblyAudit) AssemblyAudit {
	return AssemblyAudit{Omitted: append([]Omission(nil), a.Omitted...), OmittedCount: a.OmittedCount, Usage: append([]LaneUsage(nil), a.Usage...)}
}

func sealAssembledContext(input AssembledContext) [32]byte {
	input.seal = [32]byte{}
	return sealBytes(struct {
		Context AssembledContext `json:"context"`
		Audit   AssemblyAudit    `json:"audit"`
	}{Context: input, Audit: input.Audit})
}

func sealBytes(input any) [32]byte {
	raw, _ := json.Marshal(input)
	return sha256.Sum256(raw)
}
