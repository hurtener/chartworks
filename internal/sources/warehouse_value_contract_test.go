package sources

import (
	"context"
	"errors"
	"math"
	"math/big"
	"strconv"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	bruinbigquery "github.com/bruin-data/bruin/pkg/bigquery"
	bruindatabricks "github.com/bruin-data/bruin/pkg/databricks"
	"github.com/bruin-data/bruin/pkg/query"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
)

func TestWarehouseScalarArgumentContracts(t *testing.T) {
	parameters := []readexec.Parameter{
		{Kind: "null"},
		{Kind: "text", Value: "mañana"},
		{Kind: "boolean", Value: "true"},
		{Kind: "integer", Value: "9007199254740993"},
		{Kind: "number", Value: "9007199254740993.125"},
	}

	t.Run("bigquery", func(t *testing.T) {
		arguments, err := bigQueryArguments(parameters)
		if err != nil || len(arguments) != len(parameters) {
			t.Fatal("BigQuery rejected the common scalar contract", err)
		}
		wantTypes := []string{"STRING", "STRING", "BOOL", "INT64", "BIGNUMERIC"}
		for i, argument := range arguments {
			parameter, ok := argument.(bruinbigquery.ReadParameter)
			if !ok || parameter.Name != "p"+strconv.Itoa(i+1) || parameter.Type != wantTypes[i] {
				t.Fatalf("BigQuery parameter %d lost name or type: %#v", i, argument)
			}
		}
		if arguments[0].(bruinbigquery.ReadParameter).Value != nil || arguments[1].(bruinbigquery.ReadParameter).Value != "mañana" || arguments[2].(bruinbigquery.ReadParameter).Value != true || arguments[3].(bruinbigquery.ReadParameter).Value != int64(9007199254740993) {
			t.Fatal("BigQuery changed a scalar value")
		}
		decimal := arguments[4].(bruinbigquery.ReadParameter).Value.(*big.Rat)
		if decimal.RatString() != "72057594037927945/8" {
			t.Fatalf("BigQuery changed the exact decimal: %s", decimal)
		}
	})

	t.Run("databricks", func(t *testing.T) {
		arguments, err := databricksArguments(parameters)
		if err != nil || len(arguments) != len(parameters) {
			t.Fatal("Databricks rejected the common scalar contract", err)
		}
		wantTypes := []string{"STRING", "STRING", "BOOLEAN", "BIGINT", "DECIMAL(38,18)"}
		for i, argument := range arguments {
			parameter, ok := argument.(bruindatabricks.ReadParameter)
			if !ok || parameter.Name != "p"+strconv.Itoa(i+1) || parameter.Type != wantTypes[i] {
				t.Fatalf("Databricks parameter %d lost name or type: %#v", i, argument)
			}
		}
		if arguments[0].(bruindatabricks.ReadParameter).Value != nil || arguments[1].(bruindatabricks.ReadParameter).Value != "mañana" || arguments[2].(bruindatabricks.ReadParameter).Value != true || arguments[3].(bruindatabricks.ReadParameter).Value != int64(9007199254740993) {
			t.Fatal("Databricks changed a scalar value")
		}
		decimal := arguments[4].(bruindatabricks.ReadParameter).Value.(*big.Rat)
		if decimal.RatString() != "72057594037927945/8" {
			t.Fatalf("Databricks changed the exact decimal: %s", decimal)
		}
	})

	t.Run("snowflake", func(t *testing.T) {
		accepted := append([]readexec.Parameter(nil), parameters...)
		accepted[4].Value = "9007199254740993125"
		arguments, err := snowflakeArguments(accepted)
		if err != nil || len(arguments) != len(accepted) || arguments[0] != nil || arguments[1] != "mañana" || arguments[2] != true || arguments[3] != int64(9007199254740993) {
			t.Fatal("Snowflake changed the common scalar contract", err, arguments)
		}
		integer, ok := arguments[4].(*big.Int)
		if !ok || integer.String() != "9007199254740993125" {
			t.Fatalf("Snowflake changed the exact large number: %#v", arguments[4])
		}
	})

	t.Run("mysql", func(t *testing.T) {
		arguments, err := mysqlArguments(parameters)
		if err != nil || len(arguments) != len(parameters) || arguments[0] != nil || arguments[1] != "mañana" || arguments[2] != true || arguments[3] != int64(9007199254740993) || arguments[4] != "9007199254740993.125" {
			t.Fatal("MySQL changed the common scalar contract", err, arguments)
		}
	})

	t.Run("sqlserver", func(t *testing.T) {
		accepted := append([]readexec.Parameter(nil), parameters[:4]...)
		accepted = append(accepted, readexec.Parameter{Kind: "boolean", Value: "false"})
		arguments, err := sqlServerArguments(accepted)
		if err != nil || len(arguments) != len(accepted) || arguments[0] != nil || arguments[1] != "mañana" || arguments[2] != true || arguments[3] != int64(9007199254740993) || arguments[4] != false {
			t.Fatal("SQL Server changed the supported scalar contract", err, arguments)
		}
	})

	rejections := []struct {
		name string
		call func() error
		want error
	}{
		{name: "bigquery integer overflow", call: func() error {
			_, err := bigQueryArguments([]readexec.Parameter{{Kind: "integer", Value: "9223372036854775808"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "bigquery malformed decimal", call: func() error {
			_, err := bigQueryArguments([]readexec.Parameter{{Kind: "number", Value: "1.2.3"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "bigquery unsupported kind", call: func() error {
			_, err := bigQueryArguments([]readexec.Parameter{{Kind: "binary", Value: "00"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "databricks integer overflow", call: func() error {
			_, err := databricksArguments([]readexec.Parameter{{Kind: "integer", Value: "9223372036854775808"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "databricks malformed decimal", call: func() error {
			_, err := databricksArguments([]readexec.Parameter{{Kind: "number", Value: "1.2.3"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "databricks unsupported kind", call: func() error {
			_, err := databricksArguments([]readexec.Parameter{{Kind: "binary", Value: "00"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "snowflake integer overflow", call: func() error {
			_, err := snowflakeArguments([]readexec.Parameter{{Kind: "integer", Value: "9223372036854775808"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "snowflake fractional number", call: func() error {
			_, err := snowflakeArguments([]readexec.Parameter{{Kind: "number", Value: "1.25"}})
			return err
		}, want: readexec.ErrUnsupported},
		{name: "snowflake unsupported kind", call: func() error {
			_, err := snowflakeArguments([]readexec.Parameter{{Kind: "binary", Value: "00"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "mysql integer overflow", call: func() error {
			_, err := mysqlArguments([]readexec.Parameter{{Kind: "integer", Value: "9223372036854775808"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "mysql unsupported kind", call: func() error {
			_, err := mysqlArguments([]readexec.Parameter{{Kind: "binary", Value: "00"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "sqlserver integer overflow", call: func() error {
			_, err := sqlServerArguments([]readexec.Parameter{{Kind: "integer", Value: "9223372036854775808"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "sqlserver malformed boolean", call: func() error {
			_, err := sqlServerArguments([]readexec.Parameter{{Kind: "boolean", Value: "1"}})
			return err
		}, want: readexec.ErrBinding},
		{name: "sqlserver decimal unsupported", call: func() error {
			_, err := sqlServerArguments([]readexec.Parameter{{Kind: "number", Value: "1.25"}})
			return err
		}, want: readexec.ErrUnsupported},
		{name: "sqlserver unsupported kind", call: func() error {
			_, err := sqlServerArguments([]readexec.Parameter{{Kind: "binary", Value: "00"}})
			return err
		}, want: readexec.ErrBinding},
	}
	for _, rejection := range rejections {
		t.Run(rejection.name, func(t *testing.T) {
			if err := rejection.call(); !errors.Is(err, rejection.want) {
				t.Fatalf("got %v, want %v", err, rejection.want)
			}
		})
	}
}

func TestWarehouseResultValueContracts(t *testing.T) {
	date := civil.Date{Year: 2026, Month: time.September, Day: 7}
	clock := civil.Time{Hour: 12, Minute: 13, Second: 14, Nanosecond: 123456789}
	dateTime := civil.DateTime{Date: date, Time: clock}
	instant := time.Date(2026, time.September, 7, 12, 13, 14, 123456789, time.FixedZone("synthetic", -3*60*60))
	decimal := big.NewRat(1, 8)
	decimalFloat, _, err := big.ParseFloat("123.45", 10, 128, big.ToNearestEven)
	if err != nil {
		t.Fatal(err)
	}
	large := new(big.Int)
	large.SetString("9007199254740993125", 10)
	cloudCases := []struct {
		name   string
		column query.Column
		value  any
		want   string
		nil    bool
	}{
		{name: "database null", value: nil, nil: true},
		{name: "text", value: "mañana", want: "mañana"},
		{name: "binary", value: []byte{0, 255}, want: `\x00ff`},
		{name: "boolean true", value: true, want: "t"},
		{name: "boolean false", value: false, want: "f"},
		{name: "int64", value: int64(9007199254740993), want: "9007199254740993"},
		{name: "int32", value: int32(-2147483648), want: "-2147483648"},
		{name: "float64", value: float64(0.125), want: "0.125"},
		{name: "float32", value: float32(0.125), want: "0.125"},
		{name: "instant", value: instant, want: "2026-09-07T15:13:14.123456789Z"},
		{name: "date", value: date, want: "2026-09-07"},
		{name: "time", value: clock, want: "12:13:14.123456789"},
		{name: "datetime", value: dateTime, want: "2026-09-07T12:13:14.123456789"},
		{name: "scaled rational", column: query.Column{DecimalKnown: true, Scale: 3}, value: decimal, want: "0.125"},
		{name: "unscaled rational", value: decimal, want: "1/8"},
		{name: "nil rational", value: (*big.Rat)(nil), nil: true},
		{name: "large integer pointer", value: large, want: "9007199254740993125"},
		{name: "large integer value", value: *large, want: "9007199254740993125"},
		{name: "scaled float pointer", column: query.Column{DecimalKnown: true, Scale: 3}, value: decimalFloat, want: "123.450"},
		{name: "scaled float value", column: query.Column{DecimalKnown: true, Scale: 3}, value: *decimalFloat, want: "123.450"},
	}
	for _, test := range cloudCases {
		t.Run("cloud "+test.name, func(t *testing.T) {
			raw, valueErr := cloudValue(readexec.Field{Type: "text", Encoding: "string"}, test.column, test.value)
			if valueErr != nil || test.nil && raw != nil || !test.nil && string(raw) != test.want {
				t.Fatalf("value=%q err=%v, want %q", raw, valueErr, test.want)
			}
		})
	}
	for _, value := range []any{(*big.Int)(nil), (*big.Float)(nil), decimalFloat, *decimalFloat, struct{}{}} {
		if _, err := cloudValue(readexec.Field{Type: "text", Encoding: "string"}, query.Column{}, value); !errors.Is(err, readexec.ErrUnsupported) {
			t.Fatalf("cloud accepted an unqualified value %#v: %v", value, err)
		}
	}

	textField := readexec.Field{Name: "value", Type: "text", Encoding: "string", NativeType: "varchar"}
	binaryField := readexec.Field{Name: "value", Type: "binary", Encoding: "string", NativeType: "varbinary"}
	mysqlCases := []struct {
		name  string
		field readexec.Field
		value any
		want  string
		nil   bool
	}{
		{name: "database null", field: textField, value: nil, nil: true},
		{name: "binary", field: binaryField, value: []byte{0, 255}, want: `\x00ff`},
		{name: "text bytes", field: textField, value: []byte("mañana"), want: "mañana"},
		{name: "text", field: textField, value: "mañana", want: "mañana"},
		{name: "integer", field: textField, value: int64(9007199254740993), want: "9007199254740993"},
		{name: "unsigned boundary", field: textField, value: uint64(math.MaxInt64), want: "9223372036854775807"},
		{name: "float", field: textField, value: float64(0.125), want: "0.125"},
		{name: "boolean true", field: textField, value: true, want: "t"},
		{name: "boolean false", field: textField, value: false, want: "f"},
		{name: "timestamp", field: textField, value: instant, want: "2026-09-07 12:13:14.123456789"},
	}
	for _, test := range mysqlCases {
		t.Run("mysql "+test.name, func(t *testing.T) {
			raw, valueErr := mysqlResultValue(test.field, test.value)
			if valueErr != nil || test.nil && raw != nil || !test.nil && string(raw) != test.want {
				t.Fatalf("value=%q err=%v, want %q", raw, valueErr, test.want)
			}
		})
	}
	for _, test := range []struct {
		field readexec.Field
		value any
	}{
		{field: binaryField, value: "not-bytes"},
		{field: textField, value: uint64(math.MaxInt64) + 1},
		{field: textField, value: struct{}{}},
	} {
		if _, err := mysqlResultValue(test.field, test.value); !errors.Is(err, readexec.ErrType) {
			t.Fatalf("MySQL accepted an unqualified value %#v: %v", test.value, err)
		}
	}

	columns := []query.Column{
		{Name: "null", DatabaseType: "nvarchar"},
		{Name: "binary", DatabaseType: "varbinary"},
		{Name: "text_bytes", DatabaseType: "nvarchar"},
		{Name: "text", DatabaseType: "nvarchar"},
		{Name: "integer", DatabaseType: "bigint"},
		{Name: "true", DatabaseType: "bit"},
		{Name: "false", DatabaseType: "bit"},
		{Name: "float", DatabaseType: "float"},
		{Name: "real", DatabaseType: "real"},
		{Name: "date", DatabaseType: "date"},
		{Name: "time", DatabaseType: "time"},
		{Name: "datetime", DatabaseType: "datetime2"},
		{Name: "offset", DatabaseType: "datetimeoffset"},
	}
	values := []any{nil, []byte{0, 255}, []byte("mañana"), "mañana", int64(9007199254740993), true, false, float64(0.125), float64(0.125), instant, instant, instant, instant}
	want := []string{"", `\x00ff`, "mañana", "mañana", "9007199254740993", "t", "f", "0.125", "0.125", "2026-09-07", "12:13:14.1234567", "2026-09-07T12:13:14.1234567", "2026-09-07T12:13:14.123456789-03:00"}
	encoded, err := sqlServerResultValues(columns, values)
	if err != nil {
		t.Fatal("SQL Server rejected exact result values", err)
	}
	for i := range want {
		if i == 0 && encoded[i] != nil || i > 0 && string(encoded[i]) != want[i] {
			t.Fatalf("SQL Server value %d=%q, want %q", i, encoded[i], want[i])
		}
	}
	if _, err := sqlServerResultValues(columns, values[:1]); !errors.Is(err, readexec.ErrType) {
		t.Fatal("SQL Server accepted a row with the wrong width", err)
	}
	for _, value := range []any{math.NaN(), math.Inf(1), struct{}{}} {
		if _, err := sqlServerResultValues([]query.Column{{Name: "value", DatabaseType: "float"}}, []any{value}); !errors.Is(err, readexec.ErrType) {
			t.Fatalf("SQL Server accepted an unqualified value %#v: %v", value, err)
		}
	}
}

func TestWarehouseCategoryAndMetadataContracts(t *testing.T) {
	classifiers := []struct {
		name string
		fn   func(string) string
		in   map[string]string
	}{
		{name: "bigquery", fn: bigQueryCategory, in: map[string]string{"INT64": "integer", "BIGNUMERIC": "decimal", "FLOAT64": "number", "BOOL": "boolean", "BYTES": "binary", "TIMESTAMP": "temporal", "JSON": "structured", "GEOGRAPHY": "text", "INTERVAL": ""}},
		{name: "databricks", fn: databricksCategory, in: map[string]string{"TINYINT": "integer", "DECIMAL(38,3)": "decimal", "DOUBLE": "number", "BOOLEAN": "boolean", "TIMESTAMP_NTZ": "temporal", "STRING": "text", "BINARY": ""}},
		{name: "snowflake", fn: snowflakeCategory, in: map[string]string{"NUMBER(38,0)": "decimal", "REAL": "number", "BOOLEAN": "boolean", "BINARY(16)": "binary", "TIMESTAMP_TZ": "temporal", "VARIANT": "structured", "VARCHAR(128)": "text", "OBJECT": ""}},
		{name: "mysql", fn: mysqlCategory, in: map[string]string{"BIGINT": "integer", "DECIMAL": "decimal", "DOUBLE": "number", "VARCHAR": "text", "VARBINARY": "binary", "TIMESTAMP": "temporal", "JSON": "structured", "GEOMETRY": ""}},
		{name: "sqlserver", fn: sqlServerCategory, in: map[string]string{"BIGINT": "integer", "NUMERIC": "decimal", "REAL": "number", "BIT": "boolean", "NVARCHAR": "text", "VARBINARY": "binary", "DATETIMEOFFSET": "temporal", "XML": ""}},
	}
	for _, classifier := range classifiers {
		t.Run(classifier.name, func(t *testing.T) {
			for native, want := range classifier.in {
				if got := classifier.fn(native); got != want {
					t.Fatalf("%s classified as %q, want %q", native, got, want)
				}
			}
		})
	}

	fields := []struct {
		column   query.Column
		wantType string
		encoding string
	}{
		{column: query.Column{Name: "bq", DatabaseType: "INT64"}, wantType: "integer", encoding: "string"},
		{column: query.Column{Name: "sf", DatabaseType: "VARIANT"}, wantType: "structured", encoding: "string"},
		{column: query.Column{Name: "dbx", DatabaseType: "TINYINT"}, wantType: "integer", encoding: "string"},
		{column: query.Column{Name: "bool", DatabaseType: "BOOLEAN"}, wantType: "boolean", encoding: "boolean"},
		{column: query.Column{Name: "float", DatabaseType: "FLOAT64"}, wantType: "number", encoding: "number"},
	}
	for _, test := range fields {
		field, err := cloudField(test.column)
		if err != nil || field.Name != test.column.Name || field.Type != test.wantType || field.Encoding != test.encoding || field.NativeType != test.column.DatabaseType {
			t.Fatalf("cloud field changed: %#v %v", field, err)
		}
	}
	if _, err := cloudField(query.Column{Name: "unsupported", DatabaseType: "OBJECT"}); !errors.Is(err, readexec.ErrUnsupported) {
		t.Fatal("unsupported cloud metadata was accepted", err)
	}

	tableCases := []struct {
		name string
		rows *contractRows
		want string
		err  error
	}{
		{name: "base table", rows: &contractRows{rows: [][]any{{"BASE TABLE"}}}, want: "BASE TABLE"},
		{name: "missing", rows: &contractRows{}, err: readexec.ErrBinding},
		{name: "unsupported", rows: &contractRows{rows: [][]any{{"VIEW"}}}, err: readexec.ErrUnsupported},
		{name: "duplicate", rows: &contractRows{rows: [][]any{{"BASE TABLE"}, {"BASE TABLE"}}}, err: readexec.ErrUnsupported},
		{name: "malformed", rows: &contractRows{rows: [][]any{{"BASE TABLE"}}, valueErr: errors.New("synthetic row failure")}, err: readexec.ErrUnsupported},
		{name: "stream failure", rows: &contractRows{streamErr: errors.New("synthetic stream failure")}, err: store.ErrUnavailable},
	}
	for _, test := range tableCases {
		t.Run("table "+test.name, func(t *testing.T) {
			got, err := catalogTableType(test.rows, map[string]bool{"BASE TABLE": true})
			if !errors.Is(err, test.err) || got != test.want || !test.rows.closed {
				t.Fatalf("type=%q err=%v closed=%v", got, err, test.rows.closed)
			}
		})
	}

	columns, evidence, err := catalogColumns(&contractRows{rows: [][]any{{"id", "INT64", "NO"}, {"payload", "BYTES", "YES"}}}, []string{"id", "payload"}, bigQueryCategory)
	if err != nil || len(columns) != 2 || len(evidence) != 2 || columns[0].Category != "integer" || columns[0].Nullable || columns[1].Category != "binary" || !columns[1].Nullable {
		t.Fatal("ordered native metadata changed", err, columns, evidence)
	}
	metadataRejections := []struct {
		name string
		rows *contractRows
		want error
	}{
		{name: "reordered", rows: &contractRows{rows: [][]any{{"payload", "BYTES", "YES"}}}, want: readexec.ErrUnsupported},
		{name: "unsupported type", rows: &contractRows{rows: [][]any{{"id", "INTERVAL", "NO"}}}, want: readexec.ErrUnsupported},
		{name: "short row", rows: &contractRows{rows: [][]any{{"id", "INT64"}}}, want: readexec.ErrBinding},
		{name: "row error", rows: &contractRows{rows: [][]any{{"id", "INT64", "NO"}}, valueErr: errors.New("synthetic row failure")}, want: readexec.ErrBinding},
		{name: "missing declared column", rows: &contractRows{rows: [][]any{{"id", "INT64", "NO"}}}, want: readexec.ErrBinding},
		{name: "stream error", rows: &contractRows{streamErr: errors.New("synthetic stream failure")}, want: store.ErrUnavailable},
	}
	for _, test := range metadataRejections {
		t.Run("columns "+test.name, func(t *testing.T) {
			declared := []string{"id"}
			if test.name == "missing declared column" {
				declared = []string{"id", "payload"}
			}
			if _, _, err := catalogColumns(test.rows, declared, bigQueryCategory); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestCloudReadOutcomeContract(t *testing.T) {
	if cloudState("running") != "running" || cloudState("stopped") != "stopped" || cloudState("queued") != "unknown" {
		t.Fatal("cloud remote state mapping changed")
	}
	background := context.Background()
	if cloudReadFailure(background, nil) != nil || !errors.Is(cloudReadFailure(background, readexec.ErrUnsafe), readexec.ErrUnsafe) || !errors.Is(cloudReadFailure(background, errors.New("synthetic provider failure")), store.ErrUnavailable) {
		t.Fatal("cloud provider failure mapping changed")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if !errors.Is(cloudReadFailure(cancelled, errors.New("provider stopped")), readexec.ErrCancelled) {
		t.Fatal("cancelled cloud read lost its local outcome")
	}
	deadline, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	if !errors.Is(cloudReadFailure(deadline, errors.New("provider stopped")), readexec.ErrTimeout) {
		t.Fatal("timed-out cloud read lost its local outcome")
	}
}

type contractRows struct {
	rows      [][]any
	index     int
	current   []any
	valueErr  error
	streamErr error
	closed    bool
}

func (*contractRows) Columns() []query.Column { return nil }
func (r *contractRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.current = r.rows[r.index]
	r.index++
	return true
}
func (r *contractRows) Values() ([]any, error) {
	if r.valueErr != nil {
		return nil, r.valueErr
	}
	return r.current, nil
}
func (r *contractRows) Err() error   { return r.streamErr }
func (r *contractRows) Close() error { r.closed = true; return nil }
