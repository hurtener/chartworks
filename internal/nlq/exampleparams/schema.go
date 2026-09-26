// Package exampleparams owns the value-free binding contract of a reviewed SQL
// demonstration. It neither parses/approves SQL nor authorizes or executes it.
package exampleparams

import (
	"errors"
)

// Version identifies the bounded value-free learned-example contract.
const Version = "example-parameters-v1"

// MaxSlots matches the native execution parameter limit.
const MaxSlots = 64

// ErrInvalid deliberately carries no parameter content.
var ErrInvalid = errors.New("learning: invalid example parameter schema")

// Slot has no value/default field. Position is one-based and dense in SQL marker
// order. The query producing an example never lends its values to later queries.
type Slot struct {
	Position int    `json:"position"`
	Kind     string `json:"kind"`
}

// Schema is optional on historical parameter-free examples. Versioned examples
// must have at least one slot; an empty schema is not equivalent to legacy nil.
type Schema struct {
	Version string `json:"version"`
	Slots   []Slot `json:"slots"`
}

// New copies the kinds into a dense one-based schema after checking bounds.
func New(kinds []string) (*Schema, error) {
	if len(kinds) < 1 || len(kinds) > MaxSlots {
		return nil, ErrInvalid
	}
	s := &Schema{Version: Version, Slots: make([]Slot, len(kinds))}
	for i, k := range kinds {
		s.Slots[i] = Slot{Position: i + 1, Kind: k}
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Validate accepts legacy nil or the exact bounded versioned slot contract.
func (s *Schema) Validate() error {
	if s == nil {
		return nil
	}
	if s.Version != Version || len(s.Slots) < 1 || len(s.Slots) > MaxSlots {
		return ErrInvalid
	}
	for i, slot := range s.Slots {
		if slot.Position != i+1 {
			return ErrInvalid
		}
		switch slot.Kind {
		case "null", "text", "boolean", "integer", "number":
		default:
			return ErrInvalid
		}
	}
	return nil
}

// Clone detaches a trusted schema, preserving legacy nil.
func (s *Schema) Clone() *Schema {
	if s == nil {
		return nil
	}
	out := *s
	out.Slots = append([]Slot(nil), s.Slots...)
	return &out
}

// ProbeValues supplies public, fixed validation-only placeholders, never values
// to be persisted or demonstrated to the generator. Native dry planning must
// still accept them. Type-dependent casts that reject these probes remain
// unreviewable; callers must never retry using historical private bindings.
func (s *Schema) ProbeValues() ([]string, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}
	values := make([]string, len(s.Slots))
	for i, slot := range s.Slots {
		switch slot.Kind {
		case "text":
			values[i] = "example"
		case "integer", "number":
			values[i] = "1"
		case "boolean":
			values[i] = "true"
		case "null":
			values[i] = ""
		}
	}
	return values, nil
}
