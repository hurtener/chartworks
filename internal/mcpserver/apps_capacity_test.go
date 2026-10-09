package mcpserver

import (
	"strings"
	"testing"
)

func TestBundledAppResourceCapacity(t *testing.T) {
	const start = "<!doctype html><html><body>"
	const end = "</body></html>"
	html := start + strings.Repeat(" ", MaxAppBytes-len(start)-len(end)) + end
	a, err := NewAppResource("ui://synthetic/reports/v1", "Reports", "Synthetic bounded static report application", html)
	if err != nil || !a.valid() || len(a.html) != 512<<10 {
		t.Fatal("bounded larger static application rejected", err)
	}
	if _, err := NewAppResource(a.uri, a.name, a.description, html+" "); err == nil {
		t.Fatal("oversize static application accepted")
	}
}
