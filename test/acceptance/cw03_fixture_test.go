package acceptance

import (
	"testing"

	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/vindex"
)

// Real topic publication supplies the reviewed classifications. No provider
// wrapper supplies or relabels sensitivity at the narrative handoff.
func newCW03SensitivityFixture(t *testing.T, classification string) *phase17Fixture {
	t.Helper()
	f, draftsService, topicsService, model, pack := publicationFixture(t)
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	pack = phase17EnrichPack(t, f, pack)
	for dataset := range pack.Datasets {
		for column := range pack.Datasets[dataset].Columns {
			c := &pack.Datasets[dataset].Columns[column]
			if classification != "unknown" {
				c.Sensitivity = "non_sensitive"
			}
			if c.SourceName == "amount" && (classification == "sensitive" || classification == "conflicting" || classification == "cannot_declassify") {
				c.Sensitivity = "sensitive"
			}
		}
	}
	phase17PublishTopic(t, draftsService, topicsService, e, pack)
	related := cloneTopic(t, pack)
	related.Topic = "commerce-related"
	related.Name = "Related synthetic evidence"
	related.Description = "Independent reviewed same-source classifications"
	if classification == "conflicting" {
		for dataset := range related.Datasets {
			for column := range related.Datasets[dataset].Columns {
				related.Datasets[dataset].Columns[column].Sensitivity = "non_sensitive"
			}
		}
	}
	phase17PublishTopic(t, draftsService, topicsService, e, related)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	route, err := nlqroute.New(topicsService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	return &phase17Fixture{f: f, e: e, pack: pack, related: related, service: route, model: model, context: pack.Datasets[0].Source.Context}
}
