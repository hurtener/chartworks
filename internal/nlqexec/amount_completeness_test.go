package nlqexec

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/exec"
	"testing"
)

func TestProvedAmountCompletenessResult(t *testing.T) {
	receipt := &exec.AnalyticalReceipt{Version: exec.AnalyticalScopedPopulationsVersion, Outputs: []exec.AnalyticalOutput{{Metric: "amount", Column: 1}, {Metric: "missing", Column: 0}}, Completeness: &exec.AnalyticalCompleteness{Policy: exec.AnalyticalCompletenessPolicy, Obligations: []exec.AnalyticalCompletenessObligation{{Metric: "amount", UnknownCount: "missing"}}}}
	for _, tc := range []struct{ raw, status, count string }{{`"0"`, "complete", "0"}, {`"1"`, "incomplete", "1"}, {`"2.000"`, "incomplete", "2"}, {`null`, "unknown", ""}, {`"-1"`, "unknown", ""}, {`"0.5"`, "unknown", ""}, {`"NaN"`, "unknown", ""}, {`true`, "unknown", ""}} {
		result := &exec.Result{Schema: []exec.Field{{Name: "misleading_amount_alias", Type: "integer"}, {Name: "misleading_count_alias", Type: "decimal"}}, Rows: [][]json.RawMessage{{json.RawMessage(tc.raw), json.RawMessage(`"995.00"`)}}, Outcome: "succeeded"}
		got := resultAmountCompleteness(receipt, result, true)
		if len(got) != 1 || got[0].Status != tc.status || len(got[0].Rows) != 1 || got[0].Rows[0].UnknownCount != tc.count || got[0].UnknownCountColumn != 0 || got[0].ValueColumn != 1 {
			t.Fatal(tc, got)
		}
		result.Truncation = "rows"
		if got = resultAmountCompleteness(receipt, result, true); len(got) != 1 || got[0].Status != "unknown" || len(got[0].Rows) != 0 {
			t.Fatal("truncation claimed completeness", got)
		}
	}
	empty := &exec.Result{Schema: []exec.Field{{Type: "integer"}, {Type: "decimal"}}, Outcome: "empty"}
	if got := resultAmountCompleteness(receipt, empty, true); len(got) != 1 || got[0].Status != "complete" {
		t.Fatal("empty query population", got)
	}
	if got := resultAmountCompleteness(receipt, empty, false); len(got) != 1 || got[0].Status != "unknown" {
		t.Fatal("failed/stale result", got)
	}
	receipt.Version = exec.AnalyticalGroupedProgramsVersion
	if got := resultAmountCompleteness(receipt, empty, true); got != nil {
		t.Fatal("old receipt acquired output proof", got)
	}
}
