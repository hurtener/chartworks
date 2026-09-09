package chartworks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type inProcessTokenKey struct{}

func TestInProcessBoundaryIsolation(t *testing.T) {
	var calls atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Context().Value(inProcessTokenKey{}) != nil {
			t.Error("ambient authority crossed the simulated wire")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Error("request deadline lost")
		}
		if r.RemoteAddr != "127.0.0.1:0" || r.RequestURI != "/private-mount/v1/retention-policy" {
			t.Error("not a server-side request")
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("provider authority was not forwarded")
		}
		_, _ = io.WriteString(w, `{"revision":1,"audit_days":30,"operation_hours":24}`)
	})
	provider := func(ctx context.Context) (string, error) {
		token, _ := ctx.Value(inProcessTokenKey{}).(string)
		return token, nil
	}
	client, err := NewInProcessWithOptions(handler, provider, InProcessOptions{BasePath: "/private-mount/", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), inProcessTokenKey{}, "synthetic-caller")
	policy, err := client.RetentionPolicy(ctx)
	if err != nil || policy.Revision != 1 || calls.Load() != 1 {
		t.Fatal("bounded in-process call", policy, err)
	}
	if _, err = client.RetentionPolicy(t.Context()); err == nil || calls.Load() != 1 {
		t.Fatal("previous authority reused", err)
	}
}

func TestInProcessConcurrentProvidersDoNotShareAuthority(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(inProcessTokenKey{}) != nil {
			t.Error("per-caller token leaked through context")
		}
		_, _ = io.WriteString(w, r.Header.Get("Authorization"))
	})
	client, err := NewInProcess(handler, func(ctx context.Context) (string, error) {
		token, _ := ctx.Value(inProcessTokenKey{}).(string)
		return token, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := range 24 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			token := fmt.Sprintf("synthetic-%d", i)
			ctx := context.WithValue(t.Context(), inProcessTokenKey{}, token)
			for range 3 {
				var result string
				if err := client.call(ctx, "GET", "/v1/unit-transport", "", nil, &result); err != nil || result != "Bearer "+token {
					t.Error("per-call authority mixed", err)
				}
			}
		}()
	}
	wait.Wait()
}

func TestInProcessCancellationJoinsHandler(t *testing.T) {
	var returned atomic.Bool
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		returned.Store(true)
	})
	client, err := NewInProcessWithOptions(handler, func(context.Context) (string, error) { return "synthetic", nil }, InProcessOptions{Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.RetentionPolicy(t.Context())
	if !errors.Is(err, context.DeadlineExceeded) || !returned.Load() {
		t.Fatal("detached work or lost cancellation", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	returned.Store(false)
	if _, err = client.RetentionPolicy(ctx); !errors.Is(err, context.Canceled) || returned.Load() {
		t.Fatal("already canceled call dispatched", err)
	}
}

func TestInProcessResponseBoundsAndHeaderCommit(t *testing.T) {
	transport := inProcessTransport{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Receipt", "committed")
		w.WriteHeader(200)
		w.Header().Set("X-Receipt", "too-late")
		w.WriteHeader(403)
		_, _ = io.WriteString(w, "bounded")
	})}
	request, err := http.NewRequestWithContext(t.Context(), "GET", "http://127.0.0.1/v1/unit", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "bounded" || response.Header.Get("X-Receipt") != "committed" || response.StatusCode != 200 {
		t.Fatal("HTTP response commit diverged", err)
	}
	for _, status := range []int{99, 101, 600} {
		transport.handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		if _, err := transport.RoundTrip(request); err == nil {
			t.Fatal("invalid status accepted", status)
		}
	}
	transport.handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(103)
		w.(http.Flusher).Flush()
	})
	response, err = transport.RoundTrip(request)
	if err != nil || response.StatusCode != 200 || response.ContentLength != 0 {
		t.Fatal("bounded flush", err)
	}
	_ = response.Body.Close()
	transport.handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	response, err = transport.RoundTrip(request)
	if err != nil || response.StatusCode != 200 {
		t.Fatal("implicit empty success", err)
	}
	_ = response.Body.Close()
	transport.handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, inProcessResponseLimit+1))
		_, _ = w.Write([]byte("must-not-grow"))
	})
	if _, err := transport.RoundTrip(request); err == nil {
		t.Fatal("unbounded in-process response")
	}
	writer := &boundedResponse{header: make(http.Header)}
	writer.WriteHeader(http.StatusNoContent)
	if _, err := writer.Write([]byte("forbidden")); !errors.Is(err, http.ErrBodyNotAllowed) {
		t.Fatal("204 body accepted")
	}
	writer = &boundedResponse{header: make(http.Header), head: true}
	if n, err := writer.Write([]byte("omitted")); err != nil || n != 7 || writer.body.Len() != 0 {
		t.Fatal("HEAD retained a body", n, err)
	}
}

func TestClientMountAndTimeoutValidation(t *testing.T) {
	provider := func(context.Context) (string, error) { return "synthetic", nil }
	for _, base := range []string{"https://example.com/prefix", "https://example.com/prefix/v1/", "http://127.0.0.1:8000/chartworks", "http://[::1]/chartworks", "https://example.com/"} {
		client, err := New(base, nil, provider)
		if err != nil || client.http.Timeout != DefaultRequestTimeout {
			t.Fatal("safe configured mount rejected", err)
		}
	}
	for _, base := range []string{"https://example.com/a//b", "https://example.com/.", "https://example.com/..", "https://example.com/%2e%2e", "https://example.com/a%2fb", "https://example.com/?", "https://example.com/#", " https://example.com", "https://example.com/a\\b", "https://example.com/" + strings.Repeat("a", 65), "http://localhost"} {
		if _, err := New(base, nil, provider); err == nil {
			t.Fatal("unsafe backend mount accepted")
		}
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Jar: jar}
	client, err := New("https://example.com", httpClient, provider)
	if err != nil || client.http.Jar != nil || httpClient.Jar == nil || httpClient.Timeout != 0 {
		t.Fatal("caller HTTP client mutated or cookies retained", err)
	}
	for _, timeout := range []time.Duration{-1, MaximumRequestTimeout + 1} {
		if _, err := New("https://example.com", &http.Client{Timeout: timeout}, provider); err == nil {
			t.Fatal("unbounded timeout accepted")
		}
	}
	if _, err := NewInProcess(nil, provider); err == nil {
		t.Fatal("missing handler accepted")
	}
	if _, err := NewInProcessWithOptions(http.NotFoundHandler(), provider, InProcessOptions{BasePath: "relative"}); err == nil {
		t.Fatal("relative mount accepted")
	}
	for _, broken := range []*Client{nil, {}, {http: &http.Client{}}} {
		if _, err := broken.RetentionPolicy(t.Context()); err == nil {
			t.Fatal("invalid client panicked or succeeded")
		}
	}
	if _, err := client.RetentionPolicy(nil); err == nil {
		t.Fatal("missing context accepted")
	}
	for _, request := range []*http.Request{nil, {}, {URL: &url.URL{Scheme: "https", Host: "127.0.0.1"}}, {URL: &url.URL{Scheme: "http", Host: "other.example"}}} {
		if _, err := (inProcessTransport{handler: http.NotFoundHandler()}).RoundTrip(request); err == nil {
			t.Fatal("foreign in-process route accepted")
		}
	}
}

func TestClientCredentialsAndResponseCancellationStaySanitized(t *testing.T) {
	var calls atomic.Int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); _, _ = io.WriteString(w, "{}") })
	for _, token := range []string{strings.Repeat("x", (64<<10)+1), "a\x00b", "a,b", "a\tb"} {
		client, err := NewInProcess(handler, func(context.Context) (string, error) { return token, nil })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = client.RetentionPolicy(t.Context()); err == nil || strings.Contains(err.Error(), token) {
			t.Fatal("invalid credential accepted or echoed", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid provider value reached handler")
	}
	client, err := NewInProcess(handler, func(ctx context.Context) (string, error) { <-ctx.Done(); return "", errors.New("PRIVATE_PROVIDER") })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, err = client.RetentionPolicy(ctx); !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("provider deadline lost or diagnostic echoed", err)
	}
}
