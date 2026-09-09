package config

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMCPConfigurationBoundsAndIsolation(t *testing.T) {
	base := DefaultMCP()
	if err := ValidateMCP(base); err != nil {
		t.Fatal(err)
	}
	clone := base.Clone()
	clone.Groups[0] = "changed"
	clone.AllowedHosts[0] = "changed"
	if base.Groups[0] != "discovery" || base.AllowedHosts[0] != "localhost" {
		t.Fatal("mutable configuration")
	}
	for _, edit := range []func(*MCP){
		func(m *MCP) { m.MaxRequestBytes = 1023 }, func(m *MCP) { m.MaxRequestBytes = 11 << 20 },
		func(m *MCP) { m.MaxResponseBytes = 16383 }, func(m *MCP) { m.MaxResponseBytes = 33 << 20 },
		func(m *MCP) { m.MaxConcurrent = 0 }, func(m *MCP) { m.MaxConcurrent = 65 },
		func(m *MCP) { m.Timeout = Duration(time.Millisecond) }, func(m *MCP) { m.Timeout = Duration(66 * time.Second) },
		func(m *MCP) { m.Groups = nil }, func(m *MCP) { m.Groups = []string{"discovery", "discovery"} },
		func(m *MCP) { m.Groups = []string{"unbuilt"} }, func(m *MCP) { m.Groups = make([]string, 5) },
		func(m *MCP) { m.AllowedHosts = nil }, func(m *MCP) { m.AllowedHosts = make([]string, 17) },
		func(m *MCP) { m.AllowedHosts = []string{"localhost", "localhost"} }, func(m *MCP) { m.AllowedHosts = []string{"*"} },
	} {
		m := base.Clone()
		edit(&m)
		if ValidateMCP(m) == nil {
			t.Fatalf("invalid settings accepted: %+v", m)
		}
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "mcp.example", "a-b.example", "2001:db8::1"} {
		if !MCPHost(host) {
			t.Fatal("valid host", host)
		}
	}
	for _, host := range []string{"", "*", "localhost:8080", "[::1]", "::ffff:127.0.0.1", "https://host", "HOST", ".host", "host.", "-host", "host-", "x..y", strings.Repeat("x", 64) + ".example", strings.Repeat("x", 254), "h_ost", "host/path"} {
		if MCPHost(host) {
			t.Fatal("invalid host", host)
		}
	}
	// Every exported setting round-trips through the ordinary typed decoder.
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	var parsed MCP
	if err = json.Unmarshal(raw, &parsed); err != nil || ValidateMCP(parsed) != nil {
		t.Fatal("wire settings", err)
	}
}
