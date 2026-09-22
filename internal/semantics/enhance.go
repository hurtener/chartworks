package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
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
	Dataset      string           `json:"dataset"`
	Column       string           `json:"column"`
	Kind         EnhancementKind  `json:"kind"`
	Name         string           `json:"name"`
	Aggregation  Aggregation      `json:"aggregation,omitempty"`
	Role         DimensionRole    `json:"role,omitempty"`
	Geography    bool             `json:"geography,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	Description  string           `json:"description,omitempty"`
	Aliases      []string         `json:"aliases,omitempty"`
	Unit         string           `json:"unit,omitempty"`
	SemanticRole SemanticRole     `json:"semantic_role,omitempty"`
	Values       []GovernedValue  `json:"values,omitempty"`
	Temporal     *TemporalPolicy  `json:"temporal,omitempty"`
	Filters      []SemanticFilter `json:"filters,omitempty"`
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
	return ApplyRichEnhancements(model, version, proposals, nil, nil)
}

// ApplyRichEnhancements adds evidence-bearing KPI and relationship proposals to
// the same immutable draft revision as column enhancement. Relationship proposals
// remain candidate/rejected evidence and therefore cannot become executable joins
// without a later explicit entity mutation and normal review/publication.
func ApplyRichEnhancements(model Model, version string, proposals []Enhancement, kpis []KPI, relationships []RelationshipDecision) (Model, error) {
	if model.Digest() == "" || version == "" || len(proposals) < 1 || len(proposals) > 32 {
		return Model{}, invalid(CodeInvalidValue, "enhancements")
	}
	if len(kpis) > 32 || len(relationships) > 64 {
		return Model{}, invalid(CodeLimit, "enhancements")
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
			if !validLine(item.Name, 256) || !item.Aggregation.valid() || item.Role != "" || item.Geography || item.Reason != "" || !validText(item.Description, 4096) || !validAliases(item.Aliases) || !validOptionalLine(item.Unit, 64) || !item.SemanticRole.valid() || len(item.Values) != 0 || item.Temporal != nil || !validFilters(item.Filters) {
				return Model{}, invalid(CodeInvalidValue, "enhancements.measure")
			}
		case EnhancementDimension:
			if !validLine(item.Name, 256) || !item.Role.valid() || item.Geography && item.Role != DimensionCategorical || item.Aggregation != "" || item.Reason != "" || !validText(item.Description, 4096) || !validAliases(item.Aliases) || item.Unit != "" || !item.SemanticRole.valid() || !validGovernedValues(item.Values) || !validTemporal(item.Temporal, item.Role) || !validFilters(item.Filters) {
				return Model{}, invalid(CodeInvalidValue, "enhancements.dimension")
			}
		case EnhancementUnresolved:
			if item.Name != "" || item.Aggregation != "" || item.Role != "" || item.Geography || !validLine(item.Reason, 256) || item.Description != "" || len(item.Aliases) != 0 || item.Unit != "" || item.SemanticRole != "" || len(item.Values) != 0 || item.Temporal != nil || len(item.Filters) != 0 {
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
		for i := range p.Datasets {
			if p.Datasets[i].ID == item.Dataset {
				for j := range p.Datasets[i].Columns {
					if p.Datasets[i].Columns[j].ID == item.Column {
						p.Datasets[i].Columns[j].SemanticRole = item.SemanticRole
						p.Datasets[i].Columns[j].Aliases = append([]string(nil), item.Aliases...)
					}
				}
			}
		}
		id := GeneratedEntityID(item.Kind, item.Dataset, item.Column)
		switch item.Kind {
		case EnhancementMeasure:
			p.Measures = append(p.Measures, Measure{ID: id, Name: item.Name, Description: item.Description, Field: ref, Aggregation: item.Aggregation, Unit: item.Unit, Aliases: append([]string(nil), item.Aliases...), Filters: cloneFilters(item.Filters)})
		case EnhancementDimension:
			p.Dimensions = append(p.Dimensions, Dimension{ID: id, Name: item.Name, Description: item.Description, Field: ref, Role: item.Role, Geography: item.Geography, Aliases: append([]string(nil), item.Aliases...), Values: append([]GovernedValue(nil), item.Values...), Temporal: item.Temporal, Filters: cloneFilters(item.Filters)})
		case EnhancementUnresolved:
			p.Unresolved = append(p.Unresolved, UnresolvedSemantic{ID: id, Dataset: item.Dataset, Column: item.Column, Reason: item.Reason})
		}
	}
	var err error
	p.KPIs, err = mergeEnhancementEntities(p.KPIs, kpis, func(value KPI) string { return value.ID }, "enhancements.kpis")
	if err != nil {
		return Model{}, err
	}
	p.RelationshipDecisions, err = mergeEnhancementEntities(p.RelationshipDecisions, relationships, func(value RelationshipDecision) string { return value.ID }, "enhancements.relationships")
	if err != nil {
		return Model{}, err
	}
	return Compile(p)
}

func mergeEnhancementEntities[T any](existing, proposed []T, identify func(T) string, path string) ([]T, error) {
	out := append([]T(nil), existing...)
	index := map[string]int{}
	for i, value := range out {
		index[identify(value)] = i
	}
	for _, value := range proposed {
		id := identify(value)
		if prior, ok := index[id]; ok {
			if reflect.DeepEqual(out[prior], value) {
				continue
			}
			return nil, invalid(CodeDuplicateID, path)
		}
		index[id] = len(out)
		out = append(out, value)
	}
	return out, nil
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
