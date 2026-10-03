package drafts

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"

	"github.com/hurtener/chartworks/internal/gateway"
)

const authoringColumnEncoding = "profile-column-table-v1"
const authoringColumnInstructions = " Profile evidence columns use profile-column-table-v1: each rows entry contains all values in the exact fields order. No aggregate or disclosure is omitted by this lossless table encoding."

func authoringColumnFields() []string {
	return []string{"column", "observed", "nulls", "sample_distinct", "distinct_exact", "families", "disclosure"}
}

// authoringColumns changes only the prompt representation of aggregate facts.
// Every field is named once per table, every row has every value, and the context
// digest binds this exact version, header and content. Origins, catalog keys and
// business semantics remain explicit in the surrounding evidence and candidate.
// This is never a stored topic, review or checkpoint representation.
type authoringColumns []authoringAggregate

func (columns authoringColumns) MarshalJSON() ([]byte, error) {
	rows := make([][7]any, len(columns))
	for i, c := range columns {
		rows[i] = [7]any{c.Column, c.Observed, c.Nulls, c.Distinct, c.Exact, c.Families, c.Disclosure}
	}
	return json.Marshal(struct {
		Encoding string   `json:"encoding"`
		Fields   []string `json:"fields"`
		Rows     [][7]any `json:"rows"`
	}{authoringColumnEncoding, authoringColumnFields(), rows})
}

// UnmarshalJSON is the exact inverse for inspecting the internal prompt packet.
// Unknown/missing encodings, shifted headers and ragged or mistyped rows fail
// closed instead of silently acquiring a different aggregate interpretation.
func (columns *authoringColumns) UnmarshalJSON(raw []byte) error {
	if _, err := gateway.DecodeJSON(raw, maxAuthoringContextBytes); err != nil {
		return gateway.ErrInput
	}
	var wire struct {
		Encoding string              `json:"encoding"`
		Fields   []string            `json:"fields"`
		Rows     [][]json.RawMessage `json:"rows"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wire) != nil || decoder.Decode(new(any)) != io.EOF || wire.Encoding != authoringColumnEncoding || !slices.Equal(wire.Fields, authoringColumnFields()) || wire.Rows == nil || len(wire.Rows) > maxAuthoringColumns {
		return gateway.ErrInput
	}
	out := make(authoringColumns, len(wire.Rows))
	for i, row := range wire.Rows {
		if len(row) != 7 {
			return gateway.ErrInput
		}
		c := &out[i]
		values := []any{&c.Column, &c.Observed, &c.Nulls, &c.Distinct, &c.Exact, &c.Families, &c.Disclosure}
		for j, value := range values {
			if j != 5 && bytes.Equal(bytes.TrimSpace(row[j]), []byte("null")) || json.Unmarshal(row[j], value) != nil {
				return gateway.ErrInput
			}
			if j == 5 {
				// Whole families:null means withheld evidence. A null count
				// inside a disclosed family must never acquire integer zero.
				var families map[string]json.RawMessage
				if json.Unmarshal(row[j], &families) != nil {
					return gateway.ErrInput
				}
				for _, count := range families {
					if bytes.Equal(bytes.TrimSpace(count), []byte("null")) {
						return gateway.ErrInput
					}
				}
			}
		}
	}
	*columns = out
	return nil
}
