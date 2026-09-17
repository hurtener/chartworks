package semantics

import "log/slog"

// LogValue excludes raw clarification values from ordinary structured logs.
// JSON remains the explicit authorized wire and protected-storage format.
func (ClarificationValue) LogValue() slog.Value {
	return slog.StringValue("clarification-value(redacted)")
}

// String excludes raw clarification values from ordinary formatted logs.
func (ClarificationValue) String() string { return "clarification-value(redacted)" }

// GoString preserves redaction when a value is formatted with the Go syntax verb.
func (v ClarificationValue) GoString() string { return v.String() }
