package reporting

import (
	"encoding/json"
	"math/big"
	"slices"
	"strconv"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

const ReviewedAmountCompletenessPolicy = "reviewed-amount-completeness-v1"

// AmountDeclaration is reviewed business meaning, not a retained analytical
// proof. Its ordered fields are part of the immutable definition digest.
type AmountDeclaration struct {
	Label              string     `json:"label"`
	Policy             string     `json:"policy"`
	ID                 string     `json:"id"`
	Metric             string     `json:"metric"`
	ValueColumn        int        `json:"value_column"`
	ValueField         exec.Field `json:"value_field"`
	UnknownCountMetric string     `json:"unknown_count_metric"`
	UnknownCountColumn int        `json:"unknown_count_column"`
	UnknownCountField  exec.Field `json:"unknown_count_field"`
}

// AmountOutputBinding explicitly distinguishes an amount from its companion.
// A reviewed count output is supported, but never acquires an amount unit.
type AmountOutputBinding struct {
	Declaration string `json:"declaration"`
	Role        string `json:"role" jsonschema:"enum=amount,enum=unknown_count"`
}

type AmountDisclosure struct {
	ValueField        string                     `json:"value_field"`
	UnknownCountField string                     `json:"unknown_count_field"`
	Label             string                     `json:"label"`
	RowsScope         string                     `json:"rows_scope"`
	QueryOutcome      string                     `json:"query_outcome"`
	Truncation        string                     `json:"truncation,omitempty"`
	Evidence          string                     `json:"evidence"`
	DefinitionDigest  string                     `json:"definition_digest,omitempty"`
	Declaration       string                     `json:"declaration,omitempty"`
	Role              string                     `json:"role"`
	Unit              string                     `json:"unit,omitempty"`
	Result            nlqexec.AmountCompleteness `json:"result"`
}

func seedAmountDeclarations(d *Definition, input []nlqexec.CapturedAmountCompleteness) error {
	if len(input) == 0 {
		return nil
	}
	migrated, err := MigrateDefinition(*d)
	if err != nil {
		return err
	}
	*d = migrated
	for i, item := range input {
		label := item.MetricLabel
		if label == "" {
			label = "Known amount"
		}
		if item.ValueColumn < 0 || item.UnknownCountColumn < 0 || item.ValueColumn >= len(d.ExpectedSchema) || item.UnknownCountColumn >= len(d.ExpectedSchema) {
			return ErrInvalid
		}
		d.AmountCompleteness = append(d.AmountCompleteness, AmountDeclaration{Label: label, Policy: ReviewedAmountCompletenessPolicy, ID: "known-amount-" + strconv.Itoa(i+1), Metric: item.Metric, ValueColumn: item.ValueColumn, ValueField: d.ExpectedSchema[item.ValueColumn], UnknownCountMetric: item.UnknownCountMetric, UnknownCountColumn: item.UnknownCountColumn, UnknownCountField: d.ExpectedSchema[item.UnknownCountColumn]})
	}
	for i := range d.Outputs {
		o := &d.Outputs[i]
		if len(o.AmountCompleteness) != 0 {
			return ErrInvalid
		}
		fields := amountOutputFields(*o)

		if o.Mapping != nil && o.Kind != "table" {
			// One numeric series/KPI cannot mix an amount and a count as
			// interchangeable values, comparison or target. Separate count
			// outputs and independent geometric slots retain their own roles.
			m := o.Mapping
			slots := append([]string{m.Bindings.Value, m.Bindings.Comparison, m.Bindings.Target}, m.Bindings.Values...)
			amount, count := false, false
			for _, column := range m.Columns {
				if !slices.Contains(slots, column.ID) {
					continue
				}
				for _, a := range d.AmountCompleteness {
					amount = amount || column.Name == a.ValueField.Name
					count = count || column.Name == a.UnknownCountField.Name
				}
			}
			if amount && count {
				return ErrInvalid
			}
		}
		for _, a := range d.AmountCompleteness {
			if fields[a.ValueField.Name] {
				o.AmountCompleteness = append(o.AmountCompleteness, AmountOutputBinding{a.ID, "amount"})
			}
			if fields[a.UnknownCountField.Name] {
				o.AmountCompleteness = append(o.AmountCompleteness, AmountOutputBinding{a.ID, "unknown_count"})
			}
		}
	}
	return nil
}

func amountOutputFields(o Output) map[string]bool {
	out := map[string]bool{}
	if o.Narrative != nil {
		for _, f := range o.Narrative.Fields {
			out[f] = true
		}
		return out
	}
	if o.Mapping == nil {
		return out
	}
	m := o.Mapping
	b := m.Bindings
	ids := append([]string{b.Category, b.Value, b.Series, b.X, b.Y, b.Parent, b.Size, b.Comparison, b.Target}, b.Columns...)
	ids = append(ids, b.Values...)
	ids = append(ids, b.Hierarchy...)
	visible := map[string]bool{}
	if m.Kind == "table" && m.Table != nil {
		for _, column := range m.Table.Columns {
			visible[column.Column] = column.Visible
		}
	}
	for _, c := range m.Columns {
		if m.Kind == "table" && m.Table != nil && !visible[c.ID] {
			continue
		}
		if slices.Contains(ids, c.ID) {
			out[c.Name] = true
		}
	}
	return out
}

func validateAmountDeclarations(d Definition) error {
	if len(d.AmountCompleteness) > 64 || len(d.AmountCompleteness) > 0 && d.SchemaVersion != CurrentSchemaVersion {
		return ErrInvalid
	}
	byID := map[string]AmountDeclaration{}
	metrics := map[string]bool{}
	for _, a := range d.AmountCompleteness {
		if a.Label == "" || !text(a.Label, 256) || a.Policy != ReviewedAmountCompletenessPolicy || !identity.Identifier(a.ID) || !text(a.Metric, 512) || a.Metric == "" || !text(a.UnknownCountMetric, 512) || a.UnknownCountMetric == "" || a.Metric == a.UnknownCountMetric || metrics[a.Metric] || byID[a.ID].ID != "" || a.ValueColumn < 0 || a.UnknownCountColumn < 0 || a.ValueColumn == a.UnknownCountColumn || a.ValueColumn >= len(d.ExpectedSchema) || a.UnknownCountColumn >= len(d.ExpectedSchema) || d.ExpectedSchema[a.ValueColumn] != a.ValueField || d.ExpectedSchema[a.UnknownCountColumn] != a.UnknownCountField || !slices.Contains([]string{"integer", "decimal", "number"}, a.ValueField.Type) || !slices.Contains([]string{"integer", "decimal"}, a.UnknownCountField.Type) {
			return ErrInvalid
		}
		byID[a.ID] = a
		metrics[a.Metric] = true
	}
	for _, o := range d.Outputs {
		if len(o.AmountCompleteness) > 128 {
			return ErrInvalid
		}
		fields := amountOutputFields(o)
		seen := map[string]bool{}
		for _, b := range o.AmountCompleteness {
			a, ok := byID[b.Declaration]
			key := b.Declaration + ":" + b.Role
			if !ok || seen[key] || b.Role != "amount" && b.Role != "unknown_count" {
				return ErrInvalid
			}
			seen[key] = true
			name := a.ValueField.Name
			if b.Role == "unknown_count" {
				name = a.UnknownCountField.Name
			}
			if !fields[name] {
				return ErrInvalid
			}
			if b.Role == "unknown_count" && o.Mapping != nil {
				for _, c := range o.Mapping.Columns {
					if c.Name == name && (c.Format.Currency != "" || c.Format.CurrencySymbol != "" || c.Format.Percent != "" || c.Format.FractionDigits != 0) {
						return ErrInvalid
					}
				}
			}
		}

		if o.Mapping != nil && o.Kind != "table" {
			// One numeric series/KPI cannot mix an amount and a count as
			// interchangeable values, comparison or target. Separate count
			// outputs and independent geometric slots retain their own roles.
			m := o.Mapping
			slots := append([]string{m.Bindings.Value, m.Bindings.Comparison, m.Bindings.Target}, m.Bindings.Values...)
			amount, count := false, false
			for _, column := range m.Columns {
				if !slices.Contains(slots, column.ID) {
					continue
				}
				for _, a := range d.AmountCompleteness {
					amount = amount || column.Name == a.ValueField.Name
					count = count || column.Name == a.UnknownCountField.Name
				}
			}
			if amount && count {
				return ErrInvalid
			}
		}
		for _, a := range d.AmountCompleteness {
			if fields[a.ValueField.Name] && !seen[a.ID+":amount"] || fields[a.UnknownCountField.Name] && !seen[a.ID+":unknown_count"] {
				return ErrInvalid
			}
		}
	}
	return nil
}

func reviewedAmountDisclosures(d Definition, definitionDigest string, o Output, result exec.Result) []AmountDisclosure {
	var out []AmountDisclosure
	byID := map[string]AmountDeclaration{}
	for _, a := range d.AmountCompleteness {
		byID[a.ID] = a
	}
	for _, b := range o.AmountCompleteness {
		a, ok := byID[b.Declaration]
		if !ok {
			continue
		}
		item := AmountDisclosure{ValueField: a.ValueField.Name, UnknownCountField: a.UnknownCountField.Name, Label: a.Label, RowsScope: "returned_query_rows", QueryOutcome: result.Outcome, Truncation: result.Truncation, Evidence: "reviewed_definition", DefinitionDigest: definitionDigest, Declaration: a.ID, Role: b.Role, Result: nlqexec.AmountCompleteness{Policy: ReviewedAmountCompletenessPolicy, Metric: a.Metric, ValueColumn: a.ValueColumn, UnknownCountMetric: a.UnknownCountMetric, UnknownCountColumn: a.UnknownCountColumn, Status: "unknown", Scope: "returned_query_rows"}}
		if b.Role == "unknown_count" {
			item.Unit = "count"
		}
		if (result.Outcome == "succeeded" || result.Outcome == "empty") && result.Truncation == "" && a.ValueColumn >= 0 && a.UnknownCountColumn >= 0 && a.ValueColumn < len(result.Schema) && a.UnknownCountColumn < len(result.Schema) && result.Schema[a.ValueColumn] == a.ValueField && result.Schema[a.UnknownCountColumn] == a.UnknownCountField && (result.Outcome != "empty" || len(result.Rows) == 0) {
			item.Result.Status = "complete"
			for rowIndex, row := range result.Rows {
				r := nlqexec.AmountCompletenessRow{Row: rowIndex, Status: "unknown"}
				if a.ValueColumn < len(row) && a.UnknownCountColumn < len(row) {
					if n, ok := reviewedUnknownCount(row[a.UnknownCountColumn]); ok {
						r.UnknownCount = n.String()
						r.Status = "complete"
						if n.Sign() > 0 {
							r.Status = "incomplete"
						}
					}
				}
				if r.Status == "unknown" {
					item.Result.Status = "unknown"
				} else if r.Status == "incomplete" && item.Result.Status != "unknown" {
					item.Result.Status = "incomplete"
				}
				item.Result.Rows = append(item.Result.Rows, r)
			}
		}
		out = append(out, item)
	}
	return out
}

func reviewedUnknownCount(raw json.RawMessage) (*big.Int, bool) {
	var value string
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return nil, false
		}
	} else {
		value = string(raw)
	}
	if !exec.Decimal(value) {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok || n.Sign() < 0 || !n.IsInt() {
		return nil, false
	}
	if len(n.Num().String()) > 4096 {
		return nil, false
	}
	return new(big.Int).Set(n.Num()), true
}

// Existing declarations cannot disappear in an ordinary whole-definition edit.
// Older clients must fetch and preserve reviewed disclosure metadata.
func retainAmountDeclarations(before, after Definition) error {
	for _, a := range before.AmountCompleteness {
		found := false
		for _, b := range after.AmountCompleteness {
			if a.ID == b.ID {
				found = true
				break
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	return nil
}

func checkAmountDisclosures(m RunManifest, saved Output, o RetainedOutput, starting bool) error {
	if starting || o.State != "succeeded" {
		if len(o.AmountCompleteness) != 0 {
			return ErrInvalid
		}
		return nil
	}
	if len(o.AmountCompleteness) != len(saved.AmountCompleteness) {
		return ErrInvalid
	}
	expected := reviewedAmountDisclosures(m.Revision.Definition, m.Revision.Digest, saved, exec.Result{})
	for i, got := range o.AmountCompleteness {
		if err := ValidateAmountDisclosure(got); err != nil {
			return err
		}
		if i >= len(expected) || len(got.Result.Rows) > m.Limits.MaxRows || !text(got.Truncation, 128) || !slices.Contains([]string{"succeeded", "empty", "truncated"}, got.QueryOutcome) {
			return ErrInvalid
		}
		fixed := clone(got)
		fixed.QueryOutcome = ""
		fixed.Truncation = ""
		fixed.Result.Rows = nil
		fixed.Result.Status = "unknown"
		if digest(fixed) != digest(expected[i]) {
			return ErrInvalid
		}

		if o.Chart != nil {
			if len(got.Result.Rows) > 0 && len(got.Result.Rows) != o.Chart.InputRows || got.Result.Status != "unknown" && len(got.Result.Rows) != o.Chart.InputRows {
				return ErrInvalid
			}
			if o.Chart.Kind == "table" {
				if len(o.Chart.RowIndices) != len(o.Chart.Rows) {
					return ErrInvalid
				}
				rows := map[int]bool{}
				for _, row := range o.Chart.RowIndices {
					if row < 0 || row >= o.Chart.InputRows || rows[row] {
						return ErrInvalid
					}
					rows[row] = true
				}
			}
		}
		status := "complete"
		if got.Truncation != "" || got.QueryOutcome == "truncated" {
			status = "unknown"
			if len(got.Result.Rows) != 0 {
				return ErrInvalid
			}
		}
		for j, r := range got.Result.Rows {
			if r.Row != j || !slices.Contains([]string{"complete", "incomplete", "unknown"}, r.Status) {
				return ErrInvalid
			}
			if r.Status == "unknown" {
				if r.UnknownCount != "" {
					return ErrInvalid
				}
				status = "unknown"
				continue
			}
			n, ok := new(big.Int).SetString(r.UnknownCount, 10)
			if !ok || n.Sign() < 0 || n.String() != r.UnknownCount || r.Status == "complete" && n.Sign() != 0 || r.Status == "incomplete" && n.Sign() == 0 {
				return ErrInvalid
			}
			if r.Status == "incomplete" && status != "unknown" {
				status = "incomplete"
			}
		}
		// Missing/mismatched schema can yield unknown without row evidence.
		if got.Result.Status == "unknown" && len(got.Result.Rows) == 0 {
			status = "unknown"
		}
		if got.QueryOutcome == "empty" && len(got.Result.Rows) != 0 || got.Result.Status != status {
			return ErrInvalid
		}
	}
	return nil
}

// projectAmountRows preserves source-row ordinals even when the table is sorted
// or paged. The aggregate status still describes returned query rows; it is not
// silently recast as an assessment of the currently visible window.
func projectAmountRows(input []AmountDisclosure, rowIndices []int, start, end int) []AmountDisclosure {
	out := clone(input)
	selected := map[int]bool{}
	for i := start; i < end; i++ {
		row := i
		if len(rowIndices) > 0 {
			if i >= len(rowIndices) {
				continue
			}
			row = rowIndices[i]
		}
		selected[row] = true
	}
	for i := range out {
		out[i].RowsScope = "visible_source_rows"
		rows := out[i].Result.Rows
		out[i].Result.Rows = nil
		for _, row := range rows {
			if selected[row.Row] {
				out[i].Result.Rows = append(out[i].Result.Rows, row)
			}
		}
	}
	return out
}

func analyticalAmountDisclosures(input []nlqexec.AmountCompleteness, result exec.Result) []AmountDisclosure {
	var out []AmountDisclosure
	for _, item := range input {
		value, count := "", ""
		if item.ValueColumn >= 0 && item.ValueColumn < len(result.Schema) {
			value = result.Schema[item.ValueColumn].Name
		}
		if item.UnknownCountColumn >= 0 && item.UnknownCountColumn < len(result.Schema) {
			count = result.Schema[item.UnknownCountColumn].Name
		}
		out = append(out, AmountDisclosure{ValueField: value, UnknownCountField: count, Label: "Known amount", Evidence: "analytical_receipt", RowsScope: "returned_query_rows", QueryOutcome: result.Outcome, Truncation: result.Truncation, Role: "amount", Result: clone(item)})
	}
	return out
}

// ValidateAmountDisclosure checks the closed retained/display contract. It does
// not confer source or display authority and does not reconstruct an old proof.
func ValidateAmountDisclosure(d AmountDisclosure) error {
	r := d.Result
	if d.Label == "" || !text(d.Label, 256) || !slices.Contains([]string{"amount", "unknown_count"}, d.Role) || d.Role == "unknown_count" && d.Unit != "count" || d.Role == "amount" && d.Unit != "" || !slices.Contains([]string{"returned_query_rows", "visible_source_rows"}, d.RowsScope) || !slices.Contains([]string{"succeeded", "empty", "truncated"}, d.QueryOutcome) || !text(d.Truncation, 128) || r.Scope != "returned_query_rows" || r.Metric == "" || !text(r.Metric, 512) || r.UnknownCountMetric == "" || !text(r.UnknownCountMetric, 512) || r.Metric == r.UnknownCountMetric || !slices.Contains([]string{"complete", "incomplete", "unknown"}, r.Status) || len(r.Rows) > 100000 {
		return ErrInvalid
	}
	switch d.Evidence {
	case "reviewed_definition":
		if !hashValid(d.DefinitionDigest) || !identity.Identifier(d.Declaration) || r.Policy != ReviewedAmountCompletenessPolicy {
			return ErrInvalid
		}
	case "analytical_receipt":
		if d.DefinitionDigest != "" || d.Declaration != "" || d.Role != "amount" || r.Policy != nlqexec.AmountCompletenessPolicy {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if !text(d.ValueField, 256) || !text(d.UnknownCountField, 256) || r.ValueColumn >= 0 && d.ValueField == "" || r.UnknownCountColumn >= 0 && d.UnknownCountField == "" {
		return ErrInvalid
	}
	if r.ValueColumn < -1 || r.UnknownCountColumn < -1 || r.ValueColumn >= 256 || r.UnknownCountColumn >= 256 || r.Status != "unknown" && (r.ValueColumn < 0 || r.UnknownCountColumn < 0 || r.ValueColumn == r.UnknownCountColumn) {
		return ErrInvalid
	}
	if d.Truncation != "" || d.QueryOutcome == "truncated" {
		if r.Status != "unknown" || len(r.Rows) != 0 {
			return ErrInvalid
		}
	}
	if d.QueryOutcome == "empty" && len(r.Rows) != 0 {
		return ErrInvalid
	}
	seen := map[int]bool{}
	derived := "complete"
	for _, row := range r.Rows {
		if row.Row < 0 || row.Row >= 100000 || seen[row.Row] || !slices.Contains([]string{"complete", "incomplete", "unknown"}, row.Status) || len(row.UnknownCount) > 4096 {
			return ErrInvalid
		}
		seen[row.Row] = true
		if row.Status == "unknown" {
			if row.UnknownCount != "" {
				return ErrInvalid
			}
			derived = "unknown"
			continue
		}
		n, ok := new(big.Int).SetString(row.UnknownCount, 10)
		if !ok || n.Sign() < 0 || n.String() != row.UnknownCount || row.Status == "complete" && n.Sign() != 0 || row.Status == "incomplete" && n.Sign() == 0 {
			return ErrInvalid
		}
		if row.Status == "incomplete" && derived != "unknown" {
			derived = "incomplete"
		}
	}
	if r.Status == "complete" && derived != "complete" || d.RowsScope == "returned_query_rows" && r.Status == "incomplete" && derived != "incomplete" {
		return ErrInvalid
	}
	return nil
}
