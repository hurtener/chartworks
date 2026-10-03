// Package containment admits the kernel memory boundary for isolated render jobs.
package containment

import "errors"

const (
	// ContractVersion binds the paired supervisor and worker configuration.
	ContractVersion = "charged-memory-v1"
	// MaxChargedBytes is the maximum kernel cgroup memory budget, including
	// charged anonymous and file-backed pages. It is not a resident-set promise.
	MaxChargedBytes int64 = 1 << 30
	// AddressSpaceBytes accommodates Go virtual reservations independently of
	// charged memory. This ceiling never substitutes for cgroup enforcement.
	AddressSpaceBytes int64 = 3 << 30
)

var ErrUnavailable = errors.New("rendering: charged memory isolation unavailable")
var ErrCleanup = errors.New("rendering: memory isolation cleanup failed")

func ValidBudget(n int64) bool { return n >= 32<<20 && n <= MaxChargedBytes }

// HeapBytes leaves room for stacks, runtime state and charged file pages.
// Go's heap target is advisory; memory.max is the independent kernel limit.
func HeapBytes(n int64) int64 { return n / 2 }
