package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

func authoringFixture(t *testing.T) (semantics.Model, map[string]engineering.ProfileEvidence) {
	t.Helper()
	p := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: "commerce", Version: "v1", Name: "Commerce", Description: "Revenue excludes refunded orders"}
	profiles := map[string]engineering.ProfileEvidence{}
	for _, id := range []string{"accounts", "orders"} {
		pr := engineering.Profile{Version: id + "_profile", Source: "warehouse", Context: "context", Dataset: id, SourceRevision: 1, ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), PolicyHash: strings.Repeat("a", 64), Sampling: engineering.Sampling{Rows: 8, Strategy: "bounded_prefix", Complete: false}, ReadOperation: "PRIVATE_READ_OPERATION_CANARY"}
		d := semantics.Dataset{ID: id, Name: id}
		for i := 0; i < 18; i++ {
			name := fmt.Sprintf("column_%02d", i)
			pr.Schema = append(pr.Schema, readexec.Column{Name: name, NativeType: "numeric", Category: "decimal", Safe: true})
			pr.Columns = append(pr.Columns, engineering.ColumnProfile{Name: name, Observed: 8, Nulls: 1, Distinct: 4, DistinctExact: true, Families: map[string]int{"decimal": 7, "null": 1}})
			d.Columns = append(d.Columns, semantics.Column{ID: name, SourceName: name, Name: name, NativeType: "numeric", Category: "decimal", Sensitivity: semantics.LiteralNonSensitive})
		}
		d.Source = semantics.SourceReference{Source: pr.Source, Context: pr.Context, Dataset: id, SourceRevision: pr.SourceRevision, ProfileVersion: pr.Version, ProfileDigest: pr.DeterministicHash()}
		profiles[id] = engineering.ProfileEvidence{Profile: pr, Active: true}
		p.Datasets = append(p.Datasets, d)
	}
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "column_00"}
	p.Measures = []semantics.Measure{{ID: "revenue", Name: "Revenue", Description: "Net revenue after refunds", Field: field, Aggregation: semantics.AggregationSum, Unit: "USD", Aliases: []string{"net sales"}}}
	model, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	return model, profiles
}

func TestAuthoringContextCarriesBusinessAndBoundedProfileEvidence(t *testing.T) {
	model, profiles := authoringFixture(t)
	p := model.Pack()
	e := profiles["orders"]
	canary := "PRIVATE_RANGE_CANARY"
	e.Profile.Columns[0].Minimum = &canary
	e.Profile.Columns[0].Maximum = &canary
	e.Profile.Summary.Text = "PRIVATE_SUMMARY_CANARY"
	profiles["orders"] = e
	for i := range p.Datasets {
		if p.Datasets[i].ID == "orders" {
			p.Datasets[i].Source.ProfileDigest = e.Profile.DeterministicHash()
			p.Datasets[i].Columns[0].Sensitivity = semantics.LiteralSensitive
		}
	}
	model, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(packet)
	for _, want := range []string{"Revenue excludes refunded orders", "Net revenue after refunds", "net sales", "sample_distinct", "policy_digest", model.Digest(), e.Profile.DeterministicHash()} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, forbidden := range []string{"PRIVATE_RANGE_CANARY", "PRIVATE_SUMMARY_CANARY", "PRIVATE_READ_OPERATION_CANARY", "minimum", "maximum"} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("leaked %s", forbidden)
		}
	}
	if len(packet.Relationships) != 36 || len(packet.Evidence) != 2 || packet.Digest == "" {
		t.Fatal("incomplete packet", packet)
	}
	for _, e := range packet.Evidence {
		if e.Origin.Dataset == "orders" && len(e.Columns[0].Families) != 0 {
			t.Fatal("sensitive family hints escaped")
		}
	}
	// Changing business context or observed aggregates changes actual model material.
	changed := model.Pack()
	changed.Description = "Revenue includes refunded orders"
	m2, err := semantics.Compile(changed)
	if err != nil {
		t.Fatal(err)
	}
	packet2, err := buildAuthoringContext(m2, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if packet.Digest == packet2.Digest {
		t.Fatal("business definition omitted from digest")
	}
	aggregateEvidence := profiles["orders"]
	aggregateEvidence.Profile.Columns[0].Distinct = 5
	profiles["orders"] = aggregateEvidence
	aggregatePack := model.Pack()
	for i := range aggregatePack.Datasets {
		if aggregatePack.Datasets[i].ID == "orders" {
			aggregatePack.Datasets[i].Source.ProfileDigest = aggregateEvidence.Profile.DeterministicHash()
		}
	}
	aggregateModel, aggregateErr := semantics.Compile(aggregatePack)
	if aggregateErr != nil {
		t.Fatal(aggregateErr)
	}
	aggregatePacket, aggregateErr := buildAuthoringContext(aggregateModel, profiles)
	if aggregateErr != nil {
		t.Fatal(aggregateErr)
	}
	if aggregatePacket.Digest == packet.Digest || aggregatePacket.Evidence[1].Columns[0].Distinct != 5 {
		t.Fatal("changed profile evidence did not reach authoring packet")
	}
	if model.Pack().Measures[0].Description != "Net revenue after refunds" {
		t.Fatal("context mutated source model")
	}
}

func TestAuthoringContextRejectsDriftAndBudgetBeforeModel(t *testing.T) {
	model, profiles := authoringFixture(t)
	for _, mutation := range []func(*engineering.ProfileEvidence){func(e *engineering.ProfileEvidence) { e.Active = false }, func(e *engineering.ProfileEvidence) { e.Profile.Context = "other_context" }, func(e *engineering.ProfileEvidence) { e.Profile.Columns[0].Nulls++ }} {
		_, pristine := authoringFixture(t)
		clone := map[string]engineering.ProfileEvidence{}
		for k, v := range pristine {
			clone[k] = v
		}
		value := clone["orders"]
		mutation(&value)
		clone["orders"] = value
		if _, err := buildAuthoringContext(model, clone); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("profile drift accepted", err)
		}
	}
	// No profile/source service can be reached with missing signed authority.
	if _, err := (&Service{}).authoringContext(context.Background(), identity.Envelope{}, model); err == nil {
		t.Fatal("unsigned authoring accepted")
	}
	p := model.Pack()
	for i := 0; i < 40; i++ {
		m := p.Measures[0]
		m.ID = fmt.Sprintf("metric_%02d", i)
		m.Description = strings.Repeat("x", 4096)
		p.Measures = append(p.Measures, m)
	}
	large, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = buildAuthoringContext(large, profiles); !errors.Is(err, gateway.ErrBudget) {
		t.Fatal("mandatory context silently truncated", err)
	}
}

func TestAuthoringRelationshipCatalogCrossesPagesWithoutWidening(t *testing.T) {
	model, profiles := authoringFixture(t)
	packet, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	left, right := packet.Relationships[0], packet.Relationships[35]
	relation := semantics.RelationshipDecision{ID: "accounts_orders", Left: left, Right: right, Cardinality: semantics.CardinalityOneToMany, State: "candidate", Evidence: semantics.RelationshipEvidence{ID: "proposal", LeftGrain: "account", RightGrain: "order", Provenance: "profile_sample"}}
	for _, selected := range [][]semantics.Reference{{left}, {right}, {left, right}} {
		wire := enhancementWire{Relationships: []semantics.RelationshipDecision{relation}}
		for _, ref := range selected {
			wire.Results = append(wire.Results, semantics.Enhancement{Dataset: ref.Dataset, Column: ref.ID, Kind: semantics.EnhancementUnresolved, Reason: "Business meaning requires review"})
		}
		if err := validateEnhancementOutput(selected, nil, wire, packet.Relationships); err != nil {
			t.Fatal("cross-page relationship rejected", err)
		}
		changed, err := semantics.ApplyRichEnhancements(model, "v2", wire.Results, nil, wire.Relationships)
		if err != nil {
			t.Fatal(err)
		}
		if len(changed.Pack().Joins) != 0 || len(changed.Pack().RelationshipDecisions) != 1 {
			t.Fatal("proposal became executable join")
		}
		wire.Relationships[0].Right.Dataset = "unapproved"
		if err := validateEnhancementOutput(selected, nil, wire, packet.Relationships); !errors.Is(err, gateway.ErrOutput) {
			t.Fatal("unapproved endpoint accepted", err)
		}
	}
	p := model.Pack()
	p.Datasets[0], p.Datasets[1] = p.Datasets[1], p.Datasets[0]
	for i := range p.Datasets {
		for a, b := 0, len(p.Datasets[i].Columns)-1; a < b; a, b = a+1, b-1 {
			p.Datasets[i].Columns[a], p.Datasets[i].Columns[b] = p.Datasets[i].Columns[b], p.Datasets[i].Columns[a]
		}
	}
	shuffled, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	again, err := buildAuthoringContext(shuffled, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if packet.Digest != again.Digest || !reflect.DeepEqual(packet.Relationships, again.Relationships) {
		t.Fatal("input order changed relationship context")
	}
}

func TestEnhancementPreservesWithheldProtectedMeaning(t *testing.T) {
	model, _ := authoringFixture(t)
	p := model.Pack()
	ref := p.Measures[0].Field
	p.Measures[0].ID = semantics.GeneratedEntityID(semantics.EnhancementMeasure, ref.Dataset, ref.ID)
	p.Measures[0].Filters = []semantics.SemanticFilter{{ID: "population", Field: ref, Operator: "eq", Values: []string{"PRIVATE_FILTER_CANARY"}}}
	proposals := []semantics.Enhancement{{Dataset: ref.Dataset, Column: ref.ID, Kind: semantics.EnhancementMeasure}}
	if err := preserveProtectedEnhancementMeaning(p, proposals); err != nil || !reflect.DeepEqual(proposals[0].Filters, p.Measures[0].Filters) {
		t.Fatal("protected filters lost", err)
	}
	proposals[0].Kind = semantics.EnhancementDimension
	if err := preserveProtectedEnhancementMeaning(p, proposals); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("protected kind switched", err)
	}
	material := authoringContext{Candidate: p}
	redactAuthoringCandidate(&material)
	raw, _ := json.Marshal(material)
	if strings.Contains(string(raw), "PRIVATE_FILTER_CANARY") || len(material.Redactions) != 1 {
		t.Fatal("filter redaction failed")
	}
}

func TestCrossPageCompositeRelationshipsValidateEveryKeyAndSealOrigin(t *testing.T) {
	model, profiles := authoringFixture(t)
	packet, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	a, b, c, d := packet.Relationships[0], packet.Relationships[18], packet.Relationships[1], packet.Relationships[19]
	wire := enhancementWire{Results: []semantics.Enhancement{{Dataset: c.Dataset, Column: c.ID, Kind: semantics.EnhancementUnresolved, Reason: "Review"}}, Relationships: []semantics.RelationshipDecision{{ID: "relation", Left: a, Right: b, AdditionalKeys: []semantics.JoinKeyPair{{Left: c, Right: d}}, State: "candidate", Evidence: semantics.RelationshipEvidence{Provenance: "invented_approval"}}}}
	if err := validateEnhancementOutput([]semantics.Reference{c}, nil, wire, packet.Relationships); err != nil {
		t.Fatal("additional current-page key rejected", err)
	}
	sealRelationshipProvenance(model.Pack(), wire.Relationships, packet.Digest)
	if wire.Relationships[0].Evidence.Provenance != "authoring:"+packet.Digest {
		t.Fatal("model invented evidence origin")
	}
	wire.Relationships[0].AdditionalKeys[0].Right.Dataset = "outside"
	if err := validateEnhancementOutput([]semantics.Reference{c}, nil, wire, packet.Relationships); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("additional key escaped catalog", err)
	}
}
