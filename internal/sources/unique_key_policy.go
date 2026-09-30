package sources

import (
	"context"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

// A stored legacy source without key evidence never gains join authority merely
// by upgrading the binary. Reproduce its original catalog fingerprint until an
// explicit source revision rediscovers keys. A stored key-bearing binding cannot
// choose this path, so removal/change of any previously proved key stays fenced.
type legacyUniqueKeyPolicy struct{}

func withStoredKeyPolicy(ctx context.Context, b readexec.Binding) context.Context {
	for _, r := range b.Relations {
		if len(r.UniqueKeys) > 0 {
			return context.WithValue(ctx, legacyUniqueKeyPolicy{}, false)
		}
	}
	return context.WithValue(ctx, legacyUniqueKeyPolicy{}, true)
}
