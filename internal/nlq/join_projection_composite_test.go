package nlq

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSQLRecoveryCompositeJoinProjection(t *testing.T) {
	in := confirmedJoinFixture(t)
	for i := range in.Relations {
		in.Relations[i].Columns = append(in.Relations[i].Columns, "realm")
	}
	for i := range in.Constraints.Required {
		c := &in.Constraints.Required[i]
		if c.Kind != joinProjectionKind {
			continue
		}
		var j ConfirmedJoinProjection
		if json.Unmarshal([]byte(c.Text), &j) != nil {
			t.Fatal("wire")
		}
		j.Version = "confirmed-joins-v2"
		j.AdditionalKeys = []JoinProjectionPair{{Left: JoinProjectionColumn{Dataset: j.Left.Dataset, ID: "realm", Name: "realm"}, Right: JoinProjectionColumn{Dataset: j.Right.Dataset, ID: "realm", Name: "realm"}}}
		raw, _ := json.Marshal(j)
		c.Text = string(raw)
	}
	a, _ := NewDefaultContextAssembler()
	out, e := a.Assemble(context.Background(), in, TierHigh)
	if e != nil || strings.Count(out.Prompt, "realm") < 4 {
		t.Fatal("complete composite key not projected", e)
	}
	last := len(in.Constraints.Required) - 1
	var j ConfirmedJoinProjection
	_ = json.Unmarshal([]byte(in.Constraints.Required[last].Text), &j)
	j.AdditionalKeys = nil
	raw, _ := json.Marshal(j)
	in.Constraints.Required[last].Text = string(raw)
	if _, e = a.Assemble(context.Background(), in, TierHigh); e == nil {
		t.Fatal("partial independent confirmation accepted")
	}
	j.Version = "confirmed-joins-v1"
	j.AdditionalKeys = []JoinProjectionPair{{Left: j.Left, Right: j.Right}}
	raw, _ = json.Marshal(j)
	in.Constraints.Required[last].Text = string(raw)
	if _, e = a.Assemble(context.Background(), in, TierHigh); e == nil {
		t.Fatal("legacy policy gained composite authority")
	}
}
