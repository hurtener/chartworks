package reporting

import (
	"slices"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/jackc/pgx/v5"
)

const AuthoringFilteredCompilerVersion = "reviewed-dataset-postgres-v2"

// AuthoringDatasetFilter selects one reviewed dimension or physical catalog column.
// Names, types, physical columns and predicate operators are server-owned.
type AuthoringDatasetFilter struct {
	Dimension string `json:"dimension,omitempty"`
	Column    string `json:"column,omitempty"`
	Calendar  string `json:"calendar,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	Kind      string `json:"kind" jsonschema:"enum=select,enum=multi_select,enum=date_range,enum=range"`
	Default   Value  `json:"default"`
}

// AuthoringCompilerForIntent preserves legacy custody and its exact digest path.
func AuthoringCompilerForIntent(in AuthoringDatasetIntent) string {
	if in.Fields != nil {
		return AuthoringFieldCompilerVersion
	}
	if len(in.Filters) == 0 {
		return AuthoringCompilerVersion
	}
	return AuthoringFilteredCompilerVersion
}
func authoringTextColumn(c semantics.Column) bool {
	return (c.Category == "text" || c.Category == "string") && slices.Contains([]string{"text", "varchar", "character varying"}, c.NativeType)
}
func authoringDateColumn(c semantics.Column) bool {
	return (c.Category == "date" || c.Category == "temporal") && c.NativeType == "date"
}

func compileAuthoringFilters(in AuthoringDatasetIntent, publication topics.Published, binding exec.Binding, resolve func(semantics.Reference) (semantics.Column, error)) ([]string, []Parameter, error) {
	predicates := []string{}
	parameters := []Parameter{}
	if len(in.Filters) > 4 {
		return nil, nil, unsupportedPreparation("filter_limit_exceeded")
	}
	filters := slices.Clone(in.Filters)
	slices.SortFunc(filters, func(a, b AuthoringDatasetFilter) int {
		return strings.Compare(authoringFilterKey(a), authoringFilterKey(b))
	})
	previous := ""
	for _, filter := range filters {
		key := authoringFilterKey(filter)
		if key == "" || key == previous {
			return nil, nil, ErrInvalid
		}
		previous = key
		if filter.Column != "" {
			predicate, parameter, err := compileAuthoringColumnFilter(in, filter, binding, scalarSlots(parameters)+1, resolve)
			if err != nil {
				return nil, nil, err
			}
			predicates = append(predicates, predicate...)
			parameters = append(parameters, parameter)
			continue
		}
		if filter.Calendar != "" || filter.Timezone != "" {
			return nil, nil, ErrInvalid
		}
		var dimension *semantics.Dimension
		for i := range publication.Definition.Dimensions {
			d := &publication.Definition.Dimensions[i]
			if d.ID == filter.Dimension {
				if dimension != nil {
					return nil, nil, ErrInvalid
				}
				dimension = d
			}
		}
		if dimension == nil {
			return nil, nil, ErrInvalid
		}
		if reason := dimensionUnsupported(*dimension, in.Dataset); reason != "" {
			return nil, nil, unsupportedPreparation(reason)
		}
		column, err := resolve(dimension.Field)
		if err != nil {
			return nil, nil, err
		}
		name := pgx.Identifier{column.SourceName}.Sanitize()
		parameter := Parameter{Name: semanticBinding("f", dimension.ID), Required: true, Default: clone(&filter.Default), Dimension: &DimensionReference{Topic: in.Topic.Topic, Version: in.Topic.Version, Dimension: dimension.ID}}
		slot := scalarSlots(parameters) + 1
		switch filter.Kind {
		case "select":
			if !authoringTextColumn(column) {
				return nil, nil, unsupportedPreparation("filter_type_unsupported")
			}
			parameter.Type = "dimension_value"
			predicates = append(predicates, name+" = $"+strconv.Itoa(slot))
		case "multi_select":
			if !authoringTextColumn(column) {
				return nil, nil, unsupportedPreparation("filter_type_unsupported")
			}
			parameter.Type = "dimension_set"
			markers := make([]string, DimensionSetCapacity)
			for i := range markers {
				markers[i] = "$" + strconv.Itoa(slot+i)
			}
			predicates = append(predicates, name+" IN ("+strings.Join(markers, ", ")+")")
			slices.Sort(parameter.Default.Items)
		case "date_range":
			if !authoringDateColumn(column) || dimension.Role != semantics.DimensionTemporal {
				return nil, nil, unsupportedPreparation("filter_type_unsupported")
			}
			parameter.Type = "date_range"
			predicates = append(predicates, name+" >= $"+strconv.Itoa(slot), name+" < $"+strconv.Itoa(slot+1))
		default:
			return nil, nil, unsupportedPreparation("filter_kind_unsupported")
		}
		parameters = append(parameters, parameter)
	}
	if err := validateDeclarations(parameters, 4); err != nil {
		return nil, nil, err
	}
	return predicates, parameters, nil
}
