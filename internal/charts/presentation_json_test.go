package charts_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

func TestPresentationJSONStrictClosedVocabulary(t *testing.T) {
	valid := `{"version":1,"edits":[{"column":"actual","set":{"display_label":"","fraction_digits":0}}]}`
	var patch charts.PresentationPatch
	if err := json.Unmarshal([]byte(valid), &patch); err != nil || patch.Edits[0].Set.DisplayLabel == nil || *patch.Edits[0].Set.DisplayLabel != "" || patch.Edits[0].Set.FractionDigits == nil || *patch.Edits[0].Set.FractionDigits != 0 {
		t.Fatal("zero/empty JSON intent lost", err)
	}
	for _, bad := range []string{
		`null`, `[]`, `{}`, `{"version":1}`, `{"version":2,"edits":[]}`, `{"version":1,"edits":null}`, `{"version":1,"edits":[]}`, `{"version":1,"edits":[null]}`,
		`{"version":1,"version":1,"edits":[{"column":"actual","set":{"fraction_digits":3}}]}`,
		`{"version":1,"\u0076ersion":1,"edits":[{"column":"actual","set":{"fraction_digits":3}}]}`,
		`{"Version":1,"edits":[{"column":"actual","set":{"fraction_digits":3}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":3}}],"outputs":[]}`,
		`{"version":1,"edits":[{"set":{"fraction_digits":3}}]}`,
		`{"version":1,"edits":[{"column":"actual","column":"actual","set":{"fraction_digits":3}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":null}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":null}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"display_label":null}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":1.5}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":"3"}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":21}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":-1}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":3,"fraction_digits":4}}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"display_label":"a","display_label":"b"}}]}`,
		`{"version":1,"edits":[{"column":"actual","reset":null}]}`,
		`{"version":1,"edits":[{"column":"actual","reset":[]}]}`,
		`{"version":1,"edits":[{"column":"actual","reset":[null]}]}`,
		`{"version":1,"edits":[{"column":"actual","reset":["unit"]}]}`,
		`{"version":1,"edits":[{"column":"actual","reset":["display_label","display_label"]}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":3},"reset":["fraction_digits"]}]}`,
		`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":3}},{"column":"actual","reset":["display_label"]}]}`,
		valid + ` {}`,
	} {
		var got charts.PresentationPatch
		if err := json.Unmarshal([]byte(bad), &got); err == nil {
			t.Fatalf("unsafe patch accepted: %s", bad)
		}
	}
	// Semantic, execution and arbitrary renderer inputs are never part of a set.
	for _, field := range []string{"name", "id", "type", "role", "grain", "aggregation", "unit", "currency", "currency_symbol", "percent", "provenance", "columns", "format", "rows", "value", "exact", "coordinate", "expected_schema", "parameters", "completeness", "formatter", "date_pattern", "locale", "currency_display"} {
		bad := fmt.Sprintf(`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":3,%q:"injected"}}]}`, field)
		if json.Unmarshal([]byte(bad), &patch) == nil {
			t.Fatal("semantic/inert set field accepted", field)
		}
	}
	// Separate set and reset fields form a valid atomic edit.
	if err := json.Unmarshal([]byte(`{"version":1,"edits":[{"column":"actual","set":{"fraction_digits":0},"reset":["display_label"]}]}`), &patch); err != nil {
		t.Fatal(err)
	}
	invalidUTF8 := []byte(`{"version":1,"edits":[{"column":"actual","set":{"display_label":"`)
	invalidUTF8 = append(invalidUTF8, 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}}]}`)...)
	if json.Unmarshal(invalidUTF8, &patch) == nil {
		t.Fatal("invalid UTF-8 label accepted")
	}
	large := `{"version":1,"edits":[` + strings.TrimSuffix(strings.Repeat(`{"column":"actual","set":{"fraction_digits":3}},`, 257), ",") + `]}`
	if json.Unmarshal([]byte(large), &patch) == nil {
		t.Fatal("unbounded patch accepted")
	}
}

func TestPresentationStoredJSONRejectsNullDuplicateAndUnknown(t *testing.T) {
	for _, raw := range []string{
		`null`, `{}`, `{"version":1,"columns":[]}`, `{"version":1,"columns":null}`, `{"version":2,"columns":[{"column":"actual","fraction_digits":3}]}`,
		`{"version":1,"columns":[null]}`, `{"version":1,"columns":[{}]}`, `{"version":1,"columns":[{"column":"actual"}]}`,
		`{"version":1,"columns":[{"column":"actual","fraction_digits":null}]}`,
		`{"version":1,"columns":[{"column":"actual","display_label":null}]}`,
		`{"version":1,"columns":[{"column":"actual","fraction_digits":3,"fraction_digits":4}]}`,
		`{"version":1,"columns":[{"column":"actual","fraction_digits":3,"unit":"kg"}]}`,
		`{"version":1,"columns":[{"column":"actual","fraction_digits":3},{"column":"actual","fraction_digits":4}]}`,
		`{"version":1,"columns":[{"column":"actual","fraction_digits":3}],"version":1}`,
	} {
		var p charts.ColumnPresentation
		if json.Unmarshal([]byte(raw), &p) == nil {
			t.Fatal("unsafe stored extension accepted", raw)
		}
	}
	_, m := presentationTable(t)
	base := string(presentationWire(t, m))
	base = strings.TrimSuffix(base, "}")
	valid := `{"version":1,"columns":[{"column":"actual","fraction_digits":3}]}`
	for _, suffix := range []string{`,"presentation":null}`, `,"presentation":` + valid + `,"presentation":` + valid + `}`, `,"Presentation":` + valid + `}`, `,"presentation":` + valid + `,"PRESENTATION":` + valid + `}`} {
		var got charts.Mapping
		if json.Unmarshal([]byte(base+suffix), &got) == nil {
			t.Fatal("null/duplicate/miscased extension accepted", suffix)
		}
	}
	// No-overlay historical JSON still has no new required member.
	var old charts.Mapping
	if err := json.Unmarshal([]byte(base+"}"), &old); err != nil || old.Presentation != nil {
		t.Fatal("legacy mapping decode changed", err)
	}
}

func FuzzPresentationPatchDecode(f *testing.F) {
	for _, raw := range []string{`null`, `{}`, `{"version":1,"edits":[{"column":"amount","set":{"fraction_digits":0}}]}`, `{"version":1,"edits":[{"column":"amount","reset":["display_label"]}]}`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var patch charts.PresentationPatch
		if json.Unmarshal([]byte(raw), &patch) != nil {
			return
		}
		wire, err := json.Marshal(patch)
		if err != nil {
			t.Fatal(err)
		}
		var again charts.PresentationPatch
		if err = json.Unmarshal(wire, &again); err != nil {
			t.Fatal("admitted patch is not roundtrip-stable", err)
		}
	})
}
