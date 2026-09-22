// Package bffexample demonstrates the client-owned iframe boundary. It owns no
// Chartworks session or token minting: a server-side Pengui provider supplies a
// fresh scoped bearer for each request and browser credentials are never forwarded.
package bffexample

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/rendering"
)

// Binding is the already-authenticated browser identity/session coordinate.
type Binding struct{ Tenant, User, Session string }

// Authorizer verifies the client-owned browser session before any token lookup.
type Authorizer func(context.Context, *http.Request) (Binding, error)

// ScopedToken is a fresh bearer explicitly bound to the authenticated browser coordinate.
type ScopedToken struct {
	Bearer  string
	Binding Binding
}
type TokenProvider func(context.Context, Binding) (ScopedToken, error)

// Handler forwards a sealed render request and returns only static content.
type Handler struct {
	upstream  string
	client    *http.Client
	token     TokenProvider
	authorize Authorizer
	parents   string
}

// New constructs a strict client-owned iframe BFF example.
func New(upstream string, client *http.Client, authorize Authorizer, token TokenProvider, parents []string) (*Handler, error) {
	u, err := url.Parse(upstream)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || token == nil || authorize == nil {
		return nil, errors.New("bff: invalid configuration")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		return nil, errors.New("bff: HTTPS required")
	}
	for _, p := range parents {
		parent, parseErr := url.Parse(p)
		if parseErr != nil || parent.Scheme != "https" || parent.Hostname() == "" || parent.User != nil || parent.Path != "" || parent.RawQuery != "" || parent.Fragment != "" || strings.ContainsAny(p, " ;\r\n") {
			return nil, errors.New("bff: invalid parent")
		}
	}
	if len(parents) == 0 {
		return nil, errors.New("bff: parent required")
	}
	if client == nil {
		client = &http.Client{}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("bff: redirect refused") }
	return &Handler{upstream: strings.TrimSuffix(upstream, "/"), client: &copyClient, token: token, authorize: authorize, parents: strings.Join(parents, " ")}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/iframe/render" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	binding, err := h.authorize(r.Context(), r)
	if err != nil || binding.Tenant == "" || binding.User == "" || binding.Session == "" {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	var in rendering.Request
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF || (in.Format != "html" && in.Format != "svg") {
		http.Error(w, "invalid request", 400)
		return
	}
	token, err := h.token(r.Context(), binding)
	if err != nil || token.Binding != binding || token.Bearer == "" || strings.ContainsAny(token.Bearer, " \r\n\t") {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstream+"/v1/reporting/renditions", bytes.NewReader(raw))
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token.Bearer)
	req.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(req)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		http.Error(w, "upstream rejected", response.StatusCode)
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 20<<20+1))
	if err != nil || len(body) > 20<<20 {
		http.Error(w, "invalid upstream response", http.StatusBadGateway)
		return
	}
	var out rendering.Rendition
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid upstream response", http.StatusBadGateway)
		return
	}
	wantMedia := map[string]string{"html": "text/html; charset=utf-8", "svg": "image/svg+xml"}[in.Format]
	digest := sha256.Sum256([]byte(out.Content))
	heightOK := out.Height == in.Height || in.Full && in.Format == "svg" && out.Height > 0 && out.Height <= 1_000_000 && strings.Contains(out.Content, "height=\""+strconv.Itoa(out.Height)+"\"")
	if out.Content == "" || out.State != "succeeded" || out.Version != rendering.Version || out.Format != in.Format || out.MediaType != wantMedia || out.Theme != in.Theme || out.Width != in.Width || !heightOK || out.Bytes != len(out.Content) || out.Digest != hex.EncodeToString(digest[:]) {
		http.Error(w, "invalid upstream response", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors "+h.parents)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", out.MediaType)
	_, _ = io.WriteString(w, out.Content)
}
