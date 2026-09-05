package foundation

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
)

func encodeKey(t *testing.T, key map[string]any) []byte {
	t.Helper()
	b, e := json.Marshal(map[string]any{"keys": []any{key}})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestPublicKeyValidation(t *testing.T) {
	allowed := []string{"RS256", "ES256", "ES384", "ES512"}
	k, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal("fixture RSA generation failed")
	}
	rsaKey := map[string]any{"kty": "RSA", "kid": "rsa", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()), "key_ops": []string{"verify"}}
	if !validKeys(encodeKey(t, rsaKey), allowed) {
		t.Fatal("valid RSA rejected")
	}
	for _, change := range []map[string]any{{"e": "AA"}, {"n": "AA"}, {"alg": "ES256"}, {"d": "private"}, {"key_ops": []string{"sign"}}, {"kid": ""}, {"kid": " spaced "}, {"use": "enc"}, {"alg": 12}, {"alg": nil}} {
		copy := map[string]any{}
		for k, v := range rsaKey {
			copy[k] = v
		}
		for k, v := range change {
			copy[k] = v
		}
		if validKeys(encodeKey(t, copy), allowed) {
			t.Errorf("malformed RSA accepted: %v", change)
		}
	}
	for _, curve := range []elliptic.Curve{elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		key, e := ecdsa.GenerateKey(curve, rand.Reader)
		if e != nil {
			t.Fatal("EC generation failed")
		}
		size := (curve.Params().BitSize + 7) / 8
		value := map[string]any{"kty": "EC", "kid": "ec", "crv": curve.Params().Name, "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, size))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, size)))}
		if !validKeys(encodeKey(t, value), allowed) {
			t.Fatal("valid EC rejected")
		}
		value["x"] = "AA"
		if validKeys(encodeKey(t, value), allowed) {
			t.Fatal("malformed EC accepted")
		}
	}
	for _, data := range []string{`{}`, `{"keys":[]}`, `{"keys":[{"kty":"oct","kid":"x"}]}`, `{"keys":null}`, `{"keys":[],"keys":[]}`, `{"keys":[{"kid":"x","kid":"y"}]}`, `{"keys":[{"kid":"x","kty":"EC","crv":"bad"}]}`, `{} {}`, `null`} {
		if validKeys([]byte(data), allowed) {
			t.Fatal("malformed keys accepted")
		}
	}
	delete(rsaKey, "alg")
	if validKeys(encodeKey(t, rsaKey), []string{"ES256"}) {
		t.Fatal("wrong type inferred")
	}
	duplicate, _ := json.Marshal(map[string]any{"keys": []any{rsaKey, rsaKey}})
	if validKeys(duplicate, allowed) {
		t.Fatal("duplicate IDs accepted")
	}
}
func TestKeyHTTPFailures(t *testing.T) {
	for _, mode := range []string{"redirect", "oversize", "status", "invalid", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "redirect":
					http.Redirect(w, r, "https://other.example", http.StatusFound)
				case "oversize":
					_, _ = io.WriteString(w, strings.Repeat("x", (1<<20)+2))
				case "status":
					w.WriteHeader(503)
				case "invalid":
					_, _ = io.WriteString(w, `{"keys":[]}`)
				case "timeout":
					<-r.Context().Done()
				}
			}))
			defer s.Close()
			p := NewKeyProbe(config.Auth{JWKSURL: s.URL, Algorithms: []string{"ES256"}, RequestTimeout: config.Duration(50 * time.Millisecond), JWKSMaxStale: config.Duration(time.Second)}, s.Client())
			defer p.Close()
			if p.Check(context.Background()).Ready {
				t.Fatal("bad key endpoint healthy")
			}
		})
	}
	p := NewKeyProbe(config.Auth{JWKSURL: "://", RequestTimeout: config.Duration(time.Millisecond)}, nil)
	defer p.Close()
	if p.Check(context.Background()).Ready {
		t.Fatal("invalid URL healthy")
	}
}
func FuzzVerificationKeys(f *testing.F) {
	f.Add(`{"keys":[]}`)
	f.Add(`{"keys":[{"kty":"RSA","kid":"a"}]}`)
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 1<<20 {
			return
		}
		_ = validKeys([]byte(s), []string{"RS256", "ES256"})
	})
}
