// Package sqlpolicy owns the read validator's closed name vocabularies. It does
// not parse SQL, admit source access or certify a dialect's complete semantics.
// Callers receive detached snapshots; validation always consults this registry,
// never a caller-edited snapshot or a model-provided capability declaration.
package sqlpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Version changes when this vocabulary or its interpretation changes. Retained
// query/analytical policy versions remain separate; this is generation guidance.
const Version = "read-sql-vocabulary-v1"

// ErrDialect is closed and never includes untrusted dialect text.
var ErrDialect = errors.New("sqlpolicy: unsupported dialect")

// These are the existing validator lists, not all functions of the named DBMS.
// Space-delimited constants avoid mutable shared maps/slices and have one owner
// for lookup and exported snapshots. PostgreSQL names are native AST spellings.
const postgresFunctions = "count sum avg min max abs round ceil ceiling floor lower upper length char_length octet_length trim btrim ltrim rtrim substring substr replace like_escape date_trunc date_part extract row_number rank dense_rank lag lead first_value last_value nth_value ntile percent_rank cume_dist"
const warehouseFunctions = "abs avg coalesce count length lower max min round sum upper"
const postgresTypes = "int2 int4 int8 numeric float4 float8 bool text varchar bpchar date timestamp timestamptz time timetz interval uuid json jsonb bytea money"
const postgresValueOps = "SVFOP_CURRENT_DATE SVFOP_CURRENT_TIME SVFOP_CURRENT_TIME_N SVFOP_CURRENT_TIMESTAMP SVFOP_CURRENT_TIMESTAMP_N SVFOP_LOCALTIME SVFOP_LOCALTIME_N SVFOP_LOCALTIMESTAMP SVFOP_LOCALTIMESTAMP_N"
const postgresOperators = "+ - * / % = <> != < > <= >= || ~~ !~~ ~~* !~~*"

// Profile documents necessary name gates, not sufficient SQL acceptance. Lists
// name parser-recognized forms; argument types, whole-tree resolution, native
// planning and analytical restrictions can still reject any particular use.
// No source identifiers, question, values, credentials or permissions are here.
type Profile struct {
	Version            string   `json:"version"`
	Dialect            string   `json:"dialect"`
	Parser             string   `json:"parser"`
	NativeDialect      string   `json:"native_dialect"`
	ParameterStyle     string   `json:"parameter_style"`
	Functions          []string `json:"function_names"`
	FunctionNamespaces []string `json:"function_namespaces,omitempty"`
	CastTypes          []string `json:"cast_ast_type_names,omitempty"`
	Operators          []string `json:"operator_ast_names,omitempty"`
	Expressions        []string `json:"special_expressions,omitempty"`
	ValueKeywords      []string `json:"value_keywords,omitempty"`
	Digest             string   `json:"digest,omitempty"`
}

// NativeDialect maps only exact admitted source dialects to the existing parser
// families. Human synonyms (postgresql, mssql, sparksql) are not auto-admitted.
func NativeDialect(dialect string) (string, bool) {
	switch dialect {
	case "postgres":
		return "postgres", true
	case "mysql", "bigquery", "snowflake", "databricks":
		return dialect, true
	case "sqlserver":
		return "tsql", true
	default:
		return "", false
	}
}

// ForDialect creates a deterministic, bounded snapshot from the same constants
// used by the actual validator gates. Empty warehouse lists are unadvertised,
// not a promise that all casts/operators/special forms work in that parser.
func ForDialect(dialect string) (Profile, error) {
	native, ok := NativeDialect(dialect)
	if !ok {
		return Profile{}, ErrDialect
	}
	out := Profile{Version: Version, Dialect: dialect, Parser: "warehouse-native-read", NativeDialect: native, ParameterStyle: "positional-question-mark", Functions: words(warehouseFunctions)}
	switch dialect {
	case "postgres":
		out.Parser, out.ParameterStyle = "postgres-native-ast", "dollar-numbered"
		out.Functions, out.FunctionNamespaces = words(postgresFunctions), []string{"pg_catalog"}
		out.CastTypes, out.Operators = words(postgresTypes), words(postgresOperators)
		out.Expressions = []string{"coalesce", "greatest", "least", "nullif"}
		for _, op := range words(postgresValueOps) {
			keyword := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(op, "SVFOP_"), "_N"))
			if len(out.ValueKeywords) == 0 || out.ValueKeywords[len(out.ValueKeywords)-1] != keyword {
				out.ValueKeywords = append(out.ValueKeywords, keyword)
			}
		}
	case "sqlserver", "bigquery":
		out.ParameterStyle = "at-p-numbered"
	}
	raw, _ := json.Marshal(out) // Closed fields only; serialization cannot fail.
	hash := sha256.Sum256(raw)
	out.Digest = hex.EncodeToString(hash[:])
	return out, nil
}

// AllowsFunction checks the existing parsed-name policy. PostgreSQL preserves
// quoted-name case and admits only unqualified/pg_catalog forms. The warehouse
// inspection interface supplies a single name string and is case-insensitive;
// dotted user/catalog function names remain rejected.
func AllowsFunction(dialect string, parts []string) bool {
	if dialect == "postgres" {
		name, ok := postgresName(parts)
		return ok && contains(postgresFunctions, name)
	}
	if _, ok := NativeDialect(dialect); !ok || len(parts) != 1 || len(parts[0]) > 128 {
		return false
	}
	return contains(warehouseFunctions, strings.ToLower(parts[0]))
}

// AllowsPostgresType is the positive native TypeCast name gate, not a claim
// about arbitrary modifiers, values, implicit coercions or array casts.
func AllowsPostgresType(parts []string) bool {
	name, ok := postgresName(parts)
	return ok && contains(postgresTypes, name)
}

// AllowsPostgresOperator keeps schema-qualified/user-defined operators out of
// the primitive expression gate; operands still require recursive validation.
func AllowsPostgresOperator(parts []string) bool {
	return len(parts) == 1 && contains(postgresOperators, parts[0])
}

// AllowsPostgresValue accepts exactly the previously admitted SQLValueFunction
// opcodes, including optional precision. Current user/session identity is absent.
func AllowsPostgresValue(op string) bool { return contains(postgresValueOps, op) }

func postgresName(parts []string) (string, bool) {
	if len(parts) == 2 && parts[0] == "pg_catalog" {
		return parts[1], true
	}
	if len(parts) == 1 {
		return parts[0], true
	}
	return "", false
}
func contains(list, name string) bool {
	// Bound arbitrary membership requests and prevent delimiter injection without
	// trimming/normalizing quoted PostgreSQL identifiers into a different name.
	return len(name) > 0 && len(name) <= 128 && !strings.ContainsAny(name, " \t\r\n\x00") && strings.Contains(" "+list+" ", " "+name+" ")
}
func words(list string) []string { out := strings.Fields(list); sort.Strings(out); return out }

// Guidance is server-owned system context. It is derived from the registry, not
// from user instructions. Providers receive this before full-envelope fitting.
func Guidance(dialect string) (string, error) {
	profile, err := ForDialect(dialect)
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(profile)
	return " Validator name vocabulary (necessary, not sufficient): " + string(raw) + ". Use only the listed function names or special expression forms. PostgreSQL cast names are native AST spellings. These are read-validator limits, not all database capabilities; types, whole-tree safety, reviewed source scope and active analytical rules can narrow them further. Unlisted categories are not advertised. No vocabulary entry grants permission.", nil
}
