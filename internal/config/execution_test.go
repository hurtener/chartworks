package config

import (
	"math"
	"testing"
	"time"
)

func TestReadExecutionConfigBounds(t *testing.T) {
	baseline := DefaultReadValidation()
	if baseline.RowsDefault != 10000 || baseline.RowsCeiling != 100000 || baseline.PreviewRows != 200 || baseline.Timeout != Duration(time.Minute) || ValidateReadValidation(baseline) != nil {
		t.Fatal("documented read defaults differ")
	}
	for _, change := range []func(*ReadValidation){
		func(v *ReadValidation) { v.RowsDefault = 0 }, func(v *ReadValidation) { v.RowsCeiling = 9999 }, func(v *ReadValidation) { v.RowsCeiling = 100001 }, func(v *ReadValidation) { v.PreviewRows = 0 }, func(v *ReadValidation) { v.PreviewRows = 10001 }, func(v *ReadValidation) { v.BytesDefault = 0 }, func(v *ReadValidation) { v.BytesCeiling = 1024 }, func(v *ReadValidation) { v.BytesCeiling = 17 << 20 }, func(v *ReadValidation) { v.Timeout = 0 }, func(v *ReadValidation) { v.Timeout = Duration(61 * time.Second) }, func(v *ReadValidation) { v.CancelGrace = 0 }, func(v *ReadValidation) { v.CancelGrace = Duration(4 * time.Second) }, func(v *ReadValidation) { v.PlannerCostCeiling = math.NaN() }, func(v *ReadValidation) { v.PlannerCostCeiling = math.Inf(1) }, func(v *ReadValidation) { v.ExecutionConcurrency = 0 }, func(v *ReadValidation) { v.ExecutionConcurrency = 17 }, func(v *ReadValidation) { v.MaxReadAttempts = 0 }, func(v *ReadValidation) { v.MaxReadAttempts = 4 },
	} {
		v := baseline
		change(&v)
		if ValidateReadValidation(v) == nil {
			t.Fatal("invalid read execution bound accepted")
		}
	}
}
