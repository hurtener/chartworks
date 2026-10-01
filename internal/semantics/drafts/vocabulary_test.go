package drafts

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

func vocabularyFixture(t *testing.T) (semantics.Model, map[string]engineering.ProfileEvidence, AuthoringValue) {
	t.Helper()
	model, profiles := authoringFixture(t)
	p := model.Pack()
	profile := profiles["orders"]
	profile.Profile.Schema[17].NativeType = "text"
	profile.Profile.Schema[17].Category = "text"
	profiles["orders"] = profile
	var origin semantics.SourceReference
	for i := range p.Datasets {
		if p.Datasets[i].ID == "orders" {
			p.Datasets[i].Columns[17].NativeType = "text"
			p.Datasets[i].Columns[17].Category = "text"
			p.Datasets[i].Source.ProfileDigest = profile.Profile.DeterministicHash()
			origin = p.Datasets[i].Source
		}
	}
	model, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	return model, profiles, AuthoringValue{ID: "paid_status", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "column_17"}, Origin: origin, Kind: "text", Value: "P", Aliases: []string{"paid"}, Sensitivity: semantics.LiteralNonSensitive, Nulls: "exclude"}
}

func TestVocabularyAdmissionRejectsSensitivityOriginTypeAndAmbiguity(t *testing.T) {
	model, _, value := vocabularyFixture(t)
	canonical, err := AdmitAuthoringVocabulary(model, []AuthoringValue{value})
	if err != nil {
		t.Fatal(err)
	}
	canonical[0].Aliases[0] = "changed"
	if value.Aliases[0] != "paid" {
		t.Fatal("catalog alias storage was shared")
	}
	for name, mutate := range map[string]func(*AuthoringValue){"source": func(v *AuthoringValue) { v.Origin.Context = "other" }, "profile": func(v *AuthoringValue) { v.Origin.ProfileDigest = strings.Repeat("b", 64) }, "sensitive input": func(v *AuthoringValue) { v.Sensitivity = semantics.LiteralSensitive }, "null": func(v *AuthoringValue) { v.Nulls = "include" }, "number": func(v *AuthoringValue) { v.Kind = "number" }, "field": func(v *AuthoringValue) { v.Field.ID = "outside" }, "empty": func(v *AuthoringValue) { v.Value = "" }} {
		t.Run(name, func(t *testing.T) {
			v := value
			mutate(&v)
			if _, err := AdmitAuthoringVocabulary(model, []AuthoringValue{v}); err == nil {
				t.Fatal("invalid vocabulary admitted")
			}
		})
	}
	p := model.Pack()
	for i := range p.Datasets {
		if p.Datasets[i].ID == "orders" {
			p.Datasets[i].Columns[17].Sensitivity = semantics.LiteralSensitive
		}
	}
	sensitive, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AdmitAuthoringVocabulary(sensitive, []AuthoringValue{value}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("caller overrode sensitive field", err)
	}
	unknownPack := model.Pack()
	for i := range unknownPack.Datasets {
		if unknownPack.Datasets[i].ID == "orders" {
			unknownPack.Datasets[i].Columns[17].Sensitivity = ""
		}
	}
	unknown, err := semantics.Compile(unknownPack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AdmitAuthoringVocabulary(unknown, []AuthoringValue{value}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("value assertion silently classified unknown field", err)
	}
	second := value
	second.ID = "another"
	second.Value = "other"
	if _, err = AdmitAuthoringVocabulary(model, []AuthoringValue{value, second}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("ambiguous alias admitted", err)
	}
}

func TestVocabularyProducesOnlyScopedPendingFiltersAndValues(t *testing.T) {
	model, profiles, value := vocabularyFixture(t)
	catalog, err := AdmitAuthoringVocabulary(model, []AuthoringValue{value})
	if err != nil {
		t.Fatal(err)
	}
	amount := semantics.Reference{Kind: semantics.KindColumn, Dataset: "orders", ID: "column_00"}
	wire := enhancementWire{Results: []semantics.Enhancement{{Dataset: amount.Dataset, Column: amount.ID, Kind: semantics.EnhancementMeasure, Name: "Paid amount", Description: "Amount for approved paid-state mapping", Aggregation: semantics.AggregationSum, Unit: "USD"}, {Dataset: value.Field.Dataset, Column: value.Field.ID, Kind: semantics.EnhancementDimension, Name: "Status", Description: "Approved status vocabulary", Role: semantics.DimensionCategorical}}, FilterProposals: []VocabularyFilterProposal{{Measure: semantics.GeneratedEntityID(semantics.EnhancementMeasure, amount.Dataset, amount.ID), ID: "paid_only", Operator: "eq", Nulls: "exclude", VocabularyIDs: []string{value.ID}}}, ValueProposals: []VocabularyValueProposal{{Dataset: value.Field.Dataset, Column: value.Field.ID, VocabularyIDs: []string{value.ID}}}}
	if err := resolveVocabularyProposals(model.Pack(), &wire, catalog); err != nil {
		t.Fatal(err)
	}
	if wire.Results[0].Filters[0].Values[0] != "P" || wire.Results[0].Filters[0].Field != value.Field || wire.Results[1].Values[0].Provenance.Kind != "author_input" || wire.Results[1].Values[0].Provenance.Evidence != readexec.Hash(value) {
		t.Fatal("unsealed literal proposal")
	}
	changed, err := semantics.ApplyRichEnhancements(model, "v2", wire.Results, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	material, err := buildAuthoringContext(changed, profiles, catalog)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(material)
	if !strings.Contains(string(raw), `"values":["P"]`) || len(material.Redactions) != 0 {
		t.Fatal("authorized literal meaning missing from whole review")
	}
	redacted, err := buildAuthoringContext(changed, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if len(redacted.Redactions) != 2 {
		t.Fatal("omitted catalog failed to withhold literal meaning")
	}
	wire.FilterProposals[0].VocabularyIDs = []string{"invented"}
	if err := resolveVocabularyProposals(model.Pack(), &wire, catalog); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("model invented literal ID", err)
	}
	wire.FilterProposals[0].VocabularyIDs = []string{value.ID}
	wire.FilterProposals[0].Nulls = "include"
	if err := resolveVocabularyProposals(model.Pack(), &wire, catalog); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("null semantics widened", err)
	}
	wire.FilterProposals[0].Nulls = "exclude"
	if err := resolveVocabularyProposals(model.Pack(), &wire, nil); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("no-catalog population admitted", err)
	}
}

func TestVocabularyRebindingCannotInheritOtherDatasetDisclosure(t *testing.T) {
	model, _, value := vocabularyFixture(t)
	pack := model.Pack()
	for i := range pack.Datasets {
		if pack.Datasets[i].ID == value.Field.Dataset {
			old := pack.Datasets[i].Columns[17]
			prior := pack.Datasets[i].Source
			prior.Dataset = "different-public-dataset"
			pack.Datasets[i].Columns[17] = rebindColumn(old, readexec.Column{Name: old.SourceName, NativeType: old.NativeType, Category: old.Category, Nullable: old.Nullable, Safe: true}, prior, pack.Datasets[i].Source)
		}
	}
	rebound, err := semantics.Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AdmitAuthoringVocabulary(rebound, []AuthoringValue{value}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("same-name private destination inherited disclosure authority", err)
	}
}
