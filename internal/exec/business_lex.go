package exec

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This scanner locates spans for a closed SELECT transformation. It is not a
// safety parser: its output still requires the normal whole-statement validator
// and native planning before any source execution is possible.
type businessToken struct {
	start, end int
	depth      int
	kind       byte
	text       string
}

func (t businessToken) word(value string) bool {
	return t.kind == 'w' && strings.EqualFold(t.text, value)
}

func businessScan(ctx context.Context, sql string, internal bool) ([]businessToken, error) {
	if ctx == nil || len(sql) == 0 || len(sql) > 256<<10 || !utf8.ValidString(sql) {
		return nil, businessSQLFailure("unsupported_statement_size")
	}
	var out []businessToken
	depth := 0
	for i := 0; i < len(sql); {
		if len(out) > 16384 || depth > 64 {
			return nil, businessSQLFailure("unsupported_statement_depth")
		}
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		c := sql[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}
		start := i
		if internal && c == 0x1f {
			end := strings.IndexByte(sql[i+1:], 0x1f)
			if end < 0 {
				return nil, businessSQLFailure("invalid_parameter_marker")
			}
			i += end + 2
			out = append(out, businessToken{start, i, depth, 'm', sql[start+1 : i-1]})
			continue
		}
		if c < 32 || c == 127 || c == '#' || i+1 < len(sql) && (sql[i:i+2] == "--" || sql[i:i+2] == "/*") {
			return nil, businessSQLFailure("unsupported_statement_comment")
		}
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			close := c
			if c == '[' {
				close = ']'
			}
			i++
			var decoded strings.Builder
			closed := false
			for i < len(sql) {
				if sql[i] == '\\' || sql[i] < 32 || sql[i] == 127 {
					return nil, businessSQLFailure("unsupported_quoted_escape")
				}
				if sql[i] == close {
					if i+1 < len(sql) && sql[i+1] == close {
						decoded.WriteByte(close)
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				decoded.WriteByte(sql[i])
				i++
			}
			if !closed {
				return nil, businessSQLFailure("invalid_quoted_value")
			}
			kind := byte('q')
			if c == '\'' {
				kind = 's'
			}
			out = append(out, businessToken{start, i, depth, kind, decoded.String()})
			continue
		}
		if c == '?' || c == '$' || c == '@' {
			i++
			if c == '@' {
				if i >= len(sql) || sql[i] != 'p' && sql[i] != 'P' {
					return nil, businessSQLFailure("unsupported_parameter_name")
				}
				i++
			}
			if c != '?' {
				begin := i
				for i < len(sql) && sql[i] >= '0' && sql[i] <= '9' {
					i++
				}
				if begin == i {
					return nil, businessSQLFailure("unsupported_parameter_name")
				}
			}
			out = append(out, businessToken{start, i, depth, 'p', sql[start:i]})
			continue
		}
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' {
			i++
			for i < len(sql) && (sql[i] >= 'a' && sql[i] <= 'z' || sql[i] >= 'A' && sql[i] <= 'Z' || sql[i] >= '0' && sql[i] <= '9' || sql[i] == '_') {
				i++
			}
			out = append(out, businessToken{start, i, depth, 'w', sql[start:i]})
			continue
		}
		if c >= '0' && c <= '9' {
			i++
			for i < len(sql) && (sql[i] >= '0' && sql[i] <= '9' || sql[i] == '.') {
				i++
			}
			out = append(out, businessToken{start, i, depth, 'n', sql[start:i]})
			continue
		}
		if c == ')' {
			depth--
			if depth < 0 {
				return nil, businessSQLFailure("invalid_statement_parentheses")
			}
		}
		i++
		out = append(out, businessToken{start, i, depth, 'x', sql[start:i]})
		if c == '(' {
			depth++
		}
	}
	if depth != 0 {
		return nil, businessSQLFailure("invalid_statement_parentheses")
	}
	return out, ctx.Err()
}

func businessSQLFailure(code string) error {
	return &BusinessConstraintError{Code: code, Field: "sql"}
}

func businessName(token businessToken, dialect string) (string, bool) {
	if token.kind != 'w' && token.kind != 'q' {
		return "", false
	}
	name := token.text
	if token.kind == 'w' {
		switch dialect {
		case "postgres", "mysql":
			name = strings.ToLower(name)
		case "snowflake":
			name = strings.ToUpper(name)
		}
	}
	return name, SQLIdentifierForDialect(dialect, name)
}

func businessQuote(dialect, name string) string {
	switch dialect {
	case "mysql", "bigquery", "databricks":
		return "`" + name + "`"
	case "sqlserver":
		return "[" + name + "]"
	default:
		return `"` + name + `"`
	}
}

func businessPlaceholder(dialect string, index int) string {
	switch dialect {
	case "postgres":
		return "$" + strconv.Itoa(index)
	case "sqlserver", "bigquery":
		return "@p" + strconv.Itoa(index)
	default:
		return "?"
	}
}

func businessParameterIndex(token, dialect string, positional int) (int, error) {
	if dialect == "mysql" || dialect == "snowflake" || dialect == "databricks" {
		if token != "?" {
			return 0, businessSQLFailure("unsupported_parameter_style")
		}
		return positional, nil
	}
	prefix := "$"
	if dialect == "sqlserver" || dialect == "bigquery" {
		prefix = "@p"
	}
	if !strings.HasPrefix(strings.ToLower(token), prefix) {
		return 0, businessSQLFailure("unsupported_parameter_style")
	}
	value, err := strconv.Atoi(token[len(prefix):])
	if err != nil || value < 1 || strconv.Itoa(value) != token[len(prefix):] {
		return 0, businessSQLFailure("invalid_parameter_index")
	}
	return value, nil
}
