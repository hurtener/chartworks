package bifrost

import (
	"github.com/hurtener/chartworks/internal/config"
	"net/url"
)

// Bifrost's proxy transport delegates name resolution to the trusted proxy.
// Its direct-IP safety check is therefore unavailable. This constructor-only
// option is confined to the already approved fixed HTTPS provider destination,
// rather than letting an arbitrary configured URL acquire proxy network reach.
func environmentProxyDestination(p config.Provider, t TransportOptions) bool {
	if t.AllowPrivateNetwork || t.CACertPEM != "" {
		return false
	}
	kind := config.NativeProvider(p)
	if kind != "openrouter" && kind != "openrouter_rerank" {
		return false
	}
	if p.BaseURL == "" {
		return true
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host != "openrouter.ai" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawPath != "" {
		return false
	}
	if kind == "openrouter_rerank" {
		return u.Path == "" || u.Path == "/"
	}
	return u.Path == "" || u.Path == "/" || u.Path == "/api/v1" || u.Path == "/api/v1/"
}
