package foundation

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedReportAppAssetHasExplicitRegisteredOrigin(t *testing.T) {
	registry, err := reportAppAssetRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range registry.Definitions() {
		if !d.Public || d.ResponseContentType != "text/html; charset=utf-8" || d.Path != reportAppEmbeddedPath {
			t.Fatal("asset registration mismatch")
		}
	}
	handler, err := reportAppAssetHandler([]string{"https://app.example.test"}, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", reportAppEmbeddedPath, 200}, {"HEAD", reportAppEmbeddedPath, 200}, {"POST", reportAppEmbeddedPath, 405}, {"GET", reportAppEmbeddedPath + "?parent=https://evil.example", 400}, {"GET", "/unknown", 404}} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatal(tc, w.Code)
		}
		if tc.status == 200 {
			if w.Header().Get("Content-Security-Policy") != "frame-ancestors https://app.example.test" {
				t.Fatal("frame boundary")
			}
			if tc.method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD leaked body")
			}
			if tc.method == "GET" && (!strings.Contains(w.Body.String(), "https://app.example.test") || !strings.Contains(w.Body.String(), "Chartworks report app")) {
				t.Fatal("wrong shared asset")
			}
		}
	}
	for _, origins := range [][]string{{"*"}, {"null"}, {"http://app.example.test"}, {"https://app.example.test/path"}, {"https://user:secret@app.example.test"}} {
		if _, err := reportAppAssetHandler(origins, http.NotFoundHandler()); err == nil {
			t.Fatal("unsafe parent", origins)
		}
	}
}
