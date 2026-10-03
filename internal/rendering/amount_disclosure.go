package rendering

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/reporting"
)

// Labels are inert reviewed text. Numeric display is exact and never turns an
// unknown/null counter into zero or treats transport truncation as completeness.
// AmountDisclosureLines returns validated, inert disclosure text for every renderer.
func AmountDisclosureLines(input []reporting.AmountDisclosure) ([]string, error) {
	if len(input) > 128 {
		return nil, ErrInvalid
	}
	var lines []string
	seen := map[string]bool{}
	for _, d := range input {
		if err := reporting.ValidateAmountDisclosure(d); err != nil {
			return nil, err
		}
		key := d.Declaration + "\x00" + d.Result.Metric + "\x00" + d.Result.UnknownCountMetric
		if seen[key] {
			continue
		}
		seen[key] = true
		label := d.Label
		if label == "" {
			label = "Known amount"
		}
		status := "unknown"
		if d.Truncation == "" && (d.Result.Status == "complete" || d.Result.Status == "incomplete") {
			status = d.Result.Status
		}
		evidence := "unknown"
		if d.Evidence == "reviewed_definition" {
			evidence = "reviewed definition"
		} else if d.Evidence == "analytical_receipt" {
			evidence = "analytical receipt"
		}
		lines = append(lines, label+": "+status, "Evidence: "+evidence, "Scope: returned query rows")
		scope := "returned rows"
		if d.RowsScope == "visible_source_rows" {
			scope = "displayed rows"
		}
		total := new(big.Int)
		known := len(d.Result.Rows) > 0 || d.QueryOutcome == "empty" && d.Result.Status == "complete"
		for _, r := range d.Result.Rows {
			if r.Status == "unknown" || len(r.UnknownCount) > 4096 {
				known = false
				break
			}
			n, ok := new(big.Int).SetString(r.UnknownCount, 10)
			if !ok || n.Sign() < 0 || n.String() != r.UnknownCount {
				known = false
				break
			}
			total.Add(total, n)
		}
		count := "unknown"
		if known && d.Truncation == "" {
			count = total.String()
		}
		lines = append(lines, fmt.Sprintf("Unknown amount count (%s): %s", scope, count))
		if d.Truncation != "" {
			lines = append(lines, "Result truncated; amount completeness is unknown")
		}
		if d.Role == "unknown_count" {
			lines = append(lines, "Displayed metric unit: count")
		}
	}
	return lines, nil
}

func wrapDisclosureLines(lines []string, width int) []string {
	var out []string
	limit := max(16, (width-16)/8)
	for _, line := range lines {
		// Replacing authored line breaks keeps geometry deterministic; HTML/SVG
		// escaping occurs at the actual output boundary.
		r := []rune(strings.ReplaceAll(strings.ReplaceAll(line, "\r", " "), "\n", " "))
		for len(r) > limit {
			out = append(out, string(r[:limit]))
			r = r[limit:]
		}
		out = append(out, string(r))
	}
	return out
}

func renderCSVWithAmountDisclosure(out *reporting.ViewerOutput, _ reporting.ViewerPage, timezone string) ([]byte, error) {
	raw, err := renderCSV(out, timezone)
	if err != nil || len(out.AmountCompleteness) == 0 {
		return raw, err
	}
	records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil || len(records) == 0 {
		return nil, ErrInvalid
	}
	if len(out.Table.RowIndices) != len(out.Table.Rows) {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, d := range out.AmountCompleteness {
		if err := reporting.ValidateAmountDisclosure(d); err != nil {
			return nil, err
		}
		key := d.Declaration + "\x00" + d.Result.Metric + "\x00" + d.Result.UnknownCountMetric
		if seen[key] {
			continue
		}
		seen[key] = true
		prefix := "amount_" + strconv.Itoa(len(seen))
		records[0] = append(records[0], prefix+"_status", prefix+"_unknown_count", prefix+"_population_status", prefix+"_evidence", prefix+"_scope", prefix+"_value_field", prefix+"_unknown_count_field", prefix+"_counter_unit")
		byRow := map[int]struct{ status, count string }{}
		for _, r := range d.Result.Rows {
			byRow[r.Row] = struct{ status, count string }{r.Status, r.UnknownCount}
		}
		for i := 1; i < len(records); i++ {
			row := out.Table.RowIndices[i-1]
			v, ok := byRow[row]
			if !ok || d.Truncation != "" {
				v.status, v.count = "unknown", ""
			}
			records[i] = append(records[i], exportCell(v.status), exportCell(v.count), exportCell(d.Result.Status), exportCell(d.Evidence), "returned_query_rows", exportCell(d.ValueField), exportCell(d.UnknownCountField), "count")
		}
	}
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if err := w.WriteAll(records); err != nil {
		return nil, err
	}
	return b.Bytes(), w.Error()
}

// AmountCoverage is content-free retained meaning. Row/count values remain in
// the authorized output; an empty CSV has no synthetic data row carrying them.
type AmountCoverage struct {
	Label              string `json:"label"`
	Evidence           string `json:"evidence"`
	Declaration        string `json:"declaration,omitempty"`
	DefinitionDigest   string `json:"definition_digest,omitempty"`
	Policy             string `json:"policy"`
	Metric             string `json:"metric"`
	ValueField         string `json:"value_field"`
	UnknownCountMetric string `json:"unknown_count_metric"`
	UnknownCountField  string `json:"unknown_count_field"`
	CounterUnit        string `json:"counter_unit"`
	Role               string `json:"role"`
	Status             string `json:"status"`
	Scope              string `json:"scope"`
	Truncation         string `json:"truncation,omitempty"`
}

func amountCoverage(input []reporting.AmountDisclosure) ([]AmountCoverage, error) {
	if len(input) > 128 {
		return nil, ErrInvalid
	}
	var out []AmountCoverage
	for _, d := range input {
		if err := reporting.ValidateAmountDisclosure(d); err != nil {
			return nil, err
		}
		out = append(out, AmountCoverage{Label: d.Label, Evidence: d.Evidence, Declaration: d.Declaration, DefinitionDigest: d.DefinitionDigest, Policy: d.Result.Policy, Metric: d.Result.Metric, ValueField: d.ValueField, UnknownCountMetric: d.Result.UnknownCountMetric, UnknownCountField: d.UnknownCountField, CounterUnit: "count", Role: d.Role, Status: d.Result.Status, Scope: d.Result.Scope, Truncation: d.Truncation})
	}
	return out, nil
}
