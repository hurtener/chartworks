package semantics

import "log/slog"

// LogValue protects direct structured-log attributes. JSON remains the deliberate
// authorized wire/protected-storage representation, not an ordinary log format.
func (a ClarificationAnswer) LogValue() slog.Value     { return slog.StringValue(a.String()) }
func (r ClarificationResolution) LogValue() slog.Value { return slog.StringValue(r.String()) }
func (ClarificationValue) LogValue() slog.Value {
	return slog.StringValue("clarification-value(redacted)")
}
func (ClarificationValue) String() string     { return "clarification-value(redacted)" }
func (v ClarificationValue) GoString() string { return v.String() }
