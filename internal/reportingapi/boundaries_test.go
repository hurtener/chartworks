package reportingapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestReportingQueryBoundaries(t *testing.T) {
	declared := []api.Parameter{{Name: "draft"}, {Name: "revision"}}
	for _, q := range []url.Values{{"draft": {"true", "false"}}, {"unknown": {"1"}}, {"draft": {"true"}, "revision": {"1"}, "extra": {"x"}}} {
		if validQuery(q, declared) {
			t.Fatal("undeclared or ambiguous query admitted", q)
		}
	}
	if !validQuery(url.Values{}, declared) || !validQuery(url.Values{"draft": {"false"}}, declared) {
		t.Fatal("valid query rejected")
	}
	for _, raw := range []string{"revision=0", "revision=257", "revision=01", "revision=-1", "revision=x", "draft=1", "draft=true&revision=1"} {
		q, _ := url.ParseQuery(raw)
		if _, err := reference(q); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"", "draft=true", "draft=false", "revision=256"} {
		q, _ := url.ParseQuery(raw)
		if _, err := reference(q); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{"limit=0", "limit=101", "limit=01", "limit=x", "include_drafts=1"} {
		q, _ := url.ParseQuery(raw)
		if _, err := listRequest(q); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal(raw, err)
		}
	}
	q, _ := url.ParseQuery("limit=100&after=block&include_drafts=true")
	out, err := listRequest(q)
	if err != nil || out.Limit != 100 || out.After != "block" || !out.IncludeDrafts {
		t.Fatal(out, err)
	}
	out, err = listRequest(url.Values{})
	if err != nil || out.Limit != 20 {
		t.Fatal(out, err)
	}
}

func TestReportingClosedBodyAndSafeErrors(t *testing.T) {
	type request struct {
		Version int `json:"version"`
	}
	schema, err := api.SchemaFor("reportingBoundary", reflect.TypeFor[request](), false)
	if err != nil {
		t.Fatal(err)
	}
	d := api.Definition{Request: schema}
	d.MaxBodyBytes = 64
	for _, tc := range []struct {
		media, body string
		want        error
	}{
		{"application/json", `{"version":1}`, nil},
		{"", `{"version":1}`, reporting.ErrInvalid},
		{"application/json;charset=utf-8", `{"version":1}`, reporting.ErrInvalid},
		{"text/plain", `{"version":1}`, reporting.ErrInvalid},
		{"application/json", `{"version":null}`, reporting.ErrInvalid},
		{"application/json", `{"version":1,"version":2}`, reporting.ErrInvalid},
		{"application/json", `{"version":1,"tenant":"injected"}`, reporting.ErrInvalid},
		{"application/json", strings.Repeat(" ", 65), exec.ErrLimit},
	} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.media)
		var out request
		if err := decode(httptest.NewRecorder(), r, d, &out); !errors.Is(err, tc.want) {
			t.Fatal(tc, err)
		}
	}
	for _, tc := range []struct {
		err  error
		code int
	}{
		{access.ErrUnauthenticated, 401}, {access.ErrForbidden, 403}, {nlqexec.ErrInspectionRequired, 403},
		{access.ErrNotFound, 404}, {store.ErrNotFound, 404}, {nlqexec.ErrForeignSession, 404},
		{reporting.ErrInvalid, 400}, {store.ErrInvalid, 400}, {nlqexec.ErrInvalid, 400},
		{store.ErrConflict, 409}, {nlqexec.ErrNoPlan, 409}, {reporting.ErrStale, 409}, {exec.ErrBinding, 409},
		{exec.ErrLimit, 413}, {exec.ErrQuery, 422}, {reporting.ErrBusy, 429},
		{context.Canceled, 504}, {context.DeadlineExceeded, 504}, {errors.New("SQL-credential-canary"), 503},
	} {
		w := httptest.NewRecorder()
		headers(w)
		failure(w, tc.err)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "canary") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(tc, w.Code, w.Body.String())
		}
	}
	for _, h := range []http.Handler{Handler(nil, nil, nil), Handler(nil, nil, http.NotFoundHandler())} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/v1/blocks", nil))
		if w.Code != 404 {
			t.Fatal(w.Code)
		}
	}
}
