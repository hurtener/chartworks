// Package bffexample demonstrates the client-owned iframe boundary. It owns no
// Chartworks session or token minting: a server-side Pengui provider supplies a
// fresh scoped bearer for each request and browser credentials are never forwarded.
package bffexample

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/hurtener/chartworks/internal/rendering"
)

type TokenProvider func(context.Context) (string, error)
type Handler struct {
	upstream string
	client   *http.Client
	token    TokenProvider
	parents  string
}

func New(upstream string, client *http.Client, token TokenProvider, parents []string) (*Handler, error) {
	u, err := url.Parse(upstream)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || token == nil {
		return nil, errors.New("bff: invalid configuration")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")) {
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
	return &Handler{strings.TrimSuffix(upstream, "/"), &copyClient, token, strings.Join(parents, " ")}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/iframe/render" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
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
	token, err := h.token(r.Context())
	if err != nil || token == "" || strings.ContainsAny(token, " \r\n\t") {
		http.Error(w, "upstream unavailable", 502)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.upstream+"/v1/reporting/renditions", bytes.NewReader(raw))
	if err != nil {
		http.Error(w, "upstream unavailable", 502)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(req)
	if err != nil {
		http.Error(w, "upstream unavailable", 502)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		http.Error(w, "upstream rejected", response.StatusCode)
		return
	}
	var out rendering.Rendition
	if json.NewDecoder(io.LimitReader(response.Body, 20<<20)).Decode(&out) != nil || out.Content == "" {
		http.Error(w, "invalid upstream response", 502)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors "+h.parents)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", out.MediaType)
	_, _ = io.WriteString(w, out.Content)
}
