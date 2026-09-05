package acceptance

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
)

func TestActualAsymmetricAlgorithms(t *testing.T) {
	f := newTokenFixture(t)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []jwt.SigningMethod{jwt.SigningMethodRS256, jwt.SigningMethodRS384, jwt.SigningMethodRS512, jwt.SigningMethodES256, jwt.SigningMethodES384, jwt.SigningMethodES512} {
		t.Run(method.Alg(), func(t *testing.T) {
			var signingKey any = rsaKey
			jwk := map[string]any{"kid": "algorithm-test", "kty": "RSA", "alg": method.Alg(), "n": base64.RawURLEncoding.EncodeToString(rsaKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(rsaKey.E)).Bytes())}
			if strings.HasPrefix(method.Alg(), "ES") {
				var curve elliptic.Curve
				switch method.Alg() {
				case "ES256":
					curve = elliptic.P256()
				case "ES384":
					curve = elliptic.P384()
				case "ES512":
					curve = elliptic.P521()
				}
				key, err := ecdsa.GenerateKey(curve, rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				signingKey = key
				size := (curve.Params().BitSize + 7) / 8
				jwk = map[string]any{"kid": "algorithm-test", "kty": "EC", "alg": method.Alg(), "crv": curve.Params().Name, "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, size))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, size)))}
			}
			body, err := json.Marshal(map[string]any{"keys": []any{jwk}})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
			defer server.Close()
			cfg := f.cfg
			cfg.JWKSURL = server.URL
			cfg.Algorithms = []string{method.Alg()}
			verifier, err := auth.New(cfg, server.Client(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer verifier.Close()
			token := jwt.NewWithClaims(method, f.claims("tenant", "user", []string{"ops.read"}))
			token.Header["kid"] = "algorithm-test"
			signed, err := token.SignedString(signingKey)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = verifier.Verify(context.Background(), signed, auth.HTTP); err != nil {
				t.Fatal("valid asymmetric token denied", err)
			}
			// Same kid, wrong curve/key family and method may not be interpreted as interchangeable.
			bad := f.sign(t, f.claims("tenant", "user", nil), map[string]any{"kid": "algorithm-test"})
			if method.Alg() != "ES256" {
				if _, err = verifier.Verify(context.Background(), bad, auth.HTTP); err == nil {
					t.Fatal("key/algorithm confusion")
				}
			}
		})
	}
	if _, err = auth.New(config.Auth{}, nil, nil); err == nil {
		t.Fatal("unvalidated direct verifier construction")
	}
}
func TestSignedDuplicateClaimsAndKeyMetadata(t *testing.T) {
	f := newTokenFixture(t)
	c := f.claims("tenant", "user", []string{"ops.read"})
	body, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	header := `{"alg":"ES256","kid":"test-key","typ":"JWT"}`
	rawSign := func(h string, b []byte) string {
		input := base64.RawURLEncoding.EncodeToString([]byte(h)) + "." + base64.RawURLEncoding.EncodeToString(b)
		sig, err := jwt.SigningMethodES256.Sign(input, f.private)
		if err != nil {
			t.Fatal(err)
		}
		return input + "." + base64.RawURLEncoding.EncodeToString(sig)
	}
	dup := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"tenant":"foreign"}`)...)
	for _, token := range []string{rawSign(header, dup), rawSign(`{"alg":"ES256","kid":"test-key","kid":"other"}`, body), rawSign(header, []byte(`{"x":1,"x":2}`))} {
		if _, err = f.verifier.Verify(context.Background(), token, auth.HTTP); err == nil {
			t.Fatal("valid signature concealed duplicate claims/header")
		}
	}
	var jwks map[string]any
	if json.Unmarshal(f.doc, &jwks) != nil {
		t.Fatal("fixture JSON")
	}
	keys := jwks["keys"].([]any)
	key := keys[0].(map[string]any)
	key["d"] = "private-must-not-be-used"
	encoded, _ := json.Marshal(jwks)
	f.set(encoded, 200)
	if f.verifier.Check(context.Background()).Ready {
		t.Fatal("private key published as verification material")
	}
}
func FuzzActualVerifier(f *testing.F) {
	fixture := newTokenFixture(f)
	valid := fixture.sign(f, fixture.claims("tenant", "user", []string{"ops.read"}), nil)
	f.Add(valid)
	f.Add("a.b.c")
	f.Add("")
	f.Fuzz(func(t *testing.T, token string) {
		e, err := fixture.verifier.Verify(context.Background(), token, auth.HTTP)
		if err == nil && (!e.Valid() || e.Tenant() != "tenant" || e.User() != "user") {
			t.Fatal("invalid authority accepted")
		}
	})
}
