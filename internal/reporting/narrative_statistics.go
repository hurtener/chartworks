package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

func statisticalNarrativePolicy(n Narrative) error {
	if n.Reduction != "statistical_evidence" || len(n.Statistics) < 1 || len(n.Statistics) > 3 || n.MaxRows < 1 || n.MaxRows > 1000 || n.MaxBytes < 128 || n.MaxBytes > 65536 || n.MaxCharacters < 1 || n.MaxCharacters > 16384 || len(n.Fields) < 1 || len(n.Fields) > 128 {
		return ErrNarrativePolicy
	}
	seen, kinds := map[string]bool{}, map[string]bool{}
	for _, s := range n.Statistics {
		if !identity.Identifier(s.ID) || seen[s.ID] || kinds[s.Kind] || !slices.Contains(narrativeClaimKinds(n), s.Kind) ||
			s.Value != n.Statistics[0].Value || !statisticalValueRef(s.Value) || !slices.Contains(n.Fields, s.Value.Field.Name) || slices.Contains(n.RedactedFields, s.Value.Field.Name) {
			return ErrNarrativePolicy
		}
		seen[s.ID], kinds[s.Kind] = true, true
		if s.Kind == "trend" {
			if s.Time == nil || !statisticalTimeRef(*s.Time) || s.Time.Field.Column == s.Value.Column || !slices.Contains(n.Fields, s.Time.Field.Field.Name) || slices.Contains(n.RedactedFields, s.Time.Field.Field.Name) {
				return ErrNarrativePolicy
			}
		} else if s.Time != nil {
			return ErrNarrativePolicy
		}
	}
	return nil
}

func statisticalValueRef(r NarrativeFieldRef) bool {
	return r.Column >= 0 && r.Column < 256 && r.Field.Name != "" && r.Field.NativeType != "" &&
		(slices.Contains([]string{"integer", "decimal"}, r.Field.Type) && r.Field.Encoding == "string" || r.Field.Type == "number" && r.Field.Encoding == "number")
}

func statisticalTimeRef(t NarrativeTimeOrder) bool {
	f := t.Field.Field
	if t.Field.Column < 0 || t.Field.Column >= 256 || f.Name == "" || f.Type != "temporal" || f.Encoding != "string" {
		return false
	}
	switch t.Meaning {
	case "date":
		return strings.EqualFold(f.NativeType, "date")
	case "instant":
		return slices.Contains([]string{"timestamptz", "timestamp with time zone", "timestamp_tz", "timestamp_ltz", "datetimeoffset"}, strings.ToLower(f.NativeType))
	}
	return false
}

func validateStatisticalCoordinates(n Narrative, schema []exec.Field) error {
	for _, s := range n.Statistics {
		refs := []NarrativeFieldRef{s.Value}
		if s.Time != nil {
			refs = append(refs, s.Time.Field)
		}
		for _, r := range refs {
			if r.Column < 0 || r.Column >= len(schema) || schema[r.Column] != r.Field {
				return ErrInvalid
			}
		}
	}
	return nil
}

func statisticalClaimSchema(n Narrative) string {
	kinds := make([]string, len(n.Statistics))
	for i, s := range n.Statistics {
		kinds[i] = s.Kind
	}
	enum, _ := json.Marshal(kinds)
	return fmt.Sprintf(`{"type":"object","additionalProperties":false,"required":["claims"],"properties":{"claims":{"type":"array","minItems":1,"maxItems":%d,"items":{"type":"object","additionalProperties":false,"required":["kind","evidence"],"properties":{"kind":{"enum":%s},"evidence":{"type":"array","minItems":1,"maxItems":1,"items":{"type":"string","minLength":1,"maxLength":32}}}}}}}`, n.MaxClaims, enum)
}

func prepareOutputNarrative(ctx context.Context, m RunManifest, result exec.Result, saved Output) (preparedNarrative, error) {
	if saved.Narrative == nil {
		return preparedNarrative{}, ErrInvalid
	}
	n := *saved.Narrative
	if n.PolicyVersion != StatisticalNarrativePolicyVersion {
		return prepareNarrative(m, result, n)
	}
	if err := ctx.Err(); err != nil {
		return preparedNarrative{}, err
	}
	if boundedNarrativePolicy(n) != nil || m.Selection == nil || m.Revision.Definition.SchemaVersion != CurrentSchemaVersion {
		return preparedNarrative{}, ErrNarrativePolicy
	}
	matched := false
	for _, accepted := range m.Outputs {
		if accepted.ID == saved.ID && digest(accepted) == digest(saved) {
			matched = true
			break
		}
	}
	if !matched || !identity.Identifier(saved.ID) || len(result.Rows) > 100000 || len(result.Schema) < 1 || len(result.Schema) > 256 || digest(m.Revision.Definition.ExpectedSchema) != digest(result.Schema) {
		return preparedNarrative{}, ErrInvalid
	}
	fields := map[string]exec.Field{}
	for _, f := range result.Schema {
		fields[f.Name] = f
	}
	if err := validateNarrative(n, fields); err != nil {
		return preparedNarrative{}, err
	}
	if err := validateStatisticalCoordinates(n, result.Schema); err != nil {
		return preparedNarrative{}, err
	}
	allowed := restrictNarrativeFields(m.ResultPolicy, n).Fields
	for _, spec := range n.Statistics {
		if !slices.Contains(allowed, spec.Value.Field.Name) || spec.Time != nil && !slices.Contains(allowed, spec.Time.Field.Field.Name) {
			return preparedNarrative{}, ErrIncomplete
		}
	}
	// Companion policy is resolved before any arithmetic or serialization. A
	// derived status is data egress too; raw disclosure sidecars are never copied.
	amount, err := statisticalAmount(ctx, m, saved, result, allowed)
	if err != nil {
		return preparedNarrative{}, err
	}
	evidence, caveats, err := statisticalEvidence(ctx, m, saved, result, amount)
	if err != nil {
		return preparedNarrative{}, err
	}
	input, err := json.Marshal(struct {
		Instructions string              `json:"instructions"`
		Type         string              `json:"type"`
		Locale       string              `json:"locale"`
		Tone         string              `json:"tone"`
		Evidence     []NarrativeEvidence `json:"evidence"`
		Caveats      []string            `json:"caveats"`
	}{n.Instructions, n.Type, n.Locale, n.Tone, evidence, caveats})
	if err != nil || len(input) > n.MaxBytes+8192 {
		return preparedNarrative{}, ErrBudget
	}
	return preparedNarrative{evidence: evidence, caveats: caveats, input: string(input)}, nil
}

func statisticalAmount(ctx context.Context, m RunManifest, saved Output, result exec.Result, allowed []string) (*NarrativeAmountEvidence, error) {
	n := saved.Narrative
	var out *NarrativeAmountEvidence
	for _, a := range m.Revision.Definition.AmountCompleteness {
		if a.ValueField != n.Statistics[0].Value.Field || a.ValueColumn != n.Statistics[0].Value.Column {
			continue
		}
		if out != nil {
			return nil, ErrInvalid
		}
		bound := false
		for _, b := range saved.AmountCompleteness {
			bound = bound || b.Declaration == a.ID && b.Role == "amount"
		}
		if !bound || !slices.Contains(allowed, a.UnknownCountField.Name) {
			return nil, ErrIncomplete
		}
		if a.UnknownCountColumn < 0 || a.UnknownCountColumn >= len(result.Schema) || result.Schema[a.UnknownCountColumn] != a.UnknownCountField {
			return nil, ErrInvalid
		}
		out = &NarrativeAmountEvidence{Declaration: a.ID, Label: a.Label, Status: "complete", Scope: "retained_row_prefix", Companion: NarrativeFieldRef{Column: a.UnknownCountColumn, Field: a.UnknownCountField}}
		if result.Outcome == "truncated" || result.Truncation != "" {
			out.Status = "unknown"
		}
		for _, row := range result.Rows[:min(n.MaxRows, len(result.Rows))] {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if len(row) != len(result.Schema) {
				return nil, ErrInvalid
			}
			value, present := narrativeCell(row[a.UnknownCountColumn])
			count, _, ok := narrativeNumber(value)
			ok = present && ok && count.IsInt() && count.Sign() >= 0
			if !ok {
				out.Status = "unknown"
			} else if count.Sign() > 0 && out.Status != "unknown" {
				out.Status = "incomplete"
			}
		}
	}
	return out, nil
}

type statisticalObservation struct {
	row      int
	value    string
	number   *big.Rat
	scale    int
	timeText string
	instant  time.Time
}

func statisticalTime(value, meaning string) (time.Time, bool) {
	if len(value) > 128 {
		return time.Time{}, false
	}
	if meaning == "date" {
		t, err := time.Parse("2006-01-02", value)
		return t, err == nil && t.Format("2006-01-02") == value
	}
	if meaning != "instant" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999Z07"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func statisticalEvidence(ctx context.Context, m RunManifest, saved Output, result exec.Result, amount *NarrativeAmountEvidence) ([]NarrativeEvidence, []string, error) {
	n := *saved.Narrative
	if !slices.Contains([]string{"succeeded", "empty", "truncated"}, result.Outcome) || !text(result.Truncation, 128) || result.Outcome == "empty" && len(result.Rows) != 0 {
		return nil, nil, ErrInvalid
	}
	rows := min(n.MaxRows, len(result.Rows))
	if rows == 0 {
		return nil, nil, ErrIncomplete
	}
	caveats := []string{"retained_observation_not_live_source", "bounded_evidence_not_full_source_total", "statistical_population_retained_row_prefix", "null_values_excluded_not_zero"}
	if result.Outcome == "truncated" || result.Truncation != "" {
		caveats = append(caveats, "source_result_truncated")
	}
	if rows < len(result.Rows) {
		caveats = append(caveats, "narrative_rows_reduced")
	}
	if amount != nil {
		caveats = append(caveats, "known_amount_completeness_"+amount.Status)
	}
	evidence := make([]NarrativeEvidence, 0, len(n.Statistics))
	for _, spec := range n.Statistics {
		population := NarrativePopulation{Scope: "retained_row_prefix", RetainedRows: len(result.Rows), ConsideredRows: rows, SourceRows: []int{}, NullRows: []int{}, QueryOutcome: result.Outcome, Truncation: result.Truncation, RowsReduced: rows < len(result.Rows)}
		observations := make([]statisticalObservation, 0, rows)
		for i, row := range result.Rows[:rows] {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if len(row) != len(result.Schema) {
				return nil, nil, ErrInvalid
			}
			value, present := narrativeCell(row[spec.Value.Column])
			if !present {
				if string(row[spec.Value.Column]) != "null" {
					return nil, nil, ErrInvalid
				}
				population.NullRows = append(population.NullRows, i)
				continue
			}
			number, scale, valid := narrativeNumber(value)
			if !valid {
				return nil, nil, ErrInvalid
			}
			observation := statisticalObservation{row: i, value: value, number: number, scale: scale}
			if spec.Time != nil {
				value, present := narrativeCell(row[spec.Time.Field.Column])
				instant, valid := statisticalTime(value, spec.Time.Meaning)
				if !present || !valid {
					return nil, nil, ErrIncomplete
				}
				observation.timeText, observation.instant = value, instant
			}
			observations = append(observations, observation)
		}
		if len(observations) == 0 || spec.Kind != "extrema" && len(observations) < 2 {
			return nil, nil, ErrIncomplete
		}
		if spec.Time != nil {
			slices.SortFunc(observations, func(a, b statisticalObservation) int { return a.instant.Compare(b.instant) })
			for i := 1; i < len(observations); i++ {
				if observations[i-1].instant.Equal(observations[i].instant) {
					return nil, nil, ErrIncomplete
				}
			}
		}
		for _, o := range observations {
			population.SourceRows = append(population.SourceRows, o.row)
		}
		population.IncludedRows = len(observations)
		derived := &NarrativeStatisticEvidence{ID: spec.ID, Output: saved.ID, Kind: spec.Kind, Value: spec.Value, Time: clone(spec.Time), Population: population, Amount: clone(amount)}
		if err := calculateStatistic(ctx, derived, observations); err != nil {
			return nil, nil, err
		}
		// Exact input bytes, output, policy and reviewed field coordinates bind
		// provenance. Repository recomputation supplies the actual trust check.
		provenanceRows := make([][]json.RawMessage, rows)
		for i, row := range result.Rows[:rows] {
			provenanceRows[i] = []json.RawMessage{row[spec.Value.Column]}
			if spec.Time != nil {
				provenanceRows[i] = append(provenanceRows[i], row[spec.Time.Field.Column])
			}
			if amount != nil {
				provenanceRows[i] = append(provenanceRows[i], row[amount.Companion.Column])
			}
		}
		derived.Provenance = digest([]any{"retained-statistic-v1", m.Revision.Digest, saved.ID, spec, m.ResultPolicy, provenanceRows, derived})
		evidence = append(evidence, NarrativeEvidence{ID: fmt.Sprintf("e%d", len(evidence)+1), Field: spec.Value.Field.Name, Type: "statistic", Row: -1, Statistic: derived})
		encoded, err := json.Marshal(evidence)
		if err != nil || len(encoded) > n.MaxBytes || len(evidence) > 256 {
			return nil, nil, ErrBudget
		}
	}
	return evidence, caveats, ctx.Err()
}

func calculateStatistic(ctx context.Context, d *NarrativeStatisticEvidence, observations []statisticalObservation) error {
	switch d.Kind {
	case "trend":
		first, last := observations[0], observations[len(observations)-1]
		up, down, equal := false, false, false
		for i := 1; i < len(observations); i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			c := observations[i].number.Cmp(observations[i-1].number)
			up, down, equal = up || c > 0, down || c < 0, equal || c == 0
		}
		sequence := "constant"
		switch {
		case up && down:
			sequence = "mixed"
		case up && equal:
			sequence = "nondecreasing"
		case down && equal:
			sequence = "nonincreasing"
		case up:
			sequence = "increasing"
		case down:
			sequence = "decreasing"
		}
		difference := new(big.Rat).Sub(last.number, first.number).FloatString(max(first.scale, last.scale))
		if len(difference) > 4096 {
			return ErrBudget
		}
		d.Trend = &NarrativeTrendEvidence{FirstRow: first.row, LastRow: last.row, FirstTime: first.timeText, LastTime: last.timeText, FirstValue: first.value, LastValue: last.value, Difference: difference, Sequence: sequence}
	case "extrema":
		minimum, maximum := observations[0], observations[0]
		minTies, maxTies := 0, 0
		for _, o := range observations {
			if err := ctx.Err(); err != nil {
				return err
			}
			if c := o.number.Cmp(minimum.number); c < 0 {
				minimum, minTies = o, 1
			} else if c == 0 {
				minTies++
				if o.row < minimum.row {
					minimum = o
				}
			}
			if c := o.number.Cmp(maximum.number); c > 0 {
				maximum, maxTies = o, 1
			} else if c == 0 {
				maxTies++
				if o.row < maximum.row {
					maximum = o
				}
			}
		}
		d.Extrema = &NarrativeExtremaEvidence{Minimum: minimum.value, Maximum: maximum.value, MinimumRow: minimum.row, MaximumRow: maximum.row, MinimumTies: minTies, MaximumTies: maxTies}
	case "population_variance":
		// At most 1,000 already-bounded decimals. Exact rational arithmetic
		// never converts through binary float or invents a rounded variance.
		sum, squares := new(big.Rat), new(big.Rat)
		for _, o := range observations {
			if err := ctx.Err(); err != nil {
				return err
			}
			sum.Add(sum, o.number)
			squares.Add(squares, new(big.Rat).Mul(o.number, o.number))
		}
		count := new(big.Rat).SetInt64(int64(len(observations)))
		variance := new(big.Rat).Sub(new(big.Rat).Mul(count, squares), new(big.Rat).Mul(sum, sum))
		variance.Quo(variance, new(big.Rat).Mul(count, count))
		numerator, denominator := variance.Num().String(), variance.Denom().String()
		if variance.Sign() < 0 || len(numerator) > 4096 || len(denominator) > 4096 {
			return ErrBudget
		}
		d.Variance = &NarrativeVarianceEvidence{Numerator: numerator, Denominator: denominator, Divisor: len(observations)}
	default:
		return ErrNarrativePolicy
	}
	return nil
}
