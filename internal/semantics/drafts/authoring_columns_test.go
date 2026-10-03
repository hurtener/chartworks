package drafts

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

func TestAuthoringColumnTableLosslessRoundTrip(t *testing.T) {
	columns := authoringColumns{
		{Column: "amount_日本語\"", Observed: 9007199254740993, Nulls: 4, Distinct: 6, Exact: false, Families: map[string]int{"decimal": 7, "null": 4}, Disclosure: "aggregate_counts_and_type_families"},
		{Column: "sensitive", Observed: 9, Nulls: 1, Distinct: 8, Exact: true, Disclosure: "aggregate_counts_only"},
		{Column: "empty", Families: map[string]int{}, Disclosure: "aggregate_counts_and_type_families"},
	}
	raw, err := json.Marshal(columns)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Encoding string              `json:"encoding"`
		Fields   []string            `json:"fields"`
		Rows     [][]json.RawMessage `json:"rows"`
	}
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	// Independent wire oracle: these are the full original object field names,
	// not values read back from the encoder's helper or positional decoder.
	fields := []string{"column", "observed", "nulls", "sample_distinct", "distinct_exact", "families", "disclosure"}
	if wire.Encoding != "profile-column-table-v1" || !reflect.DeepEqual(wire.Fields, fields) || len(wire.Rows) != len(columns) {
		t.Fatal("incomplete or ambiguous table header")
	}
	kind := reflect.TypeFor[authoringAggregate]()
	if kind.NumField() != len(fields) {
		t.Fatal("new aggregate field missing from lossless transport")
	}
	for i, name := range fields {
		if strings.Split(kind.Field(i).Tag.Get("json"), ",")[0] != name {
			t.Fatal("aggregate field meaning shifted")
		}
	}
	for i, row := range wire.Rows {
		if len(row) != len(fields) {
			t.Fatal("ragged aggregate table")
		}
		object := map[string]json.RawMessage{}
		for j, name := range fields {
			object[name] = row[j]
		}
		value, _ := json.Marshal(object)
		var recovered authoringAggregate
		if json.Unmarshal(value, &recovered) != nil || !reflect.DeepEqual(recovered, columns[i]) {
			t.Fatal("independent reconstruction changed an aggregate, disclosure or family", i)
		}
	}
	var recovered authoringColumns
	if json.Unmarshal(raw, &recovered) != nil || !reflect.DeepEqual(columns, recovered) {
		t.Fatal("typed roundtrip changed facts")
	}
	rawAgain, _ := json.Marshal(recovered)
	if string(raw) != string(rawAgain) {
		t.Fatal("roundtrip changed digest material")
	}
	recovered[0].Families["decimal"]++
	if columns[0].Families["decimal"] != 7 {
		t.Fatal("roundtrip retained a mutable alias")
	}
}

func TestAuthoringColumnTableRejectsAmbiguousInterpretation(t *testing.T) {
	valid := `{"encoding":"profile-column-table-v1","fields":["column","observed","nulls","sample_distinct","distinct_exact","families","disclosure"],"rows":[["id",3,0,3,true,null,"aggregate_counts_only"]]}`
	for name, raw := range map[string]string{
		"missing_version":    strings.Replace(valid, `"encoding":"profile-column-table-v1",`, "", 1),
		"unknown_version":    strings.Replace(valid, "table-v1", "table-v2", 1),
		"duplicate_version":  strings.Replace(valid, `"encoding":`, `"encoding":"profile-column-table-v1","encoding":`, 1),
		"shifted_header":     strings.Replace(valid, `"observed","nulls"`, `"nulls","observed"`, 1),
		"missing_field":      strings.Replace(valid, `,"disclosure"`, "", 1),
		"extra_field":        strings.Replace(valid, `"disclosure"]`, `"disclosure","new_field"]`, 1),
		"short_row":          strings.Replace(valid, `,3,true`, `,true`, 1),
		"long_row":           strings.Replace(valid, `true,null`, `true,null,null`, 1),
		"null_scalar":        strings.Replace(valid, `["id",3`, `[null,3`, 1),
		"fractional_count":   strings.Replace(valid, `["id",3`, `["id",3.5`, 1),
		"wrong_family_type":  strings.Replace(valid, `true,null`, `true,[]`, 1),
		"null_family_count":  strings.Replace(valid, `true,null`, `true,{"decimal":null}`, 1),
		"missing_rows":       strings.Replace(valid, `,"rows":[["id",3,0,3,true,null,"aggregate_counts_only"]]`, "", 1),
		"unknown_property":   strings.Replace(valid, `{"encoding"`, `{"extra":true,"encoding"`, 1),
		"legacy_unversioned": `[{"column":"id","observed":3,"nulls":0,"sample_distinct":3,"distinct_exact":true,"disclosure":"aggregate_counts_only"}]`,
		"trailing_value":     valid + `{}`,
		"byte_bound":         strings.Repeat(" ", maxAuthoringContextBytes) + valid,
	} {
		t.Run(name, func(t *testing.T) {
			previous := authoringColumns{{Column: "unchanged"}}
			if err := previous.UnmarshalJSON([]byte(raw)); !errors.Is(err, gateway.ErrInput) {
				t.Fatal("ambiguous table accepted", err)
			}
			if !reflect.DeepEqual(previous, authoringColumns{{Column: "unchanged"}}) {
				t.Fatal("failed decode partially changed facts")
			}
		})
	}
	var empty authoringColumns
	if json.Unmarshal([]byte(strings.Replace(valid, `[["id",3,0,3,true,null,"aggregate_counts_only"]]`, `[]`, 1)), &empty) != nil || len(empty) != 0 {
		t.Fatal("explicit empty aggregate evidence rejected")
	}
}

func TestAuthoringColumnTablePreservesBoundProvenance(t *testing.T) {
	model, profiles := authoringFixture(t)
	material, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	for i := range material.Evidence {
		material.Evidence[i].Relation = &authoringRelation{Schema: "analytics", Name: material.Evidence[i].Origin.Dataset, NonNullUniqueKeys: [][]string{{"column_01", "column_02"}}, KeyEvidence: "current_authorized_catalog"}
	}
	material, err = sealAuthoringContext(material)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(material)
	var roundtrip authoringContext
	if json.Unmarshal(raw, &roundtrip) != nil || !reflect.DeepEqual(material, roundtrip) {
		t.Fatal("complete context roundtrip lost candidate, key, origin, disclosure or sampling evidence")
	}
	sealed, err := sealAuthoringContext(roundtrip)
	if err != nil || sealed.Digest != material.Digest {
		t.Fatal("context digest did not bind complete table representation")
	}
	for name, mutate := range map[string]func(*authoringContext){
		"source":          func(c *authoringContext) { c.Evidence[0].Origin.Source += "x" },
		"context":         func(c *authoringContext) { c.Evidence[0].Origin.Context += "x" },
		"source_revision": func(c *authoringContext) { c.Evidence[0].Origin.SourceRevision++ },
		"profile":         func(c *authoringContext) { c.Evidence[0].Origin.ProfileDigest = strings.Repeat("b", 64) },
		"policy":          func(c *authoringContext) { c.Evidence[0].PolicyDigest = strings.Repeat("c", 64) },
		"observation":     func(c *authoringContext) { c.Evidence[0].ObservedAt = c.Evidence[0].ObservedAt.Add(time.Second) },
		"sampling":        func(c *authoringContext) { c.Evidence[0].Sampling.Complete = true },
		"key":             func(c *authoringContext) { c.Evidence[0].Relation.NonNullUniqueKeys[0][0] = "column_03" },
		"key_origin":      func(c *authoringContext) { c.Evidence[0].Relation.KeyEvidence = "sample" },
		"counts":          func(c *authoringContext) { c.Evidence[0].Columns[0].Nulls++ },
		"distinct_exact":  func(c *authoringContext) { c.Evidence[0].Columns[0].Exact = false },
		"families":        func(c *authoringContext) { c.Evidence[0].Columns[0].Families["decimal"]++ },
		"disclosure":      func(c *authoringContext) { c.Evidence[0].Columns[0].Disclosure = "aggregate_counts_only" },
	} {
		t.Run(name, func(t *testing.T) {
			var changed authoringContext
			if json.Unmarshal(raw, &changed) != nil {
				t.Fatal("decode original")
			}
			mutate(&changed)
			got, err := sealAuthoringContext(changed)
			if err != nil || got.Digest == material.Digest {
				t.Fatal("changed evidence escaped binding", err)
			}
		})
	}
	original, _ := json.Marshal([]authoringAggregate(material.Evidence[0].Columns))
	compact, _ := json.Marshal(material.Evidence[0].Columns)
	if len(compact) >= len(original) || maxAuthoringContextBytes != 128<<10 {
		t.Fatal("lossless representation did not compact evidence or changed bounds")
	}
	t.Logf("profile aggregate bytes: object=%d table=%d", len(original), len(compact))
}
