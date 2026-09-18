package exec

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestBusinessConstraintStructuredLoggingIsValueFree(t *testing.T) {
	constraint := businessFixtureConstraint()
	for _, jsonLogs := range []bool{false, true} {
		var output bytes.Buffer
		var handler slog.Handler
		if jsonLogs {
			handler = slog.NewJSONHandler(&output, nil)
		} else {
			handler = slog.NewTextHandler(&output, nil)
		}
		slog.New(handler).Info("binding", slog.Any("constraint", constraint))
		if strings.Contains(output.String(), constraint.Value) || !strings.Contains(output.String(), "redacted") {
			t.Fatal("ordinary structured logging exposed a business scalar")
		}
	}
	raw, err := json.Marshal(constraint)
	if err != nil || !bytes.Contains(raw, []byte(constraint.Value)) {
		t.Fatal("log safety erased the protected constraint", err)
	}
}
