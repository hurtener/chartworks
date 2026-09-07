// Package semantics owns stable topic-pack entities and their exact reference graph.
// It contains no publication state, authority policy, source I/O, or model calls.
package semantics

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/identity"
)

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
	CodeInvalidValue     ValidationCode = "invalid_value"
	CodeLimit            ValidationCode = "limit_exceeded"
	CodeDuplicateID      ValidationCode = "duplicate_id"
	CodeMissingReference ValidationCode = "missing_reference"
	CodeInvalidReference ValidationCode = "invalid_reference"
	CodeReferenceCycle   ValidationCode = "reference_cycle"
	CodeAmbiguousTerm    ValidationCode = "ambiguous_canonical_term"
	CodeEvidenceMismatch ValidationCode = "evidence_mismatch"
	CodeRuleConflict     ValidationCode = "rule_conflict"
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
	KindDataset         Kind = "dataset"
	KindColumn          Kind = "column"
	KindMeasure         Kind = "measure"
	KindDimension       Kind = "dimension"
	KindKPI             Kind = "kpi"
	KindJoin            Kind = "join"
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

type Dataset struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Source  SourceReference `json:"source"`
	Columns []Column        `json:"columns"`
}

type Aggregation string

const (
	AggregationSum           Aggregation = "sum"
	AggregationAverage       Aggregation = "average"
	AggregationMinimum       Aggregation = "minimum"
	AggregationMaximum       Aggregation = "maximum"
	AggregationCount         Aggregation = "count"
	AggregationDistinctCount Aggregation = "distinct_count"
)

func (a Aggregation) valid() bool {
	switch a {
	case AggregationSum, AggregationAverage, AggregationMinimum, AggregationMaximum, AggregationCount, AggregationDistinctCount:
		return true
	}
	return false
}

type Measure struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Field       Reference   `json:"field"`
	Aggregation Aggregation `json:"aggregation"`
	Unit        string      `json:"unit"`
}

type DimensionRole string

const (
	DimensionCategorical DimensionRole = "categorical"
	DimensionTemporal    DimensionRole = "temporal"
	DimensionNumeric     DimensionRole = "numeric"
	DimensionBoolean     DimensionRole = "boolean"
	DimensionIdentifier  DimensionRole = "identifier"
)

func (r DimensionRole) valid() bool {
	switch r {
	case DimensionCategorical, DimensionTemporal, DimensionNumeric, DimensionBoolean, DimensionIdentifier:
		return true
	}
	return false
}

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

type JoinType string

const (
	JoinInner JoinType = "inner"
	JoinLeft  JoinType = "left"
)

func (t JoinType) valid() bool { return t == JoinInner || t == JoinLeft }

type Cardinality string

const (
	CardinalityOneToOne   Cardinality = "one_to_one"
	CardinalityOneToMany  Cardinality = "one_to_many"
	CardinalityManyToOne  Cardinality = "many_to_one"
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

// Reference returns the exact immutable registry revision captured by a pack.
func (e CanonicalEntity) Reference() Reference {
	return Reference{Kind: KindCanonicalEntity, ID: e.ID, Revision: e.Revision}
}

// TopicPack is an authoring definition only. Lifecycle stage, active pointers, ready
// facets, authority, and current source health are separate state owned by later work.
type TopicPack struct {
	SchemaVersion     int               `json:"schema_version"`
	Topic             string            `json:"topic"`
	Version           string            `json:"version"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Datasets          []Dataset         `json:"datasets"`
	Measures          []Measure         `json:"measures"`
	Dimensions        []Dimension       `json:"dimensions"`
	KPIs              []KPI             `json:"kpis"`
	Joins             []Join            `json:"joins"`
	CanonicalEntities []CanonicalEntity `json:"canonical_entities"`
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
