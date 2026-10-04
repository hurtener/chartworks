package reporting

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestAuthoringCompilerTypedFilters(t *testing.T) {
	in, p, d, b := preparationCompileFixture()
	d.Columns = append(d.Columns, semantics.Column{ID: "day", SourceName: "day", Name: "Day", NativeType: "date", Category: "temporal"})
	p.Definition.Datasets[0] = d
	p.Definition.Dimensions = append(p.Definition.Dimensions, semantics.Dimension{ID: "day", Name: "Day", Role: semantics.DimensionTemporal, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: "day"}})
	b.Relations[0].Columns = append(b.Relations[0].Columns, exec.Column{Name: "day", NativeType: "date", Category: "temporal", Safe: true})
	in.Filters = []AuthoringDatasetFilter{{Dimension: "region", Kind: "multi_select", Default: Value{Items: []string{"West", "East"}}}, {Dimension: "day", Kind: "date_range", Default: Value{DateRange: &DateRange{Start: "2026-01-01", EndExclusive: "2026-02-01"}}}}
	out, err := compileAuthoringDataset(in, p, d, b)
	if err != nil || len(out.Parameters) != 2 || out.Parameters[0].Type != "date_range" || out.Parameters[1].Type != "dimension_set" || !strings.Contains(out.SQL, `"day" >= $1 AND "day" < $2 AND "region" IN ($3,`) || strings.Contains(out.SQL, "West") || strings.Contains(out.SQL, "LIMIT") {
		t.Fatal(out, err)
	}
	if !reflect.DeepEqual(out.Parameters[1].Default.Items, []string{"East", "West"}) || !reflect.DeepEqual(in.Filters[0].Default.Items, []string{"West", "East"}) {
		t.Fatal("canonicalization mutated request")
	}
	if err := validateBoundedFilterSQL(t.Context(), out.SQL, out.Parameters); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(){
		func() { in.Filters[0].Kind = "not_in" },
		func() { in.Filters[0].Default.Items = []string{"West", "West"} },
		func() { p.Definition.Dimensions[1].Temporal = &semantics.TemporalPolicy{Calendar: "gregorian"} },
	} {
		previousIn, previousP := clone(in), clone(p)
		change()
		if _, err := compileAuthoringDataset(in, p, d, b); err == nil {
			t.Fatal("unsupported filter accepted")
		}
		in, p = previousIn, previousP
	}
	in.Filters = in.Filters[:1]
	in.Filters[0].Kind = "select"
	in.Filters[0].Default = Value{Literal: "x' OR true --"}
	out, err = compileAuthoringDataset(in, p, d, b)
	if err != nil || out.Parameters[0].Type != "dimension_value" || strings.Contains(out.SQL, "OR true") {
		t.Fatal(out, err)
	}
}

func TestAuthoringPreparationFilteredDefinition(t *testing.T) {
	for _, filter := range []AuthoringDatasetFilter{{Dimension: "region", Kind: "select", Default: Value{Literal: "West"}}, {Dimension: "region", Kind: "multi_select", Default: Value{Items: []string{"West"}}}} {
		t.Run(filter.Kind, func(t *testing.T) {
			s, repo, boundary, e, in := preparationServiceFixture(t)
			in.Intent.Filters = []AuthoringDatasetFilter{filter}
			prepared, err := s.PrepareDatasetChart(t.Context(), e, in)
			if err != nil || prepared.Status != "prepared" {
				t.Fatal(prepared, err)
			}
			record, err := repo.ReadAuthoringPreparation(t.Context(), e, prepared.Preparation)
			if err != nil || record.Compiler != AuthoringFilteredCompilerVersion || record.Revision.Definition.SchemaVersion != CurrentSchemaVersion || len(record.Revision.Definition.Parameters) != 1 || record.Revision.Definition.Outputs[0].Intent == nil {
				t.Fatal("filtered custody", err)
			}
			replay, err := s.PrepareDatasetChart(t.Context(), e, in)
			if err != nil || replay.Digest != prepared.Digest || boundary.physical.Load() != 1 {
				t.Fatal("filtered replay", err)
			}
			created, err := s.CreatePreparedChart(t.Context(), e, AuthoringCreatePreparedRequest{NewBlock: in.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
			if err != nil || len(created.Block.Parameters) != 1 || created.Block.SchemaVersion != CurrentSchemaVersion || created.Block.Evidence != nil {
				t.Fatal("filtered create", err)
			}
		})
	}
	in, _, _, _ := preparationCompileFixture()
	if AuthoringCompilerForIntent(in) != AuthoringCompilerVersion {
		t.Fatal("legacy compiler changed")
	}
	in.Filters = []AuthoringDatasetFilter{}
	if AuthoringCompilerForIntent(in) != AuthoringCompilerVersion {
		t.Fatal("empty filters changed legacy compiler")
	}
}
