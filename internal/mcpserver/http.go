package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
)

// Handler verifies a fresh MCP-audience bearer on every HTTP request. It never
// trusts cookies, forwarding headers, resource URIs or transport session IDs.
func (s *Server) Handler() http.Handler {
	protected := s.verifier.Middleware(auth.MCP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := s.admit(r.Context(), func(ctx context.Context) error { return s.serve(w, r.WithContext(ctx)) })
		if err != nil {
			writeError(w, httpError(err))
		}
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers(w)
		if r.URL.Path != Path {
			writeError(w, wireError{404, "not_found"})
			return
		}
		protected.ServeHTTP(w, r)
	})
}
func (s *Server) serve(w http.ResponseWriter, r *http.Request) (err error) {
	defer func() {
		if recover() != nil {
			writeError(w, wireError{503, "unavailable"})
			err = nil
		}
	}()
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	}
	if !s.allowedOrigin(r) || !s.allowedHost(r.Host) {
		writeError(w, wireError{403, "forbidden_origin"})
		return nil
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.Header.Get("Content-Encoding") != "" || len(r.Header.Values("Mcp-Session-Id")) > 0 || len(r.Header.Values("Last-Event-Id")) > 0 || len(r.Header.Values("Mcp-Protocol-Version")) > 1 {
		writeError(w, wireError{400, "invalid_request"})
		return nil
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
		writeError(w, wireError{415, "unsupported_media_type"})
		return nil
	}
	if !accepted(r.Header.Values("Accept")) {
		writeError(w, wireError{400, "invalid_request"})
		return nil
	}
	version := r.Header.Get("Mcp-Protocol-Version")
	if version != "" && version != "2025-11-25" && version != "2025-06-18" && version != "2025-03-26" && version != "2024-11-05" {
		writeError(w, wireError{400, "invalid_request"})
		return nil
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(s.settings.MaxRequestBytes)))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			writeError(w, wireError{413, "limit_exceeded"})
		} else {
			writeError(w, wireError{400, "invalid_request"})
		}
		return nil
	}
	if requestSchema.Validate(body, s.settings.MaxRequestBytes) != nil {
		writeError(w, wireError{400, "invalid_request"})
		return nil
	}
	// Retain only protocol headers before entering the SDK. The verified envelope
	// contains no bearer bytes; SDK request extras therefore cannot retain them.
	cloned := r.Clone(context.WithValue(r.Context(), requestKey{}, r.Context()))
	cloned.Header = make(http.Header)
	for _, name := range []string{"Content-Type", "Accept", "Mcp-Protocol-Version"} {
		for _, v := range r.Header.Values(name) {
			cloned.Header.Add(name, v)
		}
	}
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))
	buffered := newBoundedResponse(s.settings.MaxResponseBytes)
	s.transport.ServeHTTP(buffered, cloned)
	status, payload, tooLarge := buffered.result()
	if tooLarge {
		writeError(w, wireError{413, "limit_exceeded"})
		return nil
	}
	if status == http.StatusAccepted {
		w.WriteHeader(status)
		return nil
	}
	if status != http.StatusOK {
		writeError(w, safeTransportError(status))
		return nil
	}
	if len(payload) == 0 {
		writeError(w, wireError{504, "cancelled_or_timed_out"})
		return nil
	}
	payload = sanitizeProtocol(payload, s.settings.MaxResponseBytes)
	if payload == nil {
		writeError(w, wireError{503, "unavailable"})
		return nil
	}
	headers(w)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
	return nil
}

func accepted(values []string) bool {
	jsonOK, streamOK := false, false
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			media, params, err := mime.ParseMediaType(strings.TrimSpace(part))
			if err != nil {
				return false
			}
			if q, exists := params["q"]; exists {
				n, err := strconv.ParseFloat(q, 64)
				if err != nil || math.IsNaN(n) || n <= 0 || n > 1 {
					continue
				}
			}
			jsonOK = jsonOK || media == "application/json"
			streamOK = streamOK || media == "text/event-stream"
		}
	}
	return jsonOK && streamOK
}
func (s *Server) allowedOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	for _, origin := range s.origins {
		if origins[0] == origin {
			return true
		}
	}
	return false
}
func (s *Server) allowedHost(authority string) bool {
	host := authority
	if h, p, err := net.SplitHostPort(authority); err == nil {
		port, e := strconv.Atoi(p)
		if e != nil || port < 1 || port > 65535 {
			return false
		}
		host = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	host = strings.ToLower(host)
	if !config.MCPHost(host) {
		return false
	}
	for _, allowed := range s.settings.AllowedHosts {
		if host == allowed {
			return true
		}
	}
	return false
}

type wireError struct {
	status int
	code   string
}

func httpError(err error) wireError {
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		return wireError{401, "unauthenticated"}
	case errors.Is(err, access.ErrForbidden):
		return wireError{403, "forbidden"}
	case errors.Is(err, errBusy):
		return wireError{429, "busy"}
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return wireError{504, "cancelled_or_timed_out"}
	}
	return wireError{503, "unavailable"}
}
func safeTransportError(status int) wireError {
	switch status {
	case 400:
		return wireError{400, "invalid_request"}
	case 403:
		return wireError{403, "forbidden"}
	case 413:
		return wireError{413, "limit_exceeded"}
	case 415:
		return wireError{415, "unsupported_media_type"}
	}
	return wireError{503, "unavailable"}
}
func headers(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
}
func writeError(w http.ResponseWriter, e wireError) {
	headers(w)
	w.WriteHeader(e.status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{e.code})
}

// No SDK/native error message or data is allowed onto the HTTP wire. Normal
// successful raw JSON is retained verbatim, including lossless scalar numbers.
func sanitizeProtocol(payload []byte, limit int) []byte {
	if _, err := gateway.DecodeJSON(payload, limit); err != nil {
		return nil
	}
	var response map[string]json.RawMessage
	if json.Unmarshal(payload, &response) != nil {
		return nil
	}
	if raw, has := response["error"]; has {
		var e struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &e) != nil {
			return nil
		}
		safe := "unavailable"
		switch e.Code {
		case -32700, -32600, -32602:
			safe = "invalid_request"
		case -32601:
			safe = "not_found"
		}
		// Only our content-free, closed codes survive resource/registry failures.
		switch e.Message {
		case "not_found", "forbidden", "unauthenticated", "invalid_request", "unavailable", "busy", "limit_exceeded", "cancelled_or_timed_out", "context_changed", "conflict":
			safe = e.Message
		}
		response["error"], _ = json.Marshal(struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		}{e.Code, safe})
		encoded, err := json.Marshal(response)
		if err != nil {
			return nil
		}
		return encoded
	}
	return payload
}

type boundedResponse struct {
	mu            sync.Mutex
	header        http.Header
	body          bytes.Buffer
	status, limit int
	large         bool
}

func newBoundedResponse(limit int) *boundedResponse {
	return &boundedResponse{header: make(http.Header), limit: limit}
}
func (w *boundedResponse) Header() http.Header { return w.header }
func (w *boundedResponse) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = status
	}
}
func (w *boundedResponse) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = 200
	}
	if w.large || w.body.Len()+len(p) > w.limit {
		w.large = true
		return 0, errResult
	}
	return w.body.Write(p)
}
func (w *boundedResponse) Flush() {}
func (w *boundedResponse) result() (int, []byte, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	status := w.status
	if status == 0 {
		status = 200
	}
	return status, append([]byte(nil), w.body.Bytes()...), w.large
}
