// Package foundation composes implemented configuration, health, and store lifecycle.
// It does not authenticate requests or expose business APIs before phases 03/04.
package foundation

import (
	"context"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/config"
)

// Dependency is a bounded health observation, not an authorization decision.
type Dependency struct {
	Ready      bool
	ValidUntil time.Time
}

// KeyProbe checks usable verification-key material at a trusted configured URL.
// Phase 03 will consume the verifier's key-cache health through the same Checker seam.
// This probe does not verify tokens, sign anything, or accept caller key URLs.
type KeyProbe struct {
	mu     sync.Mutex
	auth   config.Auth
	client *http.Client
	until  time.Time
}

// NewKeyProbe pins the transport and rejects redirects. A supplied client is for trusted transport configuration.
func NewKeyProbe(auth config.Auth, client *http.Client) *KeyProbe {
	c := http.Client{}
	if client != nil {
		c = *client
	}
	c.Timeout = time.Duration(auth.RequestTimeout)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("verification key redirect refused") }
	auth.Algorithms = append([]string(nil), auth.Algorithms...)
	return &KeyProbe{auth: auth, client: &c}
}

// Check performs a bounded refresh. A failed refresh cannot extend the last success's validity.
func (p *KeyProbe) Check(ctx context.Context) Dependency {
	p.mu.Lock()
	defer p.mu.Unlock()
	ok := false
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, p.auth.JWKSURL, nil)
	if e == nil {
		var resp *http.Response
		resp, e = p.client.Do(req)
		if e == nil {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode == http.StatusOK {
				b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
				ok = err == nil && len(b) <= 1<<20 && validKeys(b, p.auth.Algorithms)
			}
		}
	}
	now := time.Now()
	if ok {
		p.until = now.Add(time.Duration(p.auth.JWKSMaxStale))
	}
	return Dependency{Ready: now.Before(p.until), ValidUntil: p.until}
}
func str(m map[string]json.RawMessage, k string) string {
	var s string
	_ = json.Unmarshal(m[k], &s)
	return s
}
func decode(s string) []byte {
	v, e := base64.RawURLEncoding.DecodeString(s)
	if e != nil {
		return nil
	}
	return v
}
func validKeys(data []byte, allowed []string) bool {
	var doc struct {
		Keys []map[string]json.RawMessage `json:"keys"`
	}
	if json.Unmarshal(data, &doc) != nil || len(doc.Keys) == 0 || len(doc.Keys) > 32 {
		return false
	}
	algorithms := map[string]bool{}
	for _, a := range allowed {
		algorithms[a] = true
	}
	seen := map[string]bool{}
	for _, k := range doc.Keys {
		kid := str(k, "kid")
		if len(kid) == 0 || len(kid) > 128 || strings.TrimSpace(kid) != kid || seen[kid] {
			return false
		}
		seen[kid] = true
		for _, name := range []string{"d", "p", "q", "dp", "dq", "qi", "oth", "k"} {
			if _, ok := k[name]; ok {
				return false
			}
		}
		if use := str(k, "use"); use != "" && use != "sig" {
			return false
		}
		if ops, exists := k["key_ops"]; exists {
			var values []string
			if json.Unmarshal(ops, &values) != nil || len(values) != 1 || values[0] != "verify" {
				return false
			}
		}
		alg := str(k, "alg")
		if alg != "" && !algorithms[alg] {
			return false
		}
		switch str(k, "kty") {
		case "RSA":
			if alg != "" && !strings.HasPrefix(alg, "RS") {
				return false
			}
			if alg == "" && !algorithms["RS256"] && !algorithms["RS384"] && !algorithms["RS512"] {
				return false
			}
			n := new(big.Int).SetBytes(decode(str(k, "n")))
			eb := decode(str(k, "e"))
			if n.BitLen() < 2048 || n.BitLen() > 8192 || len(eb) == 0 || len(eb) > 4 {
				return false
			}
			exponent := new(big.Int).SetBytes(eb).Int64()
			if exponent < 3 || exponent > 2147483647 || exponent%2 == 0 {
				return false
			}
		case "EC":
			var curve elliptic.Curve
			var expected string
			switch str(k, "crv") {
			case "P-256":
				curve = elliptic.P256()
				expected = "ES256"
			case "P-384":
				curve = elliptic.P384()
				expected = "ES384"
			case "P-521":
				curve = elliptic.P521()
				expected = "ES512"
			default:
				return false
			}
			if !algorithms[expected] || (alg != "" && alg != expected) {
				return false
			}
			x, y := decode(str(k, "x")), decode(str(k, "y"))
			size := (curve.Params().BitSize + 7) / 8
			if len(x) != size || len(y) != size || !curve.IsOnCurve(new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
