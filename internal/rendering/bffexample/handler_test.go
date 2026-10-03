package bffexample

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/rendering"
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

func TestPNGReturnsValidatedBinaryUnderBrowserBinding(t *testing.T) {
	var pixels bytes.Buffer
	if err := png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 800, 400))); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(pixels.Bytes())
	out := rendering.Rendition{State: "succeeded", Version: rendering.Version, Format: "png", MediaType: "image/png", Theme: "light", Width: 800, Height: 400, Bytes: pixels.Len(), Digest: hex.EncodeToString(sum[:]), Content: base64.StdEncoding.EncodeToString(pixels.Bytes()), ContentEncoding: "base64"}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer scoped" {
			t.Error("missing scoped token")
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer server.Close()
	binding := Binding{Tenant: "t", User: "u", Session: "s"}
	h, err := New(server.URL, server.Client(), func(context.Context, *http.Request) (Binding, error) { return binding, nil }, func(context.Context, Binding) (ScopedToken, error) {
		return ScopedToken{Bearer: "scoped", Binding: binding}, nil
	}, []string{"https://client.example"})
	if err != nil {
		t.Fatal(err)
	}
	request := []byte(`{"view":{"kind":"block","run":"run","output":"out"},"format":"png","theme":"light","width":800,"height":400}`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/iframe/render", bytes.NewReader(request)))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), pixels.Bytes()) {
		t.Fatal("binary response invalid", rec.Code)
	}
	out.Digest = "tampered"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/iframe/render", bytes.NewReader(request)))
	if rec.Code != http.StatusBadGateway {
		t.Fatal("invalid content served", rec.Code)
	}
}

func TestBinaryTransportRejectsUntrustedResponsesAndRequests(t *testing.T) {
	binding := Binding{Tenant: "t", User: "u", Session: "s"}
	authorize := func(context.Context, *http.Request) (Binding, error) { return binding, nil }
	tokenCalls := 0
	token := func(context.Context, Binding) (ScopedToken, error) {
		tokenCalls++
		return ScopedToken{Bearer: "scoped", Binding: binding}, nil
	}
	for _, raw := range []string{`{"format":"png","full":true}`, `{"format":"pdf"}`, `{"format":"png","unknown":true}`, `{`} {
		h, _ := New("https://chartworks.example", nil, authorize, token, []string{"https://client.example"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/iframe/render", bytes.NewBufferString(raw)))
		if rec.Code != 400 || tokenCalls != 0 {
			t.Fatal("invalid request reached credential provider", rec.Code)
		}
	}
	request := []byte(`{"view":{"kind":"block","run":"run","output":"out"},"format":"png","theme":"light","width":800,"height":400}`)
	for _, tc := range []struct {
		name, body string
		status     int
	}{{"bad_json", "{", 200}, {"unknown_field", `{"unexpected":"field"}`, 200}, {"bad_identity", `{"format":"png","media_type":"image/png","content":"not-base64","content_encoding":"base64"}`, 200}, {"provider_denial", "denied", 403}, {"redirect", "", 302}} {
		t.Run(tc.name, func(t *testing.T) {
			up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status == 302 {
					w.Header().Set("Location", "https://example.invalid/blocked")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer up.Close()
			h, err := New(up.URL, up.Client(), authorize, token, []string{"https://client.example"})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("POST", "/iframe/render", bytes.NewReader(request)))
			want := 502
			if tc.status == 403 {
				want = 403
			}
			if rec.Code != want {
				t.Fatal("untrusted response served", rec.Code)
			}
		})
	}
	for _, up := range []string{"http://external.example", "https://user:pass@example.com", "https://example.com?token=x", "https://example.com#fragment"} {
		if _, err := New(up, nil, authorize, token, []string{"https://client.example"}); err == nil {
			t.Fatal("invalid upstream admitted")
		}
	}
	if _, err := New("https://example.com", nil, authorize, token, []string{"https://client.example/path"}); err == nil {
		t.Fatal("open parent policy accepted")
	}
	if _, err := New("https://example.com", nil, authorize, token, nil); err == nil {
		t.Fatal("missing parent policy accepted")
	}
}
