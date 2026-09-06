package foundation

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

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
}

// NewServer rejects missing real probes, rather than treating nil dependencies as healthy.
func NewServer(cfg config.Config, r *telemetry.Reporter, store, keys Checker, protected ...http.Handler) (*Server, error) {
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
	return &Server{values: cfg.Values(), reporter: r, store: store, keys: keys, state: map[string]Dependency{}, protected: h}, nil
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

// Handler serves public content-free health and delegates operational routes to the verifier/enforcer.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
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
			response = struct {
				Live bool `json:"live"`
			}{true}
		case "/readyz":
			ready, states := s.readiness()
			if !ready {
				status = http.StatusServiceUnavailable
			}
			response = struct {
				Ready        bool              `json:"ready"`
				Dependencies map[string]string `json:"dependencies"`
			}{ready, states}
		case "/capabilities":
			response = struct {
				Phase          string   `json:"phase"`
				Implemented    []string `json:"implemented"`
				BusinessAPI    bool     `json:"business_api"`
				Authentication bool     `json:"authentication"`
			}{"01-12-engineering", s.implemented(), false, s.protected != nil}
		}
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			if e := json.NewEncoder(w).Encode(response); e != nil {
				_ = s.reporter.Record(r.Context(), telemetry.Request, false)
			}
		}
	})
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
	if s.protected != nil {
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
	return out
}
