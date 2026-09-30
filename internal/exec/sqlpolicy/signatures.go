package sqlpolicy

import "strings"

// Signature is the shared, conservative call contract. Native planning remains
// mandatory for coercion, overload resolution, collation and engine-version rules.
// Arguments use | for alternatives and ? for optional trailing arguments.
type Signature struct {
	RoundingModes   []string `json:"rounding_modes,omitempty"`
	NativeCoercions bool     `json:"native_coercions,omitempty"`
	SQLName         string   `json:"sql_name,omitempty"`
	Arguments       []string `json:"arguments"`
	Variadic        bool     `json:"variadic,omitempty"`
	Kind            string   `json:"kind"`
	Result          string   `json:"result"`
}

func FunctionSignature(dialect, name string) (Signature, bool) {
	if !AllowsFunction(dialect, []string{name}) {
		return Signature{}, false
	}
	name = strings.ToLower(name)
	s := Signature{Kind: "scalar", Result: "same", NativeCoercions: dialect != "postgres" && dialect != "bigquery"}
	switch name {
	case "count":
		s.Arguments = []string{"any"}
		s.Kind = "aggregate"
		s.Result = "integer"
	case "sum", "avg":
		s.Arguments = []string{"numeric|interval"}
		s.Kind = "aggregate"
		s.Result = "numeric|interval"
	case "min", "max":
		s.Arguments = []string{"any"}
		s.Kind = "aggregate"
	case "abs", "ceil", "ceiling", "floor":
		s.Arguments = []string{"numeric"}
		s.Result = "same"
	case "round":
		s.Arguments = []string{"numeric", "integer?"}
		s.Result = "same"
		if dialect == "bigquery" || dialect == "snowflake" {
			s.Arguments = []string{"numeric", "integer?", "text?"}
			s.RoundingModes = []string{"HALF_AWAY_FROM_ZERO", "HALF_TO_EVEN"}
			if dialect == "bigquery" {
				s.RoundingModes = []string{"ROUND_HALF_AWAY_FROM_ZERO", "ROUND_HALF_EVEN"}
			}
		}
		if dialect == "sqlserver" {
			s.Arguments = []string{"numeric", "integer", "integer?"}
		}
	case "lower", "upper":
		s.Arguments = []string{"text"}
		s.Result = "text"
		if dialect == "bigquery" {
			s.Arguments = []string{"text|binary"}
			s.Result = "same"
		}
	case "length", "octet_length":
		s.Arguments = []string{"text|binary"}
		if dialect == "postgres" && name == "length" {
			s.Arguments = []string{"text|binary", "text?"}
		}
		s.Result = "integer"
	case "char_length":
		s.Arguments = []string{"text"}
		s.Result = "integer"
	case "trim", "btrim", "ltrim", "rtrim":
		s.Arguments = []string{"text", "text?"}
		s.Result = "text"
	case "substring", "substr":
		s.Arguments = []string{"text|binary", "integer|text", "integer|text?"}
		s.Result = "same"
	case "replace":
		s.Arguments = []string{"text", "text", "text"}
		s.Result = "text"
	case "like_escape":
		s.Arguments = []string{"text", "text"}
		s.Result = "text"
	case "date_trunc":
		s.Arguments = []string{"text", "temporal|interval", "text?"}
		s.Result = "temporal|interval"
	case "date_part", "extract":
		s.Arguments = []string{"text", "temporal|interval"}
		s.Result = "numeric"
		if name == "date_part" {
			s.Result = "float"
		}
	case "row_number", "rank", "dense_rank":
		s.Kind = "window"
		s.Result = "integer"
	case "percent_rank", "cume_dist":
		s.Kind = "window"
		s.Result = "numeric"
	case "lag", "lead":
		s.Arguments = []string{"any", "integer?", "same?"}
		s.Kind = "window"
	case "first_value", "last_value":
		s.Arguments = []string{"any"}
		s.Kind = "window"
	case "nth_value":
		s.Arguments = []string{"any", "integer"}
		s.Kind = "window"
	case "ntile":
		s.Arguments = []string{"integer"}
		s.Kind = "window"
		s.Result = "integer"
	case "nullif":
		s.Arguments = []string{"any", "same"}
	case "coalesce":
		s.Arguments = []string{"any", "same?"}
		if dialect == "sqlserver" || dialect == "snowflake" {
			s.Arguments = []string{"any", "same"}
		}
		s.Variadic = true
	default:
		return Signature{}, false
	}
	if dialect != "postgres" && (name == "sum" || name == "avg") {
		s.Arguments = []string{"numeric"}
		s.Result = "same"
	}
	if dialect == "sqlserver" && name == "length" {
		s.SQLName = "len"
	}
	return s, true
}

// CallSyntax is the shared dialect syntax contract for the registered functions.
type CallSyntax struct {
	WindowFilter    bool `json:"window_filter"`
	AggregateFilter bool `json:"aggregate_filter"`
	AggregateOrder  bool `json:"aggregate_order"`
	DistinctWindow  bool `json:"distinct_window"`
}

func FunctionSyntax(dialect string) CallSyntax {
	return CallSyntax{WindowFilter: dialect == "postgres", AggregateFilter: dialect == "postgres" || dialect == "databricks", AggregateOrder: dialect == "postgres", DistinctWindow: dialect == "bigquery" || dialect == "snowflake"}
}

// AllowsCall rejects proven signature mismatches. unknown means the parser has
// no exact type evidence, never that a source plan may skip native type checking.
func AllowsCall(dialect, name string, arguments []string, star, over, distinct, filter, ordered, within bool) bool {
	s, ok := FunctionSignature(dialect, name)
	if !ok {
		return false
	}
	name = strings.ToLower(name)
	forms := FunctionSyntax(dialect)
	if filter && !forms.AggregateFilter || ordered && !forms.AggregateOrder || over && filter && !forms.WindowFilter {
		return false
	}
	if within || star && (name != "count" || len(arguments) != 0 || distinct) {
		return false
	}
	if s.Kind == "window" && !over || s.Kind == "scalar" && over {
		return false
	}
	if (distinct || filter || ordered) && s.Kind != "aggregate" {
		return false
	}
	if over && (ordered || distinct && !forms.DistinctWindow) {
		return false
	}
	if star {
		return true
	}
	required := 0
	for _, a := range s.Arguments {
		if !strings.HasSuffix(a, "?") {
			required++
		}
	}
	if len(arguments) < required || !s.Variadic && len(arguments) > len(s.Arguments) {
		return false
	}
	for i, a := range arguments {
		j := i
		if j >= len(s.Arguments) {
			j = len(s.Arguments) - 1
		}
		want := strings.TrimSuffix(s.Arguments[j], "?")
		if want == "same" {
			if !s.NativeCoercions && len(arguments) > 0 && a != "unknown" && arguments[0] != "unknown" && a != arguments[0] && !(numericType(a) && numericType(arguments[0])) {
				return false
			}
			continue
		}
		if a == "unknown" || want == "any" {
			continue
		}
		matches := false
		for _, w := range strings.Split(want, "|") {
			if w == a || w == "numeric" && numericType(a) {
				matches = true
			}
		}
		if !matches && !s.NativeCoercions {
			return false
		}
	}
	if name == "round" && len(arguments) == 3 {
		if len(s.RoundingModes) > 0 && arguments[2] != "text" && arguments[2] != "unknown" {
			return false
		}
		if (dialect == "bigquery" && arguments[0] != "numeric" || dialect == "snowflake" && arguments[0] == "float") && arguments[0] != "unknown" {
			return false
		}
	}
	// PostgreSQL's encoded length overload accepts bytea, not text.
	if dialect == "postgres" && name == "length" && len(arguments) == 2 && arguments[0] != "binary" && arguments[0] != "unknown" {
		return false
	}
	// PostgreSQL's two-argument round overload accepts numeric, not float8.
	if dialect == "postgres" && name == "round" && len(arguments) == 2 && arguments[0] == "float" {
		return false
	}
	return true
}
func numericType(s string) bool { return s == "numeric" || s == "integer" || s == "float" }

// String is a compact lossless generation view of the supported signature.
// Coercion policy is shared by every signature in a dialect and is emitted once
// on the enclosing Profile to avoid spending prompt budget on repeated flags.
func (s Signature) String() string {
	args := strings.Join(s.Arguments, ",")
	if s.Variadic {
		args += "..."
	}
	result := s.Kind + "(" + args + ")->" + s.Result
	if len(s.RoundingModes) > 0 {
		result += ";arg3={" + strings.Join(s.RoundingModes, "|") + "}"
	}
	if s.SQLName != "" {
		result = s.SQLName + ":" + result
	}
	return result
}

// AllowsLiteralArgument checks closed value domains only when the native AST
// supplies an exact string literal. Expressions/parameters still require native
// planning and execution; this helper does not invent their values.
func AllowsLiteralArgument(dialect, name string, index int, value string) bool {
	s, ok := FunctionSignature(dialect, name)
	if !ok {
		return false
	}
	if strings.EqualFold(name, "round") && index == 2 && len(s.RoundingModes) > 0 {
		for _, mode := range s.RoundingModes {
			if value == mode {
				return true
			}
		}
		return false
	}
	return true
}
