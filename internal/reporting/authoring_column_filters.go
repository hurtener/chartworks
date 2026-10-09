package reporting

import (
	"slices"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/jackc/pgx/v5"
)

func authoringFilterKey(f AuthoringDatasetFilter) string {
	if f.Column != "" && f.Dimension == "" {
		return "column:" + f.Column
	}
	if f.Dimension != "" && f.Column == "" {
		// Preserve ordering and serialization for every legacy intent.
		return f.Dimension
	}
	return ""
}

// ColumnFilterCapability advertises metadata-only physical type compatibility.
// Lookup is separate from literal authoring and is never silently executed.
type ColumnFilterCapability struct {
	Type             string   `json:"type"`
	Kinds            []string `json:"kinds"`
	MaxSetSize       int      `json:"max_set_size,omitempty"`
	CalendarRequired bool     `json:"calendar_required,omitempty"`
	TimezoneRequired bool     `json:"timezone_required,omitempty"`
	OptionLookup     bool     `json:"option_lookup"`
}

func authoringColumnFilterCapability(c semantics.Column) *ColumnFilterCapability {
	kind := columnFilterType(c)
	if kind == "" {
		return nil
	}
	out := &ColumnFilterCapability{Type: kind, Kinds: []string{"select", "multi_select"}, MaxSetSize: DimensionSetCapacity, OptionLookup: optionColumnType(kind)}
	if slices.Contains([]string{"number", "integer"}, kind) {
		out.Kinds = append(out.Kinds, "range")
	}
	if slices.Contains([]string{"date", "timestamp", "instant"}, kind) {
		out.Kinds, out.MaxSetSize, out.CalendarRequired = []string{"range"}, 0, true
		out.TimezoneRequired = kind == "instant"
	}
	return out
}

func compileAuthoringColumnFilter(in AuthoringDatasetIntent, f AuthoringDatasetFilter, binding exec.Binding, slot int, resolve func(semantics.Reference) (semantics.Column, error)) ([]string, Parameter, error) {
	if in.Fields == nil || f.Dimension != "" || f.Column == "" || binding.Dialect != "postgres" {
		return nil, Parameter{}, ErrInvalid
	}
	c, err := resolve(semantics.Reference{Kind: semantics.KindColumn, Dataset: in.Dataset, ID: f.Column})
	if err != nil {
		return nil, Parameter{}, err
	}
	capability := authoringColumnFilterCapability(c)
	if capability == nil || !slices.Contains(capability.Kinds, f.Kind) {
		return nil, Parameter{}, unsupportedPreparation("filter_type_unsupported")
	}
	pin := SourceDatasetPin{Source: binding.Source, Context: binding.Context, Dataset: in.Dataset, SourceRevision: binding.Revision}
	for _, relation := range binding.Relations {
		if relation.ID == in.Dataset {
			pin.SchemaDigest = exec.Hash(relation)
		}
	}
	if !pin.valid() || in.SourceDataset != nil && *in.SourceDataset != pin {
		return nil, Parameter{}, ErrStale
	}
	parameter := Parameter{Name: semanticBinding("fc", f.Column), Required: true, Default: clone(&f.Default), Column: &ColumnReference{SourceDataset: pin, Name: c.SourceName, Type: capability.Type, Calendar: f.Calendar, Timezone: f.Timezone}}
	name := pgx.Identifier{c.SourceName}.Sanitize()
	predicates := []string{}
	switch f.Kind {
	case "select":
		parameter.Type = "column_value"
		predicates = append(predicates, name+" = $"+strconv.Itoa(slot))
	case "multi_select":
		parameter.Type = "column_set"
		markers := make([]string, DimensionSetCapacity)
		for i := range markers {
			markers[i] = "$" + strconv.Itoa(slot+i)
		}
		predicates = append(predicates, name+" IN ("+strings.Join(markers, ", ")+")")
		slices.Sort(parameter.Default.Items)
	case "range":
		parameter.Type = "column_range"
		predicates = append(predicates, name+" >= $"+strconv.Itoa(slot), name+" < $"+strconv.Itoa(slot+1))
	}
	if err := validateColumnFilterDeclaration(parameter); err != nil {
		return nil, Parameter{}, err
	}
	return predicates, parameter, nil
}
