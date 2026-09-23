package nlqapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

func TestSQLRecoveryAnalyticalErrorsAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{exec.ErrAnalyticalMismatch, "analytical_mismatch"}, {exec.ErrAnalyticalUnsupported, "analytical_unsupported"}, {errors.Join(nlqexec.ErrValidationBudget, exec.ErrAnalyticalMismatch), "analytical_mismatch"}, {exec.ErrUnsafe, "sql_unsafe"}} {
		status, code := classify(tc.err)
		if status != http.StatusUnprocessableEntity || code != tc.code {
			t.Fatal("analytical/native classification mixed", status, code)
		}
	}
}
