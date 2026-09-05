package gateway

import (
	"sync"
	"time"
)

// Limits is a pessimistic operation reservation budget, not provider-reported usage.
type Limits struct {
	Calls, Tokens int
	Duration      time.Duration
}

// Budget is concurrency-safe, nonrefundable and bound to one actor/session/data partition.
// Input reservations use UTF-8 bytes plus protocol/schema framing; actual token usage is reported separately.
// Unknown failed attempts remain charged. A caller cannot gain retries by forgetting its last receipt.
type Budget struct {
	mu                                 sync.Mutex
	key                                string
	deadline                           time.Time
	maxCalls, maxTokens, calls, tokens int
}

func NewBudget(call Call, limits Limits) (*Budget, error) {
	if !call.Valid() || limits.Calls < 1 || limits.Calls > 64 || limits.Tokens < 1 || limits.Tokens > 16<<20 || limits.Duration <= 0 || limits.Duration > 10*time.Minute {
		return nil, ErrInput
	}
	deadline := time.Now().Add(limits.Duration)
	if call.Deadline().Before(deadline) {
		deadline = call.Deadline()
	}
	return &Budget{key: call.Key(), deadline: deadline, maxCalls: limits.Calls, maxTokens: limits.Tokens}, nil
}
func (b *Budget) Reserve(call Call, tokens int) error {
	if b == nil || !call.Valid() {
		return ErrBudget
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if call.Key() != b.key || !time.Now().Before(b.deadline) || tokens < 1 || b.calls >= b.maxCalls || tokens > b.maxTokens-b.tokens {
		return ErrBudget
	}
	b.calls++
	b.tokens += tokens
	return nil
}
func (b *Budget) Deadline() time.Time {
	if b == nil {
		return time.Time{}
	}
	return b.deadline
}
func (b *Budget) Used() (int, int) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls, b.tokens
}
