package exec

import "log/slog"

// LogValue excludes exact business values from ordinary structured logs.
func (c BusinessConstraint) LogValue() slog.Value { return slog.StringValue(c.String()) }
