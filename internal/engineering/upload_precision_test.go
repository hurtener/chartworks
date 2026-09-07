package engineering

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
)

func TestParquetFloat32WorkspacePromotionIsLossless(t *testing.T) {
	for _, value := range []float32{0, float32(math.Copysign(0, -1)), 0.1, -0.1, math.SmallestNonzeroFloat32, math.MaxFloat32} {
		cell, err := parquetCell(parquet.FloatValue(value), parquetColumn{physical: 4}, UploadColumn{Type: "number"})
		if err != nil {
			t.Fatal(err)
		}
		cell, err = normalizeCell(UploadColumn{Type: "number"}, cell)
		if err != nil {
			t.Fatal(err)
		}
		out, err := copyCell(UploadColumn{Type: "number"}, cell)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := out.(float64); !ok || math.Float64bits(got) != math.Float64bits(float64(value)) {
			t.Fatalf("float32 promotion lost its exact value: %v -> %q -> %v", value, cell.Text, out)
		}
	}
	for _, value := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		cell, err := parquetCell(parquet.FloatValue(value), parquetColumn{physical: 4}, UploadColumn{Type: "number"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = normalizeCell(UploadColumn{Type: "number"}, cell); !errors.Is(err, ErrFormat) {
			t.Fatal("nonfinite value accepted", err)
		}
	}
}

func TestUploadTimestampCannotSilentlyLosePrecision(t *testing.T) {
	column := UploadColumn{Type: "timestamp"}
	for _, input := range []string{"2026-01-02T03:04:05Z", "2026-01-02T06:04:05.123456+03:00", "2026-01-02T03:04:05.123456000Z", "0001-01-01T00:00:00Z"} {
		cell, err := normalizeCell(column, Cell{Text: input})
		if err != nil {
			t.Fatalf("representable timestamp rejected %q: %v", input, err)
		}
		copied, err := copyCell(column, cell)
		if err != nil {
			t.Fatal(err)
		}
		stamp, ok := copied.(time.Time)
		expected, err := time.Parse(time.RFC3339Nano, input)
		if !ok || err != nil || !stamp.Equal(expected) || stamp.Nanosecond()%1000 != 0 {
			t.Fatal("timestamp changed", input, copied, err)
		}
	}
	for _, input := range []string{"2026-01-02T03:04:05.000000001Z", "2026-01-02T03:04:05.123456789Z", "0000-01-01T00:00:00Z", "0001-01-01T00:00:00+01:00", "9999-12-31T23:59:59-01:00"} {
		if _, err := normalizeCell(column, Cell{Text: input}); !errors.Is(err, ErrFormat) {
			t.Fatalf("unrepresentable timestamp accepted %q: %v", input, err)
		}
	}
	if _, err := normalizeCell(UploadColumn{Type: "date"}, Cell{Text: "0000-01-01"}); !errors.Is(err, ErrFormat) {
		t.Fatal("year zero accepted", err)
	}
}
