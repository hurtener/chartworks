// Package semantics owns stable topic-pack entities and their exact reference graph.
// It contains no publication state, authority policy, source I/O, or model calls.
package semantics

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/identity"
)

// SchemaVersion is the accepted topic pack wire version.
const SchemaVersion = 1

var (
	// ErrInvalid is the common typed boundary for malformed semantic definitions.
	ErrInvalid = errors.New("semantics: invalid topic pack")
	// ErrNotFound reports an unknown canonical term without substituting a display name.
	ErrNotFound = errors.New("semantics: canonical entity not found")
)

// ValidationCode is a stable content-free classification. Path identifies the
// definition member but never includes source values, SQL, credentials, or prompts.
type ValidationCode string

const (
	// CodeInvalidValue identifies a malformed bounded value.
	CodeInvalidValue ValidationCode = "invalid_value"
	// CodeLimit identifies a bounded collection or payload overflow.
	CodeLimit ValidationCode = "limit_exceeded"
	// CodeDuplicateID identifies a repeated stable identifier.
	CodeDuplicateID ValidationCode = "duplicate_id"
	// CodeMissingReference identifies an absent dependency.
	CodeMissingReference ValidationCode = "missing_reference"
	// CodeInvalidReference identifies a malformed dependency coordinate.
	CodeInvalidReference ValidationCode = "invalid_reference"
	// CodeReferenceCycle identifies a cyclic semantic dependency.
	CodeReferenceCycle ValidationCode = "reference_cycle"
	// CodeAmbiguousTerm identifies conflicting canonical vocabulary.
	CodeAmbiguousTerm ValidationCode = "ambiguous_canonical_term"
	// CodeEvidenceMismatch identifies inconsistent source evidence.
	CodeEvidenceMismatch ValidationCode = "evidence_mismatch"
	// CodeRuleConflict identifies incompatible rule authoring.
	CodeRuleConflict ValidationCode = "rule_conflict"
)

// ValidationError keeps invalid definitions observable without echoing their content.
type ValidationError struct {
	Code ValidationCode
	Path string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("semantics: %s at %s", e.Code, e.Path) }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

func invalid(code ValidationCode, path string) error { return &ValidationError{Code: code, Path: path} }

// Kind is the closed entity namespace used by references and later rule scopes.
type Kind string

const (
	// KindDataset identifies a dataset entity namespace.
	KindDataset Kind = "dataset"
	// KindColumn identifies a dataset-qualified column namespace.
	KindColumn Kind = "column"
	// KindMeasure identifies an aggregate namespace.
	KindMeasure Kind = "measure"
	// KindDimension identifies a grouping namespace.
	KindDimension Kind = "dimension"
	// KindKPI identifies a business expression namespace.
	KindKPI Kind = "kpi"
	// KindJoin identifies a relationship namespace.
	KindJoin Kind = "join"
	// KindCanonicalEntity identifies tenant-wide business meaning.
	KindCanonicalEntity Kind = "canonical_entity"
)

func (k Kind) valid() bool {
	switch k {
	case KindDataset, KindColumn, KindMeasure, KindDimension, KindKPI, KindJoin, KindCanonicalEntity:
		return true
	}
	return false
}

// Reference addresses an entity by stable ID. Columns are qualified by their
// stable dataset ID; every other kind must leave Dataset empty.
type Reference struct {
	Kind     Kind   `json:"kind"`
	Dataset  string `json:"dataset,omitempty"`
	ID       string `json:"id"`
	Revision int64  `json:"revision,omitempty"`
}

// Valid rejects name fallback, partial column coordinates, and open kinds.
func (r Reference) Valid() bool {
	if !r.Kind.valid() || !identity.Identifier(r.ID) {
		return false
	}
	if r.Kind == KindColumn {
		return identity.Identifier(r.Dataset) && r.Revision == 0
	}
	if r.Kind == KindCanonicalEntity {
		return r.Dataset == "" && r.Revision > 0 && r.Revision < 1<<62
	}
	return r.Dataset == "" && r.Revision == 0
}

func (r Reference) key() string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d", r.Kind, r.Dataset, r.ID, r.Revision)
}

// SourceReference pins a registered dataset to one immutable source-context and
// profiling evidence revision. It is semantic provenance, never authority.
type SourceReference struct {
	Source         string `json:"source"`
	Context        string `json:"context"`
	Dataset        string `json:"dataset"`
	ProfileVersion string `json:"profile_version"`
	ProfileDigest  string `json:"profile_digest"`
	SourceRevision int64  `json:"source_revision"`
}

// Column preserves a stable semantic ID separately from the current source name.
// References use ID, so a reviewed source rename can update SourceName in a new
// draft without rewriting every semantic reference.
type Column struct {
	ID         string `json:"id"`
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	NativeType string `json:"native_type"`
	Category   string `json:"category"`
	Nullable   bool   `json:"nullable"`
}

// Dataset binds stable semantic columns to exact source evidence.
type Dataset struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Source  SourceReference `json:"source"`
	Columns []Column        `json:"columns"`
}

// Aggregation is the closed measure aggregation vocabulary.
type Aggregation string

const (
	// AggregationSum sums numeric values.
	AggregationSum Aggregation = "sum"
	// AggregationAverage computes an arithmetic mean.
	AggregationAverage Aggregation = "average"
	// AggregationMinimum selects the minimum value.
	AggregationMinimum Aggregation = "minimum"
	// AggregationMaximum selects the maximum value.
	AggregationMaximum Aggregation = "maximum"
	// AggregationCount counts values.
	AggregationCount Aggregation = "count"
	// AggregationDistinctCount counts distinct values.
	AggregationDistinctCount Aggregation = "distinct_count"
)

func (a Aggregation) valid() bool {
	switch a {
	case AggregationSum, AggregationAverage, AggregationMinimum, AggregationMaximum, AggregationCount, AggregationDistinctCount:
		return true
	}
	return false
}

// Measure defines one aggregate over an exact column.
type Measure struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Field       Reference   `json:"field"`
	Aggregation Aggregation `json:"aggregation"`
	Unit        string      `json:"unit"`
}

// DimensionRole classifies a grouping field.
type DimensionRole string

const (
	// DimensionCategorical identifies an unordered category.
	DimensionCategorical DimensionRole = "categorical"
	// DimensionTemporal identifies a time value.
	DimensionTemporal DimensionRole = "temporal"
	// DimensionNumeric identifies a numeric grouping value.
	DimensionNumeric DimensionRole = "numeric"
	// DimensionBoolean identifies a boolean grouping value.
	DimensionBoolean DimensionRole = "boolean"
	// DimensionIdentifier identifies a record identifier.
	DimensionIdentifier DimensionRole = "identifier"
)

func (r DimensionRole) valid() bool {
	switch r {
	case DimensionCategorical, DimensionTemporal, DimensionNumeric, DimensionBoolean, DimensionIdentifier:
		return true
	}
	return false
}

// Dimension defines one reviewed grouping field.
type Dimension struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Field       Reference     `json:"field"`
	Role        DimensionRole `json:"role"`
}

// KPI carries a business expression and an exact dependency list. Expression is
// not SQL and never becomes executable without a later validated query consumer.
type KPI struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Expression  string      `json:"expression"`
	Inputs      []Reference `json:"inputs"`
}

// JoinType is the closed supported join vocabulary.
type JoinType string

const (
	// JoinInner retains only matching rows.
	JoinInner JoinType = "inner"
	// JoinLeft retains every left-side row.
	JoinLeft JoinType = "left"
)

func (t JoinType) valid() bool { return t == JoinInner || t == JoinLeft }

// Cardinality records the reviewed relationship shape.
type Cardinality string

const (
	// CardinalityOneToOne identifies unique keys on both sides.
	CardinalityOneToOne Cardinality = "one_to_one"
	// CardinalityOneToMany identifies repeated right-side keys.
	CardinalityOneToMany Cardinality = "one_to_many"
	// CardinalityManyToOne identifies repeated left-side keys.
	CardinalityManyToOne Cardinality = "many_to_one"
	// CardinalityManyToMany identifies repeated keys on both sides.
	CardinalityManyToMany Cardinality = "many_to_many"
)

func (c Cardinality) valid() bool {
	switch c {
	case CardinalityOneToOne, CardinalityOneToMany, CardinalityManyToOne, CardinalityManyToMany:
		return true
	}
	return false
}

// Join is equality-only by construction: it binds two exact column references
// and has no caller-supplied condition text.
type Join struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Left        Reference   `json:"left"`
	Right       Reference   `json:"right"`
	Type        JoinType    `json:"type"`
	Cardinality Cardinality `json:"cardinality"`
}

// CanonicalEntity binds business terms to one stable identity and exact key
// columns. Consumers resolve the term first and compare keys, never display names.
type CanonicalEntity struct {
	ID       string      `json:"id"`
	Revision int64       `json:"revision"`
	Name     string      `json:"name"`
	Aliases  []string    `json:"aliases"`
	Keys     []Reference `json:"keys"`
}

// UnresolvedSemantic preserves a stable column-scoped authoring gap emitted by
// bounded enhancement. It is visible review evidence, never an executable entity.
type UnresolvedSemantic struct {
	ID      string `json:"id"`
	Dataset string `json:"dataset"`
	Column  string `json:"column"`
	Reason  string `json:"reason"`
}

// Reference returns the exact immutable registry revision captured by a pack.
func (e CanonicalEntity) Reference() Reference {
	return Reference{Kind: KindCanonicalEntity, ID: e.ID, Revision: e.Revision}
}

// TopicPack is an authoring definition only. Lifecycle stage, active pointers, ready
// facets, authority, and current source health are separate state owned by later work.
type TopicPack struct {
	SchemaVersion     int                  `json:"schema_version"`
	Topic             string               `json:"topic"`
	Version           string               `json:"version"`
	Name              string               `json:"name"`
	Description       string               `json:"description"`
	Datasets          []Dataset            `json:"datasets"`
	Measures          []Measure            `json:"measures"`
	Dimensions        []Dimension          `json:"dimensions"`
	KPIs              []KPI                `json:"kpis"`
	Joins             []Join               `json:"joins"`
	CanonicalEntities []CanonicalEntity    `json:"canonical_entities"`
	Unresolved        []UnresolvedSemantic `json:"unresolved,omitempty"`
}

func validLine(s string, maximum int) bool {
	return utf8.ValidString(s) && len(s) > 0 && len(s) <= maximum && s[0] != ' ' && s[len(s)-1] != ' ' && !containsControl(s)
}

func validText(s string, maximum int) bool {
	return utf8.ValidString(s) && len(s) <= maximum && !containsNUL(s)
}

func validOptionalLine(s string, maximum int) bool { return s == "" || validLine(s, maximum) }

func containsControl(s string) bool {
	for _, r := range s {
		if r == 0 || r == '\r' || r == '\n' || r == '\t' || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func containsNUL(s string) bool {
	for _, r := range s {
		if r == 0 {
			return true
		}
	}
	return false
}
