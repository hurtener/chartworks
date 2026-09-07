package engineering

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func TestParquetDeclaredValuesPreserveExactRepresentations(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    parquet.Value
		c    parquetColumn
		kind string
		want Cell
	}{
		{"null", parquet.NullValue(), parquetColumn{physical: 2}, "integer", Cell{Null: true}},
		{"true", parquet.BooleanValue(true), parquetColumn{physical: 0}, "boolean", Cell{Text: "true"}},
		{"false", parquet.BooleanValue(false), parquetColumn{physical: 0}, "boolean", Cell{Text: "false"}},
		{"int32-min", parquet.Int32Value(math.MinInt32), parquetColumn{physical: 1}, "integer", Cell{Text: "-2147483648"}},
		{"int64-min", parquet.Int64Value(math.MinInt64), parquetColumn{physical: 2}, "integer", Cell{Text: "-9223372036854775808"}},
		{"int64-max", parquet.Int64Value(math.MaxInt64), parquetColumn{physical: 2}, "decimal", Cell{Text: "9223372036854775807"}},
		{"int32-decimal", parquet.Int32Value(12), parquetColumn{physical: 1}, "decimal", Cell{Text: "12"}},
		{"decimal32", parquet.Int32Value(-123), parquetColumn{physical: 1, decimal: true, scale: 2}, "decimal", Cell{Text: "-1.23"}},
		{"decimal64", parquet.Int64Value(9007199254740993125), parquetColumn{physical: 2, decimal: true, scale: 3}, "decimal", Cell{Text: "9007199254740993.125"}},
		{"decimal-bytes-negative", parquet.ByteArrayValue([]byte{255}), parquetColumn{physical: 6, decimal: true, scale: 3}, "decimal", Cell{Text: "-0.001"}},
		{"decimal-fixed-positive", parquet.FixedLenByteArrayValue([]byte{0, 128}), parquetColumn{physical: 7, decimal: true, scale: 2}, "decimal", Cell{Text: "1.28"}},
		{"decimal-fixed-negative", parquet.FixedLenByteArrayValue([]byte{128}), parquetColumn{physical: 7, decimal: true, scale: 0}, "decimal", Cell{Text: "-128"}},
		{"float64", parquet.DoubleValue(1.25), parquetColumn{physical: 5}, "number", Cell{Text: "1.25"}},
		{"date-epoch", parquet.Int32Value(0), parquetColumn{physical: 1, date: true}, "date", Cell{Text: "1970-01-01"}},
		{"date-before-epoch", parquet.Int32Value(-1), parquetColumn{physical: 1, date: true}, "date", Cell{Text: "1969-12-31"}},
		{"milliseconds", parquet.Int64Value(1234), parquetColumn{physical: 2, timestamp: 1000}, "timestamp", Cell{Text: "1970-01-01T00:00:01.234Z"}},
		{"microseconds", parquet.Int64Value(-1), parquetColumn{physical: 2, timestamp: 1000000}, "timestamp", Cell{Text: "1969-12-31T23:59:59.999999Z"}},
		{"nanoseconds", parquet.Int64Value(1500000), parquetColumn{physical: 2, timestamp: 1000000000}, "timestamp", Cell{Text: "1970-01-01T00:00:00.0015Z"}},
		{"binary", parquet.ByteArrayValue([]byte{0, 255, 128}), parquetColumn{physical: 6}, "binary", Cell{Text: "00ff80"}},
		{"fixed-binary", parquet.FixedLenByteArrayValue([]byte{202, 254}), parquetColumn{physical: 7}, "binary", Cell{Text: "cafe"}},
		{"text", parquet.ByteArrayValue([]byte("dato sintético")), parquetColumn{physical: 6}, "text", Cell{Text: "dato sintético"}},
		{"json", parquet.ByteArrayValue([]byte(`{"n":9007199254740993}`)), parquetColumn{physical: 6}, "json", Cell{Text: `{"n":9007199254740993}`}},
		{"decimal-text", parquet.ByteArrayValue([]byte("9007199254740993.125")), parquetColumn{physical: 6}, "decimal", Cell{Text: "9007199254740993.125"}},
		{"date-text", parquet.ByteArrayValue([]byte("2026-09-07")), parquetColumn{physical: 6}, "date", Cell{Text: "2026-09-07"}},
		{"timestamp-text", parquet.ByteArrayValue([]byte("2026-09-07T00:00:00Z")), parquetColumn{physical: 6}, "timestamp", Cell{Text: "2026-09-07T00:00:00Z"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declared := UploadColumn{Name: "value", Type: tc.kind, Nullable: true}
			got, err := parquetCell(tc.v, tc.c, declared)
			if err != nil || got != tc.want {
				t.Fatal("physical value lost its declared representation", err, got, tc.want)
			}
			normalized, err := normalizeCell(declared, got)
			if err != nil || normalized != tc.want {
				t.Fatal("normalization changed an exact declared value", err, normalized)
			}
			if _, err = copyCell(declared, normalized); err != nil {
				t.Fatal("qualified value could not reach the managed writer", err)
			}
		})
	}
}

func TestParquetPhysicalTypeMismatchAndUnrepresentableData(t *testing.T) {
	for _, tc := range []struct {
		v parquet.Value
		c parquetColumn
	}{
		{parquet.BooleanValue(true), parquetColumn{physical: 0}},
		{parquet.Int32Value(1), parquetColumn{physical: 1}},
		{parquet.Int64Value(1), parquetColumn{physical: 2}},
		{parquet.FloatValue(1), parquetColumn{physical: 4}},
		{parquet.DoubleValue(1), parquetColumn{physical: 5}},
		{parquet.ByteArrayValue([]byte("1")), parquetColumn{physical: 6}},
		{parquet.ZeroValue(parquet.Int96), parquetColumn{physical: 3}},
		{parquet.ZeroValue(parquet.Int96), parquetColumn{physical: 3, decimal: true}},
		{parquet.Int64Value(1), parquetColumn{physical: 1}},
		{parquet.ByteArrayValue(nil), parquetColumn{physical: 6, decimal: true}},
		{parquet.ByteArrayValue(make([]byte, 33)), parquetColumn{physical: 6, decimal: true}},
		{parquet.Int32Value(math.MinInt32), parquetColumn{physical: 1, date: true}},
		{parquet.Int32Value(math.MaxInt32), parquetColumn{physical: 1, date: true}},
		{parquet.Int64Value(math.MaxInt64), parquetColumn{physical: 2, timestamp: 1000}},
		{parquet.Int64Value(math.MinInt64), parquetColumn{physical: 2, timestamp: 1000}},
	} {
		if _, err := parquetCell(tc.v, tc.c, UploadColumn{Type: "unqualified"}); !errors.Is(err, ErrFormat) {
			t.Fatal("unqualified physical conversion accepted", tc.c, err)
		}
	}
	column := UploadColumn{Type: "timestamp"}
	cell, err := parquetCell(parquet.Int64Value(1), parquetColumn{physical: 2, timestamp: 1000000000}, column)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = normalizeCell(column, cell); !errors.Is(err, ErrFormat) {
		t.Fatal("nanosecond precision silently rounded for workspace storage", err)
	}
	for _, tc := range []struct {
		kind string
		text string
	}{{"text", "=EXECUTE()"}, {"text", "private\x00value"}, {"json", "not-json"}, {"integer", "9223372036854775808"}, {"date", "0000-01-01"}} {
		if _, err = normalizeCell(UploadColumn{Type: tc.kind}, Cell{Text: tc.text}); !errors.Is(err, ErrFormat) {
			t.Fatal("unsafe or unrepresentable declared value accepted", tc.kind, err)
		}
	}
}

func deltaLengthHeader(block, mini, count uint64, first int64) []byte {
	raw := binary.AppendUvarint(nil, block)
	raw = binary.AppendUvarint(raw, mini)
	raw = binary.AppendUvarint(raw, count)
	return binary.AppendVarint(raw, first)
}

func TestDeltaLengthEncodingBoundsBeforeAllocation(t *testing.T) {
	for _, tc := range []struct {
		first, minimum int64
		packed         byte
		width          byte
		payload        int
	}{
		{1, 0, 0, 0, 2}, {1, 0, 1, 1, 3}, {3, -1, 0, 0, 5}, {0, 0, 0, 0, 0},
	} {
		raw := deltaLengthHeader(128, 4, 2, tc.first)
		raw = binary.AppendVarint(raw, tc.minimum)
		raw = append(raw, tc.width, 0, 0, 0)
		if tc.width != 0 {
			raw = append(raw, tc.packed, 0, 0, 0)
		}
		raw = append(raw, bytes.Repeat([]byte{'a'}, tc.payload)...)
		if err := guardDeltaLengths(raw, 2, 4); err != nil {
			t.Fatal("valid bounded delta lengths rejected", tc, err)
		}
	}
	one := append(deltaLengthHeader(128, 4, 1, 3), 'a', 'b', 'c')
	if err := guardDeltaLengths(one, 1, 3); err != nil {
		t.Fatal("single exact-length cell rejected", err)
	}
	if err := guardDeltaLengths(deltaLengthHeader(128, 4, 0, 0), 0, 1); err != nil {
		t.Fatal("empty length vector rejected", err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
		n    int
		max  int
		want error
	}{
		{"negative-count", nil, -1, 4, ErrLimit},
		{"huge-count", nil, 1000001, 4, ErrLimit},
		{"zero-limit", nil, 0, 0, ErrLimit},
		{"huge-cell-limit", nil, 0, 65537, ErrLimit},
		{"missing-block", nil, 1, 4, ErrFormat},
		{"small-block", deltaLengthHeader(127, 4, 1, 1), 1, 4, ErrFormat},
		{"large-block", deltaLengthHeader(65537, 4, 1, 1), 1, 4, ErrFormat},
		{"zero-mini", deltaLengthHeader(128, 0, 1, 1), 1, 4, ErrFormat},
		{"large-mini", deltaLengthHeader(128, 257, 1, 1), 1, 4, ErrFormat},
		{"uneven-mini", deltaLengthHeader(128, 3, 1, 1), 1, 4, ErrFormat},
		{"count-mismatch", deltaLengthHeader(128, 4, 2, 1), 1, 4, ErrFormat},
		{"negative-length", deltaLengthHeader(128, 4, 1, -1), 1, 4, ErrLimit},
		{"large-length", deltaLengthHeader(128, 4, 1, 5), 1, 4, ErrLimit},
		{"truncated-minimum", append(deltaLengthHeader(128, 4, 2, 1), 128), 2, 4, ErrFormat},
		{"large-minimum", binary.AppendVarint(deltaLengthHeader(128, 4, 2, 1), 5), 2, 4, ErrFormat},
		{"negative-minimum", binary.AppendVarint(deltaLengthHeader(128, 4, 2, 1), -5), 2, 4, ErrFormat},
		{"truncated-widths", append(binary.AppendVarint(deltaLengthHeader(128, 4, 2, 1), 0), 0), 2, 4, ErrFormat},
		{"large-width", append(binary.AppendVarint(deltaLengthHeader(128, 4, 2, 1), 0), 33, 0, 0, 0), 2, 4, ErrLimit},
		{"truncated-bits", append(binary.AppendVarint(deltaLengthHeader(128, 4, 2, 1), 0), 1, 0, 0, 0, 0, 0, 0), 2, 4, ErrFormat},
		{"growing-cell", append(binary.AppendVarint(deltaLengthHeader(128, 4, 2, 4), 1), 0, 0, 0, 0), 2, 4, ErrLimit},
		{"negative-cell", append(binary.AppendVarint(deltaLengthHeader(128, 4, 2, 0), -1), 0, 0, 0, 0), 2, 4, ErrLimit},
		{"short-payload", deltaLengthHeader(128, 4, 1, 3), 1, 4, ErrFormat},
		{"extra-payload", append(deltaLengthHeader(128, 4, 0, 0), 0), 0, 4, ErrFormat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := guardDeltaLengths(tc.raw, tc.n, tc.max); !errors.Is(err, tc.want) {
				t.Fatal("delta encoding failed its bound contract", err, tc.want)
			}
		})
	}
}
