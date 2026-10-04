package reportapp

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompiledReportApp(t *testing.T) {
	html := HTML()
	if URI != "ui://chartworks/report-app/v1" || html != HTML() || !strings.HasPrefix(html, "<!doctype html>") || len(html) > 256<<10 {
		t.Fatal("unstable or unbounded report app")
	}
	script, css := compiledAssets()
	for _, asset := range []string{script, css} {
		sum := sha256.Sum256([]byte(asset))
		if !strings.Contains(html, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
			t.Fatal("missing exact asset CSP hash")
		}
	}
	for _, directive := range []string{"default-src 'none'", "connect-src 'none'", "frame-src 'none'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(html, directive) {
			t.Fatal("missing CSP", directive)
		}
	}
	for _, forbidden := range []string{"localStorage", "sessionStorage", "indexedDB", "document.cookie", "fetch(", "XMLHttpRequest", ".innerHTML", "<script src=", "import {"} {
		if strings.Contains(html, forbidden) {
			t.Fatal("unsafe resource primitive", forbidden)
		}
	}
	if strings.Count(script, "function renderRetainedOutput(") != 1 || strings.Count(script, "class RetainedReport") != 1 {
		t.Fatal("retained canvas must use a single output presentation implementation")
	}
	if strings.Count(script, "export class Viewer") != 0 || strings.Count(script, "class Viewer") != 1 {
		t.Fatal("retained renderer is not shared exactly once")
	}
	if path := os.Getenv("CHARTWORKS_REPORT_APP_HTML_OUT"); path != "" {
		if err := os.WriteFile(path, []byte(html), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReportAppJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for app source verification:", err)
	}
	for _, file := range []string{"model.test.mjs", "pages.test.mjs", "mapping.test.mjs", "dataset.test.mjs", "grid.test.mjs", "bridge.test.mjs", "retained.test.mjs", "app.test.mjs", "browser_cleanup.test.mjs"} {
		command := exec.Command(node, "--test", file)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", file, err, out)
		}
	}
	script, _ := compiledAssets()
	path := filepath.Join(t.TempDir(), "compiled-app.mjs")
	if err := os.WriteFile(path, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", path).CombinedOutput(); err != nil {
		t.Fatalf("compiled resource JS: %v\n%s", err, out)
	}
}

func TestEmbeddedResourceRegistration(t *testing.T) {
	for _, parents := range [][]string{nil, {}, {"*"}, {"null"}, {"http://host.example"}, {"https://host.example/"}, {"https://user:password@host.example"}, {"https://host.example/path"}, {"https://host.example?token=x"}, {"https://*.example"}, {"https://host.example#fragment"}, {"https://host.example:443"}, {"https://HOST.example"}, {"https://host.example:"}, {"https://host.example:06543"}, {"https://host.example:999999"}, {"https://host.example", "https://host.example"}} {
		if _, err := EmbeddedHTML(parents); err == nil {
			t.Fatalf("accepted unregistered/non-origin parents %v", parents)
		}
	}
	html, err := EmbeddedHTML([]string{"https://report-host.example"})
	if err != nil {
		t.Fatal(err)
	}
	if html == HTML() || !strings.Contains(html, `const REPORT_APP_EMBEDDED_PARENTS=["https://report-host.example"];`) {
		t.Fatal("embedded resource did not select explicit registered entrypoint")
	}
	if path := os.Getenv("CHARTWORKS_REPORT_APP_EMBEDDED_HTML_OUT"); path != "" {
		if err := os.WriteFile(path, []byte(html), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
