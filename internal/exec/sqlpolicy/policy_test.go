package sqlpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Freeze the pre-refactor validator lists independently. Updating production
// names requires a deliberate policy/test review, not a regenerated golden.
const oldPostgresFunctions = "count sum avg min max abs round ceil ceiling floor lower upper length char_length octet_length trim btrim ltrim rtrim substring substr replace like_escape date_trunc date_part extract row_number rank dense_rank lag lead first_value last_value nth_value ntile percent_rank cume_dist"
const oldWarehouseFunctions = "abs avg coalesce count length lower max min round sum upper"
const oldPostgresTypes = "int2 int4 int8 numeric float4 float8 bool text varchar bpchar date timestamp timestamptz time timetz interval uuid json jsonb bytea money"
const oldPostgresOperators = "+ - * / % = <> != < > <= >= || ~~ !~~ ~~* !~~*"

func TestSQLRecoveryVocabularyPreservesValidatorLists(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			p, err := ForDialect(dialect)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Fields(oldWarehouseFunctions)
			if dialect == "postgres" {
				want = strings.Fields(oldPostgresFunctions)
			}
			slices.Sort(want)
			if !reflect.DeepEqual(p.Functions, want) {
				t.Fatal("validator function list changed")
			}
			for _, name := range p.Functions {
				if !AllowsFunction(dialect, []string{name}) {
					t.Fatal("advertised but disallowed", name)
				}
				if AllowsFunction(dialect, []string{strings.ToUpper(name)}) != (dialect != "postgres") {
					t.Fatal("changed case policy", name)
				}
				if AllowsFunction(dialect, []string{"pg_catalog", name}) != (dialect == "postgres") {
					t.Fatal("changed namespace policy", name)
				}
			}
			if dialect == "postgres" {
				for _, pair := range []struct {
					got  []string
					want string
				}{{p.CastTypes, oldPostgresTypes}, {p.Operators, oldPostgresOperators}} {
					expect := strings.Fields(pair.want)
					slices.Sort(expect)
					if !reflect.DeepEqual(pair.got, expect) {
						t.Fatal("native name gate changed")
					}
				}
				for _, name := range p.CastTypes {
					if !AllowsPostgresType([]string{name}) || !AllowsPostgresType([]string{"pg_catalog", name}) {
						t.Fatal("type snapshot disagrees")
					}
				}
				for _, name := range p.Operators {
					if !AllowsPostgresOperator([]string{name}) {
						t.Fatal("operator snapshot disagrees")
					}
				}
			} else if len(p.CastTypes)+len(p.Operators)+len(p.Expressions)+len(p.ValueKeywords)+len(p.FunctionNamespaces) != 0 {
				t.Fatal("unqualified warehouse capability advertised")
			}
		})
	}
}

func TestSQLRecoveryVocabularyExactDialectsAndMarkers(t *testing.T) {
	for _, tc := range []struct{ dialect, native, style string }{{"postgres", "postgres", "dollar-numbered"}, {"mysql", "mysql", "positional-question-mark"}, {"sqlserver", "tsql", "at-p-numbered"}, {"bigquery", "bigquery", "at-p-numbered"}, {"snowflake", "snowflake", "positional-question-mark"}, {"databricks", "databricks", "positional-question-mark"}} {
		p, err := ForDialect(tc.dialect)
		if err != nil || p.NativeDialect != tc.native || p.ParameterStyle != tc.style {
			t.Fatal(tc, err)
		}
		n, ok := NativeDialect(tc.dialect)
		if !ok || n != tc.native {
			t.Fatal("registry dispatch mismatch")
		}
	}
	for _, value := range []string{"", "postgresql", "Postgres", "tsql", "mssql", "spark", "duckdb", "postgres ", "postgres\nprivate-value", strings.Repeat("x", 10000)} {
		if p, err := ForDialect(value); !errors.Is(err, ErrDialect) || p.Dialect != "" || strings.Contains(err.Error(), "private-value") {
			t.Fatal("unsupported name admitted or echoed")
		}
		if text, err := Guidance(value); text != "" || !errors.Is(err, ErrDialect) {
			t.Fatal("unsupported dialect got guidance")
		}
	}
}

func TestSQLRecoveryVocabularyRejectsForeignNames(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks", "unknown"} {
		for _, parts := range [][]string{nil, {}, {""}, {"pg_sleep"}, {"pg_read_file"}, {"set_config"}, {"now"}, {"GETDATE"}, {"abs "}, {"abs sum"}, {"abs\x00"}, {"public.abs"}, {"public", "abs"}, {"PG_CATALOG", "abs"}, {"db", "pg_catalog", "abs"}, {strings.Repeat("a", 10000)}} {
			if AllowsFunction(dialect, parts) {
				t.Fatal("foreign function accepted", dialect, parts)
			}
		}
	}
	for _, parts := range [][]string{{"numeric "}, {"regclass"}, {"public", "numeric"}, {"pg_catalog", "int4", "extra"}, {"NUMERIC"}} {
		if AllowsPostgresType(parts) {
			t.Fatal("foreign type accepted")
		}
	}
	for _, parts := range [][]string{{"pg_catalog", "+"}, {"->>"}, {";"}, {"+ -"}, {""}} {
		if AllowsPostgresOperator(parts) {
			t.Fatal("foreign operator accepted")
		}
	}
	for _, op := range []string{"SVFOP_CURRENT_DATE", "SVFOP_CURRENT_TIME", "SVFOP_CURRENT_TIME_N", "SVFOP_CURRENT_TIMESTAMP", "SVFOP_CURRENT_TIMESTAMP_N", "SVFOP_LOCALTIME", "SVFOP_LOCALTIME_N", "SVFOP_LOCALTIMESTAMP", "SVFOP_LOCALTIMESTAMP_N"} {
		if !AllowsPostgresValue(op) {
			t.Fatal("lost native value opcode")
		}
	}
	for _, op := range []string{"SVFOP_CURRENT_USER", "SVFOP_SESSION_USER", "current_date", "", "SVFOP_CURRENT_DATE extra"} {
		if AllowsPostgresValue(op) {
			t.Fatal("unsupported value opcode")
		}
	}
}

func TestSQLRecoveryVocabularyDetachedDigestAndGuidance(t *testing.T) {
	p, _ := ForDialect("postgres")
	digest := p.Digest
	p.Digest = ""
	raw, _ := json.Marshal(p)
	sum := sha256.Sum256(raw)
	if digest != hex.EncodeToString(sum[:]) {
		t.Fatal("incomplete profile digest")
	}
	p.Functions[0] = "pg_read_file"
	p.CastTypes[0] = "regclass"
	p.Operators[0] = "hacked"
	p.Expressions[0] = "hacked"
	p.ValueKeywords[0] = "current_user"
	p.FunctionNamespaces[0] = "public"
	again, _ := ForDialect("postgres")
	if again.Digest != digest || AllowsFunction("postgres", []string{"pg_read_file"}) {
		t.Fatal("snapshot mutated live validator policy")
	}
	text, err := Guidance("postgres")
	if err != nil || len(text) > 4096 || !strings.Contains(text, digest) || !strings.Contains(text, "necessary, not sufficient") || !strings.Contains(text, "No vocabulary entry grants permission") {
		t.Fatal("unbounded or overstated guidance")
	}
	prefix := " Validator name vocabulary (necessary, not sufficient): "
	end := strings.Index(text[len(prefix):], "}. ") + len(prefix) + 1
	var decoded Profile
	if end <= len(prefix) || json.Unmarshal([]byte(text[len(prefix):end]), &decoded) != nil || !reflect.DeepEqual(decoded, again) {
		t.Fatal("guidance doesn't encode the exact profile")
	}
	mysql, _ := ForDialect("mysql")
	snowflake, _ := ForDialect("snowflake")
	if mysql.Digest == snowflake.Digest {
		t.Fatal("same list collapsed distinct dialect identities")
	}
}

func TestSQLRecoveryVocabularyConcurrent(t *testing.T) {
	expected, _ := Guidance("postgres")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 16; j++ {
				p, _ := ForDialect("postgres")
				p.Functions[0] = "mutated"
				text, err := Guidance("postgres")
				if err != nil || text != expected || !AllowsFunction("postgres", []string{"sum"}) {
					t.Error("shared mutable policy")
				}
			}
		}()
	}
	wg.Wait()
}

func FuzzSQLVocabularyNames(f *testing.F) {
	for _, seed := range []string{"abs", "ABS", "public.abs", "pg_catalog", "sum count", "SVFOP_CURRENT_DATE", "🧊", "abs\x00", "->>"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 512 {
			return
		}
		for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
			got := AllowsFunction(dialect, []string{name})
			want := strings.Fields(oldWarehouseFunctions)
			match := strings.ToLower(name)
			if dialect == "postgres" {
				want = strings.Fields(oldPostgresFunctions)
				match = name
			}
			if got != slices.Contains(want, match) {
				t.Fatal("name gate differs from baseline")
			}
		}
	})
}
