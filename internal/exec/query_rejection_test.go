package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hurtener/chartworks/internal/exec/querydiagnostic"
	"strings"
	"testing"
)

func TestSQLRecoveryQueryRejectionClosedValue(t *testing.T) {
	for _, code := range querydiagnostic.Codes() {
		err := QueryRejection(code)
		if !errors.Is(err, ErrQuery) || QueryRejectionCode(err) != code {
			t.Fatal("lost query category", code)
		}
		raw, _ := json.Marshal(err)
		if strings.Contains(string(raw), "code") {
			t.Fatal("error serialization added an unreviewed public schema")
		}
		if !strings.Contains(err.Error(), "source rejected read query") {
			t.Fatal("raw/unstable error text")
		}
	}
	for _, input := range []string{"", "unknown", "22012", "source_unavailable", "query_error PRIVATE_CANARY"} {
		err := QueryRejection(input)
		if err != ErrQuery || QueryRejectionCode(err) != "query_error" || strings.Contains(fmt.Sprintf("%v %#v", err, err), "PRIVATE_CANARY") {
			t.Fatal("untrusted code retained")
		}
	}
	for _, err := range []error{nil, errors.New("query_division_by_zero"), errors.New("exec: source rejected read query"), ErrBinding, ErrUncertain} {
		if QueryRejectionCode(err) != "" {
			t.Fatal("text classified as query rejection")
		}
	}
	wrapped := fmt.Errorf("private wrapper: %w", QueryRejection("query_invalid_text"))
	if QueryRejectionCode(wrapped) != "query_invalid_text" {
		t.Fatal("typed identity lost through wrapper")
	}
	// The classifier extracts vocabulary only. Execution/validation consumers must
	// separately reject a joined terminal outcome; an identifier is not permission.
	joined := errors.Join(QueryRejection("query_division_by_zero"), ErrUncertain)
	if !errors.Is(joined, ErrUncertain) || !errors.Is(joined, ErrQuery) {
		t.Fatal("lost independent terminal evidence")
	}
}
