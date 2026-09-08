package bifrost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	core "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
)

// TransportOptions is a trusted constructor seam for recorded TLS fixtures/private remote deployments.
// The application composition root uses zero values; no request or JSON config can enable it.
type TransportOptions struct {
	AllowPrivateNetwork bool
	CACertPEM           string
}
type route struct {
	client   *core.Bifrost
	provider schemas.ModelProvider
	name     string
}

// Engine reuses only exact immutable route configurations and never shares request contexts.
type Engine struct {
	cfg       config.Gateway
	routes    map[string]route
	clients   []*core.Bifrost
	space     string
	embedding gateway.EmbeddingSpace
	cache     *gateway.Cache
	mu        sync.Mutex
	closed    bool
	active    int
	tenants   map[string]int
	wg        sync.WaitGroup
	cancel    context.CancelFunc
	done      <-chan struct{}
	once      sync.Once
}

// New constructs the real pinned Bifrost SDK clients without performing inference.
func New(ctx context.Context, cfg config.Gateway, lookup func(string) (string, bool), transport TransportOptions) (*Engine, error) {
	if ctx == nil || lookup == nil {
		return nil, gateway.ErrInput
	}
	if err := config.ValidateGateway(cfg, true); err != nil {
		return nil, err
	}
	// Copy both public configuration and resolved credentials before launching SDK workers.
	raw, _ := json.Marshal(cfg)
	var snapshot config.Gateway
	if json.Unmarshal(raw, &snapshot) != nil {
		return nil, gateway.ErrInput
	}
	cfg = snapshot
	lifetime, cancel := context.WithCancel(ctx)
	e := &Engine{cfg: cfg, routes: map[string]route{}, tenants: map[string]int{}, cancel: cancel, done: lifetime.Done(), cache: gateway.NewCache(cfg.Limits.CacheEntries, cfg.Limits.CacheBytes, time.Duration(cfg.Limits.CacheTTL), nil)}
	used := map[string]bool{}
	for name, r := range cfg.Roles {
		if !config.OptionalRole(name) || r.Enabled {
			used[r.Provider] = true
		}
	}
	shared := map[string]*core.Bifrost{}
	for _, p := range cfg.Bifrost.Providers {
		if !used[p.Name] {
			continue
		}
		secret, ok := lookup(strings.TrimPrefix(p.APIKey, "env:"))
		if !ok || secret == "" || len(secret) > 8192 {
			e.Close()
			return nil, gateway.ErrInput
		}
		native := schemas.ModelProvider(config.NativeProvider(p))
		seconds := 1
		for name, r := range cfg.Roles {
			if r.Provider == p.Name && (!config.OptionalRole(name) || r.Enabled) {
				s := int(math.Ceil(time.Duration(r.Timeout).Seconds()))
				if s > seconds {
					seconds = s
				}
			}
		}
		fingerprint, _ := json.Marshal([]any{native, p.BaseURL, secret, transport.CACertPEM, transport.AllowPrivateNetwork, seconds, cfg.Limits.Concurrency})
		hash := sha256.Sum256(fingerprint)
		key := hex.EncodeToString(hash[:])
		client := shared[key]
		if client == nil {
			a := &account{provider: native, key: secret, endpoint: p.BaseURL, ca: transport.CACertPEM, private: transport.AllowPrivateNetwork, concurrency: cfg.Limits.Concurrency, timeout: seconds}
			var err error
			client, err = core.Init(lifetime, schemas.BifrostConfig{Account: a, Logger: quietLogger{}, InitialPoolSize: cfg.Limits.Concurrency, DropExcessRequests: true})
			if err != nil {
				e.Close()
				return nil, gateway.ErrUnavailable
			}
			shared[key] = client
			e.clients = append(e.clients, client)
		}
		e.routes[p.Name] = route{client: client, provider: native, name: p.Name}
	}
	r := cfg.Roles["embedding"]
	p := config.Provider{}
	for _, v := range cfg.Bifrost.Providers {
		if v.Name == r.Provider {
			p = v
		}
	}
	endpoint := p.BaseURL
	if endpoint == "" {
		endpoint = "default"
	}
	e.embedding = gateway.EmbeddingSpace{Provider: config.NativeProvider(p), Route: p.Name, Endpoint: endpoint, Model: r.Model, Revision: r.ModelRevision, Dimensions: r.Dimensions, Preprocessing: "utf8-exact;float32-finite", InputType: "text", Normalization: "no-normalization"}
	e.space = e.embedding.Key()
	return e, nil
}

// Space returns the complete immutable embedding-space identifier.
func (e *Engine) Space() string { return e.space }

// EmbeddingSpace returns the exact descriptor used by this immutable engine.
func (e *Engine) EmbeddingSpace() gateway.EmbeddingSpace { return e.embedding }

// Close cancels and joins active calls and closes each owned SDK client once.
func (e *Engine) Close() {
	e.once.Do(func() {
		e.mu.Lock()
		e.closed = true
		e.cancel()
		e.mu.Unlock()
		e.wg.Wait()
		for _, c := range e.clients {
			c.Shutdown()
		}
		e.cache.Clear()
	})
}
func (e *Engine) enter(call gateway.Call, b *gateway.Budget) (func(), error) {
	if b.Check(call) != nil {
		return nil, gateway.ErrBudget
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	select {
	case <-e.done:
		return nil, gateway.ErrClosed
	default:
	}
	if e.closed {
		return nil, gateway.ErrClosed
	}
	if e.active >= e.cfg.Limits.Concurrency || e.tenants[call.Tenant()] >= e.cfg.Limits.TenantConcurrency {
		return nil, gateway.ErrBusy
	}
	e.active++
	e.tenants[call.Tenant()]++
	e.wg.Add(1)
	return func() {
		e.mu.Lock()
		e.active--
		e.tenants[call.Tenant()]--
		if e.tenants[call.Tenant()] == 0 {
			delete(e.tenants, call.Tenant())
		}
		e.mu.Unlock()
		e.wg.Done()
	}, nil
}
func (e *Engine) role(name string) (config.Role, route, error) {
	r, ok := e.cfg.Roles[name]
	if !ok || config.OptionalRole(name) && !r.Enabled {
		return r, route{}, gateway.ErrDisabled
	}
	p, ok := e.routes[r.Provider]
	if !ok {
		return r, p, gateway.ErrUnavailable
	}
	return r, p, nil
}
func (e *Engine) bounded(ctx context.Context, call gateway.Call, b *gateway.Budget, r config.Role) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(time.Duration(r.Timeout))
	if b.Deadline().Before(deadline) {
		deadline = b.Deadline()
	}
	if call.Deadline().Before(deadline) {
		deadline = call.Deadline()
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	stop := context.AfterFunc(contextFromDone{done: e.done}, cancel)
	return ctx, func() { stop(); cancel() }
}

// contextFromDone bridges the application lifetime without retaining a request context.
type contextFromDone struct{ done <-chan struct{} }

// Deadline returns the effective validity deadline without extending it.
func (contextFromDone) Deadline() (time.Time, bool) { return time.Time{}, false }

// Done exposes application shutdown to the request context bridge.
func (c contextFromDone) Done() <-chan struct{} { return c.done }

// Err reports application cancellation without request data.
func (c contextFromDone) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

// Value exposes no application values to the lifetime bridge.
func (contextFromDone) Value(any) any { return nil }
func retry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt+1) * 50 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func retryable(err *schemas.BifrostError) bool {
	return err != nil && err.StatusCode != nil && (*err.StatusCode == 429 || *err.StatusCode >= 500)
}
func (e *Engine) generate(ctx context.Context, call gateway.Call, b *gateway.Budget, name, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	out := gateway.Generated{}
	r, p, err := e.role(name)
	if err != nil {
		return out, err
	}
	if name == "embedding" || name == "rerank" || schema == nil || schema.Name() == "" || prompt == "" || len(prompt)+len(system)+len(schema.Document()) > e.cfg.Limits.MaxInputBytes {
		return out, gateway.ErrInput
	}
	release, err := e.enter(call, b)
	if err != nil {
		return out, err
	}
	defer release()
	ctx, cancel := e.bounded(ctx, call, b, r)
	defer cancel()
	var definition any
	if json.Unmarshal(schema.Document(), &definition) != nil {
		return out, gateway.ErrInput
	}
	format := any(map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": schema.Name(), "schema": definition, "strict": true}})
	for attempt := 0; attempt < e.cfg.MaxAttemptsPerCall; attempt++ {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err = b.Reserve(call, len(prompt)+len(system)+len(schema.Document())+1024+r.MaxTokens); err != nil {
			return out, err
		}
		bc, bcCancel := schemas.NewBifrostContextWithCancel(ctx)
		start := time.Now()
		response, be := p.client.ChatCompletionRequest(bc, &schemas.BifrostChatRequest{Provider: p.provider, Model: r.Model, Input: []schemas.ChatMessage{{Role: schemas.ChatMessageRoleSystem, Content: &schemas.ChatMessageContent{ContentStr: &system}}, {Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: &prompt}}}, Params: &schemas.ChatParameters{ResponseFormat: &format, MaxCompletionTokens: &r.MaxTokens, Store: core.Ptr(false)}})
		bcCancel()
		actual := ""
		var raw any
		if response != nil {
			actual = observedModel(response.Model)
			raw = response.ExtraFields.RawResponse
		}
		out.Receipt.Calls = append(out.Receipt.Calls, usage(name, p, r.Model, actual, start, raw))
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err := observe(b, call, len(prompt)+len(system)+len(schema.Document())+1024+r.MaxTokens, out.Receipt); err != nil {
			return out, err
		}
		if be != nil {
			if retryable(be) && attempt+1 < e.cfg.MaxAttemptsPerCall {
				if err = retry(ctx, attempt); err != nil {
					return out, err
				}
				continue
			}
			return out, gateway.ErrUnavailable
		}
		if _, err := wire(raw, e.cfg.Limits.MaxOutputBytes+65536); err != nil {
			return out, err
		}
		if response == nil || len(response.Choices) != 1 || response.Choices[0].ChatNonStreamResponseChoice == nil {
			return out, gateway.ErrOutput
		}
		choice := response.Choices[0]
		message := choice.Message
		if message == nil || message.Content == nil || message.Content.ContentStr == nil || choice.FinishReason == nil || *choice.FinishReason != "stop" || (message.ChatAssistantMessage != nil && len(message.ToolCalls) != 0) {
			return out, gateway.ErrOutput
		}
		result := []byte(*message.Content.ContentStr)
		if err = schema.Validate(result, e.cfg.Limits.MaxOutputBytes); err != nil {
			return out, err
		}
		out.JSON = append([]byte(nil), result...)
		return out, nil
	}
	return out, gateway.ErrUnavailable
}

// Embed batches and validates finite indexed embeddings without changing generation identity.
func (e *Engine) Embed(ctx context.Context, call gateway.Call, b *gateway.Budget, expectedSpace string, texts []string) (gateway.Embedded, error) {
	out := gateway.Embedded{Space: e.space, Descriptor: e.embedding}
	r, p, err := e.role("embedding")
	if err != nil {
		return out, err
	}
	if expectedSpace != e.space {
		return out, gateway.ErrSpace
	}
	if len(texts) == 0 || len(texts) > 16384 {
		return out, gateway.ErrInput
	}
	size := 0
	for _, s := range texts {
		if s == "" || len(s) > r.MaxBatchBytes {
			return out, gateway.ErrInput
		}
		size += len(s)
		if size > e.cfg.Limits.MaxInputBytes {
			return out, gateway.ErrInput
		}
	}
	release, err := e.enter(call, b)
	if err != nil {
		return out, err
	}
	defer release()
	ctx, cancel := e.bounded(ctx, call, b, r)
	defer cancel()
	keyData, _ := json.Marshal([]any{call.Key(), e.space, texts})
	hash := sha256.Sum256(keyData)
	cacheKey := hex.EncodeToString(hash[:])
	if vectors, ok := e.cache.Get(cacheKey); ok {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Vectors = vectors
		out.Receipt.Calls = []gateway.Usage{{Role: "embedding", Provider: p.name, RequestedModel: r.Model, Cached: true}}
		return out, nil
	}
	all := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); {
		end, bytes := start, 0
		for end < len(texts) && end-start < r.MaxBatchItems && bytes+len(texts[end]) <= r.MaxBatchBytes {
			bytes += len(texts[end])
			end++
		}
		if end == start {
			return out, gateway.ErrInput
		}
		var vectors [][]float32
		for attempt := 0; attempt < e.cfg.MaxAttemptsPerCall; attempt++ {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if err = b.Reserve(call, bytes+128*(end-start)); err != nil {
				return out, err
			}
			bc, bcCancel := schemas.NewBifrostContextWithCancel(ctx)
			began := time.Now()
			response, be := p.client.EmbeddingRequest(bc, &schemas.BifrostEmbeddingRequest{Provider: p.provider, Model: r.Model, Input: &schemas.EmbeddingInput{Texts: append([]string(nil), texts[start:end]...)}, Params: &schemas.EmbeddingParameters{Dimensions: &r.Dimensions}})
			bcCancel()
			actual := ""
			var raw any
			if response != nil {
				actual = observedModel(response.Model)
				raw = response.ExtraFields.RawResponse
			}
			out.Receipt.Calls = append(out.Receipt.Calls, usage("embedding", p, r.Model, actual, began, raw))
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if err := observe(b, call, bytes+128*(end-start), out.Receipt); err != nil {
				return out, err
			}
			if be != nil {
				if retryable(be) && attempt+1 < e.cfg.MaxAttemptsPerCall {
					if err = retry(ctx, attempt); err != nil {
						return out, err
					}
					continue
				}
				return out, gateway.ErrUnavailable
			}
			if err := indexedWire(raw, "data", "embedding", end-start, e.cfg.Limits.MaxOutputBytes); err != nil {
				return out, err
			}
			if response == nil || len(response.Data) != end-start {
				return out, gateway.ErrOutput
			}
			if actual != "" && actual != r.Model {
				return out, gateway.ErrSpace
			}
			vectors = make([][]float32, end-start)
			for _, entry := range response.Data {
				index := entry.Index
				values := entry.Embedding.EmbeddingArray
				if index < 0 || index >= len(vectors) || vectors[index] != nil || len(values) != r.Dimensions {
					return out, gateway.ErrOutput
				}
				vector := make([]float32, r.Dimensions)
				for j, value := range values {
					if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > math.MaxFloat32 {
						return out, gateway.ErrOutput
					}
					vector[j] = float32(value)
				}
				vectors[index] = vector
			}
			break
		}
		if vectors == nil {
			return out, gateway.ErrUnavailable
		}
		all = append(all, vectors...)
		start = end
	}
	if !call.Valid() || ctx.Err() != nil {
		return out, gateway.ErrBudget
	}
	e.cache.Put(cacheKey, all)
	out.Vectors = all
	return out, nil
}

// Rerank orders only sealed candidates through the native SDK and validates a complete permutation.
func (e *Engine) Rerank(ctx context.Context, call gateway.Call, b *gateway.Budget, query string, candidates gateway.Candidates) (gateway.Ranked, error) {
	out := gateway.Ranked{}
	if !candidates.Valid(call) {
		return out, gateway.ErrInput
	}
	r, p, err := e.role("rerank")
	if err == gateway.ErrDisabled {
		return gateway.Preserve(candidates, "rerank_disabled", out.Receipt), nil
	}
	if err != nil {
		return out, err
	}
	items := candidates.Items()
	if query == "" || len(items) > r.MaxCandidates {
		return out, gateway.ErrInput
	}
	size := len(query)
	for _, c := range items {
		size += len(c.Text)
	}
	if size > e.cfg.Limits.MaxInputBytes {
		return out, gateway.ErrInput
	}
	release, err := e.enter(call, b)
	if err != nil {
		return out, err
	}
	defer release()
	ctx, cancel := e.bounded(ctx, call, b, r)
	defer cancel()
	documents := make([]schemas.RerankDocument, len(items))
	for i, c := range items {
		documents[i] = schemas.RerankDocument{Text: c.Text}
	}
	fallback := func(err error) (gateway.Ranked, error) {
		if r.OnFailure == "preserve_candidates" && ctx.Err() == nil && call.Valid() {
			return gateway.Preserve(candidates, "rerank_failed_original_order", out.Receipt), nil
		}
		return out, err
	}
	for attempt := 0; attempt < e.cfg.MaxAttemptsPerCall; attempt++ {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err = b.Reserve(call, size+128*len(items)); err != nil {
			return out, err
		}
		bc, bcCancel := schemas.NewBifrostContextWithCancel(ctx)
		start := time.Now()
		response, be := p.client.RerankRequest(bc, &schemas.BifrostRerankRequest{Provider: p.provider, Model: r.Model, Query: query, Documents: documents, Params: &schemas.RerankParameters{TopN: core.Ptr(len(items)), ReturnDocuments: core.Ptr(false)}})
		bcCancel()
		actual := ""
		var raw any
		if response != nil {
			actual = observedModel(response.Model)
			raw = response.ExtraFields.RawResponse
		}
		out.Receipt.Calls = append(out.Receipt.Calls, usage("rerank", p, r.Model, actual, start, raw))
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if err := observe(b, call, size+128*len(items), out.Receipt); err != nil {
			return out, err
		}
		if be != nil {
			if retryable(be) && attempt+1 < e.cfg.MaxAttemptsPerCall {
				if err = retry(ctx, attempt); err != nil {
					return out, err
				}
				continue
			}
			return fallback(gateway.ErrUnavailable)
		}
		if err := indexedWire(raw, "results", "relevance_score", len(items), e.cfg.Limits.MaxOutputBytes); err != nil {
			return fallback(err)
		}
		if response == nil || len(response.Results) != len(items) {
			return fallback(gateway.ErrOutput)
		}
		seen := map[int]bool{}
		order := map[string]int{}
		ranked := make([]gateway.RankedItem, 0, len(items))
		for i, c := range items {
			order[c.ID] = i
		}
		for _, v := range response.Results {
			if v.Index < 0 || v.Index >= len(items) || seen[v.Index] || math.IsNaN(v.RelevanceScore) || math.IsInf(v.RelevanceScore, 0) {
				return fallback(gateway.ErrOutput)
			}
			seen[v.Index] = true
			score := v.RelevanceScore
			ranked = append(ranked, gateway.RankedItem{ID: items[v.Index].ID, Score: &score})
		}
		sort.SliceStable(ranked, func(i, j int) bool {
			if *ranked[i].Score == *ranked[j].Score {
				return order[ranked[i].ID] < order[ranked[j].ID]
			}
			return *ranked[i].Score > *ranked[j].Score
		})
		out.Items = ranked
		return out, nil
	}
	return fallback(gateway.ErrUnavailable)
}

var _ gateway.Engine = (*Engine)(nil)

// Generate handles structured authoring and tool-free narratives. Visual ranking requires its sealed candidate method.
func (e *Engine) Generate(ctx context.Context, call gateway.Call, b *gateway.Budget, name, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	if name == "visual_rank" {
		return gateway.Generated{}, gateway.ErrInput
	}
	return e.generate(ctx, call, b, name, system, prompt, schema)
}
