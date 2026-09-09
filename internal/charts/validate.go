package charts

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

func identifier(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_.:-", c) {
			continue
		}
		return false
	}
	return true
}

func text(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if c < 0x20 && c != '\n' && c != '\t' || c == 0x7f {
			return false
		}
	}
	return true
}

func oneOf(s string, choices ...string) bool {
	for _, choice := range choices {
		if s == choice {
			return true
		}
	}
	return false
}

func validateColumn(c Column) error {
	if !identifier(c.ID) || c.Name == "" || !text(c.Name, 1024) ||
		!oneOf(c.Type, "integer", "decimal", "number", "text", "temporal", "boolean", "binary", "structured") ||
		!oneOf(c.Role, "unknown", "identifier", "dimension", "time", "measure", "kpi") ||
		!oneOf(c.Grain, "", "second", "minute", "hour", "day", "week", "month", "quarter", "year") ||
		!oneOf(c.Aggregation, "", "sum", "count", "average", "minimum", "maximum", "distinct_count") ||
		!text(c.Format.Unit, 64) || !oneOf(c.Format.Percent, "", "fraction", "whole") || c.Format.FractionDigits < 0 || c.Format.FractionDigits > 20 {
		return ErrInvalid
	}
	if c.Format.Currency != "" {
		if len(c.Format.Currency) != 3 || c.Format.Percent != "" {
			return ErrInvalid
		}
		for _, r := range c.Format.Currency {
			if r < 'A' || r > 'Z' {
				return ErrInvalid
			}
		}
	}
	if !numeric(c.Type) && (c.Role == "measure" || c.Role == "kpi" || c.Aggregation != "" || c.Format.Currency != "" || c.Format.Percent != "" || c.Format.FractionDigits != 0) ||
		c.Role == "time" && c.Type != "temporal" || c.Grain != "" && c.Type != "temporal" {
		return ErrInvalid
	}
	p := c.Provenance
	if p.Version != Version || p.Source == "" && p.SourceRevision != 0 || p.Source != "" && (!identifier(p.Source) || p.SourceRevision < 1) ||
		p.Topic == "" && (p.TopicVersion != "" || p.SemanticID != "") || p.Topic != "" && (!identifier(p.Topic) || !identifier(p.TopicVersion) || !identifier(p.SemanticID)) {
		return ErrInvalid
	}
	return nil
}

// ValidateData validates sizes, types and completeness before selection or output.
func ValidateData(ctx context.Context, d Data, limits Limits) error {
	if ctx == nil || limits.Validate() != nil || d.Version != Version || len(d.Columns) == 0 ||
		!oneOf(d.Completeness.Status, "complete_result", "truncated") ||
		d.Completeness.Status == "complete_result" && d.Completeness.Reason != "" ||
		d.Completeness.Status == "truncated" && !oneOf(d.Completeness.Reason, "rows", "bytes", "source_limit", "unknown") {
		return ErrInvalid
	}
	if len(d.Columns) > limits.MaxColumns || len(d.Rows) > limits.MaxRows {
		return ErrLimit
	}
	seen := make(map[string]bool, len(d.Columns))
	for _, c := range d.Columns {
		if seen[c.ID] || validateColumn(c) != nil {
			return ErrInvalid
		}
		seen[c.ID] = true
	}
	// The structural caps above bound allocation; account encoded bytes per row
	// before serializing the entire dataset or constructing candidate outputs.
	meta, _ := json.Marshal(struct {
		Version      int          `json:"version"`
		Columns      []Column     `json:"columns"`
		Completeness Completeness `json:"completeness"`
	}{d.Version, d.Columns, d.Completeness})
	bytes := len(meta) + len(`,"rows":[]`)
	if bytes > limits.MaxBytes {
		return ErrLimit
	}
	for i, row := range d.Rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(row) != len(d.Columns) {
			return ErrInvalid
		}
		for j, cell := range row {
			if len(cell.Value) > limits.MaxCellBytes {
				return ErrLimit
			}
			if !text(cell.Value, limits.MaxCellBytes) || cell.Null && cell.Value != "" {
				return ErrInvalid
			}
			if cell.Null {
				continue
			}
			switch d.Columns[j].Type {
			case "integer":
				if strings.ContainsAny(cell.Value, ".eE") {
					return ErrInvalid
				}
				if _, _, err := decimal(cell.Value); err != nil {
					return ErrInvalid
				}
			case "decimal", "number":
				if _, _, err := decimal(cell.Value); err != nil {
					return ErrInvalid
				}
			case "binary":
				if len(cell.Value)%2 != 0 {
					return ErrInvalid
				}
				for _, v := range cell.Value {
					if (v < '0' || v > '9') && (v < 'a' || v > 'f') && (v < 'A' || v > 'F') {
						return ErrInvalid
					}
				}
			case "structured":
				if !json.Valid([]byte(cell.Value)) {
					return ErrInvalid
				}
			case "boolean":
				if cell.Value != "true" && cell.Value != "false" {
					return ErrInvalid
				}
			}
		}
		encoded, _ := json.Marshal(row)
		bytes += len(encoded)
		if i > 0 {
			bytes++
		}
		if bytes > limits.MaxBytes {
			return ErrLimit
		}
	}
	return ctx.Err()
}

func optionText(s string) bool {
	lower := strings.ToLower(s)
	return text(s, 512) && !strings.ContainsAny(s, "<>\n\t") &&
		!strings.Contains(lower, "javascript:") && !strings.Contains(lower, "data:") &&
		!strings.Contains(lower, "://") && !strings.HasPrefix(strings.TrimSpace(lower), "//") &&
		!strings.Contains(lower, "file:") && !strings.Contains(lower, "blob:")
}

func validateOptions(o Options, l Limits) error {
	if !optionText(o.Title) || !oneOf(o.Legend.Position, "top", "bottom", "left", "right") || o.LabelMaxRunes < 1 || o.LabelMaxRunes > 1024 {
		return ErrInvalid
	}
	b, _ := json.Marshal(o)
	if len(b) > l.MaxOptionsBytes || l.MaxOptionsDepth < 2 {
		return ErrLimit
	}
	return nil
}

func columnIndex(columns []Column, id string) int {
	for i, c := range columns {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func temporal(s string) (time.Time, bool) {
	for _, format := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999", "2006-01-02", "2006-01", "2006"} {
		if value, err := time.Parse(format, s); err == nil {
			return value, true
		}
	}
	return time.Time{}, false
}

func category(c Column) bool {
	return c.Type == "text" || c.Type == "temporal" || c.Type == "boolean" || numeric(c.Type) && (c.Role == "dimension" || c.Role == "identifier")
}

func measure(c Column) bool {
	return numeric(c.Type) && c.Role != "identifier" && c.Role != "dimension"
}
