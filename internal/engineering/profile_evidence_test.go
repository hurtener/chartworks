package engineering

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func profileFixture(t *testing.T, fields []readexec.Field, rows [][]json.RawMessage) (ProfileRecord, readexec.ExecutionReport) {
	t.Helper()
	columns := make([]readexec.Column, len(fields))
	for i, field := range fields {
		columns[i] = readexec.Column{Name: field.Name, NativeType: field.NativeType, Category: field.Type, Nullable: true, Safe: true}
	}
	record := ProfileRecord{
		Tenant: "tenant", Actor: "actor", Session: "session",
		Spec:     ProfileSpec{ID: "version", Source: "source", Context: "source:v1", Dataset: "dataset", SkipLLM: true},
		Binding:  readexec.Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: "postgresql", Contract: "fixture-v1", Fingerprint: readexec.Hash("fixture"), Relations: []readexec.Relation{{ID: "dataset", Schema: "analytics", Name: "data", Columns: columns}}},
		Policy:   config.ProfilePolicy{ID: "redacted", Tenant: "tenant", Source: "source", RangeColumns: []string{}},
		Settings: config.DefaultProfiling(), State: "reserved", Created: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC),
	}
	record.Settings.SampleRows = 2000
	record.SpecHash = record.Digest()
	if !record.Valid() {
		t.Fatal("invalid synthetic aggregate fixture")
	}
	report := readexec.ExecutionReport{}
	report.Attempt.ID = "read-attempt"
	report.Attempt.Status = "succeeded"
	report.Attempt.Manifest.Operation = "read-operation"
	report.Attempt.Manifest.Limits.Rows = record.Settings.SampleRows
	report.Attempt.Manifest.Limits.Bytes = record.Settings.SampleBytes
	report.Attempt.Manifest.Limits.Timeout = time.Duration(record.Settings.Timeout)
	report.Attempt.Manifest.Limits.PlannerCost = record.Settings.PlannerCostCeiling
	report.Result = &readexec.Result{Schema: fields, Rows: rows, Outcome: "succeeded", Bytes: 1024}
	return record, report
}

func buildProfileFixture(t *testing.T, record ProfileRecord, report readexec.ExecutionReport) Profile {
	t.Helper()
	record.SpecHash = record.Digest()
	profile, err := BuildProfile(context.Background(), record, report, time.Millisecond, record.Created)
	if err != nil || !profile.Valid(record) {
		t.Fatal("aggregate evidence", err, profile)
	}
	return profile
}

func TestProfileDistinctIsBoundedEvidenceNotAnEstimate(t *testing.T) {
	rows := make([][]json.RawMessage, 1100)
	for i := range rows {
		value, _ := json.Marshal("PRIVATE_DISTINCT_CANARY_" + strconv.Itoa(i))
		rows[i] = []json.RawMessage{value}
	}
	record, report := profileFixture(t, []readexec.Field{{Name: "value", Type: "text", NativeType: "text", Encoding: "string"}}, rows)
	profile := buildProfileFixture(t, record, report)
	column := profile.Columns[0]
	if column.Distinct != 1025 || column.DistinctExact || column.Minimum != nil || column.Maximum != nil || column.Observed != 1100 {
		t.Fatal("distinct cap became an exact estimate or retained values", column)
	}
	wire, err := json.Marshal(profile)
	if err != nil || strings.Contains(string(wire), "PRIVATE_DISTINCT_CANARY") {
		t.Fatal("distinct sketch escaped into retained evidence", err)
	}
	if profile.Sampling.Strategy != "validated_unordered_cursor_prefix" || profile.Sampling.ScanBounded || profile.Cost.ScannedBytes != nil {
		t.Fatal("sampling claimed unproven ordering, representativeness or scan cost", profile.Sampling)
	}
	changed := profile
	changed.Summary = ProfileSummary{Status: "available", Text: "different optional commentary", Receipt: emptyModelReceipt()}
	if changed.DeterministicHash() != profile.DeterministicHash() {
		t.Fatal("optional model prose changed deterministic evidence identity")
	}
}

func TestProfileRangesStayExactAndFailClosed(t *testing.T) {
	fields := []readexec.Field{{Name: "amount", Type: "decimal", NativeType: "numeric", Encoding: "string"}}
	record, report := profileFixture(t, fields, [][]json.RawMessage{{json.RawMessage(`"9007199254740993.125"`)}, {json.RawMessage(`"-0.0000000000000001"`)}, {json.RawMessage(`null`)}})
	redacted := buildProfileFixture(t, record, report)
	if redacted.Columns[0].RangeStatus != "redacted" || redacted.Columns[0].Minimum != nil || redacted.Columns[0].Maximum != nil {
		t.Fatal("ranges were not opt-in", redacted)
	}
	record.Policy.RangeColumns = []string{"amount"}
	permitted := buildProfileFixture(t, record, report)
	if *permitted.Columns[0].Minimum != "-0.0000000000000001" || *permitted.Columns[0].Maximum != "9007199254740993.125" {
		t.Fatal("decimal range passed through a float", permitted.Columns)
	}
	long, _ := json.Marshal(strings.Repeat("9", 129))
	report.Result.Rows = [][]json.RawMessage{{long}, {json.RawMessage(`"1"`)}}
	bounded := buildProfileFixture(t, record, report)
	if bounded.Columns[0].RangeStatus != "value_too_large" || bounded.Columns[0].Minimum != nil || bounded.Columns[0].Maximum != nil {
		t.Fatal("oversized range was truncated or restored without proof", bounded.Columns)
	}
	input, err := SummaryContext(permitted)
	if err != nil || strings.Contains(input, "amount") || strings.Contains(input, "9007199254740993") || strings.Contains(input, "-0.0000000000000001") {
		t.Fatal("permitted storage range leaked into provider input", err, input)
	}
}

func TestProfileEmptyNullAndConstantRules(t *testing.T) {
	fields := []readexec.Field{{Name: "value", Type: "text", NativeType: "text", Encoding: "string"}}
	for _, tc := range []struct {
		name  string
		rows  [][]json.RawMessage
		code  string
		nulls int
	}{
		{"empty", [][]json.RawMessage{}, "empty_observation", 0},
		{"all-null", [][]json.RawMessage{{json.RawMessage(`null`)}, {json.RawMessage(`null`)}}, "all_observed_null", 2},
		{"constant", [][]json.RawMessage{{json.RawMessage(`"same"`)}, {json.RawMessage(`"same"`)}}, "constant_observed_value", 0},
		{"empty-text", [][]json.RawMessage{{json.RawMessage(`""`)}, {json.RawMessage(`"other"`)}}, "empty_text", 0},
	} {
		record, report := profileFixture(t, fields, tc.rows)
		if len(tc.rows) == 0 {
			report.Result.Outcome = "empty"
		}
		profile := buildProfileFixture(t, record, report)
		found := false
		for _, finding := range profile.Findings {
			found = found || finding.Code == tc.code
			if finding.Basis != "sample" {
				t.Fatal("quality finding invented population evidence", finding)
			}
		}
		if !found || profile.Columns[0].Nulls != tc.nulls || profile.Freshness.State != "unknown" {
			t.Fatal("fixed evidence rule changed", tc.name, profile)
		}
	}
}

func TestProfileRejectsInvalidResultAndSelection(t *testing.T) {
	fields := []readexec.Field{{Name: "id", Type: "integer", NativeType: "int8", Encoding: "string"}}
	record, report := profileFixture(t, fields, [][]json.RawMessage{{json.RawMessage(`"1"`)}})
	for name, change := range map[string]func(*ProfileRecord, *readexec.ExecutionReport){
		"invalid-record":  func(r *ProfileRecord, _ *readexec.ExecutionReport) { r.Spec.ID = "../invalid" },
		"no-result":       func(_ *ProfileRecord, r *readexec.ExecutionReport) { r.Result = nil },
		"invalid-outcome": func(_ *ProfileRecord, r *readexec.ExecutionReport) { r.Result.Outcome = "uncertain" },
		"row-bound": func(r *ProfileRecord, _ *readexec.ExecutionReport) {
			r.Settings.SampleRows = 1
			r.SpecHash = r.Digest()
		},
		"byte-bound": func(_ *ProfileRecord, r *readexec.ExecutionReport) { r.Result.Bytes = (16 << 20) + 1 },
		"unknown-dataset": func(r *ProfileRecord, _ *readexec.ExecutionReport) {
			r.Spec.Dataset = "absent"
			r.SpecHash = r.Digest()
		},
		"unknown-column": func(r *ProfileRecord, _ *readexec.ExecutionReport) {
			r.Spec.Columns = []string{"absent"}
			r.SpecHash = r.Digest()
		},
		"unsafe-column": func(r *ProfileRecord, _ *readexec.ExecutionReport) {
			r.Binding.Relations[0].Columns[0].Safe = false
			r.SpecHash = r.Digest()
		},
		"missing-time": func(r *ProfileRecord, _ *readexec.ExecutionReport) {
			r.Spec.TimeColumn = "absent"
			r.SpecHash = r.Digest()
		},
		"schema-size": func(_ *ProfileRecord, r *readexec.ExecutionReport) { r.Result.Schema = nil },
		"schema-name": func(_ *ProfileRecord, r *readexec.ExecutionReport) { r.Result.Schema[0].Name = "different" },
		"row-size":    func(_ *ProfileRecord, r *readexec.ExecutionReport) { r.Result.Rows[0] = nil },
		"wire-type":   func(_ *ProfileRecord, r *readexec.ExecutionReport) { r.Result.Rows[0][0] = json.RawMessage(`{}`) },
	} {
		r := record
		r.Binding = record.Binding.Clone()
		v := report
		result := *report.Result
		result.Schema = append([]readexec.Field(nil), result.Schema...)
		result.Rows = [][]json.RawMessage{{json.RawMessage(`"1"`)}, {json.RawMessage(`"2"`)}}
		v.Result = &result
		change(&r, &v)
		if _, err := BuildProfile(context.Background(), r, v, time.Millisecond, record.Created); err == nil {
			t.Fatal("invalid evidence accepted", name)
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := BuildProfile(ctx, record, report, time.Millisecond, record.Created); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled aggregation continued", err)
	}
	//nolint:staticcheck // Deliberately exercise the nil-context rejection boundary.
	if _, err := BuildProfile(nil, record, report, time.Millisecond, record.Created); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil aggregation context accepted", err)
	}
}

func TestProfileSchemaDiffAndProgressIdentity(t *testing.T) {
	before := []readexec.Column{{Name: "removed", NativeType: "int4", Safe: true}, {Name: "changed", NativeType: "int4", Safe: true}}
	after := []readexec.Column{{Name: "added", NativeType: "text", Safe: true}, {Name: "changed", NativeType: "text", Nullable: true, Safe: false}}
	changes := SchemaDiff(before, after)
	kinds := make([]string, len(changes))
	for i, change := range changes {
		kinds[i] = change.Column + ":" + change.Kind
	}
	want := []string{"added:added", "changed:nullability_changed", "changed:safety_changed", "changed:type_changed", "removed:removed"}
	if !reflect.DeepEqual(kinds, want) || len(SchemaDiff(before, before)) != 0 {
		t.Fatal("structural drift was not complete or deterministic", changes)
	}
	record, report := profileFixture(t, []readexec.Field{{Name: "id", Type: "integer", NativeType: "int8", Encoding: "string"}}, [][]json.RawMessage{{json.RawMessage(`"1"`)}})
	profile := buildProfileFixture(t, record, report)
	hash := record.Digest()
	record.LastReadOperation, record.LastReadDeadline = "physical-attempt", record.Created.Add(time.Minute)
	record.Result, record.SummaryStarted = &profile, true
	if record.Digest() != hash || record.Public().Profile != nil {
		t.Fatal("private checkpoint changed immutable input or became public")
	}
	record.State = "complete"
	if record.Public().Profile != &profile {
		t.Fatal("completed retained evidence unavailable")
	}
	record.Spec.SkipLLM = false
	if record.Digest() == hash || record.Valid() {
		t.Fatal("a resumed version changed its paid-work decision")
	}
}
