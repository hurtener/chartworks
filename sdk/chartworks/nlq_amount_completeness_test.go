package chartworks

import (
	"encoding/json"
	"testing"
)

func TestNLQAmountCompletenessWire(t *testing.T) {
	var out NLQRunResult
	raw := []byte(`{"status":"succeeded","amount_completeness":[{"policy":"proved-known-amount-result-v1","metric":"topic:measure:amount","value_column":1,"unknown_count_metric":"topic:kpi:unknown","unknown_count_column":2,"status":"incomplete","scope":"returned_query_rows","rows":[{"row":0,"status":"incomplete","unknown_count":"1"}]}]}`)
	if err := json.Unmarshal(raw, &out); err != nil || len(out.AmountCompleteness) != 1 || out.AmountCompleteness[0].Rows[0].UnknownCount != "1" || out.AmountCompleteness[0].UnknownCountColumn != 2 {
		t.Fatal("completeness evidence lost", err)
	}
	var round NLQRunResult
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &round); err != nil || round.AmountCompleteness[0].Status != "incomplete" {
		t.Fatal("roundtrip", err)
	}
}
