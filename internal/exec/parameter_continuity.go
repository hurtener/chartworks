package exec

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/exec/parameterscope"

	pgquery "github.com/wasilibs/go-pgquery"
)

// CheckParameterContinuity checks that an edit has not repurposed retained
// positional bindings. It does not issue a native plan, grant source access or
// certify the rest of the query. The caller still restores the protected values
// and performs ordinary whole-query validation before any execution.
//
// PostgreSQL permits edits outside parameter-bearing outer clauses. Its complete
// FROM/CTE/window graph and set-operation inputs are immutable here, including
// nested SELECTs, joins, aliases and window definitions. Parameter-bearing clauses
// and any output namespace they can reference remain identical, ignoring parser
// locations only. This proves custody of bound roles, not query equivalence or
// join cardinality. Other dialects require unchanged SQL; ordinary native and
// analytical validation still own all execution and answer-correctness decisions.
func CheckParameterContinuity(ctx context.Context, dialect, before, after string, parameterCount int) error {
	if ctx == nil || parameterCount < 1 || parameterCount > 64 {
		return ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, sql := range []string{before, after} {
		if len(sql) == 0 || len(sql) > 32<<10 || !utf8.ValidString(sql) || strings.ContainsRune(sql, 0) || strings.Count(sql, "(") > 256 {
			return ErrLimit
		}
	}
	if dialect != "postgres" {
		if dialect == "" {
			return ErrBinding
		}
		if before != after {
			return ErrUnsupported
		}
		return nil
	}
	original, err := pgquery.ParseToJSON(before)
	if err != nil {
		return ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	candidate, err := pgquery.ParseToJSON(after)
	if err != nil {
		return ErrUnsupported
	}
	err = parameterscope.Check(ctx, original, candidate, parameterCount)
	switch {
	case errors.Is(err, parameterscope.ErrBinding):
		return ErrBinding
	case errors.Is(err, parameterscope.ErrLimit):
		return ErrLimit
	case errors.Is(err, parameterscope.ErrUnsupported):
		return ErrUnsupported
	default:
		return err
	}
}
