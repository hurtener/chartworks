package config

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEngineeringDefaultsAndUploadBounds(t *testing.T) {
	u := DefaultUploads()
	p := DefaultProfiling()
	if u.Enabled || p.Enabled || p.Summaries || len(p.Policies) != 0 || u.MaxBytes != 100<<20 || u.MaxRows != 1000000 || u.MaxExpandedBytes != 256<<20 || u.StagingTTL != Duration(24*time.Hour) || p.SampleRows != 1000 || p.SampleBytes != 1<<20 || p.Timeout != Duration(30*time.Second) || ValidateUploads(u) != nil || ValidateProfiling(p) != nil {
		t.Fatal("bounded opt-in engineering defaults changed")
	}
	for _, name := range []string{"MaxBytes", "MaxRows", "MaxColumns", "MaxCells", "MaxCellBytes", "MaxExpandedBytes", "MaxArchiveEntries", "MaxSheets", "MaxExpansionRatio", "MaxPageBytes", "MaxRowGroupBytes", "MaxPerTenant", "MaxTenantBytes", "Concurrency", "Timeout", "StagingTTL"} {
		for _, value := range []int64{0, math.MaxInt64} {
			copy := u.Clone()
			reflect.ValueOf(&copy).Elem().FieldByName(name).SetInt(value)
			if ValidateUploads(copy) == nil {
				t.Fatal("unbounded upload setting accepted", name, value)
			}
		}
	}
	for _, formats := range [][]string{nil, {"csv", "csv"}, {"zip"}, {"CSV"}, {"csv", "xlsx", "parquet", "csv"}} {
		copy := u.Clone()
		copy.Formats = formats
		if ValidateUploads(copy) == nil {
			t.Fatal("ambiguous or unqualified format accepted", formats)
		}
	}
	copy := u.Clone()
	copy.MaxExpandedBytes = copy.MaxBytes - 1
	if ValidateUploads(copy) == nil {
		t.Fatal("expanded byte ceiling smaller than input accepted")
	}
	copy = u.Clone()
	copy.MaxRowGroupBytes = copy.MaxPageBytes - 1
	if ValidateUploads(copy) == nil {
		t.Fatal("row group/page bound contradiction accepted")
	}
	copy = u.Clone()
	copy.MaxTenantBytes = copy.MaxBytes - 1
	if ValidateUploads(copy) == nil {
		t.Fatal("tenant budget smaller than upload admitted")
	}
	copy = u.Clone()
	copy.Formats[0] = "changed"
	if u.Formats[0] != "csv" {
		t.Fatal("upload service configuration aliases caller storage")
	}
}

func TestProfilingCeilingsAndPrivacyPolicyValidation(t *testing.T) {
	base := DefaultProfiling()
	for _, name := range []string{"SampleRows", "SampleBytes", "Timeout", "StaleAfter", "MaxVersions"} {
		for _, value := range []int64{0, math.MaxInt64} {
			copy := base.Clone()
			reflect.ValueOf(&copy).Elem().FieldByName(name).SetInt(value)
			if ValidateProfiling(copy) == nil {
				t.Fatal("unbounded profiling setting accepted", name, value)
			}
		}
	}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 1e12 + 1} {
		copy := base.Clone()
		copy.PlannerCostCeiling = value
		if ValidateProfiling(copy) == nil {
			t.Fatal("nonfinite or unbounded planner ceiling accepted", value)
		}
	}
	for _, value := range []Duration{-1, Duration(366 * 24 * time.Hour)} {
		copy := base.Clone()
		copy.FreshFor = value
		if ValidateProfiling(copy) == nil {
			t.Fatal("unsupported freshness threshold accepted")
		}
	}
	copy := base.Clone()
	copy.StaleAfter = copy.FreshFor
	if ValidateProfiling(copy) == nil {
		t.Fatal("indistinguishable freshness thresholds accepted")
	}
	policy := ProfilePolicy{ID: "range-policy", Tenant: "tenant", Source: "source", RangeColumns: []string{"amount", "event_time"}}
	base.Policies = []ProfilePolicy{policy}
	if ValidateProfiling(base) != nil {
		t.Fatal("explicit bounded privacy policy rejected")
	}
	for name, change := range map[string]func(*Profiling){
		"duplicate": func(p *Profiling) { p.Policies = append(p.Policies, p.Policies[0]) },
		"missing-id": func(p *Profiling) { p.Policies[0].ID = "" },
		"missing-tenant": func(p *Profiling) { p.Policies[0].Tenant = "" },
		"invalid-source": func(p *Profiling) { p.Policies[0].Source = "PRIVATE_SECRET/../source" },
		"too-many": func(p *Profiling) { p.Policies = make([]ProfilePolicy, 129) },
		"too-many-columns": func(p *Profiling) { p.Policies[0].RangeColumns = make([]string, 257) },
		"duplicate-column": func(p *Profiling) { p.Policies[0].RangeColumns = []string{"amount", "amount"} },
		"expression": func(p *Profiling) { p.Policies[0].RangeColumns = []string{"amount; PRIVATE_SECRET"} },
	} {
		copy := base.Clone()
		change(&copy)
		err := ValidateProfiling(copy)
		if err == nil || strings.Contains(err.Error(), "PRIVATE_SECRET") {
			t.Fatal("unsafe policy accepted or diagnostic disclosed contents", name, err)
		}
	}
	clone := base.Clone()
	clone.Policies[0].RangeColumns[0] = "changed"
	clone.Policies[0].Source = "changed"
	if base.Policies[0].RangeColumns[0] != "amount" || base.Policies[0].Source != "source" {
		t.Fatal("privacy policy snapshot aliases mutable caller storage")
	}
}
