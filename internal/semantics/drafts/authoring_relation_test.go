package drafts

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/sources"
)

func TestAuthoringRelationUsesOnlyExactAuthorizedDiscovery(t *testing.T) {
	model, profiles := authoringFixture(t)
	dataset := model.Pack().Datasets[1]
	profile := profiles[dataset.ID].Profile
	catalog := sources.Discovery{SourceID: dataset.Source.Source, ContextID: dataset.Source.Context, Revision: dataset.Source.SourceRevision, Relations: []readexec.Relation{{ID: "unselected", Schema: "private", Name: "FORBIDDEN_RELATION_CANARY"}, {ID: dataset.ID, Schema: "analytics", Name: "orders", Columns: profile.Schema}}}
	relation, err := matchAuthoringRelation(dataset, profile, catalog)
	if err != nil || relation.Schema != "analytics" || relation.Name != "orders" {
		t.Fatal("physical relation identity lost", relation, err)
	}
	packet, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	before := packet.Digest
	packet.Evidence[1].Relation = &relation
	packet, err = sealAuthoringContext(packet)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(packet)
	if !strings.Contains(string(raw), `"relation":{"schema":"analytics","name":"orders"}`) || strings.Contains(string(raw), "FORBIDDEN_RELATION_CANARY") || packet.Digest == before {
		t.Fatal("relation not scoped and digest bound")
	}
	for _, mutate := range []func(*sources.Discovery){func(c *sources.Discovery) { c.ContextID = "different" }, func(c *sources.Discovery) { c.Revision++ }, func(c *sources.Discovery) { c.Relations[1].ID = "different" }, func(c *sources.Discovery) { c.Relations[1].Columns = nil }, func(c *sources.Discovery) { c.Relations[1].Name = "" }} {
		copy := catalog
		copy.Relations = append([]readexec.Relation(nil), catalog.Relations...)
		mutate(&copy)
		if _, err := matchAuthoringRelation(dataset, profile, copy); !errors.Is(err, readexec.ErrBinding) {
			t.Fatal("stale or mismatched relation admitted", err)
		}
	}
}
