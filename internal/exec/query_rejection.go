package exec

import (
	"errors"

	"github.com/hurtener/chartworks/internal/exec/querydiagnostic"
)

// queryRejection stores only an admitted reason. It never wraps the original
// database error: Error, JSON, formatting and logs cannot expose its body.
type queryRejection struct{ code string }

func (queryRejection) Error() string { return "exec: source rejected read query" }
func (queryRejection) Unwrap() error { return ErrQuery }

// QueryRejection constructs a detail-free classified rejection. Unrecognized
// diagnostics are deliberately collapsed to the existing generic sentinel.
// This is an adapter seam, not proof that execution is stopped or retryable.
func QueryRejection(code string) error {
	if !querydiagnostic.Known(code) || code == "query_error" {
		return ErrQuery
	}
	return queryRejection{code: code}
}

// QueryRejectionCode extracts only the trusted in-process classification and
// strips any enclosing prose. A caller's Error string cannot create a code.
func QueryRejectionCode(err error) string {
	var rejection queryRejection
	if errors.As(err, &rejection) && querydiagnostic.Known(rejection.code) {
		return rejection.code
	}
	if errors.Is(err, ErrQuery) {
		return "query_error"
	}
	return ""
}
