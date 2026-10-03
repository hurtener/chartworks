package acceptance

import (
	"context"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestStatisticalNarrativeRetainedLifecycle(t *testing.T) {
	f := statisticalNarrativeFixture(t)
	ctx := context.Background()
	if _, err := f.f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=CASE id WHEN 1 THEN 3 ELSE 1 END,created_at=CASE id WHEN 1 THEN TIMESTAMPTZ '2026-01-03T00:00:00Z' ELSE TIMESTAMPTZ '2026-01-01T00:00:00Z' END; INSERT INTO analytics.sales(id,amount,created_at,name,active) VALUES(3,NULL,'2026-01-02T00:00:00Z','public-null',true),(4,1,'2026-01-04T00:00:00Z','public-tie',true)`); err != nil {
		t.Fatal(err)
	}
	query, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	execute := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
	legacy := phase27Definition(t, f, author, "SELECT created_at,amount FROM analytics.sales ORDER BY id")
	legacy.Outputs[1].Narrative.SchemaVersion = "grounded-narrative-v1"
	d, err := reporting.MigrateDefinition(legacy)
	if err != nil {
		t.Fatal(err)
	}
	n := d.Outputs[1].Narrative
	n.PolicyVersion, n.SchemaVersion, n.Reduction = reporting.StatisticalNarrativePolicyVersion, reporting.StatisticalNarrativeSchemaVersion, "statistical_evidence"
	n.Type, n.Instructions, n.Tone = "explanation", "evidence_only", "concise"
	n.MaxRows, n.MaxBytes, n.MaxCharacters, n.MaxTokens, n.MaxClaims = 4, 16384, 6000, 8192, 3
	value := reporting.NarrativeFieldRef{Column: 1, Field: d.ExpectedSchema[1]}
	when := reporting.NarrativeTimeOrder{Field: reporting.NarrativeFieldRef{Column: 0, Field: d.ExpectedSchema[0]}, Meaning: "instant"}
	n.Statistics = []reporting.NarrativeStatistic{{ID: "chronology", Kind: "trend", Value: value, Time: &when}, {ID: "range", Kind: "extrema", Value: value}, {ID: "spread", Kind: "population_variance", Value: value}}
	created, err := blocks.Create(ctx, author, reporting.CreateRequest{ID: "statistical-evidence", Definition: d})
	if err != nil {
		t.Fatal("author statistics", err)
	}
	phase27ValidatePublish(t, blocks, author, created)
	model := newGatewayFixture(t, nil)
	runs := phase28RunService(t, f, blocks, f.f.db, model.engine, config.DefaultReportingExecution())
	model.mode.Store(phase28Chat(t, model.cfg.Roles["narrative"].Model, `{"claims":[{"kind":"trend","evidence":["e1"]},{"kind":"extrema","evidence":["e2"]},{"kind":"population_variance","evidence":["e3"]}]}`))
	accepted, err := runs.Admit(ctx, execute, created.State.ID, reporting.RunRequest{Key: "statistics-run", Outputs: []string{"narrative-main", "table-main"}, Narrative: true, PartialPolicy: "allow_partial"})
	if err != nil {
		t.Fatal(err)
	}
	before := model.requests.Load()
	done, err := runs.Run(ctx, execute, accepted.ID, false)
	if err != nil || done.State != "succeeded" || len(done.QueryAttempts) != 1 {
		t.Fatal("statistical execution", done.State, err)
	}
	out, err := runs.Output(ctx, execute, done.ID, "narrative-main")
	if err != nil || out.Narrative == nil || len(out.Narrative.Evidence) != 3 || model.requests.Load() != before+1 {
		t.Fatal("statistical retained output", out.Code, err)
	}
	e := out.Narrative.Evidence
	trend, extrema, variance := e[0].Statistic, e[1].Statistic, e[2].Statistic
	if trend == nil || trend.Trend == nil || !statisticalExact(trend.Trend.Difference, "0") || trend.Trend.Sequence != "mixed" || !reflect.DeepEqual(trend.Population.SourceRows, []int{1, 0, 3}) || !reflect.DeepEqual(trend.Population.NullRows, []int{2}) {
		t.Fatal("trend guessed source order or lost null provenance")
	}
	if extrema == nil || extrema.Extrema == nil || !statisticalExact(extrema.Extrema.Minimum, "1") || !statisticalExact(extrema.Extrema.Maximum, "3") || extrema.Extrema.MinimumTies != 2 || extrema.Extrema.MinimumRow != 1 || extrema.Extrema.MaximumRow != 0 {
		t.Fatal("extrema lost exact values/ties/source rows")
	}
	if variance == nil || variance.Variance == nil || variance.Variance.Numerator != "8" || variance.Variance.Denominator != "9" || variance.Variance.Divisor != 3 || variance.Population.ConsideredRows != 4 || variance.Population.IncludedRows != 3 {
		t.Fatal("population variance rounded or changed denominator")
	}
	if !strings.Contains(out.Narrative.Text, "8/9") || !strings.Contains(out.Narrative.Text, "mixed") {
		t.Fatal("deterministic text omitted exact statistic", out.Narrative.Text)
	}
	record, err := f.f.db.ReadFrozenRun(ctx, execute, done.ID, false)
	if err != nil || record.Manifest == nil || record.Result == nil {
		t.Fatal(err)
	}
	for _, change := range []func(*reporting.NarrativeStatisticEvidence){func(s *reporting.NarrativeStatisticEvidence) { s.Trend.Difference = "999" }, func(s *reporting.NarrativeStatisticEvidence) { s.Population.SourceRows[0] = 99 }, func(s *reporting.NarrativeStatisticEvidence) { s.Population.ConsideredRows++ }, func(s *reporting.NarrativeStatisticEvidence) { s.Value.Column = 0 }, func(s *reporting.NarrativeStatisticEvidence) { s.Provenance = strings.Repeat("a", 64) }} {
		damaged := phase27Copy(t, out)
		change(damaged.Narrative.Evidence[0].Statistic)
		damaged.Narrative.EvidenceHash = readexec.Hash(damaged.Narrative.Evidence)
		if err := reporting.CheckFrozenNarrativeEvidence(*record.Manifest, damaged, *record.Result); err == nil {
			t.Fatal("forged derived evidence accepted after rehash")
		}
	}
	lookups := f.f.lookups.Load()
	model.mode.Store("error")
	replay, err := runs.RebuildOutput(ctx, execute, done.ID, "narrative-main")
	if err != nil || replay.Digest != out.Digest || model.requests.Load() != before+1 || f.f.lookups.Load() != lookups {
		t.Fatal("retained statistical read repeated source/provider", err)
	}
	foreign := phase28Reader(t, f, "stats-foreign-context", created.State.ID, "other-context")
	if _, err := runs.Output(ctx, foreign, done.ID, "narrative-main"); err == nil {
		t.Fatal("statistics crossed context boundary")
	}
}

func statisticalExact(actual, expected string) bool {
	a, ok := new(big.Rat).SetString(actual)
	b, want := new(big.Rat).SetString(expected)
	return ok && want && a.Cmp(b) == 0
}

func TestStatisticalNarrativeCompanionEgress(t *testing.T) {
	f := statisticalNarrativeFixture(t)
	ctx := context.Background()
	if _, err := f.f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=CASE id WHEN 1 THEN 3 ELSE 1 END,created_at=CASE id WHEN 1 THEN TIMESTAMPTZ '2026-01-03T00:00:00Z' ELSE TIMESTAMPTZ '2026-01-01T00:00:00Z' END; INSERT INTO analytics.sales(id,amount,created_at,name,active) VALUES(3,NULL,'2026-01-02T00:00:00Z','public-null',true),(4,1,'2026-01-04T00:00:00Z','public-tie',true)`); err != nil {
		t.Fatal(err)
	}
	query, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	execute := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
	legacy := phase27Definition(t, f, author, "SELECT created_at,amount,CASE WHEN amount IS NULL THEN 123456789 ELSE 0 END AS unknown_count FROM analytics.sales ORDER BY id")
	legacy.Outputs[1].Narrative.SchemaVersion = "grounded-narrative-v1"
	base, err := reporting.MigrateDefinition(legacy)
	if err != nil {
		t.Fatal(err)
	}
	base.AmountCompleteness = []reporting.AmountDeclaration{{ID: "known-amount", Label: "Known amount", Policy: reporting.ReviewedAmountCompletenessPolicy, Metric: "reviewed-row-amount", ValueColumn: 1, ValueField: base.ExpectedSchema[1], UnknownCountMetric: "reviewed-unknown-count", UnknownCountColumn: 2, UnknownCountField: base.ExpectedSchema[2]}}
	for i := range base.Outputs {
		base.Outputs[i].AmountCompleteness = []reporting.AmountOutputBinding{{Declaration: "known-amount", Role: "amount"}, {Declaration: "known-amount", Role: "unknown_count"}}
	}
	n := base.Outputs[1].Narrative
	n.PolicyVersion, n.SchemaVersion, n.Reduction = reporting.StatisticalNarrativePolicyVersion, reporting.StatisticalNarrativeSchemaVersion, "statistical_evidence"
	n.Type, n.Instructions, n.Tone = "explanation", "evidence_only", "concise"
	n.MaxRows, n.MaxBytes, n.MaxCharacters, n.MaxTokens, n.MaxClaims = 4, 16384, 6000, 8192, 3
	value := reporting.NarrativeFieldRef{Column: 1, Field: base.ExpectedSchema[1]}
	when := reporting.NarrativeTimeOrder{Field: reporting.NarrativeFieldRef{Column: 0, Field: base.ExpectedSchema[0]}, Meaning: "instant"}
	n.Statistics = []reporting.NarrativeStatistic{{ID: "chronology", Kind: "trend", Value: value, Time: &when}, {ID: "range", Kind: "extrema", Value: value}, {ID: "spread", Kind: "population_variance", Value: value}}
	model := newGatewayFixture(t, nil)
	runs := phase28RunService(t, f, blocks, f.f.db, model.engine, config.DefaultReportingExecution())
	for _, name := range []string{"allowed", "sensitive_companion", "redacted_companion", "sensitive_value", "sensitive_time", "insufficient_rows", "invalid_claim"} {
		t.Run(name, func(t *testing.T) {
			d := phase27Copy(t, base)
			switch name {
			case "sensitive_companion":
				d.ResultPolicy = []reporting.ResultFieldPolicy{{Field: "unknown_count", Sensitivity: "sensitive"}}
			case "redacted_companion":
				d.ResultPolicy = []reporting.ResultFieldPolicy{{Field: "unknown_count", Redacted: true}}
			case "sensitive_value":
				d.ResultPolicy = []reporting.ResultFieldPolicy{{Field: "amount", Sensitivity: "sensitive"}}
			case "sensitive_time":
				d.ResultPolicy = []reporting.ResultFieldPolicy{{Field: "created_at", Sensitivity: "sensitive"}}
			case "insufficient_rows":
				d.Outputs[1].Narrative.MaxRows = 1
			}
			created, err := blocks.Create(ctx, author, reporting.CreateRequest{ID: "statistics-" + name, Definition: d})
			if err != nil {
				t.Fatal(err)
			}
			phase27ValidatePublish(t, blocks, author, created)
			model.mode.Store(phase28Chat(t, model.cfg.Roles["narrative"].Model, `{"claims":[{"kind":"population_variance","evidence":["e3"]}]}`))
			if name == "invalid_claim" {
				model.mode.Store(phase28Chat(t, model.cfg.Roles["narrative"].Model, `{"claims":[{"kind":"trend","evidence":["e3"]}]}`))
			}
			accepted, err := runs.Admit(ctx, execute, created.State.ID, reporting.RunRequest{Key: "statistics-" + name, Outputs: []string{"narrative-main", "table-main"}, Narrative: true, PartialPolicy: "allow_partial"})
			if err != nil {
				t.Fatal(err)
			}
			before := model.requests.Load()
			model.mu.Lock()
			start := len(model.requestBodies)
			model.mu.Unlock()
			done, err := runs.Run(ctx, execute, accepted.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			out, err := runs.Output(ctx, execute, done.ID, "narrative-main")
			if err != nil {
				t.Fatal(err)
			}
			if name == "allowed" {
				if out.State != "succeeded" || out.Narrative == nil || model.requests.Load() != before+1 {
					t.Fatal("allowed evidence unavailable", out.Code)
				}
				for _, e := range out.Narrative.Evidence {
					if e.Statistic == nil || e.Statistic.Amount == nil || e.Statistic.Amount.Status != "incomplete" {
						t.Fatal("known amount status lost")
					}
				}
				if !strings.Contains(strings.ToLower(out.Narrative.Text), "known") || !strings.Contains(out.Narrative.Text, "8/9") {
					t.Fatal("known-amount exact variance text", out.Narrative.Text)
				}
			} else if name == "invalid_claim" {
				if out.State != "failed" || out.Code != "narrative_failed" || out.ReservedCalls != 1 || model.requests.Load() != before+1 {
					t.Fatal("mismatched statistical claim accepted or budget refunded", out.Code)
				}
			} else if out.State != "failed" || out.Code != "narrative_evidence_unavailable" || out.ReservedCalls != 0 || out.ReservedTokens != 0 || model.requests.Load() != before {
				t.Fatal("restricted statistic reached model", out.Code, out.ReservedCalls)
			}
			model.mu.Lock()
			wire := strings.Join(model.requestBodies[start:], "\n")
			model.mu.Unlock()
			if strings.Contains(wire, "123456789") {
				t.Fatal("raw companion escaped through derived evidence")
			}
			table, err := runs.Output(ctx, execute, done.ID, "table-main")
			if err != nil || table.State != "succeeded" || table.Chart == nil || len(table.AmountCompleteness) == 0 {
				t.Fatal("model egress policy erased independently authorized raw output", err)
			}
		})
	}
}

func statisticalNarrativeFixture(t *testing.T) *phase17Fixture {
	t.Helper()
	// Real profiling includes the temporal field, and an explicit synthetic
	// publication classifies every available dependency before narrative egress.
	return newCW01FixtureWithPack(t, func(p *semantics.TopicPack) {
		for di := range p.Datasets {
			for ci := range p.Datasets[di].Columns {
				p.Datasets[di].Columns[ci].Sensitivity = "non_sensitive"
			}
		}
	}).phase17Fixture
}
