package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// Pure definition/wire-shape tests; these do not manufacture execution evidence.
func contractDefinition() Definition {
	column := charts.Column{ID: "c0", Name: "n", Type: "integer", Role: "measure", Provenance: charts.Provenance{Version: 1}}
	mapping := charts.Mapping{Version: charts.Version, Kind: charts.Table, Columns: []charts.Column{column}, Bindings: charts.Bindings{Columns: []string{"c0"}}, Options: charts.DefaultOptions()}
	return Definition{SchemaVersion: SchemaVersion, Metadata: []Localized{{Locale: "en-US", Title: "Count", Question: "How many?", Aliases: []string{"Item count"}}}, Source: "warehouse", Context: "readonly", Topics: []TopicPin{{Topic: "sales", Version: "v1", Digest: strings.Repeat("a", 64)}}, SQL: "SELECT count(*) AS n FROM analytics.sales", ExpectedSchema: []exec.Field{{Name: "n", Type: "integer", NativeType: "int8", Encoding: "string"}}, Outputs: []Output{{ID: "table", Kind: "table", Mapping: &mapping}}}
}
func contractNarrative() Narrative {
	return Narrative{Type: "summary", Instructions: "Summarize only the returned evidence", Fields: []string{"n"}, Reduction: "first_rows", MaxRows: 10, MaxBytes: 4096, MaxCharacters: 1024, MaxCalls: 1, MaxTokens: 1024, TimeoutMillis: 1000, PromptVersion: "v1", ModelVersion: "v1", SchemaVersion: "v1", Locale: "en-US", Tone: "neutral", RequireEvidence: true, RequireCaveats: true}
}

func TestDefinitionClosedBoundsAndDetachedOutputs(t *testing.T) {
	ctx := context.Background()
	limits := config.DefaultReporting()
	base := contractDefinition()
	if err := validateDefinition(ctx, base, limits, false); err != nil {
		t.Fatal("invalid baseline", err)
	}
	cases := []struct {
		name   string
		change func(*Definition)
	}{
		{"schema version", func(d *Definition) { d.SchemaVersion++ }},
		{"metadata missing", func(d *Definition) { d.Metadata = nil }},
		{"locale", func(d *Definition) { d.Metadata[0].Locale = "not_a_locale" }},
		{"title", func(d *Definition) { d.Metadata[0].Title = "  " }},
		{"description controls", func(d *Definition) { d.Metadata[0].Description = "\x01" }},
		{"alias duplicate", func(d *Definition) { d.Metadata[0].Aliases = []string{"HOW MANY?"} }},
		{"alias empty", func(d *Definition) { d.Metadata[0].Aliases = []string{"!?!"} }},
		{"duplicate locale", func(d *Definition) { d.Metadata = append(d.Metadata, d.Metadata[0]) }},
		{"source identifier", func(d *Definition) { d.Source = "../other" }},
		{"context identifier", func(d *Definition) { d.Context = "" }},
		{"topic absent", func(d *Definition) { d.Topics = nil }},
		{"topic duplicate", func(d *Definition) { d.Topics = append(d.Topics, d.Topics[0]) }},
		{"topic digest", func(d *Definition) { d.Topics[0].Digest = "pretend" }},
		{"template provenance", func(d *Definition) {
			d.Template = &TemplatePin{ID: "t", Version: "v1", Digest: strings.Repeat("b", 64)}
		}},
		{"sql empty", func(d *Definition) { d.SQL = " \t" }},
		{"sql bound", func(d *Definition) { d.SQL = strings.Repeat("x", limits.MaxSQLBytes+1) }},
		{"parameter union", func(d *Definition) { d.Parameters = []Parameter{{Name: "x", Type: "raw_sql"}} }},
		{"schema missing", func(d *Definition) { d.ExpectedSchema = nil }},
		{"schema duplicate", func(d *Definition) { d.ExpectedSchema = append(d.ExpectedSchema, d.ExpectedSchema[0]) }},
		{"schema native", func(d *Definition) { d.ExpectedSchema[0].NativeType = "" }},
		{"schema encoding", func(d *Definition) { d.ExpectedSchema[0].Encoding = "" }},
		{"schema type", func(d *Definition) { d.ExpectedSchema[0].Type = "opaque" }},
		{"output missing", func(d *Definition) { d.Outputs = nil }},
		{"output duplicate", func(d *Definition) { d.Outputs = append(d.Outputs, d.Outputs[0]) }},
		{"output id", func(d *Definition) { d.Outputs[0].ID = "a/b" }},
		{"output kind", func(d *Definition) { d.Outputs[0].Kind = "html" }},
		{"output mapping absent", func(d *Definition) { d.Outputs[0].Mapping = nil }},
		{"output wrong tagged kind", func(d *Definition) { d.Outputs[0].Kind = "kpi" }},
		{"output narrative union", func(d *Definition) { n := contractNarrative(); d.Outputs[0].Narrative = &n }},
		{"output foreign field", func(d *Definition) { d.Outputs[0].Mapping.Columns[0].Name = "secret" }},
		{"output field type", func(d *Definition) { d.Outputs[0].Mapping.Columns[0].Type = "decimal" }},
		{"output binding", func(d *Definition) { d.Outputs[0].Mapping.Bindings.Columns = []string{"missing"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := clone(base)
			tc.change(&d)
			if validateDefinition(ctx, d, limits, false) == nil {
				t.Fatal("invalid definition admitted")
			}
		})
	}
	if validateDefinition(nil, base, limits, false) == nil {
		t.Fatal("nil context accepted")
	}
	badLimits := limits
	badLimits.MaxOutputs = 0
	if validateDefinition(ctx, base, badLimits, false) == nil {
		t.Fatal("invalid limits accepted")
	}
	captured := clone(base)
	captured.Template = &TemplatePin{ID: "template", Version: "v1", Digest: strings.Repeat("a", 64)}
	if err := validateDefinition(ctx, captured, limits, true); err != nil {
		t.Fatal("verified capture template", err)
	}
	before := DefinitionDigest(base)
	metadata := clone(base)
	metadata.Metadata[0].Title = "Changed title"
	if before == DefinitionDigest(metadata) || ExecutionDigest(base) != ExecutionDigest(metadata) {
		t.Fatal("execution versus definition identity conflated")
	}
	metadata.SQL += " /* exact bytes */"
	if ExecutionDigest(base) == ExecutionDigest(metadata) {
		t.Fatal("SQL change lost")
	}
	selected, err := SelectOutputs(base.Outputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	selected[0].Mapping.Columns[0].Name = "changed"
	if base.Outputs[0].Mapping.Columns[0].Name != "n" {
		t.Fatal("output projection aliases saved definition")
	}
	for _, all := range [][]Output{nil, make([]Output, 65), {{ID: "bad/id"}}, {{ID: "x"}, {ID: "x"}}} {
		if _, err := SelectOutputs(all, nil); err == nil {
			t.Fatal("invalid saved output set accepted")
		}
	}
}

func TestNarrativeDefinitionIsBoundedEvidenceOnly(t *testing.T) {
	fields := map[string]exec.Field{"n": {Name: "n"}}
	base := contractNarrative()
	if err := validateNarrative(base, fields); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Narrative){
		func(n *Narrative) { n.Type = "agent" }, func(n *Narrative) { n.Instructions = "" }, func(n *Narrative) { n.Fields = nil },
		func(n *Narrative) { n.Fields = []string{"secret"} }, func(n *Narrative) { n.Fields = []string{"n", "n"} },
		func(n *Narrative) { n.RedactedFields = []string{"secret"} }, func(n *Narrative) { n.RedactedFields = []string{"n", "n"} },
		func(n *Narrative) { n.Reduction = "execute_code" }, func(n *Narrative) { n.MaxRows = 1001 }, func(n *Narrative) { n.MaxBytes = 65537 },
		func(n *Narrative) { n.MaxCharacters = 16385 }, func(n *Narrative) { n.MaxCalls = 5 }, func(n *Narrative) { n.MaxTokens = 32769 },
		func(n *Narrative) { n.TimeoutMillis = 60001 }, func(n *Narrative) { n.PromptVersion = "../v1" }, func(n *Narrative) { n.Tone = "arbitrary" },
		func(n *Narrative) { n.RequireEvidence = false }, func(n *Narrative) { n.RequireCaveats = false },
	} {
		n := clone(base)
		change(&n)
		if validateNarrative(n, fields) == nil {
			t.Fatal("invalid narrative admitted", n)
		}
	}
	n := base
	out := Output{ID: "narrative", Kind: "narrative", Narrative: &n}
	if err := validateOutput(context.Background(), out, fields, config.DefaultReporting()); err != nil {
		t.Fatal(err)
	}
	out.Narrative = nil
	if validateOutput(context.Background(), out, fields, config.DefaultReporting()) == nil {
		t.Fatal("empty tagged union accepted")
	}
}

func TestResultShapeAndPureContractFailures(t *testing.T) {
	ctx := context.Background()
	d := contractDefinition()
	limits := config.DefaultReporting()
	result := exec.Result{Schema: clone(d.ExpectedSchema), Rows: [][]json.RawMessage{{json.RawMessage(`"1"`)}}, Outcome: "succeeded", Bytes: 3}
	if err := checkResult(ctx, d, result, limits); err != nil {
		t.Fatal("valid normalized scalar", err)
	}
	for _, change := range []func(*exec.Result){func(r *exec.Result) { r.Schema[0].Name = "other" }, func(r *exec.Result) { r.Bytes = limits.PreviewBytes + 1 }, func(r *exec.Result) { r.Rows[0][0] = json.RawMessage(`true`) }, func(r *exec.Result) { r.Outcome = "unknown" }} {
		r := clone(result)
		change(&r)
		if !errors.Is(checkResult(ctx, d, r, limits), ErrStale) {
			t.Fatal("invalid result admitted")
		}
	}
	changed := clone(d)
	changed.Outputs[0].Mapping.Columns[0].Name = "missing"
	if !errors.Is(checkResult(ctx, changed, result, limits), ErrStale) {
		t.Fatal("missing output field")
	}
	if digest(math.NaN()) != "" || hashValid("A"+strings.Repeat("a", 63)) || text("\xff", 100) || text("\x7f", 100) || note(" ") {
		t.Fatal("invalid scalar hygiene")
	}
	for _, typ := range []string{"integer", "decimal", "number", "boolean", "binary", "text", "string", "uuid", "date", "time", "timestamp", "timestamptz", "datetime", "temporal", "json", "jsonb", "structured"} {
		if chartType(typ) == "" {
			t.Fatal(typ)
		}
	}
	if chartType("opaque") != "" {
		t.Fatal("untyped chart field accepted")
	}
	if CaptureFromQueries(nil) != nil {
		t.Fatal("nil query service advertised")
	}
	if _, err := (queryBlockCapture{}).Capture(ctx, identity.Envelope{}, "query"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := New(nil, nil, nil, nil, nil, nil, limits); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var service *Service
	if service.CanCapture() || service.CanValidate() || service.CanObserve() {
		t.Fatal("nil service capability")
	}
	if _, _, err := service.begin(ctx, identity.Envelope{}, "block", Read); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := (Prepared{}).Checked(identity.Envelope{}); err == nil {
		t.Fatal("zero authority prepared mutation accepted")
	}
	for _, a := range []Access{Read, Write, Validate, Preview, Publish, Certify, SQLRead} {
		if a.Action() == "" || a.Permission() == "" {
			t.Fatal(a)
		}
	}
	if Access("admin").Action() != "" || Access("admin").Permission() != "" {
		t.Fatal("local role admitted")
	}
	for _, kind := range []string{"create", "edit", "capture", "restore", "rename", "parameterize", "reject", "archive", "validate", "publish", "certify", "withdraw", "health", "preview"} {
		if (Mutation{Kind: kind}).Access() == "" {
			t.Fatal(kind)
		}
	}
	if (Mutation{Kind: "grant"}).Access() != "" {
		t.Fatal("identity mutation admitted")
	}
}
