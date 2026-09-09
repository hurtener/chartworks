package chartworks

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSafety(t *testing.T) {
	provider := func(context.Context) (string, error) { return "synthetic-token", nil }
	for _, base := range []string{"", "://bad", "https://u:p@example.com", "https://example.com/path/../escape", "https://example.com?secret=x", "http://example.com", "https://example.com#fragment"} {
		if _, err := New(base, nil, provider); err == nil {
			t.Fatal("unsafe client URL")
		}
	}
	if _, err := New("https://example.com", nil, nil); err == nil {
		t.Fatal("nil credential source")
	}
	count := 0
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { count++ }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer redirect.Close()
	c, err := New(redirect.URL, nil, provider)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.RetentionPolicy(context.Background()); err == nil || count != 0 {
		t.Fatal("credential followed redirect")
	}
	for _, value := range []string{"", "token\nsecret", "token other"} {
		bad := value
		c, err = New("https://example.com", nil, func(context.Context) (string, error) { return bad, nil })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.RetentionPolicy(context.Background()); err == nil {
			t.Fatal("invalid token")
		}
	}
	c, err = New("https://example.com", nil, func(context.Context) (string, error) { return "", errors.New("PRIVATE_SECRET") })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.RetentionPolicy(context.Background()); err == nil || strings.Contains(err.Error(), "PRIVATE_SECRET") {
		t.Fatal("provider error exposed")
	}
	for _, response := range []string{"bad-json", strings.Repeat("x", (1<<20)+1)} {
		body := response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
		c, err = New(server.URL, server.Client(), provider)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.RetentionPolicy(context.Background()); err == nil {
			t.Fatal("bad response accepted")
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "SECRET")
	}))
	defer server.Close()
	c, err = New(server.URL, server.Client(), provider)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.RetentionPolicy(context.Background())
	var status *StatusError
	if !errors.As(err, &status) || status.Status != 403 || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("bad error projection")
	}
	if _, err = c.Sweep(context.Background(), ""); err == nil {
		t.Fatal("empty operation key")
	}
}
