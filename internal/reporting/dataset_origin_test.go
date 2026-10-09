package reporting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func TestSourceDatasetOrigin(t *testing.T) {
	_, _, _, binding := fieldSelectionFixture()
	pin := SourceDatasetPin{Source: binding.Source, Context: binding.Context, Dataset: binding.Relations[0].ID, SourceRevision: binding.Revision, SchemaDigest: exec.Hash(binding.Relations[0])}
	dataset, err := sourceDatasetFields(pin, binding)
	if err != nil || len(dataset.Columns) != len(binding.Relations[0].Columns) {
		t.Fatal("physical metadata", err)
	}
	in := AuthoringDatasetIntent{SourceDataset: &pin, Dataset: pin.Dataset, Fields: &AuthoringFieldSelection{Mode: "rows", Dimensions: []AuthoringGrouping{{Kind: "column", Field: dataset.Columns[0].ID}}}, Mapping: AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"group_1"}}, Options: charts.DefaultOptions()}}
	compiled, err := compileAuthoringDataset(in, topics.Published{}, dataset, binding)
	if err != nil || !strings.Contains(compiled.SQL, `SELECT "specimen_code" AS "group_1"`) {
		t.Fatal("physical compile", err)
	}
	for _, column := range compiled.Columns {
		if column.Provenance.Topic != "" || column.Provenance.SemanticID != "" || column.Provenance.Source != pin.Source {
			t.Fatal("invented semantic provenance")
		}
	}
	for _, change := range []func(*SourceDatasetPin){func(p *SourceDatasetPin) { p.SchemaDigest = strings.Repeat("0", 64) }, func(p *SourceDatasetPin) { p.SourceRevision++ }, func(p *SourceDatasetPin) { p.Context = "other" }, func(p *SourceDatasetPin) { p.Dataset = "other" }} {
		bad := pin
		change(&bad)
		if _, err := sourceDatasetFields(bad, binding); err == nil {
			t.Fatal("changed origin accepted")
		}
	}
	mixed := in
	mixed.Topic = TopicPin{Topic: "reviewed", Version: "v1", Digest: strings.Repeat("a", 64)}
	if _, err := compileAuthoringDataset(mixed, topics.Published{}, dataset, binding); err == nil {
		t.Fatal("mixed origin compiled")
	}
	definition := Definition{SchemaVersion: CurrentSchemaVersion, SourceDataset: &pin, Source: pin.Source, Context: pin.Context}
	if !sourceDatasetDefinitionValid(definition) || definition.ParentSource() != pin.Source || definition.ParentTopic() != "" {
		t.Fatal("source parent")
	}
	for _, change := range []func(*Definition){func(d *Definition) { d.Topics = []TopicPin{mixed.Topic} }, func(d *Definition) { d.SchemaVersion = SchemaVersion }, func(d *Definition) { d.Source = "other" }, func(d *Definition) { d.Rules = []RulePin{{}} }, func(d *Definition) { d.Template = &TemplatePin{} }} {
		bad := clone(definition)
		change(&bad)
		if sourceDatasetDefinitionValid(bad) {
			t.Fatal("conflicting source definition accepted")
		}
	}
	withoutPin := clone(definition)
	withoutPin.SourceDataset = nil
	if ExecutionDigest(withoutPin) == ExecutionDigest(definition) {
		t.Fatal("source pin missing from execution identity")
	}
	legacy, _ := json.Marshal(Definition{})
	if strings.Contains(string(legacy), "source_dataset") {
		t.Fatal("optional source pin changed legacy bytes")
	}
	unsafe := binding.Clone()
	unsafe.Relations[0].Columns[0].Safe = false
	unsafePin := pin
	unsafePin.SchemaDigest = exec.Hash(unsafe.Relations[0])
	safe, err := sourceDatasetFields(unsafePin, unsafe)
	if err != nil || len(safe.Columns) != len(dataset.Columns)-1 {
		t.Fatal("unsafe physical field advertised", err)
	}
}
