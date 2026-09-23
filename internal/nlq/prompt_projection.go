package nlq

import (
	"encoding/json"
	"strings"
)

// DependencyRelations projects exact physical coordinates from already admitted
// semantic dependencies. It is shared by mandatory context and atomic optional
// candidates. It grants no authority and does not change the complete relation
// scope retained for native validation, persistence and replay.
func DependencyRelations(topic string, relations []SourceRelation, dependencies []MetricDependency) ([]SourceRelation, error) {
	if !validID(topic) || len(relations) == 0 || len(relations) > 128 || len(dependencies) == 0 || len(dependencies) > 4096 {
		return nil, &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
	}
	needed := map[string]map[string]bool{}
	for _, d := range dependencies {
		if d.Kind != "column" {
			continue
		}
		var value struct {
			Dataset string `json:"dataset"`
			Column  struct {
				ID         string `json:"id"`
				SourceName string `json:"source_name"`
			} `json:"column"`
		}
		if !validText(d.Text, 16<<10) || json.Unmarshal([]byte(d.Text), &value) != nil || !validID(value.Dataset) || !validID(value.Column.ID) || !validText(value.Column.SourceName, 256) || d.ID != value.Dataset+":"+value.Column.ID {
			return nil, &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
		}
		if needed[value.Dataset] == nil {
			needed[value.Dataset] = map[string]bool{}
		}
		needed[value.Dataset][value.Column.SourceName] = true
	}
	if len(needed) == 0 {
		return nil, &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
	}
	matched := map[string]bool{}
	var projected []SourceRelation
	for _, relation := range relations {
		columns, ok := needed[relation.Dataset]
		if !ok {
			continue
		}
		if relation.Topic != topic || matched[relation.Dataset] || !validText(relation.Name, 256) || !strings.Contains(relation.Name, ".") || len(relation.Columns) > 256 {
			return nil, &ValidationError{Code: CodeInvalidValue, Path: "projection.relations"}
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
			return nil, &ValidationError{Code: CodeInvalidValue, Path: "projection.columns"}
		}
		matched[relation.Dataset] = true
		projected = append(projected, item)
	}
	if len(matched) != len(needed) {
		return nil, &ValidationError{Code: CodeInvalidValue, Path: "projection.relations"}
	}
	return projected, nil
}

// promptRelations depends only on mandatory state, never optional candidates
// that can be pruned after the base is rendered. Legacy/unselected and multi-topic
// contexts keep their existing full rendering.
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
	var dependencies []MetricDependency
	for _, m := range input.Metrics {
		dependencies = append(dependencies, m.Dependencies...)
	}
	for _, c := range input.Constraints.Required {
		if c.Kind != "semantic_dependency" {
			continue
		}
		var d MetricDependency
		if json.Unmarshal([]byte(c.Text), &d) != nil {
			return nil, false, &ValidationError{Code: CodeInvalidValue, Path: "projection.dependencies"}
		}
		dependencies = append(dependencies, d)
	}
	projected, err := DependencyRelations(input.Topic, input.Relations, dependencies)
	return projected, err == nil, err
}

func renderPromptRelations(input ContextInput) string {
	relations, projected, err := promptRelations(input)
	if err != nil {
		// cloneAndValidateInput rejects this before any sealed context exists.
		return ""
	}
	rendered := renderRelations(relations)
	if !projected {
		return rendered
	}
	// Optional semantic candidates carry their own verified physical mappings,
	// admitted or omitted with their definitions as one bounded group. Do not
	// falsely tell the generator that this mandatory subset is the entire scope.
	return "physical_projection:selected-closure-v1\n" + strings.Replace(rendered,
		"Use only these reviewed schema-qualified physical relations and columns for SQL:",
		"Selected reviewed physical relations and columns (retained semantic candidates may supply additional reviewed mappings):", 1)
}
