package nlqroute

import (
	"context"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"strings"
	"testing"
)

func TestSQLRecoveryCompositeJoinProjectionProducer(t *testing.T) {
	items, choices, in := joinProjectionFixture(t)
	for i := range items {
		j := &items[i].publication.Definition.Joins[0]
		j.AdditionalKeys = []semantics.JoinKeyPair{{Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: j.Left.Dataset, ID: "realm"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: j.Right.Dataset, ID: "realm"}}}
		for k := range items[i].publication.Definition.Datasets {
			d := &items[i].publication.Definition.Datasets[k]
			d.Columns = append(d.Columns, semantics.Column{ID: "realm", SourceName: "realm", Name: "Realm", NativeType: "int4", Category: "integer"})
		}
	}
	for i := range in.Relations {
		in.Relations[i].Columns = append(in.Relations[i].Columns, "realm")
	}
	// Independent definitions may order the conjunction differently.
	j := &items[1].publication.Definition.Joins[0]
	old := semantics.JoinKeyPair{Left: j.Left, Right: j.Right}
	j.Left, j.Right = j.AdditionalKeys[0].Left, j.AdditionalKeys[0].Right
	j.AdditionalKeys[0] = old
	if e := attachJoinProjection(context.Background(), &in, items, choices); e != nil {
		t.Fatal(e)
	}
	a, _ := nlq.NewDefaultContextAssembler()
	out, e := a.Assemble(context.Background(), in, nlq.TierHigh)
	if e != nil || strings.Count(out.Prompt, "realm") < 4 {
		t.Fatal("producer omitted composite keys", e)
	}
	items[1].publication.Definition.Joins[0].AdditionalKeys = nil
	if e := confirmJoins(items, choices); e == nil {
		t.Fatal("partial independent relationship accepted")
	}
}
