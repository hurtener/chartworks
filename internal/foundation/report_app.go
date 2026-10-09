package foundation

import (
	"net/http"
	"strings"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/gateway"
	reportapp "github.com/hurtener/chartworks/web/report-app"
)

const reportAppEmbeddedPath = "/apps/report/v1/embedded"

// reportAppAssetRegistry exposes compiled presentation only. All data and
// mutations still use independently authenticated registered operations.
func reportAppAssetRegistry() (*api.Registry, error) {
	schema, err := gateway.NewSchema("reportAppHTML", []byte(`{"type":"string"}`))
	if err != nil {
		return nil, err
	}
	definitions := []api.Definition{}
	for _, method := range []string{"GET", "HEAD"} {
		id := "reportAppEmbedded"
		if method == "HEAD" {
			id += "Head"
		}
		definitions = append(definitions, api.Definition{Operation: api.Operation{Method: method, Path: reportAppEmbeddedPath, Effect: "public_read"}, ID: id, Summary: "Read data-free versioned embedded report application assets", Public: true, ResourceLoader: "compiled public assets and trusted configured parent origins only", Audit: "read_only_no_domain_audit", Response: schema, ResponseContentType: "text/html; charset=utf-8", Errors: []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 403, Code: "forbidden"}, {Status: 405, Code: "method_not_allowed"}}})
	}
	return api.New(definitions)
}

// reportAppAssetHandler uses explicitly registered embedded parent origins,
// independently from CORS clients. Origin values are configuration, never request/query coordinates.
// The bootstrap challenge correlates a frame; it supplies no authorization.
func reportAppAssetHandler(origins []string, next http.Handler) (http.Handler, error) {
	html, err := reportapp.EmbeddedHTML(origins)
	if err != nil {
		return nil, err
	}
	parents := strings.Join(origins, " ")
	if parents == "" {
		parents = "'none'"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != reportAppEmbeddedPath {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "frame-ancestors "+parents)
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(origins) == 0 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == "GET" {
			_, _ = w.Write([]byte(html))
		}
	}), nil
}
