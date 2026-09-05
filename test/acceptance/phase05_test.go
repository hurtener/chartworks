package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase05(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		if f.requests.Load() != 0 {
			t.Fatal("constructor called a model")
		}
		for _, role := range config.RoleNames() {
			var err error
			switch role {
			case "embedding":
				_, err = f.engine.Embed(context.Background(), f.call, gatewayBudget(t, f.call, 8), f.engine.Space(), []string{"one", "two"})
			case "rerank":
				_, err = f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 8), "question", f.candidates)
			case "visual_rank":
				_, err = f.engine.VisualRank(context.Background(), f.call, gatewayBudget(t, f.call, 8), "question", f.candidates)
			default:
				_, err = f.generate(t, role)
			}
			if err != nil {
				t.Fatalf("actual SDK role %s: %v; routes %s", role, err, f.recordString())
			}
		}
		if f.requests.Load() != 10 {
			t.Fatalf("unexpected SDK request count %d", f.requests.Load())
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, model := range f.models {
			expected := "Bearer SYNTHETIC_PRIMARY"
			if model == "model-sqlfix" {
				expected = "Bearer SYNTHETIC_SECONDARY"
			}
			if f.keys[i] != expected {
				t.Fatal("wrong role credential or shared client")
			}
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		for _, mode := range []string{"plain", "schema", "duplicate_json", "truncated", "tool", "oversize"} {
			f.mode.Store(mode)
			out, err := f.generate(t, "sqlgen")
			if err == nil || len(out.JSON) != 0 {
				t.Fatalf("invalid structured output %s accepted", mode)
			}
		}
		f.mode.Store("error")
		out, err := f.generate(t, "sqlgen")
		if err == nil || strings.Contains(err.Error(), "CANARY") || len(out.Receipt.Calls) != 2 {
			t.Fatal("provider error leakage or hidden attempts")
		}
		if _, err := gateway.NewSchema("remote", []byte(`{"$ref":"https://untrusted.example/schema.json"}`)); err == nil {
			t.Fatal("remote schema reference accepted")
		}
		for _, raw := range []string{`{"n":1e999999}`, `{"a":1,"a":2}`, `{} {}`, `[null,]`} {
			if _, err := gateway.DecodeJSON([]byte(raw), 1024); err == nil {
				t.Fatal("malformed model JSON accepted")
			}
		}
	})
	t.Run("AC03", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		f.mode.Store("error")
		b := gatewayBudget(t, f.call, 1)
		before := f.requests.Load()
		out, err := f.engine.Generate(context.Background(), f.call, b, "enhance", "system", "prompt", f.schema)
		if !errors.Is(err, gateway.ErrBudget) || f.requests.Load()-before != 1 || len(out.Receipt.Calls) != 1 {
			t.Fatalf("nested retry overspent: %v", err)
		}
		limited, err := gateway.NewBudget(f.call, gateway.Limits{Calls: 2, Tokens: 1, Duration: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		before = f.requests.Load()
		if _, err := f.engine.Generate(context.Background(), f.call, limited, "enhance", "system", "prompt", f.schema); !errors.Is(err, gateway.ErrBudget) || f.requests.Load() != before {
			t.Fatal("budget rejection performed inference")
		}
		concurrent := gatewayBudget(t, f.call, 4)
		var wg sync.WaitGroup
		var accepted atomic.Int64
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if concurrent.Reserve(f.call, 1) == nil {
					accepted.Add(1)
				}
			}()
		}
		wg.Wait()
		if accepted.Load() != 4 {
			t.Fatal("racy operation reservation")
		}
		if concurrent.Observe(f.call, 1, 8<<20) == nil {
			t.Fatal("reported overage ignored")
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		b := gatewayBudget(t, f.call, 8)
		first, err := f.engine.Embed(context.Background(), f.call, b, f.engine.Space(), []string{"one", "two"})
		if err != nil {
			t.Fatal(err)
		}
		first.Vectors[0][0] = 999
		cached, err := f.engine.Embed(context.Background(), f.call, b, f.engine.Space(), []string{"one", "two"})
		if err != nil || cached.Vectors[0][0] == 999 || !cached.Receipt.Calls[0].Cached || f.requests.Load() != 1 {
			t.Fatal("cache copied value/identity failure")
		}
		if _, err := f.engine.Embed(context.Background(), f.call, b, "wrong-space", []string{"one"}); !errors.Is(err, gateway.ErrSpace) {
			t.Fatal("embedding generation silently substituted")
		}
		f.mode.Store("error")
		if _, err := f.generate(t, "enhance"); err == nil {
			t.Fatal("provider outage hidden")
		}
		// The actual metadata service remains independent of the inference SDK's network outcome.
		db := support.Open(t, support.Database(t))
		scope := support.Scope(t, "offline-provider", "reader")
		if _, err := db.SetPolicy(context.Background(), scope, 0, store.Policy{AuditDays: 7, OperationHours: 24}); err != nil {
			t.Fatal(err)
		}
		if p, err := db.Policy(context.Background(), scope); err != nil || p.Revision != 1 {
			t.Fatal("provider outage disabled retained metadata")
		}
	})
	t.Run("AC05", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		if _, err := gateway.AdmitCandidates(f.call, "gateway.use", []gateway.Candidate{{ID: "foreign", Text: "protected", Resource: access.Resource{Tenant: "foreign", Kind: "topic", Permission: "read", ID: "foreign"}}}); err == nil {
			t.Fatal("unauthorized candidate admitted")
		}
		if _, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", gateway.Candidates{}); err == nil || f.requests.Load() != 0 {
			t.Fatal("unsealed candidate sent to provider")
		}
		f.mode.Store("visual_bad")
		if _, err := f.engine.VisualRank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", f.candidates); err == nil {
			t.Fatal("visual model invented a candidate")
		}
		disabled := newGatewayFixture(t, func(c *config.Gateway) {
			for _, name := range []string{"rerank", "narrative", "visual_rank"} {
				r := c.Roles[name]
				r.Enabled = false
				c.Roles[name] = r
			}
		})
		ranked, err := disabled.engine.Rerank(context.Background(), disabled.call, gatewayBudget(t, disabled.call, 2), "question", disabled.candidates)
		if err != nil || ranked.Receipt.Warning != "rerank_disabled" || ranked.Items[0].Score != nil {
			t.Fatal("disabled rerank not explicit")
		}
		if _, err := disabled.generate(t, "narrative"); !errors.Is(err, gateway.ErrDisabled) {
			t.Fatal("disabled narrative called")
		}
		if _, err := disabled.engine.VisualRank(context.Background(), disabled.call, gatewayBudget(t, disabled.call, 2), "question", disabled.candidates); err != nil {
			t.Fatal(err)
		}
		if disabled.requests.Load() != 0 {
			t.Fatal("disabled role called provider")
		}
	})
	t.Run("AC06", func(t *testing.T) {
		f := newGatewayFixture(t, func(c *config.Gateway) { c.Limits.Concurrency = 2; c.Limits.TenantConcurrency = 1 })
		f.mode.Store("delay")
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := f.engine.Generate(ctx, f.call, gatewayBudget(t, f.call, 2), "enhance", "system", "prompt", f.schema)
			done <- err
		}()
		deadline := time.Now().Add(3 * time.Second)
		for f.requests.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if f.requests.Load() == 0 {
			cancel()
			t.Fatal("SDK request did not start")
		}
		if _, err := f.generate(t, "sqlgen"); !errors.Is(err, gateway.ErrBusy) {
			cancel()
			t.Fatalf("tenant concurrency limit: %v", err)
		}
		cancel()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("cancellation silently succeeded")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("SDK cancellation not bounded")
		}
		closed := make(chan struct{})
		go func() { f.engine.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("SDK shutdown did not join")
		}
		if _, err := f.generate(t, "enhance"); !errors.Is(err, gateway.ErrClosed) {
			t.Fatalf("closed engine reused: %v", err)
		}
	})
	t.Run("AC07", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		for _, driver := range []string{"local", "onnx", "ollama", "openai-compatible"} {
			cfg := f.cfg
			cfg.Driver = driver
			if config.ValidateGateway(cfg, true) == nil {
				t.Fatal("alternate production inference driver accepted")
			}
		}
		for _, provider := range []string{"local", "onnx", "ollama", "huggingface-local"} {
			cfg := f.cfg
			cfg.Bifrost.Providers = append([]config.Provider(nil), cfg.Bifrost.Providers...)
			cfg.Bifrost.Providers[0].Type = provider
			if config.ValidateGateway(cfg, true) == nil {
				t.Fatal("local model provider accepted")
			}
		}
		if f.requests.Load() != 0 {
			t.Fatal("configuration validation called a model")
		}
	})
	t.Run("AC08", func(t *testing.T) {
		for _, mode := range []string{"missing", "extra", "duplicate", "index", "dimension", "overflow", "nonfinite", "model"} {
			t.Run(mode, func(t *testing.T) {
				f := newGatewayFixture(t, nil)
				f.mode.Store(mode)
				out, err := f.engine.Embed(context.Background(), f.call, gatewayBudget(t, f.call, 8), f.engine.Space(), []string{"one", "two"})
				if err == nil || out.Vectors != nil {
					t.Fatal("invalid embedding response accepted")
				}
			})
		}
		f := newGatewayFixture(t, nil)
		f.mode.Store("reversed")
		out, err := f.engine.Embed(context.Background(), f.call, gatewayBudget(t, f.call, 8), f.engine.Space(), []string{"one", "two", "three"})
		if err != nil || len(out.Vectors) != 3 || out.Vectors[0][0] != 1 || out.Vectors[1][0] != 2 || f.requests.Load() != 2 {
			t.Fatalf("batch/index association: %v", err)
		}
	})
	t.Run("AC09", func(t *testing.T) {
		for _, mode := range []string{"missing", "duplicate", "index", "nonfinite"} {
			t.Run(mode, func(t *testing.T) {
				f := newGatewayFixture(t, func(c *config.Gateway) {
					r := c.Roles["rerank"]
					r.OnFailure = "preserve_candidates"
					c.Roles["rerank"] = r
				})
				f.mode.Store(mode)
				out, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 4), "question", f.candidates)
				if err != nil || out.Receipt.Warning != "rerank_failed_original_order" || len(out.Items) != 2 || out.Items[0].ID != "first" || out.Items[0].Score != nil {
					t.Fatalf("invalid rerank fallback %v", err)
				}
			})
		}
		f := newGatewayFixture(t, nil)
		f.mode.Store("ties")
		out, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 4), "question", f.candidates)
		if err != nil || out.Items[0].ID != "first" || out.Items[1].ID != "second" {
			t.Fatalf("stable ties: %v", err)
		}
	})
	t.Run("AC10", func(t *testing.T) {
		f := newGatewayFixture(t, nil)
		changed := newGatewayFixture(t, func(c *config.Gateway) {
			r := c.Roles["embedding"]
			r.ModelRevision = "generation-2"
			c.Roles["embedding"] = r
		})
		if changed.engine.Space() == f.engine.Space() {
			t.Fatal("same-dimensional model revision did not change space")
		}
		e := f.token.envelope(t, "gateway-tenant", "other-operator", "gateway.use", "cw.tenant.use:gateway-tenant")
		other, err := gateway.Authorize(e, "gateway.use", "actual-context-v1", access.Tenant(e, "use"))
		if err != nil {
			t.Fatal(err)
		}
		b := gatewayBudget(t, f.call, 8)
		if _, err := f.engine.Embed(context.Background(), f.call, b, f.engine.Space(), []string{"one"}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.engine.Embed(context.Background(), f.call, gatewayBudget(t, other, 2), f.engine.Space(), []string{"one"}); !errors.Is(err, gateway.ErrBudget) {
			t.Fatal("cache bypassed operation ownership")
		}
		if _, err := f.engine.Embed(context.Background(), other, gatewayBudget(t, other, 2), f.engine.Space(), []string{"one"}); err != nil || f.requests.Load() != 2 {
			t.Fatal("actor cache isolation")
		}
		data, err := os.ReadFile("../../examples/chartworks.gateway.json")
		if err != nil {
			t.Fatal(err)
		}
		var excerpt struct {
			Gateway config.Gateway `json:"gateway"`
		}
		excerpt.Gateway = config.Defaults().Gateway
		if json.Unmarshal(data, &excerpt) != nil || config.ValidateGateway(excerpt.Gateway, false) != nil {
			t.Fatal("reference config not consumable")
		}
	})
}
