package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
)

func statisticalFixture() (RunManifest, Output, exec.Result) {
	value := exec.Field{Name: "amount", Type: "decimal", Encoding: "string", NativeType: "numeric"}
	date := exec.Field{Name: "observed", Type: "temporal", Encoding: "string", NativeType: "date"}
	n := contractNarrative()
	n.PolicyVersion, n.SchemaVersion, n.Instructions = StatisticalNarrativePolicyVersion, StatisticalNarrativeSchemaVersion, "evidence_only"
	n.Type, n.Reduction, n.Fields = "explanation", "statistical_evidence", []string{"amount", "observed"}
	n.MaxClaims, n.MaxBytes, n.MaxCharacters = 3, 16384, 16384
	v := NarrativeFieldRef{Column: 0, Field: value}
	n.Statistics = []NarrativeStatistic{
		{ID: "movement", Kind: "trend", Value: v, Time: &NarrativeTimeOrder{Field: NarrativeFieldRef{Column: 1, Field: date}, Meaning: "date"}},
		{ID: "range", Kind: "extrema", Value: v},
		{ID: "spread", Kind: "population_variance", Value: v},
	}
	saved := Output{ID: "statistics", Kind: "narrative", Narrative: &n, Intent: &OutputIntent{Enabled: true, DefaultSelected: true, DisplayOrder: 0, Metadata: []OutputMetadata{{Locale: "en-US", DisplayName: "Statistics"}}}}
	result := exec.Result{Schema: []exec.Field{value, date}, Outcome: "succeeded", Rows: [][]json.RawMessage{
		{json.RawMessage(`"3"`), json.RawMessage(`"2026-01-03"`)},
		{json.RawMessage(`"1"`), json.RawMessage(`"2026-01-01"`)},
		{json.RawMessage(`null`), json.RawMessage(`"2026-01-02"`)},
		{json.RawMessage(`"1"`), json.RawMessage(`"2026-01-04"`)},
	}}
	d := contractDefinition()
	d.SchemaVersion, d.Outputs, d.ExpectedSchema = CurrentSchemaVersion, []Output{saved}, result.Schema
	m := RunManifest{Revision: Revision{Definition: d, Digest: strings.Repeat("a", 64)}, Selection: &OutputSelection{Version: 2}, Outputs: []Output{saved}, ResultPolicy: []EffectiveFieldPolicy{{Field: "amount", Status: "allowed"}, {Field: "observed", Status: "allowed"}}}
	return m, saved, result
}

func syncStatisticalFixture(m *RunManifest, saved Output, result exec.Result) {
	m.Outputs = []Output{saved}
	m.Revision.Definition.Outputs = []Output{saved}
	m.Revision.Definition.ExpectedSchema = result.Schema
}

func statisticalAnswer() NarrativeAnswer {
	return NarrativeAnswer{Claims: []NarrativeClaim{{Kind: "trend", Evidence: []string{"e1"}}, {Kind: "extrema", Evidence: []string{"e2"}}, {Kind: "population_variance", Evidence: []string{"e3"}}}}
}

func TestStatisticalNarrativeExactRetainedEvidence(t *testing.T) {
	m, saved, result := statisticalFixture()
	if err := validateDefinition(t.Context(), m.Revision.Definition, config.DefaultReporting(), false); err != nil {
		t.Fatal("reviewed definition", err)
	}
	prepared, err := prepareOutputNarrative(t.Context(), m, result, saved)
	if err != nil {
		t.Fatal(err)
	}
	trend, extrema, variance := prepared.evidence[0].Statistic, prepared.evidence[1].Statistic, prepared.evidence[2].Statistic
	if !reflect.DeepEqual(trend.Population.SourceRows, []int{1, 0, 3}) || !reflect.DeepEqual(trend.Population.NullRows, []int{2}) || trend.Trend.Difference != "0" || trend.Trend.Sequence != "mixed" || trend.Trend.FirstRow != 1 || trend.Trend.LastRow != 3 {
		t.Fatal(trend)
	}
	if extrema.Extrema.Minimum != "1" || extrema.Extrema.Maximum != "3" || extrema.Extrema.MinimumRow != 1 || extrema.Extrema.MaximumRow != 0 || extrema.Extrema.MinimumTies != 2 {
		t.Fatal(extrema)
	}
	if variance.Variance.Numerator != "8" || variance.Variance.Denominator != "9" || variance.Variance.Divisor != 3 {
		t.Fatal(variance)
	}
	for _, locale := range []string{"en-US", "es-AR"} {
		for _, tone := range []string{"neutral", "concise", "technical"} {
			n := clone(*saved.Narrative)
			n.Locale, n.Tone = locale, tone
			text, err := groundedText(statisticalAnswer(), prepared.evidence, n)
			if err != nil || !strings.Contains(text, "8/9") || !strings.Contains(text, "[e1]") || !strings.Contains(text, "NULL") {
				t.Fatal(locale, tone, text, err)
			}
			if locale == "en-US" && (!strings.Contains(text, "squared value units") || !strings.Contains(text, "mixed")) {
				t.Fatal(text)
			}
		}
	}
	out := RetainedOutput{ID: saved.ID, Kind: "narrative", State: "succeeded", Narrative: &NarrativeResult{Evidence: prepared.evidence, Caveats: prepared.caveats}}
	if err := CheckFrozenNarrativeEvidence(m, out, result); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := CheckFrozenNarrativeEvidenceContext(ctx, m, out, result); !errors.Is(err, context.Canceled) {
		t.Fatal("repository recomputation ignored cancellation", err)
	}
	for _, mutate := range []func(*RetainedOutput){
		func(o *RetainedOutput) { o.Narrative.Evidence[0].Statistic.Population.SourceRows[0] = 0 },
		func(o *RetainedOutput) { o.Narrative.Evidence[0].Statistic.Trend.Sequence = "increasing" },
		func(o *RetainedOutput) { o.Narrative.Evidence[1].Statistic.Extrema.MinimumTies = 1 },
		func(o *RetainedOutput) { o.Narrative.Evidence[2].Statistic.Variance.Numerator = "9" },
		func(o *RetainedOutput) { o.Narrative.Evidence[2].Statistic.Provenance = strings.Repeat("b", 64) },
		func(o *RetainedOutput) { o.Narrative.Evidence[2].Statistic.Output = "other" },
		func(o *RetainedOutput) { o.Narrative.Caveats = nil },
	} {
		forged := clone(out)
		mutate(&forged)
		if CheckFrozenNarrativeEvidence(m, forged, result) == nil {
			t.Fatal("forged statistic retained")
		}
	}
}

func TestStatisticalNarrativeTemporalAndSequenceContract(t *testing.T) {
	for _, tc := range []struct {
		values   []string
		sequence string
	}{{[]string{"1", "2", "3"}, "increasing"}, {[]string{"3", "2", "1"}, "decreasing"}, {[]string{"1", "1", "1"}, "constant"}, {[]string{"1", "1", "2"}, "nondecreasing"}, {[]string{"2", "1", "1"}, "nonincreasing"}, {[]string{"1", "3", "2"}, "mixed"}} {
		m, saved, result := statisticalFixture()
		result.Rows = result.Rows[:3]
		for i, v := range tc.values {
			b, _ := json.Marshal(v)
			day, _ := json.Marshal([]string{"2026-01-01", "2026-01-02", "2026-01-03"}[i])
			result.Rows[i] = []json.RawMessage{b, day}
		}
		p, err := prepareOutputNarrative(t.Context(), m, result, saved)
		if err != nil || p.evidence[0].Statistic.Trend.Sequence != tc.sequence {
			t.Fatal(tc, err)
		}
	}
	for _, tc := range []struct {
		name, time1, time2 string
		duplicate          bool
	}{{"equivalent offsets", "2026-01-01T01:00:00+01:00", "2026-01-01T00:00:00Z", true}, {"ordered offsets", "2026-01-01T01:00:00+01:00", "2026-01-01T00:00:01Z", false}, {"postgres offsets", "2026-01-01 00:00:00+00", "2026-01-01 00:00:01+00", false}, {"wall timestamp", "2026-01-01 00:00:00", "2026-01-01T00:00:01Z", true}} {
		t.Run(tc.name, func(t *testing.T) {
			m, saved, result := statisticalFixture()
			result.Schema[1].NativeType = "timestamptz"
			saved.Narrative.Statistics = saved.Narrative.Statistics[:1]
			saved.Narrative.Statistics[0].Time = &NarrativeTimeOrder{Field: NarrativeFieldRef{Column: 1, Field: result.Schema[1]}, Meaning: "instant"}
			a, _ := json.Marshal(tc.time1)
			b, _ := json.Marshal(tc.time2)
			result.Rows = [][]json.RawMessage{{json.RawMessage(`"1"`), a}, {json.RawMessage(`"2"`), b}}
			syncStatisticalFixture(&m, saved, result)
			_, err := prepareOutputNarrative(t.Context(), m, result, saved)
			if (err != nil) != tc.duplicate {
				t.Fatal(tc, err)
			}
		})
	}
	for _, date := range []string{"2026", "2026-01", "2026-02-30", "2026-1-1", "infinity"} {
		if _, ok := statisticalTime(date, "date"); ok {
			t.Fatal(date)
		}
	}
}

func TestStatisticalNarrativeClosedClaimsAndPolicy(t *testing.T) {
	m, saved, result := statisticalFixture()
	p, err := prepareOutputNarrative(t.Context(), m, result, saved)
	if err != nil {
		t.Fatal(err)
	}
	schemaText, err := narrativeClaimSchema(*saved.Narrative)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := gateway.NewSchema("statistics", []byte(schemaText))
	if err != nil {
		t.Fatal(err)
	}
	good, _ := json.Marshal(statisticalAnswer())
	if schema.Validate(good, 16384) != nil {
		t.Fatal("valid provider response rejected")
	}
	for _, raw := range []string{`{"claims":[{"kind":"cause","evidence":["e1"]}]}`, `{"claims":[{"kind":"trend","evidence":["e1","e2"]}]}`, `{"claims":[{"kind":"trend","evidence":["e1"],"value":99}]}`, `{"claims":[],"sql":"select 1"}`} {
		if schema.Validate([]byte(raw), 16384) == nil {
			t.Fatal(raw)
		}
	}
	for _, answer := range []NarrativeAnswer{
		{Claims: []NarrativeClaim{{Kind: "extrema", Evidence: []string{"e1"}}}},
		{Claims: []NarrativeClaim{{Kind: "trend", Evidence: []string{"invented"}}}},
		{Claims: []NarrativeClaim{{Kind: "trend", Evidence: []string{"e1"}}, {Kind: "trend", Evidence: []string{"e1"}}}},
	} {
		if _, err := groundedText(answer, p.evidence, *saved.Narrative); !errors.Is(err, gateway.ErrOutput) {
			t.Fatal(answer, err)
		}
	}
	for _, mutate := range []func(*Narrative){
		func(n *Narrative) { n.Type = "summary" }, func(n *Narrative) { n.Type = "comparison" },
		func(n *Narrative) { n.Statistics[1].Kind = "trend" }, func(n *Narrative) { n.Statistics[1].ID = n.Statistics[0].ID },
		func(n *Narrative) { n.Statistics[1].Value.Column = 2 }, func(n *Narrative) { n.Statistics[0].Time = nil },
		func(n *Narrative) { n.Statistics[0].Time.Meaning = "timezone_guess" }, func(n *Narrative) { n.Statistics[0].Time.Field.Field.NativeType = "timestamp" },
		func(n *Narrative) { n.Statistics[1].Time = clone(n.Statistics[0].Time) }, func(n *Narrative) { n.Reduction = "first_rows" },
		func(n *Narrative) { n.SchemaVersion = "grounded-narrative-v1" }, func(n *Narrative) { n.PolicyVersion = NarrativePolicyVersion },
		func(n *Narrative) { n.Statistics = append(n.Statistics, n.Statistics[0]) },
	} {
		n := clone(*saved.Narrative)
		mutate(&n)
		if boundedNarrativePolicy(n) == nil {
			t.Fatal("unsupported statistical policy accepted", n)
		}
	}
	changed := clone(saved)
	changed.ID = "other"
	if _, err := prepareOutputNarrative(t.Context(), m, result, changed); !errors.Is(err, ErrInvalid) {
		t.Fatal("unaccepted output", err)
	}
	if _, err := prepareNarrative(m, result, *saved.Narrative); !errors.Is(err, ErrNarrativePolicy) {
		t.Fatal("unbound specification", err)
	}
	badSchema := clone(result)
	badSchema.Schema[0].Encoding = "number"
	if _, err := prepareOutputNarrative(t.Context(), m, badSchema, saved); !errors.Is(err, ErrInvalid) {
		t.Fatal("schema drift", err)
	}
	badCoordinate := clone(saved)
	badCoordinate.Narrative.Statistics[0].Time.Field.Column = 2
	syncStatisticalFixture(&m, badCoordinate, result)
	if _, err := prepareOutputNarrative(t.Context(), m, result, badCoordinate); !errors.Is(err, ErrInvalid) {
		t.Fatal("ordinal drift", err)
	}
}

func TestStatisticalNarrativeBoundsNullsAndCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Output, *exec.Result)
		want   error
	}{
		{"empty", func(_ *Output, r *exec.Result) { r.Rows = nil; r.Outcome = "empty" }, ErrIncomplete},
		{"all null", func(_ *Output, r *exec.Result) {
			for i := range r.Rows {
				r.Rows[i][0] = json.RawMessage(`null`)
			}
		}, ErrIncomplete},
		{"one row", func(o *Output, _ *exec.Result) { o.Narrative.MaxRows = 1 }, ErrIncomplete},
		{"bytes", func(o *Output, _ *exec.Result) { o.Narrative.MaxBytes = 128 }, ErrBudget},
		{"missing time", func(_ *Output, r *exec.Result) { r.Rows[0][1] = json.RawMessage(`null`) }, ErrIncomplete},
		{"duplicate date", func(_ *Output, r *exec.Result) { r.Rows[1][1] = r.Rows[0][1] }, ErrIncomplete},
		{"shape", func(_ *Output, r *exec.Result) { r.Rows[0] = nil }, ErrInvalid},
		{"nonfinite", func(_ *Output, r *exec.Result) { r.Rows[0][0] = json.RawMessage(`"NaN"`) }, ErrInvalid},
		{"exponent", func(_ *Output, r *exec.Result) { r.Rows[0][0] = json.RawMessage(`"1e9999"`) }, ErrInvalid},
		{"missing cell", func(_ *Output, r *exec.Result) { r.Rows[0][0] = nil }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, o, r := statisticalFixture()
			tc.mutate(&o, &r)
			syncStatisticalFixture(&m, o, r)
			if _, err := prepareOutputNarrative(t.Context(), m, r, o); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
		})
	}
	m, o, r := statisticalFixture()
	o.Narrative.MaxRows = 3
	r.Outcome, r.Truncation = "truncated", "rows"
	syncStatisticalFixture(&m, o, r)
	p, err := prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil {
		t.Fatal(err)
	}
	text, err := groundedText(statisticalAnswer(), p.evidence, *o.Narrative)
	if err != nil || !strings.Contains(text, "first 3 of 4") || !strings.Contains(text, "source result is truncated") || !strings.Contains(text, "row limit reduced") || p.evidence[2].Statistic.Variance.Numerator != "1" {
		t.Fatal(text, err)
	}
	o.Narrative.MaxCharacters = 1
	if _, err := groundedText(statisticalAnswer(), p.evidence, *o.Narrative); !errors.Is(err, ErrBudget) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := prepareOutputNarrative(ctx, m, r, o); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Exactly one observation supports extrema but never creates a variance.
	m, o, r = statisticalFixture()
	o.Narrative.Statistics = o.Narrative.Statistics[1:2]
	r.Rows = r.Rows[:1]
	syncStatisticalFixture(&m, o, r)
	p, err = prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil || p.evidence[0].Statistic.Extrema.MinimumTies != 1 {
		t.Fatal(p, err)
	}
}

func TestStatisticalNarrativeEgressAndAmountCompanions(t *testing.T) {
	for _, field := range []int{0, 1} {
		for _, status := range []string{"sensitive", "unknown", "conflicting", "redacted"} {
			m, o, r := statisticalFixture()
			m.ResultPolicy[field].Status = status
			if _, err := prepareOutputNarrative(t.Context(), m, r, o); !errors.Is(err, ErrIncomplete) {
				t.Fatal(field, status, err)
			}
		}
	}
	m, o, r := statisticalFixture()
	companion := exec.Field{Name: "unknown_count", Type: "integer", Encoding: "string", NativeType: "int8"}
	r.Schema = append(r.Schema, companion)
	for i := range r.Rows {
		v := json.RawMessage(`"0"`)
		if i == 2 {
			v = json.RawMessage(`"127"`)
		}
		r.Rows[i] = append(r.Rows[i], v)
	}
	o.Narrative.Fields = append(o.Narrative.Fields, "unknown_count")
	o.AmountCompleteness = []AmountOutputBinding{{Declaration: "known-amount", Role: "amount"}, {Declaration: "known-amount", Role: "unknown_count"}}
	m.Revision.Definition.AmountCompleteness = []AmountDeclaration{{ID: "known-amount", Label: "Known amount", Policy: ReviewedAmountCompletenessPolicy, Metric: "amount", ValueColumn: 0, ValueField: r.Schema[0], UnknownCountMetric: "unknown", UnknownCountColumn: 2, UnknownCountField: companion}}
	m.ResultPolicy = append(m.ResultPolicy, EffectiveFieldPolicy{Field: "unknown_count", Status: "allowed"})
	syncStatisticalFixture(&m, o, r)
	p, err := prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil {
		t.Fatal(err)
	}
	if p.evidence[0].Statistic.Amount.Status != "incomplete" || strings.Contains(p.input, `"127"`) || strings.Contains(p.input, `\"127\"`) {
		t.Fatal("companion leaked or lost status", p.input)
	}
	text, err := groundedText(statisticalAnswer(), p.evidence, *o.Narrative)
	if err != nil || !strings.Contains(text, "completeness incomplete") || !strings.Contains(text, "squared value units") {
		t.Fatal(text, err)
	}
	m.ResultPolicy[2].Status = "redacted"
	if _, err := prepareOutputNarrative(t.Context(), m, r, o); !errors.Is(err, ErrIncomplete) {
		t.Fatal("redacted companion", err)
	}
	m.ResultPolicy[2].Status = "allowed"
	r.Rows[2][2] = json.RawMessage(`null`)
	p, err = prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil || p.evidence[0].Statistic.Amount.Status != "unknown" {
		t.Fatal(p, err)
	}
	// An unrelated excluded column must not affect derived provenance or prompts.
	m, o, r = statisticalFixture()
	r.Schema = append(r.Schema, exec.Field{Name: "secret", Type: "text", Encoding: "string", NativeType: "text"})
	m.ResultPolicy = append(m.ResultPolicy, EffectiveFieldPolicy{Field: "secret", Status: "redacted"})
	for i := range r.Rows {
		r.Rows[i] = append(r.Rows[i], json.RawMessage(`"PRIVATE_CANARY"`))
	}
	syncStatisticalFixture(&m, o, r)
	p, err = prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil || strings.Contains(p.input, "PRIVATE_CANARY") {
		t.Fatal(err)
	}
	r.Rows[0][2] = json.RawMessage(`"CHANGED_PRIVATE_CANARY"`)
	again, err := prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil || digest(p.evidence) != digest(again.evidence) {
		t.Fatal("excluded cell entered derivation hash", err)
	}
}

func TestStatisticalNarrativeExactNumericAndLegacyBytes(t *testing.T) {
	for _, values := range [][]string{{"9007199254740993.125", "9007199254740993.375"}, {"1e-9", "2e-9"}, {"-1.25", "1.25"}} {
		m, o, r := statisticalFixture()
		o.Narrative.Statistics = o.Narrative.Statistics[1:]
		r.Rows = r.Rows[:2]
		for i, v := range values {
			r.Rows[i][0], _ = json.Marshal(v)
		}
		syncStatisticalFixture(&m, o, r)
		p, err := prepareOutputNarrative(t.Context(), m, r, o)
		if err != nil {
			t.Fatal(err)
		}
		x, _, _ := narrativeNumber(values[0])
		y, _, _ := narrativeNumber(values[1])
		delta := new(big.Rat).Sub(x, y)
		want := new(big.Rat).Quo(new(big.Rat).Mul(delta, delta), big.NewRat(4, 1))
		v := p.evidence[1].Statistic.Variance
		got, ok := new(big.Rat).SetString(v.Numerator + "/" + v.Denominator)
		if !ok || got.Cmp(want) != 0 {
			t.Fatal(values, v, want)
		}
	}
	legacy := NarrativeEvidence{ID: "e1", Field: "n", Type: "integer", Value: "3", Row: 0}
	raw, _ := json.Marshal(legacy)
	if string(raw) != `{"id":"e1","field":"n","type":"integer","value":"3","row":0}` {
		t.Fatal("legacy bytes changed", string(raw))
	}
	n := contractNarrative()
	raw, _ = json.Marshal(n)
	if strings.Contains(string(raw), "statistics") {
		t.Fatal(string(raw))
	}
	legacy.Statistic = &NarrativeStatisticEvidence{}
	if _, err := groundedText(NarrativeAnswer{Claims: []NarrativeClaim{{Kind: "value", Evidence: []string{"e1"}}}}, []NarrativeEvidence{legacy}, n); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("legacy accepted v3 evidence", err)
	}
}

func TestStatisticalNarrativeOrderingAndExactBudgets(t *testing.T) {
	m, o, r := statisticalFixture()
	// An earlier observation outside the retained prefix does not displace an
	// included row. Temporal sorting happens after the reviewed row reduction.
	o.Narrative.MaxRows = 2
	r.Rows[0][1], r.Rows[1][1], r.Rows[3][1] = json.RawMessage(`"2026-01-03"`), json.RawMessage(`"2026-01-04"`), json.RawMessage(`"2026-01-01"`)
	syncStatisticalFixture(&m, o, r)
	p, err := prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil || !reflect.DeepEqual(p.evidence[0].Statistic.Population.SourceRows, []int{0, 1}) {
		t.Fatal("sorting changed the declared population", p, err)
	}
	// Reordering declared kinds reorders their IDs, never their mathematical
	// populations, and is part of the saved output and provenance identity.
	m, o, r = statisticalFixture()
	o.Narrative.Statistics[0], o.Narrative.Statistics[2] = o.Narrative.Statistics[2], o.Narrative.Statistics[0]
	syncStatisticalFixture(&m, o, r)
	p, err = prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil || p.evidence[0].ID != "e1" || p.evidence[0].Statistic.Kind != "population_variance" || p.evidence[2].ID != "e3" || p.evidence[2].Statistic.Kind != "trend" {
		t.Fatal(p, err)
	}
	encoded, _ := json.Marshal(p.evidence)
	o.Narrative.MaxBytes = len(encoded)
	syncStatisticalFixture(&m, o, r)
	if _, err := prepareOutputNarrative(t.Context(), m, r, o); err != nil {
		t.Fatal("exact byte boundary rejected", err)
	}
	o.Narrative.MaxBytes--
	syncStatisticalFixture(&m, o, r)
	if _, err := prepareOutputNarrative(t.Context(), m, r, o); !errors.Is(err, ErrBudget) {
		t.Fatal("partial statistic admitted at byte boundary", err)
	}
	m, o, r = statisticalFixture()
	o.Narrative.Locale = "es-AR"
	syncStatisticalFixture(&m, o, r)
	p, err = prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil {
		t.Fatal(err)
	}
	text, err := groundedText(statisticalAnswer(), p.evidence, *o.Narrative)
	if err != nil {
		t.Fatal(err)
	}
	o.Narrative.MaxCharacters = utf8.RuneCountInString(text)
	if _, err := groundedText(statisticalAnswer(), p.evidence, *o.Narrative); err != nil {
		t.Fatal("exact Unicode character boundary rejected", err)
	}
	o.Narrative.MaxCharacters--
	if _, err := groundedText(statisticalAnswer(), p.evidence, *o.Narrative); !errors.Is(err, ErrBudget) {
		t.Fatal("character ceiling bypassed", err)
	}
}

func TestStatisticalNarrativeRejectsMalformedDerivedEvidence(t *testing.T) {
	m, o, r := statisticalFixture()
	p, err := prepareOutputNarrative(t.Context(), m, r, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]NarrativeEvidence){
		func(e []NarrativeEvidence) { e[0].Statistic = nil },
		func(e []NarrativeEvidence) { e[0].Statistic.Time = nil },
		func(e []NarrativeEvidence) { e[0].Statistic.Population.NullRows = []int{0} },
		func(e []NarrativeEvidence) { e[0].Statistic.Trend.Difference = "99" },
		func(e []NarrativeEvidence) { e[0].Statistic.Trend.FirstTime = "not-a-date" },
		func(e []NarrativeEvidence) { e[0].Statistic.Trend.Sequence = "increasing" },
		func(e []NarrativeEvidence) { e[1].Statistic.Extrema.MinimumTies = 4 },
		func(e []NarrativeEvidence) { e[1].Statistic.Extrema.Minimum = "10" },
		func(e []NarrativeEvidence) { e[2].Statistic.Variance.Numerator = "-1" },
		func(e []NarrativeEvidence) { e[2].Statistic.Variance.Numerator = "08" },
		func(e []NarrativeEvidence) { e[2].Statistic.Variance.Denominator = "0" },
		func(e []NarrativeEvidence) {
			e[2].Statistic.Variance.Numerator = "16"
			e[2].Statistic.Variance.Denominator = "18"
		},
		func(e []NarrativeEvidence) { e[2].Statistic.Variance.Divisor = 2 },
		func(e []NarrativeEvidence) { e[2].Statistic.Extrema = clone(e[1].Statistic.Extrema) },
	} {
		evidence := clone(p.evidence)
		mutate(evidence)
		if _, err := groundedText(statisticalAnswer(), evidence, *o.Narrative); !errors.Is(err, gateway.ErrOutput) {
			t.Fatal("malformed derived record accepted", err)
		}
	}
}
