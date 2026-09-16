package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func intentFixture(t *testing.T) Definition {
	t.Helper()
	d := contractDefinition()
	other := clone(d.Outputs[0])
	other.ID = "second"
	d.Outputs = append(d.Outputs, other)
	d, err := MigrateDefinition(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Outputs[0].Intent.DisplayOrder = 10
	d.Outputs[1].Intent.DisplayOrder = 2
	d.Outputs[1].Intent.Metadata = []OutputMetadata{{Locale: "es-AR", DisplayName: "Detalle", Description: "Evidencia sintética"}, {Locale: "en", DisplayName: "Details", Description: "Synthetic evidence"}}
	return d
}

func TestCW03VersionedSelectionAndDetachedRoundTrips(t *testing.T) {
	d := intentFixture(t)
	if err := validateDefinition(context.Background(), d, config.DefaultReporting(), false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		requested []string
		want      []string
		code      string
	}{
		{"defaults", nil, []string{"second", "table"}, ""},
		{"explicit_order", []string{"table", "second"}, []string{"table", "second"}, ""},
		{"empty", []string{}, nil, "output_selection_empty"},
		{"duplicate", []string{"table", "table"}, nil, "output_duplicate"},
		{"unknown", []string{"missing"}, nil, "output_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outputs, selection, err := ResolveOutputSelection(d, tc.requested)
			if SelectionErrorCode(err) != tc.code {
				t.Fatal(selection, err)
			}
			if tc.code != "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatal(err)
				}
				return
			}
			if err != nil || !slices.Equal(selection.Selected, tc.want) || len(outputs) != len(tc.want) || selection.Choices[0].ID != "second" {
				t.Fatal(selection, err)
			}
			before := DefinitionDigest(d)
			outputs[0].Intent.Metadata[0].DisplayName = "altered projection"
			selection.Choices[0].Intent.Metadata[0].DisplayName = "altered choice"
			if before != DefinitionDigest(d) {
				t.Fatal("projection aliases immutable definition")
			}
		})
	}
	for _, requested := range [][]string{nil, {}} {
		request := RunRequest{Outputs: requested}
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		var decoded RunRequest
		if json.Unmarshal(raw, &decoded) != nil {
			t.Fatal("round-trip failed")
		}
		if (requested == nil) != (decoded.Outputs == nil) {
			t.Fatal("empty collapsed to omitted", string(raw))
		}
	}
	if got := LocalizedOutput(*d.Outputs[1].Intent, "es-MX"); got.DisplayName != "Detalle" {
		t.Fatal(got)
	}
	if got := LocalizedOutput(*d.Outputs[1].Intent, "en-US"); got.DisplayName != "Details" {
		t.Fatal(got)
	}
	d.Outputs[1].Intent.Enabled = false
	outputs, selection, err := ResolveOutputSelection(d, nil)
	if err != nil || len(outputs) != 1 || outputs[0].ID != "table" || selection.Choices[0].State != "disabled" {
		t.Fatal(selection, err)
	}
	if _, _, err := ResolveOutputSelection(d, []string{"second"}); SelectionErrorCode(err) != "output_disabled" {
		t.Fatal(err)
	}
	d.Outputs[0].Intent.DefaultSelected = false
	_, selection, err = ResolveOutputSelection(d, nil)
	if SelectionErrorCode(err) != "output_selection_empty" || len(selection.Choices) != 2 || selection.Choices[1].Code != "not_default" {
		t.Fatal("no-default selector lost typed metadata", selection, err)
	}
	for _, mutation := range []func(*Definition){
		func(d *Definition) { d.Outputs[0].Intent = nil },
		func(d *Definition) { d.Outputs[0].Intent.DisplayOrder = d.Outputs[1].Intent.DisplayOrder },
		func(d *Definition) { d.Outputs[0].Intent.Metadata[0].DisplayName = "" },
		func(d *Definition) {
			d.Outputs[0].Intent.Metadata = append(d.Outputs[0].Intent.Metadata, d.Outputs[0].Intent.Metadata[0])
		},
		func(d *Definition) { d.QueryLimits = &QueryLimits{MaxRows: -1} },
	} {
		bad := clone(d)
		mutation(&bad)
		if validateDefinition(context.Background(), bad, config.DefaultReporting(), false) == nil {
			t.Fatal("invalid v2 intent accepted")
		}
	}
}

func TestCW03LegacyMigrationDoesNotRewriteHistoricalIdentity(t *testing.T) {
	legacy := contractDefinition()
	before := DefinitionDigest(legacy)
	execution := ExecutionDigest(legacy)
	for _, requested := range [][]string{nil, {}} {
		_, selected, err := ResolveOutputSelection(legacy, requested)
		if err != nil || selected.Mode != "legacy_all" || !slices.Equal(selected.Selected, []string{"table"}) {
			t.Fatal(selected, err)
		}
	}
	migrated, err := MigrateDefinition(legacy)
	if err != nil || migrated.SchemaVersion != 2 || migrated.Outputs[0].Intent == nil || !migrated.Outputs[0].Intent.Enabled || !migrated.Outputs[0].Intent.DefaultSelected {
		t.Fatal(migrated, err)
	}
	if legacy.Outputs[0].Intent != nil || DefinitionDigest(legacy) != before || ExecutionDigest(legacy) != execution || before == DefinitionDigest(migrated) {
		t.Fatal("migration changed historical bytes or reused identity")
	}
	n := contractNarrative()
	n.SchemaVersion = "grounded-narrative-v1"
	legacy.Outputs = append(legacy.Outputs, Output{ID: "narrative", Kind: "narrative", Narrative: &n})
	migrated, err = MigrateDefinition(legacy)
	if err != nil || migrated.Outputs[1].Narrative.Instructions != "evidence_only" || migrated.Outputs[1].Narrative.MaxClaims != 32 {
		t.Fatal(migrated, err)
	}
	for _, change := range []func(*Narrative){func(n *Narrative) { n.Instructions = "Write an arbitrary essay" }, func(n *Narrative) { n.SchemaVersion = "unsupported" }, func(n *Narrative) { n.Locale = "fr" }} {
		bad := clone(legacy)
		change(bad.Outputs[1].Narrative)
		if _, err := MigrateDefinition(bad); !errors.Is(err, ErrNarrativePolicy) {
			t.Fatal("unsupported mapping dropped", err)
		}
	}
}

func TestCW03SensitivityInheritanceAndPreInputRedaction(t *testing.T) {
	d := contractDefinition()
	deps := []Dependency{{Source: d.Source, Context: d.Context, Dataset: "sales", Columns: []exec.Column{{Name: "n"}}}}
	definitions := []topics.Definition{{Topic: "sales", Version: "v1", Datasets: []topics.Dataset{{ID: "sales", Source: topics.Binding{Source: d.Source, Context: d.Context, Dataset: "sales"}, Columns: []semantics.Column{{ID: "n", SourceName: "n", Sensitivity: semantics.LiteralNonSensitive}}}}}}
	for _, tc := range []struct {
		name   string
		mutate func(*Definition, []topics.Definition)
		want   string
	}{
		{"reviewed", func(*Definition, []topics.Definition) {}, "allowed"},
		{"unknown", func(_ *Definition, defs []topics.Definition) { defs[0].Datasets[0].Columns[0].Sensitivity = "" }, "unknown"},
		{"sensitive", func(_ *Definition, defs []topics.Definition) {
			defs[0].Datasets[0].Columns[0].Sensitivity = semantics.LiteralSensitive
		}, "sensitive"},
		{"conflicting", func(_ *Definition, defs []topics.Definition) {
			column := defs[0].Datasets[0].Columns[0]
			column.Sensitivity = semantics.LiteralSensitive
			defs[0].Datasets[0].Columns = append(defs[0].Datasets[0].Columns, column)
		}, "conflicting"},
		{"manual", func(d *Definition, _ []topics.Definition) {
			d.ResultPolicy = []ResultFieldPolicy{{Field: "n", Redacted: true}}
		}, "redacted"},
		{"cannot_declassify", func(d *Definition, defs []topics.Definition) {
			d.ResultPolicy = []ResultFieldPolicy{{Field: "n", Sensitivity: semantics.LiteralNonSensitive}}
			defs[0].Datasets[0].Columns[0].Sensitivity = semantics.LiteralSensitive
		}, "conflicting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition, defs := clone(d), clone(definitions)
			tc.mutate(&definition, defs)
			policy := ResolveResultPolicy(definition, deps, defs)
			if len(policy) != 1 || policy[0].Status != tc.want || !hashValid(policy[0].ProvenanceDigest) {
				t.Fatal(policy)
			}
			n := contractNarrative()
			n.SchemaVersion = "grounded-narrative-v1"
			m := RunManifest{Selection: &OutputSelection{Version: 2}, Revision: Revision{Definition: definition}, ResultPolicy: policy}
			result := exec.Result{Schema: d.ExpectedSchema, Rows: [][]json.RawMessage{{json.RawMessage(`"731"`)}}}
			input, err := prepareNarrative(m, result, n)
			if tc.want == "allowed" {
				if err != nil || !strings.Contains(input.input, "731") {
					t.Fatal(input, err)
				}
			} else if !errors.Is(err, ErrIncomplete) || input.input != "" {
				t.Fatal("excluded value reached constructed model input", input, err)
			}
		})
	}
	// Manual redaction is independently applied even to a reviewed-safe field.
	n := contractNarrative()
	n.SchemaVersion = "grounded-narrative-v1"
	n.Fields = []string{"n", "secret"}
	n.RedactedFields = []string{"secret"}
	m := RunManifest{Selection: &OutputSelection{Version: 2}, ResultPolicy: []EffectiveFieldPolicy{{Field: "n", Status: "allowed"}, {Field: "secret", Status: "allowed"}}}
	result := exec.Result{Schema: []exec.Field{{Name: "n", Type: "integer"}, {Name: "secret", Type: "string"}}, Rows: [][]json.RawMessage{{json.RawMessage(`"731"`), json.RawMessage(`"SYNTHETIC_PRIVATE_VALUE"`)}}}
	input, err := prepareNarrative(m, result, n)
	if err != nil || strings.Contains(input.input, "SYNTHETIC_PRIVATE_VALUE") || !strings.Contains(input.input, "731") {
		t.Fatal(input, err)
	}
}

func TestCW03NarrativeHardBoundsAndQueryCeilings(t *testing.T) {
	n := contractNarrative()
	n.SchemaVersion = "grounded-narrative-v1"
	n.MaxClaims = 1
	evidence := []NarrativeEvidence{{ID: "e1", Field: "n", Type: "integer", Value: "3"}, {ID: "e2", Field: "n", Type: "integer", Value: "2"}}
	answer := NarrativeAnswer{Claims: []NarrativeClaim{{Kind: "value", Evidence: []string{"e1"}}, {Kind: "value", Evidence: []string{"e2"}}}}
	if _, err := groundedText(answer, evidence, n); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("claim ceiling bypassed", err)
	}
	answer.Claims = answer.Claims[:1]
	n.MaxCharacters = 1
	if _, err := groundedText(answer, evidence, n); !errors.Is(err, ErrBudget) {
		t.Fatal("character ceiling bypassed", err)
	}
	accepted := config.DefaultReportingExecution()
	q, err := resolveQueryLimits(accepted, 3, &QueryLimits{MaxRows: 2, MaxBytes: 4096, TimeoutMillis: 1000, QueryAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	m := RunManifest{Limits: limitsForQuery(accepted, q), QueryLimits: &q}
	current := accepted
	current.MaxRows = 1
	current.PageRows = 1
	current.Timeout = config.Duration(time.Second)
	current.NarrativeTimeout = current.Timeout
	got, err := RuntimeQueryLimits(m, current)
	if err != nil || got.MaxRows != 1 || got.MaxBytes != 4096 || got.TimeoutMillis != 1000 || got.QueryAttempts != 1 {
		t.Fatal(got, err)
	}
	larger, err := RuntimeQueryLimits(m, accepted)
	if err != nil || !reflect.DeepEqual(larger, q) {
		t.Fatal("stored lower ceiling raised", larger, err)
	}
	result := exec.Result{Rows: [][]json.RawMessage{{json.RawMessage(`"1"`)}, {json.RawMessage(`"2"`)}}}
	if !errors.Is(CheckRuntimeResult(m, current, result), ErrBudget) {
		t.Fatal("older retained result bypassed lower runtime rows")
	}
	for _, bad := range []QueryLimits{{MaxRows: -1}, {MaxRows: 10001}, {MaxBytes: 100}, {MaxBytes: 5 << 20}, {TimeoutMillis: 999}, {TimeoutMillis: 60001}, {QueryAttempts: 4}} {
		if _, err := resolveQueryLimits(accepted, 3, &bad); err == nil {
			t.Fatal("unbounded query policy admitted", bad)
		}
	}
}
