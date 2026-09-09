package acceptance

import (
	"testing"

	"github.com/hurtener/chartworks/internal/api"
)

// Both initial verifier denial and expiration between verification and dispatch
// must fit the public error contract of every concrete protected operation.
func TestPhase21SharedGuardErrorContracts(t *testing.T) {
	for _, operation := range phase21Registry(t).Definitions() {
		if operation.Public {
			continue
		}
		for _, required := range []api.ErrorResponse{
			{Status: 401, Code: "unauthorized"},
			{Status: 401, Code: "unauthenticated"},
			{Status: 403, Code: "forbidden"},
		} {
			found := false
			for _, declared := range operation.Errors {
				found = found || declared.Status == required.Status && declared.Code == required.Code
			}
			if !found {
				t.Errorf("%s omits shared admission error %d/%s", operation.ID, required.Status, required.Code)
			}
		}
	}
}
