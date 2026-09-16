package exec

import (
	"encoding/hex"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// BusinessConstraint is a closed row-filter contract. Its coordinates come from
// reviewed semantics and current source metadata, never a display label. It is
// not authority and cannot construct an executable Plan.
type BusinessConstraint struct {
	Resolution string `json:"resolution"`
	Dataset string `json:"dataset"`
	Column string `json:"column"`
	SourceRevision int64 `json:"source_revision"`
	Kind string `json:"kind"`
	Operator string `json:"operator"`
	Nulls string `json:"nulls"`
	Unit string `json:"unit,omitempty"`
	Precision int `json:"precision,omitempty"`
	Scale int `json:"scale,omitempty"`
	Bounds string `json:"bounds,omitempty"`
	TemporalType string `json:"temporal_type,omitempty"`
	Calendar string `json:"calendar,omitempty"`
	TimeZone string `json:"time_zone,omitempty"`
	Grain string `json:"grain,omitempty"`
	Value string `json:"value,omitempty"`
	Upper string `json:"upper,omitempty"`
	Null bool `json:"null,omitempty"`
}

// BusinessConstraintError exposes a content-free, actionable binding failure.
type BusinessConstraintError struct {
	Code string `json:"code"`
	Resolution string `json:"resolution,omitempty"`
	Field string `json:"field"`
}
func (e *BusinessConstraintError) Error() string { return "exec: business constraint could not be bound" }
func (e *BusinessConstraintError) Unwrap() error {
	if strings.HasPrefix(e.Code, "unsupported") { return ErrUnsupported }
	return ErrBinding
}
func (BusinessConstraint) String() string { return "business-constraint(redacted)" }
func (c BusinessConstraint) GoString() string { return c.String() }

func businessError(c BusinessConstraint, field, code string) error {
	return &BusinessConstraintError{Code: code, Resolution: c.Resolution, Field: field}
}

// ValidateBusinessConstraints runs before model work and again before binding.
// The existing validator still owns whole-statement safety, authority, native
// planning, and executable plan issuance after the constraints are applied.
func ValidateBusinessConstraints(binding Binding, constraints []BusinessConstraint) error {
	if !binding.Valid() || len(constraints) > 64 { return ErrBinding }
	seen := map[string]bool{}
	for _, c := range constraints {
		digest, err := hex.DecodeString(c.Resolution)
		if err != nil || len(digest) != 32 || strings.ToLower(c.Resolution) != c.Resolution || seen[c.Resolution] || c.SourceRevision != binding.Revision {
			return businessError(c, "resolution", "stale_constraint")
		}
		seen[c.Resolution] = true
		var column Column
		found := false
		for _, relation := range binding.Relations {
			if relation.ID != c.Dataset { continue }
			for _, candidate := range relation.Columns {
				if candidate.Name == c.Column && candidate.Safe { column, found = candidate, true; break }
			}
		}
		if !found { return businessError(c, "target", "foreign_constraint_target") }
		if c.Nulls != "exclude" && c.Nulls != "include" && c.Nulls != "only" { return businessError(c, "nulls", "invalid_null_semantics") }
		if !businessColumnCompatible(binding.Dialect, column, c) { return businessError(c, "target", "unsupported_constraint_type") }
		if c.Null || c.Nulls == "only" {
			if !c.Null || c.Nulls == "exclude" || c.Value != "" || c.Upper != "" { return businessError(c, "value", "invalid_null_value") }
			continue
		}
		if !utf8.ValidString(c.Value) || !utf8.ValidString(c.Upper) || strings.ContainsAny(c.Value+c.Upper, "\x00\r\n") { return businessError(c, "value", "invalid_value") }
		switch c.Kind {
		case "number":
			maxPrecision, maxScale := 38, 38
			switch binding.Dialect { case "postgres", "bigquery": maxPrecision = 76; case "mysql": maxPrecision, maxScale = 65, 30 }
			if c.Precision < 1 || c.Precision > maxPrecision || c.Scale < 0 || c.Scale > c.Precision || c.Scale > maxScale || c.Unit == "" || len(c.Unit) > 64 { return businessError(c, "precision", "unsupported_exact_precision") }
			lower, ok := businessDecimal(c.Value, c.Precision, c.Scale)
			if !ok { return businessError(c, "value", "invalid_number") }
			if c.Operator == "range" {
				upper, ok := businessDecimal(c.Upper, c.Precision, c.Scale)
				if !ok || (c.Bounds != "[]" && c.Bounds != "[)" && c.Bounds != "(]" && c.Bounds != "()") || lower.Cmp(upper) > 0 || lower.Cmp(upper) == 0 && c.Bounds != "[]" { return businessError(c, "upper", "invalid_range") }
			} else if c.Upper != "" || c.Bounds != "" || !businessOperator(c.Operator) { return businessError(c, "operator", "invalid_operator") }
		case "boolean":
			if c.Operator != "eq" || (c.Value != "true" && c.Value != "false") || c.Upper != "" { return businessError(c, "value", "invalid_boolean") }
		case "entity", "text":
			if c.Value == "" || len(c.Value) > 512 || c.Upper != "" || (c.Operator != "eq" && c.Operator != "ne") { return businessError(c, "value", "invalid_governed_value") }
		case "time_window":
			if c.Operator != "range" || c.Bounds != "[)" || c.Calendar != "gregorian" || c.Grain != "day" && c.Grain != "week" && c.Grain != "month" && c.Grain != "quarter" && c.Grain != "year" { return businessError(c, "time", "unsupported_time_contract") }
			if _, err := time.LoadLocation(c.TimeZone); err != nil { return businessError(c, "time_zone", "invalid_time_zone") }
			layout := "2006-01-02"
			if c.TemporalType == "timestamptz" { layout = time.RFC3339 }
			lower, err := time.Parse(layout, c.Value)
			upper, upperErr := time.Parse(layout, c.Upper)
			if err != nil || upperErr != nil || !lower.Before(upper) { return businessError(c, "time", "invalid_time_bounds") }
		default:
			return businessError(c, "kind", "unsupported_constraint_kind")
		}
	}
	return nil
}

func businessOperator(op string) bool { return op == "eq" || op == "ne" || op == "lt" || op == "lte" || op == "gt" || op == "gte" }

func businessDecimal(value string, precision, scale int) (*big.Rat, bool) {
	if len(value) == 0 || len(value) > 128 || !regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`).MatchString(value) { return nil, false }
	parts := strings.Split(strings.TrimPrefix(value, "-"), ".")
	fraction := 0
	if len(parts) == 2 { fraction = len(parts[1]) }
	if fraction > scale || len(strings.TrimLeft(parts[0], "0")) > precision-scale { return nil, false }
	out, ok := new(big.Rat).SetString(value)
	return out, ok
}

func businessColumnCompatible(dialect string, column Column, c BusinessConstraint) bool {
	native := strings.ToLower(column.NativeType)
	category := strings.ToLower(column.Category)
	switch c.Kind {
	case "number":
		return category == "integer" || category == "number" || category == "decimal" || category == "numeric" || strings.HasPrefix(native, "numeric") || strings.HasPrefix(native, "decimal") || strings.HasPrefix(native, "number") || native == "bigint" || native == "integer" || native == "int" || native == "smallint" || native == "int64"
	case "boolean":
		return category == "boolean" || native == "boolean" || native == "bool" || native == "bit"
	case "entity", "text":
		return category == "text" || category == "string" || strings.Contains(native, "char") || native == "text" || native == "string"
	case "time_window":
		switch c.TemporalType {
		case "date": return native == "date"
		case "timestamp": return native == "timestamp" && dialect != "bigquery" || strings.HasPrefix(native, "timestamp without time zone") || strings.HasPrefix(native, "timestamp_ntz") || strings.HasPrefix(native, "datetime") && !strings.HasPrefix(native, "datetimeoffset")
		case "timestamptz": return native == "timestamptz" || strings.HasPrefix(native, "timestamp with time zone") || strings.HasPrefix(native, "timestamp_tz") || strings.HasPrefix(native, "datetimeoffset") || native == "timestamp" && (dialect == "bigquery" || dialect == "databricks")
		}
	}
	return false
}
