package semantics

// RuleCategory classifies business reasoning, never caller authority.
type RuleCategory string

// RuleComputation, RuleSemantic, and RuleStructural classify the kind of
// business reasoning represented by a rule.
const (
	RuleComputation RuleCategory = "computation"
	RuleSemantic    RuleCategory = "semantic"
	RuleStructural  RuleCategory = "structural"
)

// RuleClass distinguishes executable constraints from bounded advisory text.
type RuleClass string

// RuleExecutionConstraint and RuleAdvisoryContext identify the supported rule
// classes; neither class grants caller authority.
const (
	RuleExecutionConstraint RuleClass = "execution_constraint"
	RuleAdvisoryContext     RuleClass = "advisory_context"
)

// RuleScopeKind identifies whether a rule applies to a topic or selected entities.
type RuleScopeKind string

// RuleScopeTopic and RuleScopeEntities identify the supported rule scopes.
const (
	RuleScopeTopic    RuleScopeKind = "topic"
	RuleScopeEntities RuleScopeKind = "entities"
)

// RuleScope identifies the objects governed by the rule. Targets are not
// activation predicates, and a scope never grants permission to read its objects.
type RuleScope struct {
	Kind    RuleScopeKind `json:"kind"`
	Targets []Reference   `json:"targets"`
}

// ProvenanceKind identifies the source of authoring evidence for a rule.
type ProvenanceKind string

// ProvenanceHuman, ProvenanceModel, ProvenanceFeedback, and ProvenanceImport
// identify supported authoring evidence sources.
const (
	ProvenanceHuman    ProvenanceKind = "human"
	ProvenanceModel    ProvenanceKind = "model"
	ProvenanceFeedback ProvenanceKind = "feedback"
	ProvenanceImport   ProvenanceKind = "import"
)

// RuleProvenance names authoring evidence; it is not proof of identity or review.
type RuleProvenance struct {
	Kind     ProvenanceKind `json:"kind"`
	Evidence string         `json:"evidence"`
}

// ConstraintKind identifies the closed hard-constraint operations.
type ConstraintKind string

// ConstraintRequireReference and ConstraintExcludeReference identify the
// supported dependency-presence constraints.
const (
	ConstraintRequireReference ConstraintKind = "require_reference"
	ConstraintExcludeReference ConstraintKind = "exclude_reference"
)

// Constraint governs presence in the semantic dependency graph, not output
// visibility or data authorization. No SQL, arbitrary predicate, or prompt text
// can be substituted for its closed kind and exact reference.
type Constraint struct {
	Kind   ConstraintKind `json:"kind"`
	Target Reference      `json:"target"`
}

// LiteralSensitivity records the declared sensitivity of bounded rule text.
type LiteralSensitivity string

// LiteralNonSensitive and LiteralSensitive identify the supported sensitivity
// declarations.
const (
	LiteralNonSensitive LiteralSensitivity = "non_sensitive"
	LiteralSensitive    LiteralSensitivity = "sensitive"
)

// AdvisoryGuidance carries bounded authoring guidance that is not an execution
// constraint or authority decision.
type AdvisoryGuidance struct {
	Text        string             `json:"text"`
	Sensitivity LiteralSensitivity `json:"sensitivity"`
}

// RuleDefinition is an authoring revision. CompileRules cannot activate a rule
// or turn feedback/model provenance into review approval.
type RuleDefinition struct {
	ID         string            `json:"id"`
	Version    string            `json:"version"`
	Category   RuleCategory      `json:"category"`
	Class      RuleClass         `json:"class"`
	Scope      RuleScope         `json:"scope"`
	Priority   int               `json:"priority"`
	Provenance RuleProvenance    `json:"provenance"`
	Constraint *Constraint       `json:"constraint,omitempty"`
	Guidance   *AdvisoryGuidance `json:"guidance,omitempty"`
}

// SlotKind identifies the input shape of a clarification slot.
type SlotKind string

// SlotChoice, SlotText, SlotNumber, SlotBoolean, and SlotDate identify the
// supported clarification slot shapes.
const (
	SlotChoice  SlotKind = "choice"
	SlotText    SlotKind = "text"
	SlotNumber  SlotKind = "number"
	SlotBoolean SlotKind = "boolean"
	SlotDate    SlotKind = "date"
)

// ClarificationChoice is one bounded choice offered for a clarification slot.
type ClarificationChoice struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Target *Reference `json:"target,omitempty"`
}

// ClarificationSlot defines one typed clarification input. Slot/choice order is
// author-selected presentation order and affects the digest.
// Sensitivity is a required declaration, not a claim of automatic PII detection.
type ClarificationSlot struct {
	ID          string                `json:"id"`
	Prompt      string                `json:"prompt"`
	Required    bool                  `json:"required"`
	Kind        SlotKind              `json:"kind"`
	Sensitivity LiteralSensitivity    `json:"sensitivity"`
	Choices     []ClarificationChoice `json:"choices"`
}

// ClarificationPattern defines slots only; matching a question is later work.
type ClarificationPattern struct {
	ID         string              `json:"id"`
	Version    string              `json:"version"`
	Targets    []Reference         `json:"targets"`
	Provenance RuleProvenance      `json:"provenance"`
	Slots      []ClarificationSlot `json:"slots"`
}

// RuleSetDefinition pins all rules and patterns to one exact compiled topic.
// Lifecycle, current authority, query state and token budgets are separate.
type RuleSetDefinition struct {
	SchemaVersion int                    `json:"schema_version"`
	ID            string                 `json:"id"`
	Version       string                 `json:"version"`
	Topic         string                 `json:"topic"`
	TopicVersion  string                 `json:"topic_version"`
	PackDigest    string                 `json:"pack_digest"`
	Rules         []RuleDefinition       `json:"rules"`
	Patterns      []ClarificationPattern `json:"patterns"`
}

// RuleConflictError identifies a required dependency that another mandatory rule
// excludes. Error() stays content-free; structured details remain caller-owned.
type RuleConflictError struct {
	RequiredBy string
	ExcludedBy string
	Target     Reference
}

func (e *RuleConflictError) Error() string { return "semantics: conflicting mandatory rules" }
func (e *RuleConflictError) Unwrap() error { return invalid(CodeRuleConflict, "rules") }
