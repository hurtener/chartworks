package acceptance

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	sdk "github.com/hurtener/chartworks/internal/gateway/bifrost"
)

type gatewayFixture struct {
	engine              *sdk.Engine
	cfg                 config.Gateway
	token               *tokenFixture
	call                gateway.Call
	candidates          gateway.Candidates
	schema              *gateway.Schema
	mode                atomic.Value
	requests            atomic.Int64
	mu                  sync.Mutex
	models, keys, paths []string
	ca                  string
	server              *httptest.Server
}

func newGatewayFixture(t *testing.T, change func(*config.Gateway)) *gatewayFixture {
	t.Helper()
	f := &gatewayFixture{token: newTokenFixture(t)}
	f.mode.Store("normal")
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		defer func() { _ = r.Body.Close() }()
		data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		var input map[string]any
		if json.Unmarshal(data, &input) != nil {
			w.WriteHeader(400)
			return
		}
		model, _ := input["model"].(string)
		f.mu.Lock()
		f.models = append(f.models, model)
		f.keys = append(f.keys, r.Header.Get("Authorization"))
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		mode := f.mode.Load().(string)
		if mode == "error" {
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"error":{"message":"PROVIDER_ERROR_CANARY_SECRET"}}`)
			return
		}
		if mode == "delay" {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
		switch {
		case strings.Contains(r.URL.Path, "embedding"):
			var texts []any
			switch v := input["input"].(type) {
			case []any:
				texts = v
			case string:
				texts = []any{v}
			}
			result := []map[string]any{}
			for i := range texts {
				result = append(result, map[string]any{"object": "embedding", "index": i, "embedding": []float64{float64(i + 1), float64(i + 2)}})
			}
			if len(result) == 0 {
				w.WriteHeader(400)
				return
			}
			switch mode {
			case "reversed":
				for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
					result[i], result[j] = result[j], result[i]
				}
			case "missing":
				result = result[:len(result)-1]
			case "extra":
				result = append(result, map[string]any{"index": len(result), "embedding": []float64{0, 1}})
			case "duplicate":
				if len(result) > 1 {
					result[1]["index"] = 0
				}
			case "index":
				result[0]["index"] = -1
			case "dimension":
				result[0]["embedding"] = []float64{1}
			case "overflow":
				result[0]["embedding"] = []float64{1e40, 2}
			case "model":
				model = "different-embedding-space"
			case "nonfinite":
				_, _ = io.WriteString(w, `{"model":"embedding-model","data":[{"index":0,"embedding":[1e400,2]}]}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "model": model, "data": result, "usage": map[string]any{"prompt_tokens": 3, "total_tokens": 3}})
		case strings.Contains(r.URL.Path, "rerank"):
			documents, _ := input["documents"].([]any)
			results := []map[string]any{}
			for i := range documents {
				score := float64(len(documents) - i)
				if mode == "ties" {
					score = 1
				}
				results = append(results, map[string]any{"index": i, "relevance_score": score})
			}
			switch mode {
			case "missing":
				if len(results) > 0 {
					results = results[:len(results)-1]
				}
			case "duplicate":
				if len(results) > 1 {
					results[1]["index"] = 0
				}
			case "index":
				if len(results) > 0 {
					results[0]["index"] = 10000
				}
			case "nonfinite":
				_, _ = io.WriteString(w, `{"results":[{"index":0,"relevance_score":1e400}]}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "recorded-rerank", "model": model, "results": results, "usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 0, "total_tokens": 4}})
		default:
			content := `{"summary":"fixture result"}`
			finish := "stop"
			if strings.Contains(model, "visual_rank") {
				content = `{"order":["second","first"]}`
			}
			switch mode {
			case "plain":
				content = "not JSON"
			case "schema":
				content = `{"unexpected":true}`
			case "duplicate_json":
				content = `{"summary":"first","summary":"second"}`
			case "truncated":
				finish = "length"
			case "visual_bad":
				content = `{"order":["foreign","first"]}`
			case "oversize":
				content = `{"summary":"` + strings.Repeat("x", 2048) + `"}`
			}
			message := map[string]any{"role": "assistant", "content": content}
			if mode == "tool" {
				message["tool_calls"] = []any{map[string]any{"id": "tool-call", "type": "function", "function": map[string]any{"name": "execute_sql", "arguments": "{}"}}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "recorded-chat", "object": "chat.completion", "model": model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}})
		}
	}))
	t.Cleanup(f.server.Close)
	f.ca = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.server.TLS.Certificates[0].Certificate[0]}))
	f.cfg = config.Defaults().Gateway
	f.cfg.Bifrost.Providers = []config.Provider{{Name: "primary", Type: "openrouter", APIKey: "env:PRIMARY_KEY", BaseURL: f.server.URL}, {Name: "secondary", Type: "openrouter", APIKey: "env:SECONDARY_KEY", BaseURL: f.server.URL}}
	f.cfg.Roles = map[string]config.Role{}
	for _, name := range config.RoleNames() {
		f.cfg.Roles[name] = config.Role{Enabled: true, Provider: "primary", Model: "model-" + name, Timeout: config.Duration(2 * time.Second), MaxTokens: 64}
	}
	embedding := f.cfg.Roles["embedding"]
	embedding.Model = "embedding-model"
	embedding.ModelRevision = "generation-1"
	embedding.Dimensions = 2
	embedding.MaxBatchItems = 2
	embedding.MaxBatchBytes = 128
	f.cfg.Roles["embedding"] = embedding
	rerank := f.cfg.Roles["rerank"]
	rerank.MaxCandidates = 64
	rerank.OnFailure = "fail"
	f.cfg.Roles["rerank"] = rerank
	fix := f.cfg.Roles["sqlfix"]
	fix.Provider = "secondary"
	f.cfg.Roles["sqlfix"] = fix
	if change != nil {
		change(&f.cfg)
	}
	var err error
	f.engine, err = sdk.New(context.Background(), f.cfg, func(name string) (string, bool) {
		switch name {
		case "PRIMARY_KEY":
			return "SYNTHETIC_PRIMARY", true
		case "SECONDARY_KEY":
			return "SYNTHETIC_SECONDARY", true
		}
		return "", false
	}, sdk.TransportOptions{AllowPrivateNetwork: true, CACertPEM: f.ca})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.engine.Close)
	e := f.token.envelope(t, "gateway-tenant", "operator", "gateway.use", "cw.tenant.use:gateway-tenant", "cw.topic.read:first", "cw.topic.read:second")
	f.call, err = gateway.Authorize(e, "gateway.use", "actual-context-v1", access.Tenant(e, "use"))
	if err != nil {
		t.Fatal(err)
	}
	f.candidates, err = gateway.AdmitCandidates(f.call, "gateway.use", []gateway.Candidate{{ID: "first", Text: "first candidate", Resource: access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: "first"}}, {ID: "second", Text: "second candidate", Resource: access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: "second"}}})
	if err != nil {
		t.Fatal(err)
	}
	f.schema, err = gateway.NewSchema("summary", []byte(`{"type":"object","additionalProperties":false,"required":["summary"],"properties":{"summary":{"type":"string","maxLength":256}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func gatewayBudget(t *testing.T, call gateway.Call, calls int) *gateway.Budget {
	t.Helper()
	b, err := gateway.NewBudget(call, gateway.Limits{Calls: calls, Tokens: 4 << 20, Duration: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func (f *gatewayFixture) generate(t *testing.T, role string) (gateway.Generated, error) {
	t.Helper()
	return f.engine.Generate(context.Background(), f.call, gatewayBudget(t, f.call, 8), role, "Trusted task instructions", "untrusted user input", f.schema)
}
func (f *gatewayFixture) recordString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fmt.Sprint(f.models, f.paths)
}
