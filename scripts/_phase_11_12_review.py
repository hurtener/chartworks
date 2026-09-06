#!/usr/bin/env python3
"""Apply only inspected recovery deltas; read-only CI verifies the resulting commit."""
from pathlib import Path
import subprocess


def replace(path: str, before: str, after: str) -> None:
    file = Path(path)
    text = file.read_text()
    if text.count(before) != 1:
        raise RuntimeError(f"{path}: expected exactly one reviewed anchor: {before!r}")
    file.write_text(text.replace(before, after))


# CopyFrom EOF was corrected previously, but the same file still uses io in the
# bounded wire frontend. Restore the import without changing COPY completion.
replace('internal/engineering/workspace.go', '\t"errors"\n', '\t"errors"\n\t"io"\n')

# The workspace destination is float64. A shortest float32 decimal does NOT
# preserve the original binary value when it is subsequently parsed as float64.
replace('internal/engineering/parse_parquet.go',
        "strconv.FormatFloat(float64(v.Float()), 'g', -1, 32)",
        "strconv.FormatFloat(float64(v.Float()), 'g', -1, 64)")

# PostgreSQL 17 stores timestamps at microsecond resolution and has no year zero.
# Reject unrepresentable data rather than silently round/truncate declared values.
replace('internal/engineering/parse.go',
        'if e != nil || v.Format("2006-01-02") != s {',
        'if e != nil || v.Year() < 1 || v.Format("2006-01-02") != s {')
replace('internal/engineering/parse.go',
        '''v, e := time.Parse(time.RFC3339Nano, s)
		if e != nil {
			return Cell{}, ErrFormat
		}
		s = v.UTC().Format(time.RFC3339Nano)''',
        '''v, e := time.Parse(time.RFC3339Nano, s)
		if e != nil || v.Nanosecond()%1000 != 0 || v.UTC().Year() < 1 || v.UTC().Year() > 9999 {
			return Cell{}, ErrFormat
		}
		s = v.UTC().Format(time.RFC3339Nano)''')

files = {
'internal/engineering/upload_precision_test.go': r'''package engineering

import (
 "errors"
 "math"
 "testing"
 "time"

 "github.com/parquet-go/parquet-go"
)

func TestParquetFloat32WorkspacePromotionIsLossless(t *testing.T) {
 for _, value := range []float32{0, float32(math.Copysign(0,-1)), 0.1, -0.1, math.SmallestNonzeroFloat32, math.MaxFloat32} {
  cell, err := parquetCell(parquet.FloatValue(value), parquetColumn{physical:4}, UploadColumn{Type:"number"})
  if err != nil { t.Fatal(err) }
  cell, err = normalizeCell(UploadColumn{Type:"number"}, cell)
  if err != nil { t.Fatal(err) }
  out, err := copyCell(UploadColumn{Type:"number"},cell)
  if err != nil { t.Fatal(err) }
  if got,ok:=out.(float64); !ok || math.Float64bits(got)!=math.Float64bits(float64(value)) {
   t.Fatalf("float32 promotion lost its exact value: %v -> %q -> %v",value,cell.Text,out)
  }
 }
 for _, value := range []float32{float32(math.NaN()),float32(math.Inf(1)),float32(math.Inf(-1))} {
  cell, err := parquetCell(parquet.FloatValue(value),parquetColumn{physical:4},UploadColumn{Type:"number"})
  if err != nil { t.Fatal(err) }
  if _,err=normalizeCell(UploadColumn{Type:"number"},cell); !errors.Is(err,ErrFormat) {t.Fatal("nonfinite value accepted",err)}
 }
}

func TestUploadTimestampCannotSilentlyLosePrecision(t *testing.T) {
 column:=UploadColumn{Type:"timestamp"}
 for _, input:=range []string{"2026-01-02T03:04:05Z","2026-01-02T06:04:05.123456+03:00","2026-01-02T03:04:05.123456000Z","0001-01-01T00:00:00Z"} {
  cell,err:=normalizeCell(column,Cell{Text:input})
  if err!=nil {t.Fatalf("representable timestamp rejected %q: %v",input,err)}
  copied,err:=copyCell(column,cell)
  if err!=nil {t.Fatal(err)}
  stamp,ok:=copied.(time.Time)
  expected,err:=time.Parse(time.RFC3339Nano,input)
  if !ok || err!=nil || !stamp.Equal(expected) || stamp.Nanosecond()%1000!=0 {t.Fatal("timestamp changed",input,copied,err)}
 }
 for _, input:=range []string{"2026-01-02T03:04:05.000000001Z","2026-01-02T03:04:05.123456789Z","0000-01-01T00:00:00Z","0001-01-01T00:00:00+01:00","9999-12-31T23:59:59-01:00"} {
  if _,err:=normalizeCell(column,Cell{Text:input}); !errors.Is(err,ErrFormat) {t.Fatalf("unrepresentable timestamp accepted %q: %v",input,err)}
 }
 if _,err:=normalizeCell(UploadColumn{Type:"date"},Cell{Text:"0000-01-01"}); !errors.Is(err,ErrFormat) {t.Fatal("year zero accepted",err)}
}
'''
}
for name, content in files.items():
    path=Path(name)
    if path.exists():
        raise RuntimeError(f'refusing to overwrite {name}')
    path.write_text(content)
subprocess.run(['git','add','--',*files],check=True)
print('Applied missing io import, exact float promotion and representable timestamp checks with regression tests.')
