package reporting

import (
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

func validateDeclarations(parameters []Parameter, max int) error {
	if len(parameters) > max {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range parameters {
		if !identity.Identifier(p.Name) || seen[p.Name] || !slices.Contains([]string{"date", "datetime", "relative_period", "dimension_value", "number", "integer", "boolean", "grain", "top_n"}, p.Type) || len(p.Enum) > 256 {
			return ErrInvalid
		}
		seen[p.Name] = true
		if p.Type == "dimension_value" {
			if p.Dimension == nil || !identity.Identifier(p.Dimension.Topic) || !identity.Identifier(p.Dimension.Version) || !identity.Identifier(p.Dimension.Dimension) {
				return ErrInvalid
			}
		} else if p.Dimension != nil {
			return ErrInvalid
		}
		if slices.Contains([]string{"relative_period", "dimension_value", "boolean", "grain"}, p.Type) && (p.Min != "" || p.Max != "") {
			return ErrInvalid
		}
		if p.Type == "relative_period" && len(p.Enum) != 0 {
			return ErrInvalid
		}
		base := p
		base.Min, base.Max, base.Enum = "", "", nil
		for _, bound := range []string{p.Min, p.Max} {
			if bound != "" {
				if _, err := scalar(base, bound); err != nil {
					return err
				}
			}
		}
		if p.Min != "" && p.Max != "" {
			comparison, err := compareScalar(p.Type, p.Min, p.Max)
			if err != nil || comparison > 0 {
				return ErrInvalid
			}
		}
		values := map[string]bool{}
		for _, value := range p.Enum {
			if values[value] {
				return ErrInvalid
			}
			values[value] = true
			withoutEnum := p
			withoutEnum.Enum = nil
			if _, err := scalar(withoutEnum, value); err != nil {
				return err
			}
		}
		if p.Default != nil {
			if p.Type == "relative_period" {
				if p.Default.Period == nil || p.Default.Literal != "" || validatePeriod(*p.Default.Period) != nil {
					return ErrInvalid
				}
			} else if p.Default.Period != nil {
				return ErrInvalid
			} else if _, err := scalar(p, p.Default.Literal); err != nil {
				return err
			}
		}
	}
	return nil
}

func decimalRat(s string) (*big.Rat, bool) {
	if len(s) > 256 || !exec.Decimal(s) {
		return nil, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	return r, ok
}

func scalar(p Parameter, literal string) (exec.Parameter, error) {
	out := exec.Parameter{Kind: "text", Value: literal}
	if !text(literal, 4096) {
		return out, ErrInvalid
	}
	switch p.Type {
	case "date":
		if t, err := time.Parse("2006-01-02", literal); err != nil || t.Year() < 1 || t.Format("2006-01-02") != literal {
			return out, ErrInvalid
		}
	case "datetime":
		if t, err := time.Parse(time.RFC3339Nano, literal); err != nil || t.Year() < 1 || t.Year() > 9999 {
			return out, ErrInvalid
		}
	case "dimension_value":
		// A dimension value is always a bind value, including an empty category.
	case "number":
		if _, ok := decimalRat(literal); !ok {
			return out, ErrInvalid
		}
		out.Kind = "number"
	case "integer", "top_n":
		n, err := strconv.ParseInt(literal, 10, 64)
		if err != nil || strconv.FormatInt(n, 10) != literal || p.Type == "top_n" && (n < 1 || n > 10000) {
			return out, ErrInvalid
		}
		out.Kind = "integer"
	case "boolean":
		if literal != "true" && literal != "false" {
			return out, ErrInvalid
		}
		out.Kind = "boolean"
	case "grain":
		if !slices.Contains([]string{"second", "minute", "hour", "day", "week", "month", "quarter", "year"}, literal) {
			return out, ErrInvalid
		}
	default:
		return out, ErrInvalid
	}
	if len(p.Enum) > 0 && !slices.Contains(p.Enum, literal) {
		return out, ErrInvalid
	}
	for _, bound := range []struct {
		value string
		lower bool
	}{{p.Min, true}, {p.Max, false}} {
		if bound.value == "" {
			continue
		}
		comparison, err := compareScalar(p.Type, literal, bound.value)
		if err != nil || bound.lower && comparison < 0 || !bound.lower && comparison > 0 {
			return out, ErrInvalid
		}
	}
	return out, nil
}

func compareScalar(kind, a, b string) (int, error) {
	switch kind {
	case "number", "integer", "top_n":
		x, ok := decimalRat(a)
		y, good := decimalRat(b)
		if !ok || !good {
			return 0, ErrInvalid
		}
		return x.Cmp(y), nil
	case "date", "datetime":
		layout := time.RFC3339Nano
		if kind == "date" {
			layout = "2006-01-02"
		}
		x, e1 := time.Parse(layout, a)
		y, e2 := time.Parse(layout, b)
		if e1 != nil || e2 != nil {
			return 0, ErrInvalid
		}
		return x.Compare(y), nil
	default:
		return 0, ErrInvalid
	}
}

// ResolveParameters applies block defaults and explicit invocation values only.
// Future report/filter precedence is owned by phase 29. Unknown/duplicate values
// fail closed. Periods occupy two consecutive scalar slots (start, end).
func ResolveParameters(parameters []Parameter, arguments []Argument, resolution Resolution) (Resolved, error) {
	out := Resolved{Values: []BoundValue{}, Parameters: []exec.Parameter{}, At: resolution.At.UTC(), Timezone: resolution.Timezone}
	if validateDeclarations(parameters, 64) != nil || len(arguments) > len(parameters) || resolution.At.IsZero() || resolution.At.Year() < 1 || resolution.At.Year() > 9999 {
		return out, ErrInvalid
	}
	zone, err := namedZone(resolution.Timezone)
	if err != nil {
		return out, err
	}
	if resolution.ScheduleWindow != nil && !windowValid(*resolution.ScheduleWindow) {
		return out, ErrInvalid
	}
	provided := map[string]Value{}
	known := map[string]bool{}
	for _, p := range parameters {
		known[p.Name] = true
	}
	for _, a := range arguments {
		if _, duplicate := provided[a.Name]; duplicate || !known[a.Name] {
			return out, ErrInvalid
		}
		provided[a.Name] = clone(a.Value)
	}
	for _, p := range parameters {
		v, explicit := provided[p.Name]
		provenance := "invocation"
		if !explicit {
			if p.Default != nil {
				v, provenance = clone(*p.Default), "block_default"
			} else if p.Required {
				return out, ErrInvalid
			} else {
				nulls := []exec.Parameter{{Kind: "null"}}
				if p.Type == "relative_period" {
					nulls = append(nulls, exec.Parameter{Kind: "null"})
				}
				out.Parameters = append(out.Parameters, nulls...)
				out.Values = append(out.Values, BoundValue{Name: p.Name, Type: p.Type, Provenance: "omitted", Digest: digest(nulls)})
				continue
			}
		}
		value := BoundValue{Name: p.Name, Type: p.Type, Provenance: provenance}
		if p.Type == "relative_period" {
			if v.Period == nil || v.Literal != "" {
				return out, ErrInvalid
			}
			w, err := resolvePeriod(*v.Period, resolution, zone)
			if err != nil {
				return out, err
			}
			value.Window = &w
			bound := []exec.Parameter{{Kind: "text", Value: w.Start.UTC().Format(time.RFC3339Nano)}, {Kind: "text", Value: w.End.UTC().Format(time.RFC3339Nano)}}
			value.Digest = digest(bound)
			out.Parameters = append(out.Parameters, bound...)
		} else {
			if v.Period != nil {
				return out, ErrInvalid
			}
			bound, err := scalar(p, v.Literal)
			if err != nil {
				return out, err
			}
			value.Digest = digest(bound)
			out.Parameters = append(out.Parameters, bound)
		}
		out.Values = append(out.Values, value)
	}
	if len(out.Parameters) > 64 {
		return Resolved{}, ErrInvalid
	}
	return out, nil
}

func scalarSlots(parameters []Parameter) int {
	n := 0
	for _, p := range parameters {
		n++
		if p.Type == "relative_period" {
			n++
		}
	}
	return n
}

// scalarDefaults preserves exact, source-backed capture values without inventing
// a certified parameter declaration. Captured placeholders retain their order.
func scalarDefaults(values []exec.Parameter) ([]Parameter, error) {
	out := make([]Parameter, 0, len(values))
	for i, v := range values {
		kind := v.Kind
		if kind == "text" {
			// Capture cannot guess which reviewed dimension an arbitrary text value
			// denotes. A typed manual amendment is required rather than fake lineage.
			return nil, ErrInvalid
		}
		if !slices.Contains([]string{"number", "integer", "boolean"}, kind) {
			return nil, ErrInvalid
		}
		p := Parameter{Name: "p" + strconv.Itoa(i+1), Type: kind, Required: true, Default: &Value{Literal: v.Value}}
		if _, err := scalar(p, v.Value); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func safeIdentifierToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return !strings.ContainsAny(s, "\x00\r\n")
}

// parameterDigest canonicalizes zero bind values independently of nil slices.
func parameterDigest(values []exec.Parameter) string {
	return digest(append([]exec.Parameter{}, values...))
}
