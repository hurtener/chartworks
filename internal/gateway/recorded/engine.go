// Package recorded is an exact-input gateway for release integration evidence.
// It performs no provider I/O and never identifies fixtures as live model work.
package recorded

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

// Recording is operator-supplied, synthetic fixture material. InputDigest is
// computed from the verified actor and signed reach, reviewed model
// configuration, and exact request body; changed inputs have no fallback.
type Recording struct {
	Role        string
	InputDigest string
	JSON        json.RawMessage
	Vectors     [][]float32
	Items       []gateway.RankedItem
}

// Engine implements gateway.Engine only for explicitly recorded integration.
// Production composition never selects it from a user request or profile.
type Engine struct {
	mu      sync.RWMutex
	closed  bool
	space   gateway.EmbeddingSpace
	models  map[string]string
	records map[string]Recording
}

var _ gateway.Engine = (*Engine)(nil)

func New(space gateway.EmbeddingSpace, models map[string]string, recordings []Recording) (*Engine, error) {
	if space.Provider != "recorded" || space.Dimensions < 1 || space.Dimensions > 4096 || space.Model == "" || len(models) == 0 || len(models) > 16 || len(recordings) == 0 || len(recordings) > 2048 {
		return nil, gateway.ErrInput
	}
	e := &Engine{space: space, models: make(map[string]string, len(models)), records: make(map[string]Recording, len(recordings))}
	for role, model := range models {
		if role == "" || len(role) > 64 || model == "" || len(model) > 256 {
			return nil, gateway.ErrInput
		}
		e.models[role] = model
	}
	if e.models["embedding"] != space.Model {
		return nil, gateway.ErrInput
	}
	for _, record := range recordings {
		if e.models[record.Role] == "" || !digestValid(record.InputDigest) || e.records[record.InputDigest].Role != "" {
			return nil, gateway.ErrInput
		}
		populated := 0
		if len(record.JSON) > 0 {
			if len(record.JSON) > 128<<10 {
				return nil, gateway.ErrInput
			}
			if _, err := gateway.DecodeJSON(record.JSON, 128<<10); err != nil {
				return nil, gateway.ErrInput
			}
			populated++
		}
		if len(record.Vectors) > 0 {
			if len(record.Vectors) > 16384 {
				return nil, gateway.ErrInput
			}
			for _, vector := range record.Vectors {
				if len(vector) != space.Dimensions {
					return nil, gateway.ErrInput
				}
				for _, value := range vector {
					if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
						return nil, gateway.ErrInput
					}
				}
			}
			populated++
		}
		if len(record.Items) > 0 {
			if len(record.Items) > 1024 {
				return nil, gateway.ErrInput
			}
			seen := map[string]bool{}
			for _, item := range record.Items {
				if item.ID == "" || seen[item.ID] || item.Score != nil && (math.IsNaN(*item.Score) || math.IsInf(*item.Score, 0)) {
					return nil, gateway.ErrInput
				}
				seen[item.ID] = true
			}
			populated++
		}
		if populated != 1 {
			return nil, gateway.ErrInput
		}
		copy := Recording{Role: record.Role, InputDigest: record.InputDigest, JSON: append(json.RawMessage(nil), record.JSON...), Vectors: cloneVectors(record.Vectors), Items: cloneItems(record.Items)}
		e.records[record.InputDigest] = copy
	}
	return e, nil
}

func (e *Engine) PerformanceModelMode() string           { return "recorded" }
func (e *Engine) Space() string                          { return e.space.Key() }
func (e *Engine) EmbeddingSpace() gateway.EmbeddingSpace { return e.space }
func (e *Engine) Close()                                 { e.mu.Lock(); e.closed = true; e.mu.Unlock() }

// InputDigest is the fixture key for one exact reviewed model input. It keeps
// signed authority but excludes the ephemeral frozen-run ID from the call's
// data partition, allowing independent runs to use the same recorded input.
func InputDigest(call gateway.Call, role, model, configDigest string, parts ...any) string {
	if !call.Valid() || role == "" || model == "" {
		return ""
	}
	raw, err := json.Marshal([]any{call.AuthorityKey(), role, model, configDigest, parts})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (e *Engine) lookup(ctx context.Context, call gateway.Call, budget *gateway.Budget, role string, parts ...any) (Recording, gateway.Usage, error) {
	if e == nil || ctx == nil || !call.Valid() {
		return Recording{}, gateway.Usage{}, gateway.ErrInput
	}
	if err := ctx.Err(); err != nil {
		return Recording{}, gateway.Usage{}, err
	}
	configured := e.models[role]
	if configured == "" {
		return Recording{}, gateway.Usage{}, gateway.ErrDisabled
	}
	model, _, configDigest := gateway.ApplyRuntimeConfig(ctx, role, configured, "")
	if model != configured || !digestValid(configDigest) {
		return Recording{}, gateway.Usage{}, gateway.ErrInput
	}
	key := InputDigest(call, role, model, configDigest, parts...)
	raw, err := json.Marshal(parts)
	if err != nil || len(raw) > 1<<20 {
		return Recording{}, gateway.Usage{}, gateway.ErrInput
	}
	e.mu.RLock()
	record, ok := e.records[key]
	closed := e.closed
	e.mu.RUnlock()
	if closed {
		return Recording{}, gateway.Usage{}, gateway.ErrClosed
	}
	if !ok || record.Role != role {
		return Recording{}, gateway.Usage{}, gateway.ErrUnavailable
	}
	if err := gateway.ReserveAttempt(ctx, budget, call, 128+len(raw)); err != nil {
		return Recording{}, gateway.Usage{}, err
	}
	return record, gateway.Usage{Role: role, Provider: "recorded", RequestedModel: model, ConfigurationDigest: configDigest, Attempts: 1}, nil
}

func (e *Engine) Generate(ctx context.Context, call gateway.Call, budget *gateway.Budget, name, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	if e == nil || schema == nil || name == "" || len(prompt) == 0 || len(prompt) > 1<<20 || len(system) > 1<<16 {
		return gateway.Generated{}, gateway.ErrInput
	}
	model := e.models[name]
	_, effectiveSystem, _ := gateway.ApplyRuntimeConfig(ctx, name, model, system)
	started := time.Now()
	record, usage, err := e.lookup(ctx, call, budget, name, name, effectiveSystem, prompt, schema.Name(), schema.Document())
	if err != nil {
		return gateway.Generated{}, err
	}
	usage.DurationMS = time.Since(started).Milliseconds()
	receipt := gateway.Receipt{Calls: []gateway.Usage{usage}}
	if len(record.JSON) == 0 || schema.Validate(record.JSON, 128<<10) != nil {
		return gateway.Generated{Receipt: receipt}, gateway.ErrOutput
	}
	return gateway.Generated{JSON: append(json.RawMessage(nil), record.JSON...), Receipt: receipt}, nil
}

func (e *Engine) Embed(ctx context.Context, call gateway.Call, budget *gateway.Budget, expectedSpace string, texts []string) (gateway.Embedded, error) {
	if e == nil || expectedSpace != e.Space() || len(texts) == 0 || len(texts) > 16384 {
		return gateway.Embedded{}, gateway.ErrSpace
	}
	for _, value := range texts {
		if value == "" || len(value) > 1<<20 {
			return gateway.Embedded{}, gateway.ErrInput
		}
	}
	started := time.Now()
	record, usage, err := e.lookup(ctx, call, budget, "embedding", expectedSpace, texts)
	if err != nil {
		return gateway.Embedded{}, err
	}
	usage.DurationMS = time.Since(started).Milliseconds()
	receipt := gateway.Receipt{Calls: []gateway.Usage{usage}}
	if len(record.Vectors) != len(texts) {
		return gateway.Embedded{Receipt: receipt}, gateway.ErrOutput
	}
	return gateway.Embedded{Vectors: cloneVectors(record.Vectors), Space: e.Space(), Descriptor: e.space, Receipt: receipt}, nil
}

func (e *Engine) Rerank(ctx context.Context, call gateway.Call, budget *gateway.Budget, query string, candidates gateway.Candidates) (gateway.Ranked, error) {
	return e.rank(ctx, call, budget, "rerank", query, candidates)
}
func (e *Engine) VisualRank(ctx context.Context, call gateway.Call, budget *gateway.Budget, query string, candidates gateway.Candidates) (gateway.Ranked, error) {
	return e.rank(ctx, call, budget, "visual_rank", query, candidates)
}
func (e *Engine) rank(ctx context.Context, call gateway.Call, budget *gateway.Budget, role, query string, candidates gateway.Candidates) (gateway.Ranked, error) {
	if !candidates.Valid(call) || query == "" || len(query) > 1<<20 {
		return gateway.Ranked{}, gateway.ErrInput
	}
	items := candidates.Items()
	started := time.Now()
	record, usage, err := e.lookup(ctx, call, budget, role, query, items)
	if err != nil {
		return gateway.Ranked{}, err
	}
	usage.DurationMS = time.Since(started).Milliseconds()
	receipt := gateway.Receipt{Calls: []gateway.Usage{usage}}
	if len(record.Items) != len(items) {
		return gateway.Ranked{Receipt: receipt}, gateway.ErrOutput
	}
	allowed := map[string]bool{}
	for _, item := range items {
		allowed[item.ID] = true
	}
	for _, item := range record.Items {
		if !allowed[item.ID] {
			return gateway.Ranked{Receipt: receipt}, gateway.ErrOutput
		}
	}
	return gateway.Ranked{Items: cloneItems(record.Items), Receipt: receipt}, nil
}

func digestValid(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' && char < 'a' || char > 'f' {
			return false
		}
	}
	return true
}

func cloneVectors(values [][]float32) [][]float32 {
	out := make([][]float32, len(values))
	for i, value := range values {
		out[i] = append([]float32(nil), value...)
	}
	return out
}
func cloneItems(values []gateway.RankedItem) []gateway.RankedItem {
	out := make([]gateway.RankedItem, len(values))
	for i, item := range values {
		out[i].ID = item.ID
		if item.Score != nil {
			score := *item.Score
			out[i].Score = &score
		}
	}
	return out
}
