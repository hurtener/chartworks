package semantics

import (
	"reflect"
	"slices"
	"testing"
)

func TestVersionDiffUsesStableIdentityAndSeparatesColumnChanges(t *testing.T) {
	before, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	p := testPack()
	p.Version = "commerce:v2"
	p.Name = "Revised commerce"
	p.Datasets[0].Columns[1].SourceName = "renamed_amount"
	p.Datasets[1].Source.ProfileVersion = "customers_profile:v2"
	p.CanonicalEntities[0].Revision++
	p.Dimensions = nil
	p.Measures = append(p.Measures, Measure{ID: "minimum_order", Name: "Minimum order", Field: Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}, Aggregation: AggregationMinimum})
	after, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := DiffModels(before, after)
	if err != nil || !diff.MetadataChanged || diff.BeforeVersion != "commerce:v1" || diff.AfterVersion != "commerce:v2" || diff.BeforeDigest != before.Digest() || diff.AfterDigest != after.Digest() {
		t.Fatalf("invalid version pins/metadata: %#v, %v", diff, err)
	}
	want := []ChangeCount{{Kind: KindCanonicalEntity, Modified: 1}, {Kind: KindColumn, Modified: 1}, {Kind: KindDataset, Modified: 1}, {Kind: KindDimension, Removed: 1}, {Kind: KindMeasure, Added: 1}}
	if !reflect.DeepEqual(diff.Counts, want) || len(diff.Changes) != 5 {
		t.Fatalf("unexpected change counts: %#v / %#v", diff.Counts, diff.Changes)
	}
	for _, change := range diff.Changes {
		if change.Kind == KindDataset && change.ID != "customers" {
			t.Fatal("column-only change was counted twice as dataset metadata")
		}
		if change.Change == ChangeAdded && change.BeforeDigest != "" || change.Change == ChangeRemoved && change.AfterDigest != "" || change.Change == ChangeModified && (change.BeforeDigest == "" || change.AfterDigest == "") {
			t.Fatalf("wrong per-entity content pins: %#v", change)
		}
	}
	slices.Reverse(p.Datasets)
	slices.Reverse(p.Measures)
	reordered, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	again, err := DiffModels(before, reordered)
	if err != nil || !reflect.DeepEqual(diff, again) {
		t.Fatalf("diff changed with collection order: %v", err)
	}
	diff.Changes[0].ID = "mutated"
	diff.Counts[0].Added = 999
	again, _ = DiffModels(before, after)
	if again.Changes[0].ID == "mutated" || again.Counts[0].Added == 999 {
		t.Fatal("diff result shares mutable state")
	}
}

func TestVersionDiffEmptyRevisionAndInvalidComparisons(t *testing.T) {
	before, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	p := testPack()
	p.Version = "commerce:v2"
	after, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := DiffModels(before, after)
	if err != nil || len(diff.Changes) != 0 || len(diff.Counts) != 0 || diff.MetadataChanged || diff.BeforeDigest == diff.AfterDigest {
		t.Fatalf("version-only change: %#v, %v", diff, err)
	}
	p.Topic = "other"
	other, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, right := range []Model{{}, other} {
		if _, err := DiffModels(before, right); validationCode(t, err) != CodeEvidenceMismatch {
			t.Fatal(err)
		}
	}
}
