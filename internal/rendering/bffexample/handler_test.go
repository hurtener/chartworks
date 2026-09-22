package bffexample

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserBindingPrecedesTokenAndMustMatch(t *testing.T) {
	called := false
	request := []byte(`{"view":{"kind":"block","run":"run","output":"out"},"format":"html","theme":"light","width":800,"height":400}`)
	denied, _ := New("https://chartworks.example", nil, func(context.Context, *http.Request) (Binding, error) { return Binding{}, errors.New("no session") }, func(context.Context, Binding) (ScopedToken, error) { called = true; return ScopedToken{}, nil }, []string{"https://client.example"})
	rec := httptest.NewRecorder()
	denied.ServeHTTP(rec, httptest.NewRequest("POST", "/iframe/render", bytes.NewReader(request)))
	if rec.Code != http.StatusUnauthorized || called {
		t.Fatal(rec.Code, called)
	}
	binding := Binding{Tenant: "t", User: "u", Session: "s"}
	mismatch, _ := New("https://chartworks.example", nil, func(context.Context, *http.Request) (Binding, error) { return binding, nil }, func(context.Context, Binding) (ScopedToken, error) {
		return ScopedToken{Bearer: "fresh", Binding: Binding{Tenant: "other", User: "u", Session: "s"}}, nil
	}, []string{"https://client.example"})
	rec = httptest.NewRecorder()
	mismatch.ServeHTTP(rec, httptest.NewRequest("POST", "/iframe/render", bytes.NewReader(request)))
	if rec.Code != http.StatusBadGateway {
		t.Fatal(rec.Code)
	}
}
