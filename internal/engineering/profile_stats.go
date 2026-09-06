package engineering

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

// FreshnessAt is deterministic at the recorded observation instant. A truncated
// sample does not establish full-dataset freshness, even if its newest row is old.
func FreshnessAt(observed time.Time, latest *time.Time, complete bool, l config.Profiling, permitValue bool) Freshness {
	out := Freshness{State: "unknown", Reason: "no_event_time", Basis: "observed_sample_max", SampleState: "unknown"}
	if latest == nil || observed.IsZero() {
		return out
	}
	if latest.After(observed) {
		out.Reason = "future_event_time"
		return out
	}
	age := observed.Sub(*latest)
	out.SampleState = "fresh"
	if age >= time.Duration(l.StaleAfter) {
		out.SampleState = "stale"
	} else if age > time.Duration(l.FreshFor) {
		out.SampleState = "aging"
	}
	if permitValue {
		value := latest.UTC()
		out.Latest = &value
	}
	if !complete {
		out.Reason = "partial_sample"
		return out
	}
	out.State = out.SampleState
	out.Reason = "configured_thresholds"
	out.Basis = "complete_result_event_max"
	return out
}
func relationFor(r ProfileRecord) (readexec.Relation, error) {
	for _, rel := range r.Binding.Relations {
		if rel.ID == r.Spec.Dataset {
			return rel, nil
		}
	}
	return readexec.Relation{}, ErrInvalid
}
func selectedColumns(r ProfileRecord) ([]readexec.Column, error) {
	rel, err := relationFor(r)
	if err != nil {
		return nil, err
	}
	byName := map[string]readexec.Column{}
	for _, c := range rel.Columns {
		byName[c.Name] = c
	}
	names := r.Spec.Columns
	if len(names) == 0 {
		for _, c := range rel.Columns {
			names = append(names, c.Name)
		}
	}
	out := make([]readexec.Column, len(names))
	hasTime := r.Spec.TimeColumn == ""
	for i, name := range names {
		c, ok := byName[name]
		if !ok || !c.Safe {
			return nil, readexec.ErrUnsupported
		}
		out[i] = c
		if name == r.Spec.TimeColumn {
			hasTime = true
		}
	}
	if !hasTime {
		return nil, ErrInvalid
	}
	return out, nil
}
func permittedRange(r ProfileRecord, name string) bool {
	for _, n := range r.Policy.RangeColumns {
		if name == n {
			return true
		}
	}
	return false
}

// BuildProfile consumes a qualified bounded result without retaining raw rows,
// top-value lists, sketches containing source text or unapproved range values.
func BuildProfile(ctx context.Context, r ProfileRecord, report readexec.ExecutionReport, elapsed time.Duration, observed time.Time) (Profile, error) {
	if ctx == nil || !r.Valid() || report.Result == nil || elapsed < 0 || observed.IsZero() {
		return Profile{}, ErrInvalid
	}
	data := report.Result
	if data.Outcome != "succeeded" && data.Outcome != "empty" && data.Outcome != "truncated" {
		return Profile{}, ErrState
	}
	if len(data.Rows) > r.Settings.SampleRows || data.Bytes > r.Settings.SampleBytes {
		return Profile{}, ErrLimit
	}
	columns, err := selectedColumns(r)
	if err != nil {
		return Profile{}, err
	}
	if len(columns) != len(data.Schema) {
		return Profile{}, readexec.ErrType
	}
	relation, err := relationFor(r)
	if err != nil {
		return Profile{}, err
	}
	out := Profile{Version: r.Spec.ID, Source: r.Spec.Source, Context: r.Spec.Context, Dataset: r.Spec.Dataset, SourceRevision: r.Binding.Revision, ObservedAt: observed.UTC(), Schema: append([]readexec.Column(nil), relation.Columns...), Columns: make([]ColumnProfile, len(columns)), Findings: []QualityFinding{}, Cost: data.Cost, ExecutionNS: int64(elapsed), ReadOperation: report.Attempt.Manifest.Operation, ReadAttempt: report.Attempt.ID, PolicyHash: readexec.Hash(r.Policy), Summary: ProfileSummary{Status: "not_requested", Receipt: emptyModelReceipt()}}
	out.Sampling = Sampling{Strategy: "validated_ordered_cursor_prefix", Rows: len(data.Rows), Bytes: data.Bytes, Complete: data.Outcome != "truncated", Truncation: data.Truncation, ScanBounded: false, RowCeiling: report.Attempt.Manifest.Limits.Rows, ByteCeiling: report.Attempt.Manifest.Limits.Bytes, PlannerCeiling: report.Attempt.Manifest.Limits.PlannerCost, TimeoutNS: int64(report.Attempt.Manifest.Limits.Timeout)}
	distinct := make([]map[[32]byte]struct{}, len(columns))
	var latest *time.Time
	for i, c := range columns {
		if data.Schema[i].Name != c.Name {
			return Profile{}, readexec.ErrType
		}
		out.Columns[i] = ColumnProfile{Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable, Observed: len(data.Rows), DistinctExact: true, Families: map[string]int{}, RangeStatus: "not_applicable"}
		if rangeType(data.Schema[i].Type) {
			out.Columns[i].RangeStatus = "redacted"
			if permittedRange(r, c.Name) {
				out.Columns[i].RangeStatus = "empty"
			}
		}
		distinct[i] = map[[32]byte]struct{}{}
	}
	for _, row := range data.Rows {
		if err = ctx.Err(); err != nil {
			return Profile{}, err
		}
		if len(row) != len(columns) {
			return Profile{}, readexec.ErrType
		}
		for i, raw := range row {
			c := &out.Columns[i]
			field := data.Schema[i]
			if string(raw) == "null" {
				c.Nulls++
				c.Families["null"]++
				continue
			}
			hash := sha256.Sum256(raw)
			if c.DistinctExact {
				if _, ok := distinct[i][hash]; !ok {
					if len(distinct[i]) == 1024 {
						c.Distinct = 1025
						c.DistinctExact = false
						distinct[i] = nil
					} else {
						distinct[i][hash] = struct{}{}
						c.Distinct = len(distinct[i])
					}
				}
			}
			text, err := profileText(field, raw)
			if err != nil {
				return Profile{}, err
			}
			family := valueFamily(field.Type, text)
			c.Families[family]++
			if c.Name == r.Spec.TimeColumn {
				stamp, valid := eventTime(field, text)
				if !valid {
					return Profile{}, readexec.ErrType
				}
				if latest == nil || stamp.After(*latest) {
					v := stamp
					latest = &v
				}
			}
			if rangeType(field.Type) && permittedRange(r, c.Name) {
				if len(text) > 128 {
					c.RangeStatus = "value_too_large"
					c.Minimum = nil
					c.Maximum = nil
					continue
				}
				if c.RangeStatus == "value_too_large" {
					continue
				}
				if c.Minimum == nil {
					lo, hi := text, text
					c.Minimum = &lo
					c.Maximum = &hi
					c.RangeStatus = "permitted"
				} else {
					less, err := compareProfile(field, text, *c.Minimum)
					if err != nil {
						return Profile{}, err
					}
					if less < 0 {
						v := text
						c.Minimum = &v
					}
					greater, err := compareProfile(field, text, *c.Maximum)
					if err != nil {
						return Profile{}, err
					}
					if greater > 0 {
						v := text
						c.Maximum = &v
					}
				}
			}
		}
	}
	for _, c := range out.Columns {
		if c.Observed == 0 {
			out.Findings = append(out.Findings, QualityFinding{Code: "empty_observation", Column: c.Name, Basis: "sample"})
			continue
		}
		if c.Nulls > 0 {
			code := "contains_nulls"
			if c.Nulls == c.Observed {
				code = "all_observed_null"
			}
			out.Findings = append(out.Findings, QualityFinding{Code: code, Column: c.Name, Count: c.Nulls, Basis: "sample"})
		}
		if !c.Nullable && c.Nulls > 0 {
			out.Findings = append(out.Findings, QualityFinding{Code: "nonnullable_violation", Column: c.Name, Count: c.Nulls, Basis: "sample"})
		}
		if c.DistinctExact && c.Distinct == 1 && c.Observed-c.Nulls > 1 {
			out.Findings = append(out.Findings, QualityFinding{Code: "constant_observed_value", Column: c.Name, Count: c.Observed - c.Nulls, Basis: "sample"})
		}
		if c.Families["empty_text"] > 0 {
			out.Findings = append(out.Findings, QualityFinding{Code: "empty_text", Column: c.Name, Count: c.Families["empty_text"], Basis: "sample"})
		}
	}
	out.Freshness = FreshnessAt(out.ObservedAt, latest, out.Sampling.Complete, r.Settings, permittedRange(r, r.Spec.TimeColumn))
	if !out.Valid(r) {
		return Profile{}, readexec.ErrType
	}
	return out, nil
}
func rangeType(kind string) bool {
	return kind == "integer" || kind == "decimal" || kind == "number" || kind == "temporal"
}
func profileText(field readexec.Field, raw json.RawMessage) (string, error) {
	if field.Encoding == "string" {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return "", readexec.ErrType
		}
		return text, nil
	}
	if field.Encoding == "boolean" && (string(raw) == "true" || string(raw) == "false") {
		return string(raw), nil
	}
	if field.Encoding == "number" && json.Valid(raw) {
		return string(raw), nil
	}
	return "", readexec.ErrType
}
func eventTime(field readexec.Field, s string) (time.Time, bool) {
	var t time.Time
	var err error
	switch field.NativeType {
	case "date":
		t, err = time.Parse("2006-01-02", s)
	case "timestamptz":
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999Z07"} {
			t, err = time.Parse(layout, s)
			if err == nil {
				break
			}
		}
	case "timestamp":
		t, err = time.ParseInLocation("2006-01-02 15:04:05.999999999", s, time.UTC)
	default:
		return time.Time{}, false
	}
	return t.UTC(), err == nil
}
func compareProfile(field readexec.Field, a, b string) (int, error) {
	if field.Type == "temporal" {
		x, ok := eventTime(field, a)
		y, ok2 := eventTime(field, b)
		if !ok || !ok2 {
			return 0, readexec.ErrType
		}
		if x.Before(y) {
			return -1, nil
		}
		if x.After(y) {
			return 1, nil
		}
		return 0, nil
	}
	x, ok := new(big.Rat).SetString(a)
	y, ok2 := new(big.Rat).SetString(b)
	if !ok || !ok2 {
		return 0, readexec.ErrType
	}
	return x.Cmp(y), nil
}

var emailShape = regexp.MustCompile(`^[^@\s]{1,128}@[^@\s]{1,128}\.[A-Za-z]{2,32}$`)
var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func valueFamily(kind, text string) string {
	switch kind {
	case "text":
		if text == "" {
			return "empty_text"
		}
		if len(text) <= 260 && emailShape.MatchString(text) {
			return "email_like"
		}
		if uuidShape.MatchString(text) {
			return "uuid_like"
		}
		if _, err := time.Parse("2006-01-02", text); err == nil {
			return "date_like"
		}
		if strings.TrimSpace(text) == "" {
			return "whitespace_text"
		}
		return "text"
	case "structured":
		return "json"
	case "integer", "decimal", "number", "boolean", "binary", "temporal":
		return kind
	}
	return "unsupported"
}
