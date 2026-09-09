package chartapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/gateway"
)

func TestTypedErrorsAndClosedDecode(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{errors.New("private SQL secret"), 503}, {access.ErrNotFound, 404}, {charts.ErrMappingChanged, 409}, {charts.ErrUnsuitable, 422}, {context.Canceled, 504}, {chartservice.ErrBusy, 429}, {charts.ErrLimit, 413}, {charts.ErrInvalid, 400}, {access.ErrForbidden, 403}, {access.ErrUnauthenticated, 401}} {
		w := httptest.NewRecorder()
		failure(w, test.err)
		if w.Code != test.status || strings.Contains(w.Body.String(), "private SQL") {
			t.Fatal("unsafe error", w.Code)
		}
	}
	w := httptest.NewRecorder()
	failure(w, &chartservice.Failure{Cause: context.DeadlineExceeded, Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: "visual_rank", Attempts: 1}}}})
	if w.Code != 504 || !strings.Contains(w.Body.String(), `"receipt"`) {
		t.Fatal("late usage lost")
	}
	registry, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := registry.Match("POST", "/v1/charts/select")
	for _, r := range []*http.Request{
		httptest.NewRequest("POST", d.Path, strings.NewReader(`{}`)),
		httptest.NewRequest("POST", d.Path, brokenReader{}),
	} {
		r.Header.Set("Content-Type", "application/json")
		if decode(httptest.NewRecorder(), r, d, &chartservice.SelectRequest{}) == nil {
			t.Fatal("broken body accepted")
		}
	}
	Handler(nil, nil, http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/v1/charts/catalog", nil))
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
