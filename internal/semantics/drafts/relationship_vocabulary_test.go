package drafts

import (
	"errors"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"reflect"
	"testing"
)

func TestRelationshipVocabularyRequiresExactConfirmedInnerPath(t *testing.T) {
	model, _, value := vocabularyFixture(t)
	pack := model.Pack()
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: "accounts", ID: "column_00"}
	measure := semantics.GeneratedEntityID(semantics.EnhancementMeasure, field.Dataset, field.ID)
	join := semantics.Join{ID: "accounts_orders", Name: "Reviewed parent relationship", Left: field, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "column_00"}, Type: semantics.JoinInner, Cardinality: semantics.CardinalityManyToOne, Evidence: semantics.RelationshipEvidence{ID: "human_review", LeftGrain: "account", RightGrain: "order", Provenance: "reviewed_profile"}}
	pack.Joins = []semantics.Join{join}
	model, err := semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	proposal := VocabularyFilterProposal{Measure: measure, ID: "parent_paid", Operator: "eq", Nulls: "exclude", VocabularyIDs: []string{value.ID}, JoinID: join.ID}
	filter, err := ResolveAuthoringFilter(model, []AuthoringValue{value}, proposal)
	if err != nil || filter.Relationship != join.ID || filter.Field != value.Field {
		t.Fatal("relationship origin not retained", filter, err)
	}
	for name, mutate := range map[string]func(*semantics.TopicPack, *VocabularyFilterProposal){"missing": func(p *semantics.TopicPack, v *VocabularyFilterProposal) { v.JoinID = "" }, "unknown": func(p *semantics.TopicPack, v *VocabularyFilterProposal) { v.JoinID = "unknown" }, "candidate": func(p *semantics.TopicPack, v *VocabularyFilterProposal) {
		p.Joins = nil
		p.RelationshipDecisions = []semantics.RelationshipDecision{{ID: join.ID, Left: join.Left, Right: join.Right, Cardinality: join.Cardinality, State: "candidate", Evidence: join.Evidence}}
	}, "left": func(p *semantics.TopicPack, v *VocabularyFilterProposal) { p.Joins[0].Type = semantics.JoinLeft }, "inverted": func(p *semantics.TopicPack, v *VocabularyFilterProposal) {
		p.Joins[0].Left, p.Joins[0].Right = p.Joins[0].Right, p.Joins[0].Left
	}, "multiplying": func(p *semantics.TopicPack, v *VocabularyFilterProposal) {
		p.Joins[0].Cardinality = semantics.CardinalityOneToMany
	}, "unreviewed": func(p *semantics.TopicPack, v *VocabularyFilterProposal) {
		p.Joins[0].Evidence = semantics.RelationshipEvidence{}
	}} {
		t.Run(name, func(t *testing.T) {
			p := model.Pack()
			v := proposal
			mutate(&p, &v)
			m, err := semantics.Compile(p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = ResolveAuthoringFilter(m, []AuthoringValue{value}, v); !errors.Is(err, gateway.ErrOutput) {
				t.Fatal("unadmitted population path accepted", err)
			}
		})
	}
	primary := semantics.Enhancement{Dataset: field.Dataset, Column: field.ID, Kind: semantics.EnhancementMeasure, Name: "Known child sum", Aggregation: semantics.AggregationSum, Filters: []semantics.SemanticFilter{filter}}
	changed, err := semantics.ApplyEnhancements(model, "v2", []semantics.Enhancement{primary})
	if err != nil {
		t.Fatal(err)
	}
	for _, relationship := range []string{"", "changed"} {
		candidate := primary
		candidate.Filters = append([]semantics.SemanticFilter(nil), primary.Filters...)
		candidate.Filters[0].Relationship = relationship
		if err := preserveProtectedEnhancementMeaning(changed.Pack(), []semantics.Enhancement{candidate}); !errors.Is(err, gateway.ErrOutput) {
			t.Fatal("relationship changed or dropped without approval", err)
		}
	}
	omitted := primary
	omitted.Filters = nil
	proposals := []semantics.Enhancement{omitted}
	if err := preserveProtectedEnhancementMeaning(changed.Pack(), proposals); err != nil || !reflect.DeepEqual(proposals[0].Filters, primary.Filters) {
		t.Fatal("relationship not inherited intact", err)
	}
}
