// Package chartservice enforces Pengui authority around the pure specification
// core. It accepts caller-supplied results, never source handles, SQL or URLs.
package chartservice

import (
	"context"
	"errors"
	"math"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

// ErrBusy rejects excess concurrent work without allocating a waiting queue.
var ErrBusy = errors.New("charts: concurrency exhausted")

// Failure preserves attempted model usage when late cancellation or expired
// authority suppresses the result. It never carries data or a candidate mapping.
type Failure struct {
	Cause   error
	Receipt gateway.Receipt
}

func (f *Failure) Error() string { return "charts: selection interrupted" }
func (f *Failure) Unwrap() error { return f.Cause }

func copyReceipt(in gateway.Receipt) gateway.Receipt {
	out := gateway.Receipt{Warning: in.Warning, Calls: append([]gateway.Usage{}, in.Calls...)}
	for i := range out.Calls {
		c := &out.Calls[i]
		if c.InputTokens != nil {
			v := *c.InputTokens
			c.InputTokens = &v
		}
		if c.OutputTokens != nil {
			v := *c.OutputTokens
			c.OutputTokens = &v
		}
		if c.CostUSD != nil {
			v := *c.CostUSD
			c.CostUSD = &v
		}
	}
	return out
}

// Options bounds transformations and the optional, author-requested rank call.
type Options struct {
	Limits                charts.Limits
	MaxConcurrent         int
	Timeout               time.Duration
	RankEnabled           bool
	RankCalls, RankTokens int
	RankTimeout           time.Duration
}

// Validate rejects unusable or unbounded service configuration.
func (o Options) Validate() error {
	if o.Limits.Validate() != nil || o.MaxConcurrent < 1 || o.MaxConcurrent > 64 || o.Timeout < time.Second || o.Timeout > 30*time.Second ||
		o.RankCalls < 1 || o.RankCalls > 4 || o.RankTokens < 1024 || o.RankTokens > 65536 || o.RankTimeout < time.Second || o.RankTimeout > o.Timeout {
		return charts.ErrInvalid
	}
	return nil
}

// Service is immutable except for its bounded concurrency admission semaphore.
type Service struct {
	options Options
	engine  gateway.Engine
	slots   chan struct{}
}

// New never creates a model client, store or warehouse connection.
func New(options Options, engine gateway.Engine) (*Service, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	return &Service{options: options, engine: engine, slots: make(chan struct{}, options.MaxConcurrent)}, nil
}

// ConcurrencyLimit lets a transport bound decoding before the service admission
// point. The service independently enforces its limit for in-process callers.
func (s *Service) ConcurrencyLimit() int {
	if s == nil {
		return 0
	}
	return s.options.MaxConcurrent
}

// CatalogResult reports real capabilities and their active input limits.
type CatalogResult struct {
	Version           int                   `json:"version"`
	Kinds             []charts.CatalogEntry `json:"kinds"`
	Limits            charts.Limits         `json:"limits"`
	RankingConfigured bool                  `json:"ranking_configured"`
}

// SelectRequest opts into paid exploratory ranking explicitly. Intent is bounded
// author text; no rows, labels, source metadata or saved definition reach a model.
type SelectRequest struct {
	Data   charts.Data `json:"data"`
	Rank   bool        `json:"rank"`
	Intent string      `json:"intent"`
}

// Provenance distinguishes deterministic suitability scores from optional remote
// ranking. Reservations are not provider-reported token usage or cost.
type Provenance struct {
	Input          string               `json:"input"`
	RulesVersion   int                  `json:"rules_version"`
	Ranking        string               `json:"ranking"`
	Ranked         []gateway.RankedItem `json:"ranked"`
	Receipt        gateway.Receipt      `json:"receipt"`
	ReservedCalls  int                  `json:"reserved_calls"`
	ReservedTokens int                  `json:"reserved_tokens"`
	MaxCalls       int                  `json:"max_calls"`
	MaxTokens      int                  `json:"max_tokens"`
	MaxDurationMS  int64                `json:"max_duration_ms"`
}

// SelectionResult carries portable saved candidate definitions, not renderer code.
type SelectionResult struct {
	Selection  charts.Selection `json:"selection"`
	Limits     charts.Limits    `json:"limits"`
	Provenance Provenance       `json:"provenance"`
}

// SpecifyRequest creates a new explicit authoring definition, not a saved rebind.
type SpecifyRequest struct {
	Data     charts.Data     `json:"data"`
	Kind     charts.Kind     `json:"kind"`
	Bindings charts.Bindings `json:"bindings"`
	Order    []charts.Order  `json:"order"`
	Options  charts.Options  `json:"options"`
}

// BuildRequest applies an exact saved mapping. It has no rank or SQL field.
type BuildRequest struct {
	Data    charts.Data    `json:"data"`
	Mapping charts.Mapping `json:"mapping"`
}

// BuildResult contains sealed typed drawing input and exact labels, not pixels.
type BuildResult struct {
	Output charts.Output `json:"output"`
	Limits charts.Limits `json:"limits"`
	Input  string        `json:"input"`
}

func (s *Service) begin(ctx context.Context, e identity.Envelope, action string) (context.Context, func(), error) {
	if s == nil || ctx == nil {
		return nil, nil, charts.ErrInvalid
	}
	if err := access.Require(e, action, access.Tenant(e, "read")); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	select {
	case s.slots <- struct{}{}:
	default:
		return nil, nil, ErrBusy
	}
	deadline := time.Now().Add(s.options.Timeout)
	if e.Deadline().Before(deadline) {
		deadline = e.Deadline()
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	return bounded, func() { cancel(); <-s.slots }, nil
}
func finish(ctx context.Context, e identity.Envelope, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return access.Require(e, action, access.Tenant(e, "read"))
}

// Catalog requires current signed tenant read reach even without data I/O.
func (s *Service) Catalog(ctx context.Context, e identity.Envelope) (CatalogResult, error) {
	ctx, done, err := s.begin(ctx, e, "charts.read")
	if err != nil {
		return CatalogResult{}, err
	}
	defer done()
	return CatalogResult{1, charts.Catalog(), s.options.Limits, s.options.RankEnabled && s.engine != nil}, finish(ctx, e, "charts.read")
}

// Select uses deterministic rules first; the ranker may only reorder that sealed
// suitable set. No rejected kind or invented binding may enter through ranking.
func (s *Service) Select(ctx context.Context, e identity.Envelope, in SelectRequest) (SelectionResult, error) {
	ctx, done, err := s.begin(ctx, e, "charts.select")
	if err != nil {
		return SelectionResult{}, err
	}
	defer done()
	if len(in.Intent) > 1024 || !utf8.ValidString(in.Intent) {
		return SelectionResult{}, charts.ErrInvalid
	}
	selection, err := charts.Select(ctx, in.Data, s.options.Limits)
	if err != nil {
		return SelectionResult{}, err
	}
	out := SelectionResult{Selection: selection, Limits: s.options.Limits, Provenance: Provenance{Input: "caller_supplied", RulesVersion: 1, Ranking: "not_requested", Ranked: []gateway.RankedItem{}, Receipt: gateway.Receipt{Calls: []gateway.Usage{}}, MaxCalls: s.options.RankCalls, MaxTokens: s.options.RankTokens, MaxDurationMS: s.options.RankTimeout.Milliseconds()}}
	if in.Rank {
		switch {
		case !s.options.RankEnabled || s.engine == nil:
			out.Provenance.Ranking = "disabled"
		case selection.Fallback || len(selection.Alternatives) == 0:
			out.Provenance.Ranking = "not_applicable"
		default:
			s.rank(ctx, e, in.Intent, &out)
		}
	}
	if err = finish(ctx, e, "charts.select"); err != nil {
		if len(out.Provenance.Receipt.Calls) != 0 {
			return SelectionResult{}, &Failure{Cause: err, Receipt: out.Provenance.Receipt}
		}
		return SelectionResult{}, err
	}
	return out, nil
}

func (s *Service) rank(ctx context.Context, e identity.Envelope, intent string, out *SelectionResult) {
	call, err := gateway.Authorize(e, "charts.select", "chart-specifications-v1", access.Tenant(e, "read"))
	if err != nil {
		out.Provenance.Ranking = "rules_preserved"
		return
	}
	ordered := append([]charts.Candidate{out.Selection.Selected}, out.Selection.Alternatives...)
	items := make([]gateway.Candidate, len(ordered))
	byID := make(map[string]charts.Candidate, len(ordered))
	for i, c := range ordered {
		id := string(c.Mapping.Kind)
		items[i] = gateway.Candidate{ID: id, Text: id + ": " + c.Reason, Resource: access.Tenant(e, "read")}
		byID[id] = c
	}
	candidates, err := gateway.AdmitCandidates(call, "charts.select", items)
	if err != nil {
		out.Provenance.Ranking = "rules_preserved"
		return
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: s.options.RankCalls, Tokens: s.options.RankTokens, Duration: s.options.RankTimeout})
	if err != nil {
		out.Provenance.Ranking = "rules_preserved"
		return
	}
	bounded, cancel := context.WithDeadline(ctx, budget.Deadline())
	defer cancel()
	ranked, err := s.engine.VisualRank(bounded, call, budget, intent, candidates)
	out.Provenance.ReservedCalls, out.Provenance.ReservedTokens = budget.Used()
	out.Provenance.Receipt = copyReceipt(ranked.Receipt)
	out.Provenance.Ranking = "rules_preserved"
	if err != nil || bounded.Err() != nil || budget.Check(call) != nil || ranked.Receipt.Warning != "" {
		return
	}
	if len(ranked.Items) != len(items) || out.Provenance.ReservedCalls == 0 || len(ranked.Receipt.Calls) == 0 {
		out.Provenance.Ranking = "invalid_response"
		return
	}
	seen := make(map[string]bool, len(items))
	reordered := make([]charts.Candidate, len(items))
	for i, item := range ranked.Items {
		original, ok := byID[item.ID]
		if !ok || seen[item.ID] || item.Score != nil && (math.IsNaN(*item.Score) || math.IsInf(*item.Score, 0)) {
			out.Provenance.Ranking = "invalid_response"
			return
		}
		seen[item.ID] = true
		reordered[i] = original
	}
	out.Selection.Selected = reordered[0]
	out.Selection.Alternatives = reordered[1:]
	out.Provenance.Ranking = "gateway_ranked"
	out.Provenance.Ranked = append([]gateway.RankedItem(nil), ranked.Items...)
	for i := range out.Provenance.Ranked {
		if out.Provenance.Ranked[i].Score != nil {
			v := *out.Provenance.Ranked[i].Score
			out.Provenance.Ranked[i].Score = &v
		}
	}
}

// Specify builds an explicitly selected kind; it never falls back to a table.
func (s *Service) Specify(ctx context.Context, e identity.Envelope, in SpecifyRequest) (BuildResult, error) {
	ctx, done, err := s.begin(ctx, e, "charts.bind")
	if err != nil {
		return BuildResult{}, err
	}
	defer done()
	m, err := charts.Bind(ctx, in.Data, in.Kind, in.Bindings, in.Order, in.Options, s.options.Limits)
	if err != nil {
		return BuildResult{}, err
	}
	return s.build(ctx, e, in.Data, m)
}

// Build uses only a previously chosen exact mapping, never the selector/ranker.
func (s *Service) Build(ctx context.Context, e identity.Envelope, in BuildRequest) (BuildResult, error) {
	ctx, done, err := s.begin(ctx, e, "charts.bind")
	if err != nil {
		return BuildResult{}, err
	}
	defer done()
	return s.build(ctx, e, in.Data, in.Mapping)
}
func (s *Service) build(ctx context.Context, e identity.Envelope, data charts.Data, mapping charts.Mapping) (BuildResult, error) {
	out, err := charts.Build(ctx, data, mapping, s.options.Limits)
	if err != nil {
		return BuildResult{}, err
	}
	if err = finish(ctx, e, "charts.bind"); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Output: out, Limits: s.options.Limits, Input: "caller_supplied"}, nil
}

// Rebind returns an authoring proposal. Nothing is approved, persisted or edited.
func (s *Service) Rebind(ctx context.Context, e identity.Envelope, in BuildRequest) (charts.Proposal, error) {
	ctx, done, err := s.begin(ctx, e, "charts.bind")
	if err != nil {
		return charts.Proposal{}, err
	}
	defer done()
	out, err := charts.Rebind(ctx, in.Data, in.Mapping, s.options.Limits)
	if err != nil {
		return charts.Proposal{}, err
	}
	if err = finish(ctx, e, "charts.bind"); err != nil {
		return charts.Proposal{}, err
	}
	return out, nil
}
