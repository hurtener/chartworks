// Package gateway owns bounded inference contracts. Production inference is implemented only by bifrost.
package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

var (
	// ErrInput rejects an invalid or unauthorized inference input.
	ErrInput = errors.New("gateway: invalid input")
	// ErrOutput rejects a malformed or incomplete provider result.
	ErrOutput = errors.New("gateway: invalid provider output")
	// ErrDisabled reports an operation that is not explicitly enabled.
	ErrDisabled = errors.New("gateway: role disabled")
	// ErrUnavailable hides provider transport and credential details.
	ErrUnavailable = errors.New("gateway: provider unavailable")
	// ErrBudget reports an exhausted operation budget.
	ErrBudget = errors.New("gateway: operation budget exhausted")
	// ErrBusy reports bounded concurrency admission refusal.
	ErrBusy = errors.New("gateway: concurrency limit reached")
	// ErrClosed reports an engine that has begun shutdown.
	ErrClosed = errors.New("gateway: closed")
	// ErrSpace requires reindexing rather than mixing embedding spaces.
	ErrSpace = errors.New("gateway: embedding space mismatch; reindex required")
)

// Call is an immutable, authorized data partition. Domain services supply actual resolved references.
// It cannot turn a caller's label into a narrower source context; source validation owns that proof.
type Call struct {
	envelope  identity.Envelope
	partition string
	key       string
}

// Authorize validates every supplied reference before making any input eligible for inference.
func Authorize(e identity.Envelope, action, partition string, resources ...access.Resource) (Call, error) {
	if len(partition) == 0 || len(partition) > 256 || len(resources) == 0 {
		return Call{}, ErrInput
	}
	if err := access.Require(e, action, resources...); err != nil {
		return Call{}, err
	}
	scopes := e.Scopes()
	sort.Strings(scopes)
	refs := append([]access.Resource(nil), resources...)
	sort.Slice(refs, func(i, j int) bool {
		a, _ := json.Marshal(refs[i])
		b, _ := json.Marshal(refs[j])
		return string(a) < string(b)
	})
	material, _ := json.Marshal([]any{e.Tenant(), e.User(), e.Session(), partition, action, scopes, refs})
	hash := sha256.Sum256(material)
	return Call{envelope: e, partition: partition, key: hex.EncodeToString(hash[:])}, nil
}

// Valid checks the value's invariants and any attached authority expiry.
func (c Call) Valid() bool { return c.envelope.Valid() && c.partition != "" && c.key != "" }

// Tenant returns the verified tenant partition.
func (c Call) Tenant() string { return c.envelope.Tenant() }

// Deadline returns the effective validity deadline without extending it.
func (c Call) Deadline() time.Time { return c.envelope.Deadline() }

// Key returns the digest of the exact identity, signed reach and data context.
func (c Call) Key() string {
	return c.key
}

// Candidate contains an already resolved reference; IDs are stable output associations, not authority.
type Candidate struct {
	ID, Text string
	Resource access.Resource
}

// Candidates is a sealed, copied candidate set. Zero values and expired authority cannot be sent.
type Candidates struct {
	call  Call
	items []Candidate
}

// AdmitCandidates checks every resolved reference and seals a detached candidate set.
func AdmitCandidates(call Call, action string, items []Candidate) (Candidates, error) {
	if !call.Valid() || len(items) == 0 || len(items) > 1024 {
		return Candidates{}, ErrInput
	}
	seen := map[string]bool{}
	out := make([]Candidate, len(items))
	for i, c := range items {
		if !identity.Identifier(c.ID) || seen[c.ID] || c.Text == "" || len(c.Text) > 256<<10 {
			return Candidates{}, ErrInput
		}
		if err := access.Require(call.envelope, action, c.Resource); err != nil {
			return Candidates{}, err
		}
		seen[c.ID] = true
		out[i] = c
	}
	return Candidates{call: call, items: out}, nil
}

// Valid checks the value's invariants and any attached authority expiry.
func (c Candidates) Valid(call Call) bool {
	return call.Valid() && c.call.Valid() && c.call.Key() == call.Key() && len(c.items) > 0
}

// Items returns detached input associations; it does not authorize additional candidates.
func (c Candidates) Items() []Candidate { return append([]Candidate(nil), c.items...) }

// Usage reports observations, never a fabricated zero bill. Nil cost/counts mean unknown.
type Usage struct {
	Role           string   `json:"role"`
	Provider       string   `json:"provider"`
	RequestedModel string   `json:"requested_model"`
	ActualModel    string   `json:"actual_model,omitempty"`
	Attempts       int      `json:"attempts"`
	DurationMS     int64    `json:"duration_ms"`
	InputTokens    *int     `json:"input_tokens,omitempty"`
	OutputTokens   *int     `json:"output_tokens,omitempty"`
	CostUSD        *float64 `json:"cost_usd,omitempty"`
	Cached         bool     `json:"cached"`
}

// Receipt includes every attempted provider call, including failures with unknown usage.
type Receipt struct {
	Calls   []Usage `json:"calls"`
	Warning string  `json:"warning,omitempty"`
}

// Append adds all observed attempts and preserves a visible fallback warning.
func (r *Receipt) Append(other Receipt) {
	r.Calls = append(r.Calls, other.Calls...)
	if other.Warning != "" {
		r.Warning = other.Warning
	}
}

// Generated is validated JSON and an honest attempt receipt. Raw provider errors never escape.
type Generated struct {
	JSON    json.RawMessage
	Receipt Receipt
}

// Embedded preserves input order and identifies the entire embedding generation.
type Embedded struct {
	Vectors [][]float32
	Space   string
	Receipt Receipt
}

// RankedItem preserves stable caller IDs. A nil Score means unchanged order, not an invented zero.
type RankedItem struct {
	ID    string   `json:"id"`
	Score *float64 `json:"score,omitempty"`
}

// Ranked contains only original candidate IDs and observed scores or an explicit unchanged-order receipt.
type Ranked struct {
	Items   []RankedItem
	Receipt Receipt
}

// Preserve returns the original authorized order with unknown scores and a visible warning.
func Preserve(c Candidates, warning string, receipt Receipt) Ranked {
	receipt.Warning = warning
	out := Ranked{Receipt: receipt, Items: make([]RankedItem, len(c.items))}
	for i, item := range c.items {
		out.Items[i].ID = item.ID
	}
	return out
}

// StableRank orders finite scored candidates with ties resolved by their original input positions.
func StableRank(items []RankedItem, original map[string]int) {
	sort.SliceStable(items, func(i, j int) bool {
		if *items[i].Score == *items[j].Score {
			return original[items[i].ID] < original[items[j].ID]
		}
		return *items[i].Score > *items[j].Score
	})
}

// Engine is the only production inference seam. Constructors cannot silently select another driver.
type Engine interface {
	Generate(context.Context, Call, *Budget, string, string, string, *Schema) (Generated, error)
	Embed(context.Context, Call, *Budget, string, []string) (Embedded, error)
	Rerank(context.Context, Call, *Budget, string, Candidates) (Ranked, error)
	VisualRank(context.Context, Call, *Budget, string, Candidates) (Ranked, error)
	Space() string
	Close()
}
