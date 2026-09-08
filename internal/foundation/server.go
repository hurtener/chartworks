package foundation

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/telemetry"
)

// Checker observes a dependency. Implementations must honor cancellation.
type Checker func(context.Context) Dependency

// Server serves only sanitized foundation health and implementation metadata.
type Server struct {
	values      config.Values
	reporter    *telemetry.Reporter
	store, keys Checker
	mu          sync.RWMutex
	state       map[string]Dependency
	protected   http.Handler
	registry    *api.Registry
}

// NewServer rejects missing real probes, rather than treating nil dependencies as healthy.
func NewServer(cfg config.Config, r *telemetry.Reporter, store, keys Checker, protected ...http.Handler) (*Server, error) {
	return newServer(cfg, r, store, keys, nil, protected...)
}

// NewServerWithRegistry is the phase 21 composition entry point. The registry
// drives public OpenAPI output without replacing any domain handler.
func NewServerWithRegistry(cfg config.Config, r *telemetry.Reporter, store, keys Checker, registry *api.Registry, protected ...http.Handler) (*Server, error) {
	return newServer(cfg, r, store, keys, registry, protected...)
}

func newServer(cfg config.Config, r *telemetry.Reporter, store, keys Checker, registry *api.Registry, protected ...http.Handler) (*Server, error) {
	if r == nil || store == nil || keys == nil || cfg.StoreDSN() == "" {
		return nil, errors.New("foundation: configuration, telemetry and dependency probes required")
	}
	var h http.Handler
	if len(protected) > 1 {
		return nil, errors.New("foundation: one protected router required")
	}
	if len(protected) == 1 {
		h = protected[0]
	}
	values := cfg.Values()
	if values.Server.BasePath == "" {
		values.Server.BasePath = "/"
	}
	return &Server{values: values, reporter: r, store: store, keys: keys, state: map[string]Dependency{}, protected: h, registry: registry}, nil
}
func (s *Server) observe(ctx context.Context, name string, check Checker, interval time.Duration) {
	refresh := func() {
		v := check(ctx)
		s.mu.Lock()
		previous := s.state[name]
		s.state[name] = v
		s.mu.Unlock()
		_ = s.reporter.Dependency(name, v.Ready)
		if previous.Ready != v.Ready {
			_ = s.reporter.Record(ctx, telemetry.DependencyChanged, v.Ready)
		}
	}
	refresh()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}
func (s *Server) readiness() (bool, map[string]string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]string{}
	ready := true
	now := time.Now()
	for _, name := range []string{"store", "verification_keys"} {
		v, ok := s.state[name]
		state := "unavailable"
		switch {
		case !ok:
			state = "starting"
		case !v.ValidUntil.IsZero() && !now.Before(v.ValidUntil):
			state = "stale"
		case v.Ready:
			state = "ready"
		}
		out[name] = state
		if state != "ready" {
			ready = false
		}
	}
	return ready, out
}

// Handler serves public health/document routes and delegates operational routes
// to the verifier/enforcer after the configured transport prefix is removed.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.originAllowed(w, r) {
			return
		}
		if r.Method == http.MethodOptions && r.Header.Get("Origin") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		path, ok := trimBasePath(s.values.Server.BasePath, r.URL.Path)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if path != r.URL.Path {
			clone := r.Clone(r.Context())
			u := *r.URL
			u.Path, u.RawPath = path, ""
			clone.URL = &u
			r = clone
		}
		s.handle(w, r)
	})
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/openapi.json" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.URL.RawQuery != "" {
			w.Header().Set("Connection", "close")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.registry == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		document, err := s.registry.OpenAPIAt("Chartworks HTTP API", "1", s.values.Server.BasePath)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(document)
		}
		return
	}
	if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && r.URL.Path != "/capabilities" {
		if s.protected != nil {
			s.protected.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		w.Header().Set("Connection", "close")
		if r.ContentLength > s.values.Server.MaxBodyBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}
	_ = s.reporter.Record(r.Context(), telemetry.Request, true)
	status := http.StatusOK
	var response any
	switch r.URL.Path {
	case "/healthz":
		response = HealthResponse{Live: true}
	case "/readyz":
		ready, states := s.readiness()
		if !ready {
			status = http.StatusServiceUnavailable
		}
		response = ReadyResponse{Ready: ready, Dependencies: states}
	case "/capabilities":
		response = CapabilitiesResponse{Phase: "01-21-http", Implemented: s.implemented(), BusinessAPI: s.businessAPI(), Authentication: s.protected != nil}
	}
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		if e := json.NewEncoder(w).Encode(response); e != nil {
			_ = s.reporter.Record(r.Context(), telemetry.Request, false)
		}
	}
}

func (s *Server) originAllowed(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	for _, allowed := range s.values.Server.CORSAllowlist {
		if origin == allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Add("Vary", "Origin")
			return true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden_origin"})
	return false
}

func trimBasePath(basePath, path string) (string, bool) {
	if basePath == "" || basePath == "/" {
		return path, true
	}
	if path == basePath {
		return "/", true
	}
	if strings.HasPrefix(path, basePath+"/") {
		return strings.TrimPrefix(path, basePath), true
	}
	return "", false
}

// Serve owns and joins the listener, HTTP server and two bounded dependency monitors.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return errors.New("foundation: listener required")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for _, p := range []struct {
		name     string
		check    Checker
		interval time.Duration
	}{{"store", s.store, time.Second}, {"verification_keys", s.keys, time.Duration(s.values.Auth.RefreshInterval)}} {
		wg.Add(1)
		go func(name string, check Checker, interval time.Duration) {
			defer wg.Done()
			s.observe(ctx, name, check, interval)
		}(p.name, p.check, p.interval)
	}
	cfg := s.values.Server
	h := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: time.Duration(cfg.ReadHeaderTimeout), ReadTimeout: time.Duration(cfg.ReadTimeout), WriteTimeout: time.Duration(cfg.WriteTimeout), IdleTimeout: time.Duration(cfg.IdleTimeout), MaxHeaderBytes: cfg.MaxHeaderBytes, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan error, 1)
	go func() { done <- h.Serve(listener) }()
	_ = s.reporter.Record(ctx, telemetry.Started, true)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-done:
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(cfg.ShutdownGrace))
	defer stop()
	err := h.Shutdown(shutdown)
	if err != nil {
		_ = h.Close()
	}
	if serveErr == nil {
		serveErr = <-done
	}
	wg.Wait()
	_ = s.reporter.Record(context.WithoutCancel(ctx), telemetry.Stopped, err == nil)
	if err != nil {
		return errors.New("foundation: shutdown failed")
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return errors.New("foundation: listener failed")
	}
	return nil
}

func (s *Server) implemented() []string {
	out := []string{"configuration", "health", "postgresql_metadata"}
	if s.registry != nil {
		out = append(out, "http_api", "openapi")
	}
	if s.businessAPI() {
		out = append(out, "jwt_verification", "signed_scope_enforcement", "operational_api")
	}
	if s.values.Features.Gateway {
		out = append(out, "remote_bifrost_gateway")
	}
	if s.values.Jobs.Enabled {
		out = append(out, "durable_operations", "scheduling")
	}
	if s.values.Sources.Enabled {
		out = append(out, "governed_sources", "validated_read_execution")
	}
	if s.values.Uploads.Enabled {
		out = append(out, "governed_uploads")
	}
	if s.values.Profiling.Enabled {
		out = append(out, "versioned_profile_evidence")
	}
	if s.values.Pipelines.Enabled {
		out = append(out, "managed_sql_pipelines")
	}
	return out
}

func (s *Server) businessAPI() bool {
	if s.protected == nil || s.registry == nil {
		return false
	}
	for _, definition := range s.registry.Definitions() {
		if !definition.Public {
			return true
		}
	}
	return false
}
