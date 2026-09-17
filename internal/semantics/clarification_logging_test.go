package semantics

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestClarificationStructuredLoggingRedactsValues(t *testing.T) {
	secret := "synthetic-private-answer-731"
	value := ClarificationValue{Text: &secret}
	answer := ClarificationAnswer{Topic: "sales", Pattern: "customer", Slot: "customer", Value: &value}
	resolution := ClarificationResolution{Sensitivity: LiteralSensitive, Value: secret, Effect: &ClarificationEffect{Kind: "entity", Values: []GovernedClarificationValue{{Canonical: secret, Label: "Customer"}}}}
	for _, jsonLogs := range []bool{false, true} {
		var output bytes.Buffer
		var handler slog.Handler
		if jsonLogs {
			handler = slog.NewJSONHandler(&output, nil)
		} else {
			handler = slog.NewTextHandler(&output, nil)
		}
		slog.New(handler).Info("clarification", slog.Any("answer", answer), slog.Any("resolution", resolution), slog.Any("value", value))
		if strings.Contains(output.String(), secret) || !strings.Contains(output.String(), "redacted") {
			t.Fatal("ordinary structured logging exposed a typed answer")
		}
	}
	// Redaction must not silently erase data from explicitly authorized DTOs or
	// canonical protected records used for replay and binding.
	for _, dto := range []any{answer, resolution, value} {
		raw, err := json.Marshal(dto)
		if err != nil || !bytes.Contains(raw, []byte(secret)) {
			t.Fatal("log safety changed the wire/storage contract", err)
		}
	}
}
