// Package foundation composes configuration, verified operational APIs and lifecycle.
package foundation

import (
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"net/http"
)

// Dependency is a sanitized observation from the shared verifier cache.
type Dependency = auth.Dependency

// KeyProbe is retained for earlier foundation callers; the implementation is shared with auth.
type KeyProbe = auth.KeyProbe

// NewKeyProbe delegates to the single trusted public-key loader.
func NewKeyProbe(cfg config.Auth, c *http.Client) *KeyProbe { return auth.NewKeyProbe(cfg, c) }
func validKeys(data []byte, allowed []string) bool          { return auth.ValidPublicKeys(data, allowed) }
