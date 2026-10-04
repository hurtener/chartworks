// Package reportapp bundles the optional manual report app. It owns no identity,
// persistence, source execution or host admission; its adapters call public tools.
package reportapp

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	reportviewer "github.com/hurtener/chartworks/web/report-viewer"
)

const URI = "ui://chartworks/report-app/v1"

const maxResourceBytes = 256 << 10

//go:embed generated/report-app.js
var appScript string

//go:embed styles.css
var appStyles string

// The deterministic build consumes readable authored modules, including the
// canonical retained-output presentation. Regenerate with npm run build; tests
// reject stale generated assets. No compiler or network is used at runtime.
func compiledAssets() (string, string) {
	_, css := reportviewer.Assets()
	return appScript, css + "\n" + appStyles
}

// HTML is the explicit MCP Apps entrypoint. It never falls back to embedding.
func HTML() string { script, css := compiledAssets(); return htmlDocument(script, css) }

// EmbeddedHTML selects the independent embedded adapter for explicitly registered
// parent origins. Only public routing metadata is compiled into this resource;
// identities, report definitions, grants and credentials are never arguments.
// The serving route must also set an equivalent frame-ancestors response header.
func EmbeddedHTML(parents []string) (string, error) {
	if len(parents) < 1 || len(parents) > 16 {
		return "", errors.New("invalid registered parent origins")
	}
	seen := map[string]bool{}
	for _, origin := range parents {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.Host != strings.ToLower(u.Host) || strings.HasSuffix(u.Host, ":") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.ForceQuery || u.String() != origin || strings.ContainsAny(origin, "*\\\r\n\t ") || seen[origin] {
			return "", errors.New("invalid registered parent origin")
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
				return "", errors.New("invalid registered parent port")
			}
		}
		if u.Port() == "443" {
			return "", errors.New("registered parent origin must be canonical")
		}
		seen[origin] = true
	}
	encoded, err := json.Marshal(parents)
	if err != nil {
		return "", err
	}
	script, css := compiledAssets()
	script = "const REPORT_APP_EMBEDDED_PARENTS=" + string(encoded) + ";\n" + script
	html := htmlDocument(script, css)
	if len(html) > maxResourceBytes {
		return "", errors.New("registered parent origins exceed report app resource limit")
	}
	return html, nil
}

func htmlDocument(script, css string) string {
	jsHash, cssHash := sha256.Sum256([]byte(script)), sha256.Sum256([]byte(css))
	policy := "default-src 'none'; script-src 'sha256-" + base64.StdEncoding.EncodeToString(jsHash[:]) + "'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(cssHash[:]) + "'; connect-src 'none'; img-src 'none'; font-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"
	return "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><meta http-equiv=\"Content-Security-Policy\" content=\"" + policy + "\"><meta name=\"referrer\" content=\"no-referrer\"><title>Chartworks report app</title><style>" + css + "</style></head><body><main id=\"report-app\" aria-label=\"Chartworks report app\"><p role=\"status\">Opening reports…</p></main><script type=\"module\">" + script + "</script></body></html>"
}
