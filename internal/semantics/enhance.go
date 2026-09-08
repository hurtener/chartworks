package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// EnhancementKind is the closed set of model proposals accepted by topic generation.
type EnhancementKind string

const (
	// EnhancementMeasure creates an aggregate candidate.
	EnhancementMeasure EnhancementKind = "measure"
	// EnhancementDimension creates a grouping candidate.
	EnhancementDimension EnhancementKind = "dimension"
	// EnhancementUnresolved preserves an explicit authoring gap.
	EnhancementUnresolved EnhancementKind = "unresolved"
)

// Enhancement proposes semantics for exactly one existing stable column reference.
type Enhancement struct {
	Dataset     string          `json:"dataset"`
	Column      string          `json:"column"`
	Kind        EnhancementKind `json:"kind"`
	Name        string          `json:"name"`
	Aggregation Aggregation     `json:"aggregation,omitempty"`
	Role        DimensionRole   `json:"role,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

// GeneratedEntityID derives a stable server-owned ID from the entity kind and
// physical-independent stable dataset/column IDs.
func GeneratedEntityID(kind EnhancementKind, dataset, column string) string {
	if !identityIdentifierPair(dataset, column) || kind != EnhancementMeasure && kind != EnhancementDimension && kind != EnhancementUnresolved {
		return ""
	}
	sum := sha256.Sum256([]byte(string(kind) + "\x00" + dataset + "\x00" + column))
	prefix := "unresolved_"
	switch kind {
	case EnhancementMeasure:
		prefix = "measure_"
	case EnhancementDimension:
		prefix = "dimension_"
	}
	return prefix + hex.EncodeToString(sum[:12])
}

func identityIdentifierPair(dataset, column string) bool {
	return Reference{Kind: KindColumn, Dataset: dataset, ID: column}.Valid()
}

// ApplyEnhancements returns a new compiled version. Every proposal must cover a
// distinct existing column; executable and unresolved outcomes are mutually exclusive.
func ApplyEnhancements(model Model, version string, proposals []Enhancement) (Model, error) {
	if model.Digest() == "" || version == "" || len(proposals) < 1 || len(proposals) > 32 {
		return Model{}, invalid(CodeInvalidValue, "enhancements")
	}
	p := model.Pack()
	p.Version = version
	seen := map[Reference]bool{}
	processed := map[Reference]bool{}
	for _, item := range proposals {
		ref := Reference{Kind: KindColumn, Dataset: item.Dataset, ID: item.Column}
		if !model.Contains(ref) || seen[ref] {
			return Model{}, invalid(CodeInvalidReference, "enhancements.column")
		}
		seen[ref], processed[ref] = true, true
		id := GeneratedEntityID(item.Kind, item.Dataset, item.Column)
		if id == "" {
			return Model{}, invalid(CodeInvalidValue, "enhancements.kind")
		}
		switch item.Kind {
		case EnhancementMeasure:
			if !validLine(item.Name, 256) || !item.Aggregation.valid() || item.Role != "" || item.Reason != "" {
				return Model{}, invalid(CodeInvalidValue, "enhancements.measure")
			}
		case EnhancementDimension:
			if !validLine(item.Name, 256) || !item.Role.valid() || item.Aggregation != "" || item.Reason != "" {
				return Model{}, invalid(CodeInvalidValue, "enhancements.dimension")
			}
		case EnhancementUnresolved:
			if item.Name != "" || item.Aggregation != "" || item.Role != "" || !validLine(item.Reason, 256) {
				return Model{}, invalid(CodeInvalidValue, "enhancements.unresolved")
			}
		default:
			return Model{}, invalid(CodeInvalidValue, "enhancements.kind")
		}
	}
	p.Measures = removeProcessedMeasures(p.Measures, processed)
	p.Dimensions = removeProcessedDimensions(p.Dimensions, processed)
	p.Unresolved = removeProcessedUnresolved(p.Unresolved, processed)
	// Re-add this step after removing any prior generated outcome for the same columns.
	for _, item := range proposals {
		ref := Reference{Kind: KindColumn, Dataset: item.Dataset, ID: item.Column}
		id := GeneratedEntityID(item.Kind, item.Dataset, item.Column)
		switch item.Kind {
		case EnhancementMeasure:
			p.Measures = append(p.Measures, Measure{ID: id, Name: item.Name, Field: ref, Aggregation: item.Aggregation})
		case EnhancementDimension:
			p.Dimensions = append(p.Dimensions, Dimension{ID: id, Name: item.Name, Field: ref, Role: item.Role})
		case EnhancementUnresolved:
			p.Unresolved = append(p.Unresolved, UnresolvedSemantic{ID: id, Dataset: item.Dataset, Column: item.Column, Reason: item.Reason})
		}
	}
	return Compile(p)
}

func removeProcessedMeasures(values []Measure, processed map[Reference]bool) []Measure {
	out := values[:0]
	for _, value := range values {
		if !processed[value.Field] || value.ID != GeneratedEntityID(EnhancementMeasure, value.Field.Dataset, value.Field.ID) {
			out = append(out, value)
		}
	}
	return out
}

func removeProcessedDimensions(values []Dimension, processed map[Reference]bool) []Dimension {
	out := values[:0]
	for _, value := range values {
		if !processed[value.Field] || value.ID != GeneratedEntityID(EnhancementDimension, value.Field.Dataset, value.Field.ID) {
			out = append(out, value)
		}
	}
	return out
}

func removeProcessedUnresolved(values []UnresolvedSemantic, processed map[Reference]bool) []UnresolvedSemantic {
	out := values[:0]
	for _, value := range values {
		ref := Reference{Kind: KindColumn, Dataset: value.Dataset, ID: value.Column}
		// The stable unresolved ID records its original enhancement identity and
		// intentionally survives dataset replacement. Resolution follows the
		// rewritten semantic column coordinate instead of regenerating that ID.
		if !processed[ref] {
			out = append(out, value)
		}
	}
	return out
}

// GenerationColumns returns stable column coordinates in compiler order.
func GenerationColumns(model Model) []Reference {
	if model.Digest() == "" {
		return nil
	}
	var out []Reference
	for _, dataset := range model.pack.Datasets {
		for _, column := range dataset.Columns {
			out = append(out, Reference{Kind: KindColumn, Dataset: dataset.ID, ID: column.ID})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}
