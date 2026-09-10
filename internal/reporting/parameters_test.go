package reporting

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
)

func testResolution() Resolution {
	return Resolution{At: time.Date(2024, 3, 31, 12, 0, 0, 0, time.UTC), Timezone: "UTC"}
}
func literalDefault(s string) *Value { return &Value{Literal: s} }

func TestTypedParameterResolution(t *testing.T) {
	cases := []struct {
		name        string
		p           Parameter
		value, kind string
	}{
		{"date", Parameter{Name: "date", Type: "date"}, "2024-02-29", "text"},
		{"datetime", Parameter{Name: "time", Type: "datetime"}, "2024-03-01T01:00:00-03:00", "text"},
		{"number exact", Parameter{Name: "amount", Type: "number", Min: "-1", Max: "9007199254740994"}, "9007199254740993.125", "number"},
		{"integer", Parameter{Name: "count", Type: "integer"}, "9223372036854775807", "integer"},
		{"boolean", Parameter{Name: "enabled", Type: "boolean"}, "true", "boolean"},
		{"grain", Parameter{Name: "grain", Type: "grain"}, "quarter", "text"},
		{"top n", Parameter{Name: "top", Type: "top_n", Min: "1", Max: "20"}, "20", "integer"},
		{"dimension bind not SQL", Parameter{Name: "region", Type: "dimension_value", Dimension: &DimensionReference{Topic: "sales", Version: "v1", Dimension: "region"}}, "x' OR true --", "text"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ResolveParameters([]Parameter{tt.p}, []Argument{{Name: tt.p.Name, Value: Value{Literal: tt.value}}}, testResolution())
			if err != nil || len(out.Parameters) != 1 || out.Parameters[0].Kind != tt.kind || out.Parameters[0].Value != tt.value || out.Values[0].Provenance != "invocation" {
				t.Fatalf("typed bind mismatch: %#v %v", out, err)
			}
		})
	}
	p := Parameter{Name: "n", Type: "integer", Default: literalDefault("2"), Required: true}
	out, err := ResolveParameters([]Parameter{p}, nil, testResolution())
	if err != nil || out.Parameters[0].Value != "2" || out.Values[0].Provenance != "block_default" {
		t.Fatal(out, err)
	}
	out, err = ResolveParameters([]Parameter{p}, []Argument{{Name: "n", Value: Value{Literal: "3"}}}, testResolution())
	if err != nil || out.Parameters[0].Value != "3" {
		t.Fatal(out, err)
	}
	p.Default = nil
	if _, err := ResolveParameters([]Parameter{p}, nil, testResolution()); err == nil {
		t.Fatal("missing required argument accepted")
	}
	p.Required = false
	out, err = ResolveParameters([]Parameter{p}, nil, testResolution())
	if err != nil || out.Parameters[0].Kind != "null" || out.Values[0].Provenance != "omitted" {
		t.Fatal(out, err)
	}
}

func TestParameterRejection(t *testing.T) {
	bad := []Parameter{
		{Name: "x", Type: "free_sql"}, {Name: "x", Type: "top_n", Default: literalDefault("10001")},
		{Name: "x", Type: "integer", Default: literalDefault("1.5")}, {Name: "x", Type: "integer", Default: literalDefault("+1")},
		{Name: "x", Type: "number", Default: literalDefault("NaN")}, {Name: "x", Type: "number", Default: literalDefault("1e10000")},
		{Name: "x", Type: "number", Min: "2", Max: "1"}, {Name: "x", Type: "number", Min: "1", Default: literalDefault("0.999999999999999999")},
		{Name: "x", Type: "date", Default: literalDefault("2023-02-29")}, {Name: "x", Type: "date", Default: literalDefault("2024-1-01")},
		{Name: "x", Type: "datetime", Default: literalDefault("2024-01-01 12:00:00")},
		{Name: "x", Type: "boolean", Default: literalDefault("yes")}, {Name: "x", Type: "grain", Default: literalDefault("DROP TABLE")},
		{Name: "x", Type: "dimension_value"}, {Name: "x", Type: "boolean", Dimension: &DimensionReference{Topic: "a", Version: "b", Dimension: "c"}},
		{Name: "x", Type: "integer", Enum: []string{"1", "1"}}, {Name: "x", Type: "integer", Enum: []string{"1"}, Default: literalDefault("2")},
		{Name: "x", Type: "boolean", Min: "false"}, {Name: "x", Type: "relative_period", Enum: []string{"previous"}},
		{Name: "x", Type: "relative_period", Default: literalDefault("last month")},
		{Name: "x", Type: "date", Min: "2025-01-01", Max: "2024-01-01"},
	}
	for i, p := range bad {
		if validateDeclarations([]Parameter{p}, 64) == nil {
			t.Fatalf("invalid declaration %d accepted: %#v", i, p)
		}
	}
	valid := []Parameter{{Name: "x", Type: "integer"}}
	for _, args := range [][]Argument{{{Name: "unknown", Value: Value{Literal: "1"}}}, {{Name: "x", Value: Value{Literal: "1"}}, {Name: "x", Value: Value{Literal: "2"}}}, {{Name: "x", Value: Value{Period: &Period{}}}}} {
		if _, err := ResolveParameters(valid, args, testResolution()); err == nil {
			t.Fatal("ambiguous/unknown argument accepted")
		}
	}
	if validateDeclarations(append(valid, valid...), 64) == nil {
		t.Fatal("duplicate parameter accepted")
	}
	for _, zone := range []string{"", "Local", "../UTC", "/etc/passwd", "not-a-zone"} {
		r := testResolution()
		r.Timezone = zone
		if _, err := ResolveParameters(nil, nil, r); err == nil {
			t.Fatal("unsafe zone accepted", zone)
		}
	}
	r := testResolution()
	r.At = time.Time{}
	if _, err := ResolveParameters(nil, nil, r); err == nil {
		t.Fatal("implicit clock accepted in pure resolver")
	}
	if parameterDigest(nil) != parameterDigest([]exec.Parameter{}) {
		t.Fatal("zero parameters depend on nil slice")
	}
	if _, err := scalarDefaults([]exec.Parameter{{Kind: "text", Value: "unreviewed category"}}); err == nil {
		t.Fatal("capture guessed a dimension")
	}
}

func TestPeriodCalendarPolicies(t *testing.T) {
	cases := []struct {
		name, at, zone, mode, unit, start, end string
		count                                  int
	}{
		{"previous leap month", "2024-03-31T12:00:00Z", "UTC", "previous", "month", "2024-02-01T00:00:00Z", "2024-03-01T00:00:00Z", 1},
		{"rolling month clamps", "2024-03-31T12:00:00Z", "UTC", "rolling", "month", "2024-02-29T12:00:00Z", "2024-03-31T12:00:00Z", 1},
		{"quarter", "2024-05-02T12:00:00Z", "UTC", "previous", "quarter", "2024-01-01T00:00:00Z", "2024-04-01T00:00:00Z", 1},
		{"previous ISO week", "2024-04-03T12:00:00Z", "UTC", "previous", "week", "2024-03-25T00:00:00Z", "2024-04-01T00:00:00Z", 1},
		{"calendar spring day", "2024-03-11T12:00:00Z", "America/New_York", "previous", "day", "2024-03-10T05:00:00Z", "2024-03-11T04:00:00Z", 1},
		{"calendar fall day", "2024-11-04T12:00:00Z", "America/New_York", "previous", "day", "2024-11-03T04:00:00Z", "2024-11-04T05:00:00Z", 1},
		{"rolling leap year", "2024-02-29T12:00:00Z", "UTC", "rolling", "year", "2023-02-28T12:00:00Z", "2024-02-29T12:00:00Z", 1},
		{"hour is elapsed time", "2024-03-10T07:30:00Z", "America/New_York", "rolling", "hour", "2024-03-10T06:30:00Z", "2024-03-10T07:30:00Z", 1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, tt.at)
			if err != nil {
				t.Fatal(err)
			}
			p := Period{Mode: tt.mode, Unit: tt.unit, Count: tt.count, DSTPolicy: "reject", MonthPolicy: "clamp"}
			out, err := ResolveParameters([]Parameter{{Name: "window", Type: "relative_period", Required: true, Default: &Value{Period: &p}}}, nil, Resolution{At: at, Timezone: tt.zone})
			if err != nil || len(out.Parameters) != 2 || out.Parameters[0].Value != tt.start || out.Parameters[1].Value != tt.end {
				t.Fatalf("window mismatch: %#v %v", out, err)
			}
		})
	}
	zone, _ := namedZone("America/New_York")
	if _, err := civil(2024, time.March, 10, 2, 30, 0, 0, zone, "earlier"); err == nil {
		t.Fatal("DST gap silently normalized")
	}
	if _, err := civil(2024, time.November, 3, 1, 30, 0, 0, zone, "reject"); err == nil {
		t.Fatal("DST fold accepted without declared policy")
	}
	fold, err := civil(2024, time.November, 3, 1, 30, 0, 0, zone, "earlier")
	if err != nil || fold.UTC().Format(time.RFC3339) != "2024-11-03T05:30:00Z" {
		t.Fatal(fold, err)
	}
	utc, _ := namedZone("UTC")
	p := Period{Mode: "rolling", Unit: "month", Count: 1, DSTPolicy: "reject", MonthPolicy: "reject"}
	if _, err := resolvePeriod(p, testResolution(), utc); err == nil {
		t.Fatal("month-end rollover accepted under reject policy")
	}
	p = Period{Mode: "explicit", Start: "2024-03-01", End: "2024-03-01", DSTPolicy: "reject", MonthPolicy: "clamp"}
	if _, err := resolvePeriod(p, testResolution(), utc); err == nil {
		t.Fatal("empty half-open interval accepted")
	}
}

func TestScheduleAndExplicitPeriodPolicies(t *testing.T) {
	zone, _ := namedZone("UTC")
	resolution := testResolution()
	p := Period{Mode: "schedule_window", FirstOccurrence: "reject", DSTPolicy: "reject", MonthPolicy: "clamp"}
	if _, err := resolvePeriod(p, resolution, zone); err == nil {
		t.Fatal("missing first occurrence policy accepted")
	}
	resolution.ScheduleWindow = &Window{Start: resolution.At.Add(-time.Hour), End: resolution.At}
	w, err := resolvePeriod(p, resolution, zone)
	if err != nil || !w.Start.Equal(resolution.ScheduleWindow.Start) || !w.End.Equal(resolution.ScheduleWindow.End) {
		t.Fatal(w, err)
	}
	resolution.ScheduleWindow = nil
	p.FirstOccurrence = "from_date"
	p.FromDate = "2024-03-01"
	w, err = resolvePeriod(p, resolution, zone)
	if err != nil || w.Start.Format(time.DateOnly) != "2024-03-01" || !w.End.Equal(resolution.At) {
		t.Fatal(w, err)
	}
	p.FirstOccurrence = "previous"
	p.FromDate = ""
	p.Unit = "day"
	p.Count = 1
	if _, err := resolvePeriod(p, resolution, zone); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Period{{Mode: "rolling", Unit: "month", Count: 1}, {Mode: "previous", Unit: "minute", Count: 1, DSTPolicy: "reject", MonthPolicy: "clamp"}, {Mode: "explicit", Start: "x", End: "y", DSTPolicy: "reject", MonthPolicy: "clamp"}, {Mode: "schedule_window", FirstOccurrence: "guess", DSTPolicy: "reject", MonthPolicy: "clamp"}} {
		if validatePeriod(bad) == nil {
			t.Fatal("invalid period policy accepted", bad)
		}
	}
}

func TestReportingLimitsAndQuestionNormalization(t *testing.T) {
	valid := config.DefaultReporting()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*config.Reporting){func(c *config.Reporting) { c.MaxSQLBytes = 1 << 20 }, func(c *config.Reporting) { c.PreviewRows = 10001 }, func(c *config.Reporting) { c.EvidenceTTL = config.Duration(8 * 24 * time.Hour) }, func(c *config.Reporting) { c.QuestionThreshold = math.NaN() }, func(c *config.Reporting) { c.QuestionThreshold = math.Inf(1) }, func(c *config.Reporting) { c.MaxRevisions = 1 }} {
		v := valid
		change(&v)
		if v.Validate() == nil {
			t.Fatal("unbounded configuration accepted")
		}
	}
	if questionScore("  ＳＡＬＥＳ? last month! ", "sales last month") != 1 || normalizeQuestion("!!!") != "" {
		t.Fatal("normalization mismatch")
	}
	if questionScore("sales last month", "how many new customers") != 0 || questionScore("sales last month", "sales previous month") >= .8 {
		t.Fatal("lexical assessment overclaimed equivalence")
	}
	if metadataValid([]Localized{{Locale: "en-US", Title: "Sales", Question: "Sales?", Aliases: []string{"sales!"}}}, valid) {
		t.Fatal("duplicate normalized alias accepted")
	}
}

func FuzzParameterScalarDoesNotBecomeSQL(f *testing.F) {
	for _, seed := range []string{"x' OR true --", "", strings.Repeat("a", 4096), "\x00", "2024-02-29", "9e99999"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, literal string) {
		p := Parameter{Name: "dimension", Type: "dimension_value", Dimension: &DimensionReference{Topic: "t", Version: "v", Dimension: "d"}}
		out, err := scalar(p, literal)
		if err == nil && (out.Kind != "text" || out.Value != literal || len(out.Value) > 4096) {
			t.Fatal("bind value was reinterpreted")
		}
		if err == nil {
			if _, err := json.Marshal(out); err != nil {
				t.Fatal(err)
			}
		}
	})
}
