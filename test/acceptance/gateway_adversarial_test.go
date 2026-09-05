package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
)

func TestGatewayAdversarialWire(t *testing.T) {
	f := newGatewayFixture(t, nil)
	t.Run("embedding-field-presence", func(t *testing.T) {
		for _, row := range []string{`{"embedding":[1,2]}`, `{"index":null,"embedding":[1,2]}`, `{"index":"0","embedding":[1,2]}`, `{"index":0,"index":0,"embedding":[1,2]}`, `{"index":0,"embedding":null}`, `{"index":0,"embedding":[1,2],"embedding":[3,4]}`} {
			f.mode.Store(`raw:{"model":"embedding-model","data":[` + row + `]}`)
			out, err := f.engine.Embed(context.Background(), f.call, gatewayBudget(t, f.call, 2), f.engine.Space(), []string{"not-cached"})
			if err == nil || out.Vectors != nil {
				t.Fatalf("malformed wire accepted: %s", row)
			}
		}
	})
	t.Run("rerank-field-presence", func(t *testing.T) {
		for _, row := range []string{`{"relevance_score":0.7}`, `{"index":null,"relevance_score":0.7}`, `{"index":0}`, `{"index":0,"relevance_score":null}`, `{"index":0,"index":0,"relevance_score":0.7}`, `{"index":0,"relevance_score":0.7,"relevance_score":0.9}`} {
			f.mode.Store(`raw:{"results":[` + row + `,{"index":1,"relevance_score":0.1}]}`)
			out, err := f.engine.Rerank(context.Background(), f.call, gatewayBudget(t, f.call, 2), "question", f.candidates)
			if err == nil || len(out.Items) != 0 {
				t.Fatalf("malformed ranking accepted: %s", row)
			}
		}
	})
	t.Run("unknown-and-aggregate-billing", func(t *testing.T) {
		for _, test := range []struct {
			usage string
			cost  *float64
		}{{`{}`, nil}, {`{"cost":{"input_tokens_cost":1,"total_cost":2}}`, ptrCost(2)}, {`{"cost":{"input_tokens_cost":1,"output_tokens_cost":2}}`, nil}, {`{"cost":0}`, ptrCost(0)}, {`{"cost":-1}`, nil}} {
			f.mode.Store(`raw:{"model":"model-enhance","choices":[{"index":0,"message":{"role":"assistant","content":"{\"summary\":\"ok\"}"},"finish_reason":"stop"}],"usage":` + test.usage + `}`)
			out, err := f.generate(t, "enhance")
			if err != nil {
				t.Fatal(err)
			}
			u := out.Receipt.Calls[0]
			if u.InputTokens != nil || u.OutputTokens != nil {
				t.Fatal("absent counts reported as zero")
			}
			if test.cost == nil && u.CostUSD != nil || test.cost != nil && (u.CostUSD == nil || *u.CostUSD != *test.cost) {
				t.Fatal("cost was fabricated or double-counted")
			}
		}
		f.mode.Store(`raw:{"model":"model-enhance","choices":[{"index":0,"message":{"role":"assistant","content":"{\"summary\":\"ok\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10000000,"completion_tokens":5}}`)
		budget := gatewayBudget(t, f.call, 2)
		if _, err := f.engine.Generate(context.Background(), f.call, budget, "enhance", "system", "prompt", f.schema); !errors.Is(err, gateway.ErrBudget) {
			t.Fatal("reported overage unbounded", err)
		}
		before := f.requests.Load()
		if _, err := f.engine.Generate(context.Background(), f.call, budget, "enhance", "system", "prompt", f.schema); !errors.Is(err, gateway.ErrBudget) || f.requests.Load() != before {
			t.Fatal("overage funded more inference")
		}
	})
	t.Run("scope-narrowing-does-not-reuse-sealed-inputs", func(t *testing.T) {
		narrow := f.token.envelope(t, "gateway-tenant", "operator", "gateway.use", "cw.tenant.use:gateway-tenant", "cw.topic.read:first")
		narrowed, err := gateway.Authorize(narrow, "gateway.use", "actual-context-v1", access.Tenant(narrow, "use"))
		if err != nil {
			t.Fatal(err)
		}
		if narrowed.Key() == f.call.Key() || f.candidates.Valid(narrowed) {
			t.Fatal("narrowed scope reused previously sealed authority")
		}
		before := f.requests.Load()
		if _, err := f.engine.Rerank(context.Background(), narrowed, gatewayBudget(t, narrowed, 2), "question", f.candidates); err == nil || f.requests.Load() != before {
			t.Fatal("revoked candidate sent to provider")
		}
		b := gatewayBudget(t, f.call, 2)
		if b.Check(narrowed) == nil || b.Reserve(narrowed, 1) == nil || b.Observe(narrowed, 1, 1) == nil {
			t.Fatal("budget accepted changed authority")
		}
		f.mode.Store("normal")
		if _, err := f.engine.Embed(context.Background(), f.call, gatewayBudget(t, f.call, 2), f.engine.Space(), []string{"partition-key"}); err != nil {
			t.Fatal(err)
		}
		before = f.requests.Load()
		if _, err := f.engine.Embed(context.Background(), narrowed, gatewayBudget(t, narrowed, 2), f.engine.Space(), []string{"partition-key"}); err != nil || f.requests.Load() != before+1 {
			t.Fatal("narrowed context shared cache", err)
		}
	})
}
func ptrCost(v float64) *float64 { return &v }

func TestGatewayLocalBounds(t *testing.T) {
	now := time.Now()
	cache := gateway.NewCache(1, 100, time.Second, func() time.Time { return now })
	cache.Put("a", [][]float32{{1, 2}})
	cache.Put("a", [][]float32{{3, 4}})
	if got, ok := cache.Get("a"); !ok || got[0][0] != 3 {
		t.Fatal("cache replacement")
	}
	cache.Put("b", [][]float32{{5, 6}})
	if _, ok := cache.Get("a"); ok {
		t.Fatal("LRU not bounded")
	}
	now = now.Add(2 * time.Second)
	if _, ok := cache.Get("b"); ok {
		t.Fatal("TTL not enforced")
	}
	cache.Put(strings.Repeat("x", 101), [][]float32{{1}})
	if _, ok := cache.Get(strings.Repeat("x", 101)); ok {
		t.Fatal("cache byte limit")
	}
	disabled := gateway.NewCache(0, 0, 0, nil)
	disabled.Put("a", [][]float32{{1}})
	if _, ok := disabled.Get("a"); ok {
		t.Fatal("disabled cache")
	}
	cache.Clear()
	for _, s := range []string{"", `[`, `{"x":`, string([]byte{0xff}), strings.Repeat("[", 34) + strings.Repeat("]", 34), `{"x":` + strings.Repeat("9", 129) + `}`, `[` + strings.Repeat("0,", 65536) + `0]`} {
		if _, err := gateway.DecodeJSON([]byte(s), 1<<20); err == nil {
			t.Fatal("unbounded JSON accepted")
		}
	}
	for _, name := range []string{"", "bad.name", strings.Repeat("s", 65)} {
		if _, err := gateway.NewSchema(name, []byte(`{}`)); err == nil {
			t.Fatal("schema name")
		}
	}
	for _, doc := range []string{`[]`, `{"type":"not-real"}`, `null`} {
		if _, err := gateway.NewSchema("s", []byte(doc)); err == nil {
			t.Fatal("invalid schema")
		}
	}
	var schema *gateway.Schema
	if schema.Name() != "" || schema.Document() != nil || schema.Validate([]byte(`{}`), 100) == nil {
		t.Fatal("nil schema")
	}
	var budget *gateway.Budget
	if budget.Check(gateway.Call{}) == nil || budget.Reserve(gateway.Call{}, 1) == nil || budget.Observe(gateway.Call{}, 1, 1) == nil || !budget.Deadline().IsZero() {
		t.Fatal("nil budget")
	}
	if c, n := budget.Used(); c != 0 || n != 0 {
		t.Fatal("nil budget accounting")
	}
	receipt := gateway.Receipt{}
	receipt.Append(gateway.Receipt{Warning: "visible", Calls: []gateway.Usage{{Role: "test"}}})
	if receipt.Warning != "visible" || len(receipt.Calls) != 1 {
		t.Fatal("receipt merge")
	}
	f := newGatewayFixture(t, nil)
	if _, err := gateway.Authorize(f.token.envelope(t, "t", "u"), "gateway.use", ""); err == nil {
		t.Fatal("invalid call")
	}
	for _, limits := range []gateway.Limits{{}, {Calls: 65, Tokens: 1, Duration: time.Second}, {Calls: 1, Tokens: 1, Duration: 11 * time.Minute}} {
		if _, err := gateway.NewBudget(f.call, limits); err == nil {
			t.Fatal("invalid budget")
		}
	}
	items := f.candidates.Items()
	for _, bad := range [][]gateway.Candidate{nil, {{ID: "invalid/", Text: "a"}}, {items[0], items[0]}} {
		if _, err := gateway.AdmitCandidates(f.call, "gateway.use", bad); err == nil {
			t.Fatal("bad candidates")
		}
	}
	b := gatewayBudget(t, f.call, 2)
	if b.Observe(f.call, 1, 20) != nil {
		t.Fatal("honest overage")
	}
	if _, n := b.Used(); n != 19 {
		t.Fatal("overage not charged")
	}
	if b.Observe(f.call, 0, 0) == nil || b.Reserve(f.call, 0) == nil {
		t.Fatal("invalid reservation")
	}
	// Copy isolation protects compiled schemas and sealed candidates from caller mutation.
	doc := f.schema.Document()
	doc[0] = 'x'
	if !json.Valid(f.schema.Document()) {
		t.Fatal("mutable schema")
	}
	items[0].ID = "changed"
	if f.candidates.Items()[0].ID == "changed" {
		t.Fatal("mutable candidate")
	}
	_ = fmt.Sprint(receipt)
}

// All 128 invocations traverse the real shared SDK under bounded concurrent admission.
func TestGatewayConcurrentReuse(t *testing.T) {
	f := newGatewayFixture(t, nil)
	f.mode.Store("echo")
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			prompt := fmt.Sprintf("unique-input-%d", i)
			out, err := f.engine.Generate(context.Background(), f.call, gatewayBudget(t, f.call, 2), "enhance", "system", prompt, f.schema)
			if err != nil {
				t.Error(err)
				return
			}
			var body struct {
				Summary string `json:"summary"`
			}
			if json.Unmarshal(out.JSON, &body) != nil || body.Summary != prompt {
				t.Error("SDK request/response context bleed")
			}
		}(i)
	}
	wg.Wait()
	if f.requests.Load() != 128 {
		t.Fatal("missing real SDK invocations", f.requests.Load())
	}
}
