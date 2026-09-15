// Package reportviewer bundles presentation code only. It has no tenant data,
// credential, store, source, model, listener or runtime-template dependency.
package reportviewer

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"strings"
)

// URI is immutable for this version of the shared reporting result contract.
const URI = "ui://chartworks/report-viewer/v1"

//go:embed app.js
var script string

//go:embed styles.css
var styles string

// HTML returns the same public-data-free document for every caller. Hashes bind
// only compiled assets; labels, results and authority never enter HTML strings.
func HTML() string {
	js := sha256.Sum256([]byte(script))
	css := sha256.Sum256([]byte(styles))
	policy := "default-src 'none'; script-src 'sha256-" + base64.StdEncoding.EncodeToString(js[:]) + "'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(css[:]) + "'; connect-src 'none'; img-src 'none'; font-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"
	return strings.Join([]string{
		"<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><meta http-equiv=\"Content-Security-Policy\" content=\"", policy,
		"\"><meta name=\"referrer\" content=\"no-referrer\"><title>Chartworks reporting</title><style>", styles,
		"</style></head><body><main id=\"report-viewer\" aria-label=\"Reporting viewer\"><p role=\"status\">Opening retained report…</p></main><script type=\"module\">", script,
		"</script></body></html>",
	}, "")
}
