package config

import (
	"net/url"
	"strings"
)

// ReportingApp configures public embedded presentation admission, not user grants.
// Disabled is the default. Enabling requires deliberate exact parent registration.
type ReportingApp struct {
	EmbeddedEnabled         bool     `json:"embedded_enabled"`
	RegisteredParentOrigins []string `json:"registered_parent_origins"`
}

func (c ReportingApp) Validate() error {
	if len(c.RegisteredParentOrigins) > 16 || c.EmbeddedEnabled && len(c.RegisteredParentOrigins) == 0 {
		return invalid("reporting.app", "enabled embed requires 1..16 registered exact HTTPS parent origins")
	}
	seen := map[string]bool{}
	for _, origin := range c.RegisteredParentOrigins {
		u, err := url.Parse(origin)
		if err != nil || len(origin) > 256 || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.ForceQuery || u.Port() == "443" || strings.ContainsAny(origin, "*\\\r\n\t ") || origin != u.String() || seen[origin] {
			return invalid("reporting.app.registered_parent_origins", "unique canonical exact HTTPS origins required")
		}
		seen[origin] = true
	}
	return nil
}
