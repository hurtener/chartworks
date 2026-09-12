package config

import (
	"net"
	"strings"
	"time"
)

// MCP bounds the optional stateless shared-port transport. Host names are exact
// deployment addresses, not identities or a replacement for Pengui authority.
type MCP struct {
	MaxRequestBytes  int      `json:"max_request_bytes"`
	MaxResponseBytes int      `json:"max_response_bytes"`
	MaxConcurrent    int      `json:"max_concurrent"`
	Timeout          Duration `json:"timeout"`
	Groups           []string `json:"groups"`
	AllowedHosts     []string `json:"allowed_hosts"`
}

// DefaultMCP supplies bounded settings; features.mcp still defaults to false.
func DefaultMCP() MCP {
	return MCP{MaxRequestBytes: 10 << 20, MaxResponseBytes: 16 << 20, MaxConcurrent: 16, Timeout: Duration(65 * time.Second), Groups: []string{"discovery", "query", "byo", "charts", "reporting"}, AllowedHosts: []string{"localhost", "127.0.0.1", "::1"}}
}

// Clone detaches operator-owned settings from request state.
func (m MCP) Clone() MCP {
	m.Groups = append([]string{}, m.Groups...)
	m.AllowedHosts = append([]string{}, m.AllowedHosts...)
	return m
}

// ValidateMCP rejects unknown groups, unbounded limits, and wildcard host trust.
func ValidateMCP(m MCP) error {
	if m.MaxRequestBytes < 1024 || m.MaxRequestBytes > 10<<20 || m.MaxResponseBytes < 16384 || m.MaxResponseBytes > 32<<20 || m.MaxConcurrent < 1 || m.MaxConcurrent > 64 || m.Timeout < Duration(time.Second) || m.Timeout > Duration(65*time.Second) {
		return invalid("mcp", "bounded request, response, concurrency and timeout limits required")
	}
	if len(m.Groups) < 1 || len(m.Groups) > 5 {
		return invalid("mcp.groups", "one or more implemented groups required")
	}
	seen := map[string]bool{}
	for _, g := range m.Groups {
		if seen[g] || (g != "discovery" && g != "query" && g != "byo" && g != "charts" && g != "reporting") {
			return invalid("mcp.groups", "unknown or duplicate group")
		}
		seen[g] = true
	}
	if len(m.AllowedHosts) < 1 || len(m.AllowedHosts) > 16 {
		return invalid("mcp.allowed_hosts", "bounded exact host allowlist required")
	}
	seen = map[string]bool{}
	for _, h := range m.AllowedHosts {
		if !MCPHost(h) || seen[h] {
			return invalid("mcp.allowed_hosts", "canonical exact host names or IP addresses required")
		}
		seen[h] = true
	}
	return nil
}

// MCPHost accepts canonical DNS names or IP literals, without ports or wildcards.
func MCPHost(h string) bool {
	if len(h) < 1 || len(h) > 253 || h != strings.ToLower(h) {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.String() == h
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if c != '-' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
				return false
			}
		}
	}
	return true
}
