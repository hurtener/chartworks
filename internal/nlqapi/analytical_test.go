package nlqapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
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

func TestAnalyticalErrorRegistrationMatchesHandlers(t *testing.T) {
	for _, build := range []func() (*api.Registry, error){ExecutionRegistry, func() (*api.Registry, error) { return BYORegistry(true) }} {
		registry, err := build()
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range registry.Definitions() {
			for _, failure := range []error{exec.ErrAnalyticalMismatch, exec.ErrAnalyticalUnsupported} {
				status, code := classify(failure)
				found := false
				for _, response := range operation.Errors {
					found = found || response.Status == status && response.Code == code
				}
				if !found {
					t.Fatalf("%s omits handler error %d %s", operation.ID, status, code)
				}
			}
		}
	}
}
