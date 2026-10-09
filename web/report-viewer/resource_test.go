package reportviewer

import (
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestCompiledViewerResource(t *testing.T) {
	html := HTML()
	if html != HTML() || !strings.HasPrefix(html, "<!doctype html>") || len(html) > 256<<10 || URI != "ui://chartworks/report-viewer/v1" {
		t.Fatal("resource is not stable and bounded")
	}
	for _, asset := range []string{script, styles} {
		hash := sha256.Sum256([]byte(asset))
		if !strings.Contains(html, "'sha256-"+base64.StdEncoding.EncodeToString(hash[:])+"'") {
			t.Fatal("compiled asset lacks its exact CSP hash")
		}
	}
	for _, directive := range []string{"default-src 'none'", "connect-src 'none'", "frame-src 'none'", "object-src 'none'", "base-uri 'none'", "form-action 'none'", "font-src 'none'"} {
		if !strings.Contains(html, directive) {
			t.Fatal("missing restrictive CSP", directive)
		}
	}
	for _, pattern := range []string{`(?i)<script[^>]+src=`, `(?i)<link\b`, `(?i)@import\s`, `(?i)<[^>]*\s(on\w+)\s*=`, `(?i)\beval\s*\(`, `(?i)\bnew\s+Function\b`, `\.innerHTML\s*=`, `\.outerHTML\s*=`} {
		if regexp.MustCompile(pattern).MatchString(html) {
			t.Fatal("unsafe or non-self-contained compiled resource", pattern)
		}
	}
	for _, primitive := range []string{"localStorage", "sessionStorage", "indexedDB", "document.cookie", "XMLHttpRequest", "fetch(", "Authorization:"} {
		if strings.Contains(script, primitive) {
			t.Fatal("component bypasses the authorized host bridge", primitive)
		}
	}
	for _, method := range []string{"ui/initialize", "ui/notifications/initialized", "ui/notifications/tool-result", "ui/notifications/host-context-changed", "ui/notifications/size-changed", "ui/resource-teardown", "tools/call"} {
		if !strings.Contains(script, method) {
			t.Fatal("established bridge method absent", method)
		}
	}
}

func TestSharedPresentationExcludesViewerHost(t *testing.T) {
	javascript, css := Assets()
	if javascript != presentationScript || css != styles {
		t.Fatal("presentation asset is not the canonical module")
	}
	for _, forbidden := range []string{"class Bridge", "class Viewer", "ui/initialize", "tools/call", "getElementById('report-viewer')"} {
		if strings.Contains(javascript, forbidden) {
			t.Fatal("viewer host leaked into shared presentation", forbidden)
		}
		if !strings.Contains(script, forbidden) {
			t.Fatal("viewer host lost required implementation", forbidden)
		}
	}
	for _, required := range []string{"function renderRetainedOutput", "function validateRetainedView", "function renderChart", "function exact"} {
		if strings.Count(javascript, required) != 1 || strings.Count(script, required) != 1 {
			t.Fatal("presentation duplicated or missing", required)
		}
	}
	if strings.Contains(script, "import ") || strings.Contains(script, "export {") {
		t.Fatal("compiled viewer has external module references")
	}
}
