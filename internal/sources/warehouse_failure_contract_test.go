package sources

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/bruin-data/bruin/pkg/query"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestCloudRowCollectionFailsClosed(t *testing.T) {
	rowFailure := errors.New("synthetic row failure")
	observerFailure := errors.New("synthetic observer failure")
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	validColumn := query.Column{Name: "value", DatabaseType: "VARCHAR"}
	limits := readexec.Limits{Rows: 10, Bytes: 4096}

	tests := []struct {
		name     string
		ctx      context.Context
		rows     *failureContractRows
		limits   readexec.Limits
		observer readexec.Observer
		want     error
	}{
		{
			name:   "unsupported schema",
			ctx:    t.Context(),
			rows:   &failureContractRows{columns: []query.Column{{Name: "value", DatabaseType: "OBJECT"}}},
			limits: limits,
			want:   readexec.ErrUnsupported,
		},
		{
			name:   "invalid collection limit",
			ctx:    t.Context(),
			rows:   &failureContractRows{columns: []query.Column{validColumn}},
			limits: readexec.Limits{Rows: 0, Bytes: 4096},
			want:   readexec.ErrLimit,
		},
		{
			name:   "cancelled before first value",
			ctx:    cancelled,
			rows:   &failureContractRows{columns: []query.Column{validColumn}, rows: [][]any{{"unreturned"}}},
			limits: limits,
			want:   context.Canceled,
		},
		{
			name:     "observer rejects before first value",
			ctx:      t.Context(),
			rows:     &failureContractRows{columns: []query.Column{validColumn}, rows: [][]any{{"unreturned"}}},
			limits:   limits,
			observer: failureContractObserver{err: observerFailure},
			want:     observerFailure,
		},
		{
			name:   "row retrieval fails",
			ctx:    t.Context(),
			rows:   &failureContractRows{columns: []query.Column{validColumn}, rows: [][]any{{"unreturned"}}, valueErr: rowFailure},
			limits: limits,
			want:   rowFailure,
		},
		{
			name:   "provider value is not qualified",
			ctx:    t.Context(),
			rows:   &failureContractRows{columns: []query.Column{validColumn}, rows: [][]any{{struct{}{}}}},
			limits: limits,
			want:   readexec.ErrUnsupported,
		},
		{
			name: "row is shorter than schema",
			ctx:  t.Context(),
			rows: &failureContractRows{
				columns: []query.Column{validColumn, {Name: "other", DatabaseType: "VARCHAR"}},
				rows:    [][]any{{"only one value"}},
			},
			limits: limits,
			want:   readexec.ErrType,
		},
		{
			name:   "row is longer than schema",
			ctx:    t.Context(),
			rows:   &failureContractRows{columns: []query.Column{validColumn}, rows: [][]any{{"first", "unexpected"}}},
			limits: limits,
			want:   readexec.ErrType,
		},
		{
			name:   "nonfinite number",
			ctx:    t.Context(),
			rows:   &failureContractRows{columns: []query.Column{{Name: "value", DatabaseType: "FLOAT64"}}, rows: [][]any{{math.NaN()}}},
			limits: limits,
			want:   readexec.ErrType,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := collectCloudRows(test.ctx, test.rows, test.limits, test.observer)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
			if len(result.Schema) != 0 || len(result.Rows) != 0 || result.Outcome != "" || result.Bytes != 0 {
				t.Fatalf("failure exposed a partial result: %+v", result)
			}
		})
	}
}

func TestCloudRowCollectionReportsBoundedPrefix(t *testing.T) {
	rows := &failureContractRows{
		columns: []query.Column{{Name: "value", DatabaseType: "VARCHAR"}},
		rows:    [][]any{{"retained"}, {"lookahead"}},
	}
	result, err := collectCloudRows(t.Context(), rows, readexec.Limits{Rows: 1, Bytes: 4096}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || string(result.Rows[0][0]) != `"retained"` || result.Outcome != "truncated" || result.Truncation != "rows" {
		t.Fatalf("bounded prefix lost its truncation evidence: %+v", result)
	}
}

func TestSQLServerMetadataReadsFailClosed(t *testing.T) {
	rowFailure := errors.New("synthetic SQL Server row failure")
	streamFailure := errors.New("synthetic SQL Server stream failure")
	tests := []struct {
		name string
		rows *failureContractRows
		want error
	}{
		{name: "row limit", rows: &failureContractRows{rows: [][]any{{"first"}, {"lookahead"}}}, want: readexec.ErrLimit},
		{name: "row failure", rows: &failureContractRows{rows: [][]any{{"unreturned"}}, valueErr: rowFailure}, want: rowFailure},
		{name: "byte limit", rows: &failureContractRows{rows: [][]any{{strings.Repeat("x", (1<<20)+1)}}}, want: readexec.ErrLimit},
		{name: "stream failure", rows: &failureContractRows{streamErr: streamFailure}, want: streamFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := sqlServerRows(t.Context(), failureContractSession{rows: test.rows}, "SELECT synthetic metadata", nil, 1)
			if !errors.Is(err, test.want) || result != nil || !test.rows.closed {
				t.Fatalf("result=%v err=%v closed=%v, want nil/%v/true", result, err, test.rows.closed, test.want)
			}
		})
	}
}

func TestSQLServerColumnMetadataRejectsAmbiguousRows(t *testing.T) {
	valid := []any{"value", "bigint", int64(8), int64(19), int64(0), false, int64(1), false, int64(0), nil, false, false}
	clone := func() []any { return append([]any(nil), valid...) }
	tests := []struct {
		name     string
		rows     [][]any
		declared []string
		want     error
	}{
		{name: "missing declared row", declared: []string{"value"}, want: readexec.ErrBinding},
		{name: "unsupported native type", rows: func() [][]any { row := clone(); row[1] = "geography"; return [][]any{row} }(), declared: []string{"value"}, want: readexec.ErrUnsupported},
		{name: "computed column", rows: func() [][]any { row := clone(); row[7] = true; return [][]any{row} }(), declared: []string{"value"}, want: readexec.ErrUnsupported},
		{name: "encrypted column", rows: func() [][]any { row := clone(); row[9] = "encryption-key"; return [][]any{row} }(), declared: []string{"value"}, want: readexec.ErrUnsupported},
		{name: "malformed nullable flag", rows: func() [][]any { row := clone(); row[5] = struct{}{}; return [][]any{row} }(), declared: []string{"value"}, want: readexec.ErrBinding},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			columns, err := sqlServerColumns(test.rows, test.declared)
			if !errors.Is(err, test.want) || columns != nil {
				t.Fatalf("columns=%v err=%v, want nil/%v", columns, err, test.want)
			}
		})
	}

	temporal := clone()
	temporal[1] = []byte("time")
	temporal[4] = int32(7)
	columns, err := sqlServerColumns([][]any{temporal}, []string{"value"})
	if err != nil || len(columns) != 1 || columns[0].NativeType != "time(7)" || columns[0].Category != "temporal" {
		t.Fatalf("precise temporal metadata changed: %+v %v", columns, err)
	}

	fields, err := sqlServerResultFields([]query.Column{{Name: "ratio", DatabaseType: "FLOAT"}})
	if err != nil || len(fields) != 1 || fields[0].Type != "number" || fields[0].Encoding != "number" {
		t.Fatalf("supported numeric result metadata changed: %+v %v", fields, err)
	}
	if fields, err = sqlServerResultFields([]query.Column{{Name: "document", DatabaseType: "XML"}}); !errors.Is(err, readexec.ErrType) || fields != nil {
		t.Fatalf("unsupported result metadata was accepted: %+v %v", fields, err)
	}
}

type failureContractRows struct {
	columns   []query.Column
	rows      [][]any
	index     int
	current   []any
	valueErr  error
	streamErr error
	closed    bool
}

func (r *failureContractRows) Columns() []query.Column {
	return append([]query.Column(nil), r.columns...)
}
func (r *failureContractRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.current = append([]any(nil), r.rows[r.index]...)
	r.index++
	return true
}
func (r *failureContractRows) Values() ([]any, error) {
	if r.valueErr != nil {
		return nil, r.valueErr
	}
	return append([]any(nil), r.current...), nil
}
func (r *failureContractRows) Err() error   { return r.streamErr }
func (r *failureContractRows) Close() error { r.closed = true; return nil }

type failureContractObserver struct{ err error }

func (failureContractObserver) Dispatch(context.Context, readexec.RemoteQuery, bool) error {
	return nil
}
func (o failureContractObserver) Check(context.Context) error { return o.err }

type failureContractSession struct {
	rows query.RowStream
	err  error
}

func (s failureContractSession) Query(context.Context, *query.Query) (query.RowStream, error) {
	return s.rows, s.err
}
