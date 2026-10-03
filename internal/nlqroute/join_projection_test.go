package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

func joinProjectionFixture(t *testing.T) ([]admittedTopic, []JoinChoice, nlq.ContextInput) {
	t.Helper()
	p, q := recoveryPublication(), recoveryPublication()
	q.State.Topic, q.Definition.Topic = "related", "related"
	p.Definition.Joins[0].Cardinality, q.Definition.Joins[0].Cardinality = semantics.CardinalityOneToOne, semantics.CardinalityOneToOne
	items := []admittedTopic{{id: "topic", publication: p}, {id: "related", publication: q}}
	for i := range items {
		for _, d := range items[i].publication.Definition.Datasets {
			r := readexec.Relation{ID: d.ID, Schema: "analytics", Name: d.ID}
			for _, c := range d.Columns {
				r.Columns = append(r.Columns, readexec.Column{Name: c.SourceName, NativeType: c.NativeType, Category: c.Category, Safe: true})
			}
			items[i].relations = append(items[i].relations, r)
		}
	}
	if err := initialSemanticSelection(context.Background(), RouteRequest{Locale: nlq.LanguageEnglish, Question: "Product family"}, items, nil); err != nil {
		t.Fatal(err)
	}
	for i := range items {
		if err := expandSelectedFacts(context.Background(), &items[i]); err != nil {
			t.Fatal(err)
		}
	}
	metrics, err := applySelectedContext(items, nil)
	if err != nil {
		t.Fatal(err)
	}
	relations, err := sourceRelations(items)
	if err != nil {
		t.Fatal(err)
	}
	input := nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategyMultiTopic, Topic: "topic", TopicVersion: "v1", Topics: topicRevisions(items), Question: "Product family", Relations: relations, Metrics: metrics, Constraints: mergeConstraints(items)}
	return items, []JoinChoice{{Topic: "topic", JoinID: "sales_products"}, {Topic: "related", JoinID: "sales_products"}}, input
}

func TestSQLRecoveryConfirmedJoinProjectionProducer(t *testing.T) {
	items, choices, in := joinProjectionFixture(t)
	before := readexec.Hash(items)
	if err := attachJoinProjection(context.Background(), &in, items, choices); err != nil {
		t.Fatal(err)
	}
	a, _ := nlq.NewDefaultContextAssembler()
	got, err := a.Assemble(context.Background(), in, nlq.TierHigh)
	if err != nil || !strings.Contains(got.Prompt, "confirmed-join-closure-v3") || !reflect.DeepEqual(got.Relations, in.Relations) {
		t.Fatal("confirmed producer", err)
	}
	var lines string
	for _, line := range strings.Split(got.Prompt, "\n") {
		if strings.HasPrefix(line, "relation[") {
			lines += line + "\n"
		}
	}
	for _, key := range []string{"product_id", "family_name"} {
		if !strings.Contains(lines, key) {
			t.Fatal("join key or dimension lost", key)
		}
	}
	if strings.Contains(lines, "sales_amount") || strings.Contains(lines, "sales_cost") {
		t.Fatal("unselected shared physical columns retained", lines)
	}
	if before != readexec.Hash(items) {
		t.Fatal("producer mutated admitted selection or source metadata")
	}
	// The transport shape is data only and detached from the producer inputs.
	encoded, _ := json.Marshal(in)
	var round nlq.ContextInput
	_ = json.Unmarshal(encoded, &round)
	replay, err := a.Assemble(context.Background(), round, nlq.TierHigh)
	if err != nil || replay.Prompt != got.Prompt {
		t.Fatal("deterministic prompt reconstruction", err)
	}
}

func TestSQLRecoveryConfirmedJoinProjectionAdmissionAtomic(t *testing.T) {
	for _, name := range []string{"missing-choice", "foreign-join", "cardinality", "unconfirmed-source", "missing-physical", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			items, choices, in := joinProjectionFixture(t)
			ctx := context.Background()
			switch name {
			case "missing-choice":
				choices = choices[:1]
			case "foreign-join":
				choices[0].JoinID = "other"
			case "cardinality":
				items[0].publication.Definition.Joins[0].Cardinality = semantics.CardinalityManyToOne
			case "unconfirmed-source":
				items[1].publication.Definition.Datasets[0].Source.Source = "other"
			case "missing-physical":
				in.Relations = in.Relations[:1]
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			before := readexec.Hash(in)
			err := attachJoinProjection(ctx, &in, items, choices)
			if err == nil || before != readexec.Hash(in) {
				t.Fatal("incomplete join proof mutated the input", err)
			}
			if name == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
