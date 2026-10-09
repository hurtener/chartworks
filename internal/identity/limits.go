package identity

// The provider authority budget is shared with Pengui's exact-scope mint.
// These are size ceilings, never permission to infer, truncate or widen reach.
const (
	MaxScopes          = 128
	MaxScopeBytes      = 256
	MaxTotalScopeBytes = 16 << 10
)
