package config

import (
	"strings"
	"testing"
	"time"
)

func TestQueryBundleConfiguration(t *testing.T) {
	want := DefaultQueryBundles()
	if ValidateQueryBundles(want) != nil || Defaults().QueryBundles != want {
		t.Fatal("invalid/missing defaults")
	}
	for _, change := range []func(*QueryBundles){
		func(v *QueryBundles) { v.TTL = 0 }, func(v *QueryBundles) { v.TTL = Duration(time.Hour + time.Nanosecond) },
		func(v *QueryBundles) { v.Retention = v.TTL - 1 }, func(v *QueryBundles) { v.Retention = Duration(7*24*time.Hour + time.Nanosecond) },
		func(v *QueryBundles) { v.MaxBytes = 4095 }, func(v *QueryBundles) { v.MaxBytes = 1<<20 + 1 },
		func(v *QueryBundles) { v.PerSession = 0 }, func(v *QueryBundles) { v.PerSession = 65 },
		func(v *QueryBundles) { v.PerTenant = v.PerSession - 1 }, func(v *QueryBundles) { v.PerTenant = 4097 },
		func(v *QueryBundles) { v.MaxSteps = 0 }, func(v *QueryBundles) { v.MaxSteps = 33 },
		func(v *QueryBundles) { v.StepTimeout = 0 }, func(v *QueryBundles) { v.StepTimeout = Duration(time.Minute + time.Nanosecond) },
	} {
		v := want
		change(&v)
		if err := ValidateQueryBundles(v); err == nil || !strings.Contains(err.Error(), "query_bundles") {
			t.Fatal("unbounded configuration accepted", v, err)
		}
	}
	for _, v := range []QueryBundles{
		{TTL: Duration(time.Second), Retention: Duration(time.Second), MaxBytes: 4096, PerSession: 1, PerTenant: 1, MaxSteps: 1, StepTimeout: Duration(time.Second)},
		{TTL: Duration(time.Hour), Retention: Duration(7 * 24 * time.Hour), MaxBytes: 1 << 20, PerSession: 64, PerTenant: 4096, MaxSteps: 32, StepTimeout: Duration(time.Minute)},
	} {
		if err := ValidateQueryBundles(v); err != nil {
			t.Fatal("boundary", err)
		}
	}
}
