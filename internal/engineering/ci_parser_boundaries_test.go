package engineering

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zstd"
)

func TestCompactThriftIntegerAndAllocationBoundaries(t *testing.T) {
	for _, value := range []byte{0, 1, 127, 128, 255} {
		r := compactReader{data: []byte{value}}
		got, err := r.value(3, 0, false)
		want := int64(value)
		if value >= 128 {
			want -= 256
		}
		if err != nil || got.n != want || r.at != 1 {
			t.Fatal("signed byte changed", value, got.n, err)
		}
	}
	for _, value := range []uint64{0, 1, 127, 128, math.MaxUint64} {
		r := compactReader{data: binary.AppendUvarint(nil, value)}
		got, err := r.unsigned()
		if err != nil || got != value || r.at != len(r.data) {
			t.Fatal("unsigned boundary changed", value, got, err)
		}
	}
	for _, raw := range [][]byte{nil, {128}, bytes.Repeat([]byte{255}, 10), append(bytes.Repeat([]byte{128}, 9), 2)} {
		r := compactReader{data: raw}
		if _, err := r.unsigned(); !errors.Is(err, ErrFormat) {
			t.Fatal("truncated or overflowing varint accepted", raw, err)
		}
	}
	for _, tc := range []struct {
		kind byte
		in   int64
		bad  bool
	}{
		{4, math.MinInt16, false}, {4, math.MaxInt16, false}, {4, math.MaxInt16 + 1, true}, {4, math.MinInt16 - 1, true},
		{5, math.MinInt32, false}, {5, math.MaxInt32, false}, {5, math.MaxInt32 + 1, true}, {5, math.MinInt32 - 1, true},
		{6, math.MinInt64, false}, {6, math.MaxInt64, false},
	} {
		r := compactReader{data: binary.AppendVarint(nil, tc.in)}
		value, err := r.value(tc.kind, 0, false)
		if tc.bad {
			if !errors.Is(err, ErrFormat) {
				t.Fatal("narrow integer overflow accepted", tc, err)
			}
		} else if err != nil || value.n != tc.in {
			t.Fatal("signed integer boundary changed", tc, value.n, err)
		}
	}
	for _, tc := range []struct {
		name    string
		kind    byte
		raw     []byte
		depth   int
		nodes   int
		element bool
		want    error
	}{
		{"truncated-byte", 3, nil, 0, 0, false, ErrFormat},
		{"truncated-integer", 5, []byte{128}, 0, 0, false, ErrFormat},
		{"truncated-double", 7, make([]byte, 7), 0, 0, false, ErrFormat},
		{"double", 7, make([]byte, 8), 0, 0, false, nil},
		{"true-element", 1, []byte{1}, 0, 0, true, nil},
		{"false-element", 1, []byte{2}, 0, 0, true, nil},
		{"bad-boolean", 1, []byte{3}, 0, 0, true, ErrFormat},
		{"truncated-boolean", 2, nil, 0, 0, true, ErrFormat},
		{"truncated-string", 8, []byte{4, 'a'}, 0, 0, false, ErrFormat},
		{"huge-string", 8, binary.AppendUvarint(nil, 65537), 0, 0, false, ErrLimit},
		{"string-boundary", 8, append(binary.AppendUvarint(nil, 65536), make([]byte, 65536)...), 0, 0, false, nil},
		{"boolean-list", 9, []byte{0x21, 1, 2}, 0, 0, false, nil},
		{"truncated-list", 9, nil, 0, 0, false, ErrFormat},
		{"truncated-list-length", 9, []byte{0xf3, 128}, 0, 0, false, ErrFormat},
		{"huge-list", 9, append([]byte{0xf3}, binary.AppendUvarint(nil, 8193)...), 0, 0, false, ErrLimit},
		{"node-budget", 3, []byte{0}, 0, 65536, false, ErrLimit},
		{"depth-budget", 12, []byte{0}, 17, 0, false, ErrLimit},
		{"unsupported-map", 11, nil, 0, 0, false, ErrFormat},
		{"duplicate-field", 12, []byte{0x15, 0, 0x05, 2, 0, 0}, 0, 0, false, ErrFormat},
		{"zero-field-id", 12, []byte{0x05, 0, 0, 0}, 0, 0, false, ErrFormat},
		{"truncated-field-id", 12, []byte{0x05, 128}, 0, 0, false, ErrFormat},
		{"truncated-struct", 12, []byte{0x15, 0}, 0, 0, false, ErrFormat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := compactReader{data: tc.raw, nodes: tc.nodes}
			_, err := r.value(tc.kind, tc.depth, tc.element)
			if !errors.Is(err, tc.want) {
				t.Fatal("compact boundary returned wrong result", err, tc.want)
			}
		})
	}
}

func TestParquetChecksumsPreserveSignedThriftBits(t *testing.T) {
	// IEEE CRC32("123456789") is 0xcbf43926, represented as this negative i32.
	body := []byte("123456789")
	good := compactValue{fields: map[int]compactValue{4: {kind: 5, n: -873187034}}}
	if err := parquetChecksum(good, body); err != nil {
		t.Fatal("signed checksum rejected", err)
	}
	if err := parquetChecksum(compactValue{}, body); err != nil {
		t.Fatal("optional checksum became required", err)
	}
	if err := parquetChecksum(compactValue{fields: map[int]compactValue{4: {kind: 5, n: 0}}}, nil); err != nil {
		t.Fatal("zero checksum rejected", err)
	}
	for _, value := range []compactValue{{kind: 5, n: 0}, {kind: 6, n: -873187034}, {kind: 8}, {kind: 5, n: math.MinInt32 - 1}, {kind: 5, n: math.MaxInt32 + 1}} {
		if err := parquetChecksum(compactValue{fields: map[int]compactValue{4: value}}, body); !errors.Is(err, ErrFormat) {
			t.Fatal("corrupt or incorrectly typed checksum accepted", value, err)
		}
	}
	if err := parquetChecksum(good, []byte("123456788")); !errors.Is(err, ErrFormat) {
		t.Fatal("corrupted page accepted", err)
	}
}

func TestParquetPlainAndRLEBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		col  parquetColumn
		n    int
		max  int
		want error
	}{
		{"boolean", []byte{5}, parquetColumn{physical: 0}, 3, 16, nil},
		{"boolean-short", nil, parquetColumn{physical: 0}, 3, 16, ErrFormat},
		{"int32", make([]byte, 8), parquetColumn{physical: 1}, 2, 16, nil},
		{"double", make([]byte, 16), parquetColumn{physical: 5}, 2, 16, nil},
		{"fixed", make([]byte, 6), parquetColumn{physical: 7, width: 3}, 2, 16, nil},
		{"short-fixed", make([]byte, 5), parquetColumn{physical: 7, width: 3}, 2, 16, ErrFormat},
		{"bytes", []byte{2, 0, 0, 0, 'a', 'b'}, parquetColumn{physical: 6}, 1, 2, nil},
		{"short-length", []byte{2, 0}, parquetColumn{physical: 6}, 1, 2, ErrFormat},
		{"oversized-cell", []byte{2, 0, 0, 0, 'a', 'b'}, parquetColumn{physical: 6}, 1, 1, ErrLimit},
		{"truncated-cell", []byte{2, 0, 0, 0, 'a'}, parquetColumn{physical: 6}, 1, 2, ErrLimit},
		{"trailing-data", []byte{0}, parquetColumn{physical: 6}, 0, 2, ErrFormat},
		{"negative-count", nil, parquetColumn{physical: 1}, -1, 2, ErrLimit},
		{"huge-count", nil, parquetColumn{physical: 1}, 1000001, 2, ErrLimit},
		{"unsupported", nil, parquetColumn{physical: 3}, 0, 2, ErrFormat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := guardPlain(tc.data, tc.col, tc.n, tc.max); !errors.Is(err, tc.want) {
				t.Fatal("plain encoding boundary", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name                  string
		data                  []byte
		width, count, ceiling int
		want                  int64
		bad                   bool
	}{
		{"repeat", []byte{6, 1}, 1, 3, 2, 3, false},
		{"packed-tail", []byte{3, 5}, 1, 3, 2, 2, false},
		{"zero-width", []byte{6}, 0, 3, 1, 0, false},
		{"invalid-width", nil, 33, 0, 2, 0, true},
		{"negative-count", nil, 1, -1, 2, 0, true},
		{"invalid-run", []byte{0}, 1, 3, 2, 0, true},
		{"truncated-run", []byte{128}, 1, 3, 2, 0, true},
		{"excess-repeat", []byte{8, 1}, 1, 3, 2, 0, true},
		{"truncated-value", []byte{6}, 1, 3, 2, 0, true},
		{"out-of-dictionary", []byte{6, 2}, 2, 3, 2, 0, true},
		{"out-of-width", []byte{6, 2}, 1, 3, 3, 0, true},
		{"insufficient-values", []byte{2, 1}, 1, 3, 2, 0, true},
		{"packed-too-long", []byte{5, 0, 0}, 1, 3, 2, 0, true},
		{"packed-short", []byte{3}, 1, 3, 2, 0, true},
		{"packed-bad-index", []byte{3, 1}, 1, 1, 1, 0, true},
		{"padding-not-final", []byte{3, 0, 2, 0}, 1, 3, 2, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := guardRLE(tc.data, tc.width, tc.count, tc.ceiling, true)
			if tc.bad {
				if !errors.Is(err, ErrFormat) {
					t.Fatal("invalid RLE accepted", got, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatal("RLE changed decoded counts", got, err)
			}
		})
	}
}

func TestParquetBoundedDecompression(t *testing.T) {
	plain := []byte("bounded synthetic payload")
	var gz bytes.Buffer
	writer := gzip.NewWriter(&gz)
	if _, err := writer.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll(plain, nil)
	if err = encoder.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		codec int
		raw   []byte
	}{{0, plain}, {1, snappy.Encode(nil, plain)}, {2, gz.Bytes()}, {6, compressed}} {
		got, err := boundedDecompress(tc.codec, tc.raw, len(plain))
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatal("qualified codec changed payload", tc.codec, err)
		}
		for _, size := range []int{len(plain) - 1, len(plain) + 1} {
			if _, err = boundedDecompress(tc.codec, tc.raw, size); !errors.Is(err, ErrFormat) {
				t.Fatal("wrong decoded-size declaration accepted", tc.codec, size, err)
			}
		}
		if _, err = boundedDecompress(tc.codec, tc.raw[:len(tc.raw)-1], len(plain)); !errors.Is(err, ErrFormat) {
			t.Fatal("truncated compressed payload accepted", tc.codec, err)
		}
	}
	for _, size := range []int{-1, (8 << 20) + 1} {
		if _, err = boundedDecompress(0, nil, size); !errors.Is(err, ErrLimit) {
			t.Fatal("decoded allocation bound ignored", err)
		}
	}
	if _, err = boundedDecompress(99, nil, 0); !errors.Is(err, ErrFormat) {
		t.Fatal("unqualified codec accepted", err)
	}
}

func TestUploadZIP64SizeDeclarationCannotOverflow(t *testing.T) {
	var raw bytes.Buffer
	z := zip.NewWriter(&raw)
	_, err := z.CreateRaw(&zip.FileHeader{Name: "oversized.xml", Method: zip.Store, CompressedSize64: math.MaxUint64, UncompressedSize64: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	data := raw.Bytes()
	spec := parseSpec("xlsx", data, UploadColumn{Name: "id", Type: "integer"})
	emitted := false
	_, err = Parse(context.Background(), data, spec, config.DefaultUploads(), func([]Cell) error {
		emitted = true
		return nil
	})
	if !errors.Is(err, ErrFormat) || emitted {
		t.Fatal("overflowing ZIP64 metadata reached the writer", err, emitted)
	}
}
