package drafts

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/sources"
)

func TestAuthoringCompleteNonNullCatalogKeysReachReview(t *testing.T) {
	model, profiles := authoringFixture(t)
	dataset := model.Pack().Datasets[1]
	profile := profiles[dataset.ID].Profile
	dataset.Columns[1].ID = "public_order_id"
	catalog := sources.Discovery{SourceID: dataset.Source.Source, ContextID: dataset.Source.Context, Revision: dataset.Source.SourceRevision,
		Relations: []readexec.Relation{{ID: dataset.ID, Schema: "analytics", Name: "orders", Columns: profile.Schema,
			UniqueKeys: [][]string{{"column_01", "column_02"}}}}}
	relation, err := matchAuthoringRelation(dataset, profile, catalog)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(relation)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Keys   [][]string `json:"non_null_unique_keys"`
		Policy string     `json:"key_evidence"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire.Keys, [][]string{{"column_02", "public_order_id"}}) || wire.Policy != "current_authorized_catalog" {
		t.Fatalf("complete source-backed key was dropped or lost its candidate column identities: %s", raw)
	}
}

func TestAuthoringCatalogKeysNeverBecomePartialOrNullableGuarantees(t *testing.T) {
	for _, kind := range []string{"no_key", "hidden_component", "nullable_component", "unsafe_component"} {
		t.Run(kind, func(t *testing.T) {
			model, profiles := authoringFixture(t)
			dataset := model.Pack().Datasets[1]
			profile := profiles[dataset.ID].Profile
			profile.Schema = append([]readexec.Column(nil), profile.Schema...)
			relation := readexec.Relation{ID: dataset.ID, Schema: "analytics", Name: "orders", Columns: profile.Schema, UniqueKeys: [][]string{{"column_01", "column_02"}}}
			switch kind {
			case "no_key":
				relation.UniqueKeys = nil // Perfect sampled distinct counts are not a key.
			case "hidden_component":
				dataset.Columns = append(dataset.Columns[:2:2], dataset.Columns[3:]...)
			case "nullable_component":
				relation.Columns[2].Nullable = true
			case "unsafe_component":
				relation.Columns[2].Safe = false
			}
			catalog := sources.Discovery{SourceID: dataset.Source.Source, ContextID: dataset.Source.Context, Revision: dataset.Source.SourceRevision, Relations: []readexec.Relation{relation}}
			out, err := matchAuthoringRelation(dataset, profile, catalog)
			if err != nil || len(out.NonNullUniqueKeys) != 0 || out.KeyEvidence != "" {
				t.Fatalf("unproved whole-row key: keys=%v err=%v", out.NonNullUniqueKeys, err)
			}
		})
	}
}

func TestAuthoringCatalogKeysAreClosedBoundedAndDetached(t *testing.T) {
	fixture := func() (semantics.Dataset, engineering.Profile, sources.Discovery) {
		model, profiles := authoringFixture(t)
		dataset := model.Pack().Datasets[1]
		profile := profiles[dataset.ID].Profile
		return dataset, profile, sources.Discovery{SourceID: dataset.Source.Source, ContextID: dataset.Source.Context, Revision: dataset.Source.SourceRevision, Relations: []readexec.Relation{{ID: dataset.ID, Schema: "analytics", Name: "orders", Columns: profile.Schema, UniqueKeys: [][]string{{"column_01", "column_02"}}}}}
	}
	for name, mutate := range map[string]func(*semantics.Dataset, *sources.Discovery){
		"source":             func(_ *semantics.Dataset, c *sources.Discovery) { c.SourceID = "foreign" },
		"context":            func(_ *semantics.Dataset, c *sources.Discovery) { c.ContextID = "foreign" },
		"revision":           func(_ *semantics.Dataset, c *sources.Discovery) { c.Revision++ },
		"duplicate_relation": func(_ *semantics.Dataset, c *sources.Discovery) { c.Relations = append(c.Relations, c.Relations[0]) },
		"unknown_column": func(_ *semantics.Dataset, c *sources.Discovery) {
			c.Relations[0].UniqueKeys = [][]string{{"FORBIDDEN_KEY_CANARY"}}
		},
		"duplicate_component": func(_ *semantics.Dataset, c *sources.Discovery) {
			c.Relations[0].UniqueKeys = [][]string{{"column_01", "column_01"}}
		},
		"unsorted_components": func(_ *semantics.Dataset, c *sources.Discovery) {
			c.Relations[0].UniqueKeys = [][]string{{"column_02", "column_01"}}
		},
		"duplicate_key": func(_ *semantics.Dataset, c *sources.Discovery) {
			c.Relations[0].UniqueKeys = append(c.Relations[0].UniqueKeys, c.Relations[0].UniqueKeys[0])
		},
		"empty_key":     func(_ *semantics.Dataset, c *sources.Discovery) { c.Relations[0].UniqueKeys = [][]string{{}} },
		"too_many_keys": func(_ *semantics.Dataset, c *sources.Discovery) { c.Relations[0].UniqueKeys = make([][]string, 33) },
		"too_many_components": func(_ *semantics.Dataset, c *sources.Discovery) {
			c.Relations[0].UniqueKeys = [][]string{make([]string, 17)}
		},
		"ambiguous_projection": func(d *semantics.Dataset, _ *sources.Discovery) { d.Columns[2].SourceName = d.Columns[1].SourceName },
		"duplicate_identity":   func(d *semantics.Dataset, _ *sources.Discovery) { d.Columns[2].ID = d.Columns[1].ID },
	} {
		t.Run(name, func(t *testing.T) {
			d, p, c := fixture()
			mutate(&d, &c)
			_, err := matchAuthoringRelation(d, p, c)
			if !errors.Is(err, readexec.ErrBinding) && !errors.Is(err, readexec.ErrLimit) {
				t.Fatal("invalid catalog proof accepted", err)
			}
		})
	}
	d, p, c := fixture()
	out, err := matchAuthoringRelation(d, p, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Relations[0].UniqueKeys[0][0] = "changed"
	if !reflect.DeepEqual(out.NonNullUniqueKeys, [][]string{{"column_01", "column_02"}}) {
		t.Fatal("catalog key alias retained")
	}
	model, profiles := authoringFixture(t)
	packet, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	before := packet.Digest
	packet.Evidence[1].Relation = &out
	packet, err = sealAuthoringContext(packet)
	if err != nil || packet.Digest == before {
		t.Fatal("catalog evidence not digest-bound", err)
	}
	raw, _ := json.Marshal(packet)
	if strings.Contains(string(raw), "FORBIDDEN_KEY_CANARY") || strings.Contains(string(raw), "PRIVATE_READ_OPERATION_CANARY") {
		t.Fatal("unrelated source evidence exposed")
	}
}
