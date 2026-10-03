package topicfeedback

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"testing"
)

func TestProposalVocabularyFiltersAndValues(t *testing.T) {
	p := proposalTestPack()
	p.Datasets[0].Columns = append(p.Datasets[0].Columns, semantics.Column{ID: "status", SourceName: "status", Name: "Status", NativeType: "text", Category: "text", Sensitivity: semantics.LiteralNonSensitive})
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "status"}
	p.Dimensions = []semantics.Dimension{{ID: "status", Name: "Status", Description: "Order state", Role: semantics.DimensionCategorical, Field: field}}
	catalog := []drafts.AuthoringValue{{ID: "paid", Field: field, Origin: p.Datasets[0].Source, Kind: "text", Value: "P", Sensitivity: semantics.LiteralNonSensitive, Nulls: "exclude"}}
	filter := drafts.VocabularyFilterProposal{Measure: "revenue", ID: "paid_only", Operator: "eq", Nulls: "exclude", VocabularyIDs: []string{"paid"}}
	edits := []Edit{{Kind: semantics.KindMeasure, ID: "revenue", NewFilters: []drafts.VocabularyFilterProposal{filter}}, {Kind: semantics.KindDimension, ID: "status", VocabularyIDs: []string{"paid"}}}
	out, err := applyEdits(p, "v2", edits, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Measures[0].Filters) != 2 || len(out.Dimensions[0].Values) != 1 || out.Dimensions[0].Values[0].Provenance.Kind != "author_input" {
		t.Fatal("missing governed filter/value", out)
	}
	if len(p.Measures[0].Filters) != 1 || len(p.Dimensions[0].Values) != 0 {
		t.Fatal("mutated input")
	}
	if _, err = applyEdits(p, "v2", edits); err == nil {
		t.Fatal("new values accepted without admitted catalog")
	}
	schema, _ := proposalSchema()
	raw, _ := json.Marshal(map[string]any{"edits": edits})
	if err = schema.Validate(raw, 65536); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"edits":[{"kind":"measure","id":"revenue","new_filters":[{"measure":"revenue","id":"paid_only","operator":"eq","nulls":"exclude","vocabulary_ids":["paid"],"values":["secret"]}]}]}`, `{"edits":[{"kind":"measure","id":"revenue","new_filters":[{"measure":"revenue","id":"paid_only","operator":"eq","nulls":"include","vocabulary_ids":["paid"]}]}]}`} {
		if schema.Validate([]byte(raw), 65536) == nil {
			t.Fatal("open literal output")
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*semantics.TopicPack, []drafts.AuthoringValue, []Edit)
	}{
		{"foreign_origin", func(_ *semantics.TopicPack, c []drafts.AuthoringValue, _ []Edit) {
			c[0].Origin.ProfileVersion = "foreign"
		}},
		{"sensitive", func(p *semantics.TopicPack, _ []drafts.AuthoringValue, _ []Edit) {
			p.Datasets[0].Columns[1].Sensitivity = semantics.LiteralSensitive
		}},
		{"unknown_sensitivity", func(p *semantics.TopicPack, _ []drafts.AuthoringValue, _ []Edit) {
			p.Datasets[0].Columns[1].Sensitivity = ""
		}},
		{"wrong_type", func(p *semantics.TopicPack, _ []drafts.AuthoringValue, _ []Edit) {
			p.Datasets[0].Columns[1].Category = "decimal"
		}},
		{"unknown_id", func(_ *semantics.TopicPack, _ []drafts.AuthoringValue, e []Edit) {
			e[0].NewFilters[0].VocabularyIDs = []string{"absent"}
		}},
		{"null_policy", func(_ *semantics.TopicPack, _ []drafts.AuthoringValue, e []Edit) {
			e[0].NewFilters[0].Nulls = "include"
		}},
		{"wrong_measure", func(_ *semantics.TopicPack, _ []drafts.AuthoringValue, e []Edit) {
			e[0].NewFilters[0].Measure = "other"
		}},
		{"overwrite", func(_ *semantics.TopicPack, _ []drafts.AuthoringValue, e []Edit) { e[0].NewFilters[0].ID = "known" }},
		{"unsafe_operator", func(_ *semantics.TopicPack, _ []drafts.AuthoringValue, e []Edit) {
			e[0].NewFilters[0].Operator = "like"
		}},
		{"wrong_dimension", func(_ *semantics.TopicPack, _ []drafts.AuthoringValue, e []Edit) { e[1].ID = "unknown" }},
		{"duplicate_alias", func(_ *semantics.TopicPack, c []drafts.AuthoringValue, _ []Edit) { c[0].Aliases = []string{"P"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(struct {
				P semantics.TopicPack
				C []drafts.AuthoringValue
				E []Edit
			}{p, catalog, edits})
			var clone struct {
				P semantics.TopicPack
				C []drafts.AuthoringValue
				E []Edit
			}
			_ = json.Unmarshal(raw, &clone)
			tc.mutate(&clone.P, clone.C, clone.E)
			if _, err := applyEdits(clone.P, "v2", clone.E, clone.C); err == nil {
				t.Fatal("accepted unsafe vocabulary")
			}
		})
	}
}

func TestProposalVocabularyEmptyDigestStable(t *testing.T) {
	p := Proposal{Vocabulary: []drafts.AuthoringValue{}}
	before := proposalDigest(p)
	raw, _ := json.Marshal(p)
	var after Proposal
	if err := json.Unmarshal(raw, &after); err != nil || proposalDigest(after) != before {
		t.Fatal("empty catalog changed persisted digest", err)
	}
}
