package nlq

import (
	"encoding/json"
	"strings"
)

// promptRelations derives a generation-only projection from the mandatory
// selected semantic closure. Relations remains the complete reviewed physical
// scope in the sealed context: native admission, persistence and replay still
// compare it exactly. No retrieved optional text can widen this projection.
// Legacy/unselected and multi-topic contexts keep their existing full rendering.
func promptRelations(input ContextInput) ([]SourceRelation, bool, error) {
	selected := false
	if input.Constraints != nil {
		for _, c := range input.Constraints.Required {
			selected = selected || c.Kind == "selected_semantics"
		}
	}
	if !selected || input.Strategy != StrategySingleTopic || len(input.Metrics) == 0 || len(input.Relations) == 0 {
		return input.Relations, false, nil
	}
	needed := map[string]map[string]bool{}
	add := func(d MetricDependency) error {
		if d.Kind != "column" {
			return nil
		}
		// Only read stable dataset/physical column coordinates. Full native type,
		// nullability, IDs and reviewed meaning remain in the mandatory closure.
		var value struct {
			Dataset string `json:"dataset"`
			Column  struct {
				ID         string `json:"id"`
				SourceName string `json:"source_name"`
			} `json:"column"`
		}
		if json.Unmarshal([]byte(d.Text), &value) != nil || !validID(value.Dataset) || !validID(value.Column.ID) || !validText(value.Column.SourceName, 256) || d.ID != value.Dataset+":"+value.Column.ID {
			return &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
		}
		if needed[value.Dataset] == nil {
			needed[value.Dataset] = map[string]bool{}
		}
		needed[value.Dataset][value.Column.SourceName] = true
		return nil
	}
	for _, m := range input.Metrics {
		for _, d := range m.Dependencies {
			if err := add(d); err != nil {
				return nil, false, err
			}
		}
	}
	for _, c := range input.Constraints.Required {
		if c.Kind != "semantic_dependency" {
			continue
		}
		var d MetricDependency
		if json.Unmarshal([]byte(c.Text), &d) != nil {
			return nil, false, &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
		}
		if err := add(d); err != nil {
			return nil, false, err
		}
	}
	if len(needed) == 0 {
		return nil, false, &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
	}
	matched := map[string]bool{}
	var projected []SourceRelation
	for _, relation := range input.Relations {
		columns, ok := needed[relation.Dataset]
		if !ok {
			continue
		}
		if relation.Topic != input.Topic || matched[relation.Dataset] {
			return nil, false, &ValidationError{Code: CodeInvalidValue, Path: "projection.relations"}
		}
		item := SourceRelation{Topic: relation.Topic, Dataset: relation.Dataset, Name: relation.Name}
		seen := map[string]bool{}
		for _, column := range relation.Columns {
			if columns[column] && !seen[column] {
				item.Columns = append(item.Columns, column)
				seen[column] = true
			}
		}
		if len(seen) != len(columns) {
			return nil, false, &ValidationError{Code: CodeInvalidValue, Path: "projection.columns"}
		}
		matched[relation.Dataset] = true
		projected = append(projected, item)
	}
	if len(matched) != len(needed) {
		return nil, false, &ValidationError{Code: CodeInvalidValue, Path: "projection.relations"}
	}
	return projected, true, nil
}

func renderPromptRelations(input ContextInput) string {
	relations, projected, err := promptRelations(input)
	if err != nil {
		// cloneAndValidateInput rejects this before any sealed context exists.
		return ""
	}
	var b strings.Builder
	if projected {
		b.WriteString("physical_projection:selected-closure-v1\n")
	}
	b.WriteString(renderRelations(relations))
	return b.String()
}
