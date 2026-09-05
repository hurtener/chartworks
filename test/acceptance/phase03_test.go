package acceptance

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestPhase03(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newTokenFixture(t)
		good := f.sign(t, f.claims("tenant", "user", []string{"ops.read", "cw.tenant.read:tenant"}), nil)
		if _, err := f.verifier.Verify(context.Background(), good, auth.HTTP); err != nil {
			t.Fatal(err)
		}
		for name, h := range map[string]map[string]any{"jku": {"jku": "https://untrusted.invalid"}, "embedded-key": {"jwk": map[string]any{"kty": "RSA"}}, "critical": {"crit": []string{"exp"}}, "x5u": {"x5u": "https://untrusted.invalid"}, "wrong-type": {"typ": "registration"}, "unknown-key": {"kid": "unknown"}, "key-type": {"kid": 123}} {
			t.Run(name, func(t *testing.T) {
				s := f.sign(t, f.claims("tenant", "user", nil), h)
				if _, err := f.verifier.Verify(context.Background(), s, auth.HTTP); err == nil {
					t.Fatal("accepted forbidden JOSE header")
				}
			})
		}
		parts := strings.Split(good, ".")
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		sig[0] ^= 1
		parts[2] = base64.RawURLEncoding.EncodeToString(sig)
		for _, s := range []string{"", strings.Join(parts, "."), good + "=", good + ".extra", strings.Repeat("x", 65537)} {
			if _, err := f.verifier.Verify(context.Background(), s, auth.HTTP); err == nil {
				t.Fatal("accepted bad token")
			}
		}
		for _, method := range []jwt.SigningMethod{jwt.SigningMethodHS256, jwt.SigningMethodNone} {
			tok := jwt.NewWithClaims(method, f.claims("tenant", "user", nil))
			tok.Header["kid"] = "test-key"
			var key any = []byte("not-an-authority")
			if method == jwt.SigningMethodNone {
				key = jwt.UnsafeAllowNoneSignatureType
			}
			s, err := tok.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.verifier.Verify(context.Background(), s, auth.HTTP); err == nil {
				t.Fatal("accepted unsupported algorithm")
			}
		}
		before := f.requests.Load()
		f.set(nil, http.StatusServiceUnavailable)
		f.clock.Add(2)
		if !f.verifier.Check(context.Background()).Ready {
			t.Fatal("discarded fresh prior keys")
		}
		f.clock.Add(20)
		if f.verifier.Check(context.Background()).Ready {
			t.Fatal("extended failed refresh freshness")
		}
		if _, err := f.verifier.Verify(context.Background(), good, auth.HTTP); err == nil {
			t.Fatal("accepted stale verification material")
		}
		if f.requests.Load() < before+1 {
			t.Fatal("did not refresh")
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newTokenFixture(t)
		for _, field := range []string{"tenant", "user", "session", "scopes", "iss", "aud", "iat", "exp"} {
			t.Run("missing-"+field, func(t *testing.T) {
				c := f.claims("tenant", "user", nil)
				delete(c, field)
				if _, err := f.verifier.Verify(context.Background(), f.sign(t, c, nil), auth.HTTP); err == nil {
					t.Fatal("missing mandatory claim accepted")
				}
			})
		}
		changes := map[string]map[string]any{
			"issuer": {"iss": "https://different.invalid"}, "audience": {"aud": "chartworks:mcp"}, "subject": {"sub": "different"}, "float-exp": {"exp": float64(f.clock.Load()) + 300.5}, "string-iat": {"iat": "123"}, "boolean-nbf": {"nbf": false}, "null-nbf": {"nbf": nil}, "future-iat": {"iat": f.clock.Load() + 90}, "future-nbf": {"nbf": f.clock.Load() + 90}, "expired": {"exp": f.clock.Load() - 60, "iat": f.clock.Load() - 100}, "too-long": {"exp": f.clock.Load() + 901}, "bad-order": {"exp": f.clock.Load()}, "nbf-after-exp": {"nbf": f.clock.Load() + 400}, "duplicate-aud": {"aud": []string{"chartworks:http", "chartworks:http"}}, "many-aud": {"aud": []string{"chartworks:http", "one", "two"}}, "wrong-tenant-type": {"tenant": 12}, "blank-session": {"session": " "}, "upper-service": {"user": "Svc:worker", "sub": "Svc:worker"}, "blank-service": {"user": "svc:", "sub": "svc:"},
		}
		for name, patch := range changes {
			t.Run(name, func(t *testing.T) {
				c := f.claims("tenant", "user", nil)
				for k, v := range patch {
					c[k] = v
				}
				if _, err := f.verifier.Verify(context.Background(), f.sign(t, c, nil), auth.HTTP); err == nil {
					t.Fatal("malformed identity/time accepted")
				}
			})
		}
		c := f.claims("tenant", "user", nil)
		c["aud"] = []string{"chartworks:http", "chartworks:mcp"}
		s := f.sign(t, c, nil)
		for _, surface := range []auth.Surface{auth.HTTP, auth.MCP} {
			if _, err := f.verifier.Verify(context.Background(), s, surface); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.verifier.Verify(context.Background(), s, 99); err == nil {
			t.Fatal("unknown surface accepted")
		}
		c["aud"] = "chartworks:http"
		delete(c, "nbf")
		delete(c, "sub")
		if _, err := f.verifier.Verify(context.Background(), f.sign(t, c, nil), auth.HTTP); err != nil {
			t.Fatal("optional absent sub/nbf rejected")
		}
	})
	t.Run("AC03", func(t *testing.T) {
		f := newTokenFixture(t)
		e := f.envelope(t, "tenant", "user", "ops.read", "cw.tenant.read:tenant")
		s := e.Scopes()
		s[0] = "ops.write"
		r := e.Reach()
		r[0].ID = "*"
		if e.Has("ops.write") || e.Reach()[0].ID == "*" {
			t.Fatal("mutable authority")
		}
		if strings.Contains(fmt.Sprintf("%v %+v %#v", e, e, e), "tenant") {
			t.Fatal("implicit identity logging")
		}
		encoded, err := json.Marshal(e)
		if err != nil || strings.Contains(string(encoded), "user") {
			t.Fatal("authority serialized")
		}
		if _, err = identity.FromContext(context.Background()); err == nil {
			t.Fatal("ambient authority")
		}
		if _, err = (identity.Envelope{}).Context(context.Background()); err == nil {
			t.Fatal("zero authority")
		}
		called := false
		h := f.verifier.Middleware(auth.HTTP, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			env, err := identity.FromContext(r.Context())
			if err != nil || env.Tenant() != "tenant" || env.User() != "user" || env.Session() != "test-session" {
				t.Error("identity override")
			}
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		token := f.sign(t, f.claims("tenant", "user", nil), nil)
		w := callProtected(t, h, "POST", "/?tenant=foreign&access_token=bad", token, `{"tenant":"foreign","user":"admin"}`, map[string]string{"X-Tenant-ID": "foreign", "X-User-ID": "admin"})
		if w.Code != 204 || !called {
			t.Fatal("verified middleware not called")
		}
		f.clock.Add(400)
		if e.Valid() || e.Has("ops.read") {
			t.Fatal("expired envelope reusable")
		}
		if _, err = e.Context(context.Background()); err == nil {
			t.Fatal("expired context installed")
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newTokenFixture(t)
		e := f.envelope(t, "tenant", "svc:worker", "ops.read", "cw.tenant.read:tenant")
		if !e.Service() || e.User() != "svc:worker" || e.Has("ops.maintain") {
			t.Fatal("service attribution or implicit privilege")
		}
		e = f.envelope(t, "tenant", "user", "admin", "creator", "agent")
		if e.Service() || e.Has("reporting.execute") {
			t.Fatal("role inference")
		}
		cases := [][]string{{"ops.read", "ops.read"}, {""}, {"cw.source.query:prefix*"}, {"cw.source.query:a%2Fb"}, {"cw.source.query:a/b"}, {"cw.source.query:"}, {"cw.unknown.read:id"}, {"cw.source.own:id"}, {"ops.read other"}, {strings.Repeat("x", 257)}}
		tooMany := []string{}
		for i := 0; i < 33; i++ {
			tooMany = append(tooMany, fmt.Sprintf("scope-%d", i))
		}
		cases = append(cases, tooMany)
		tooLarge := []string{}
		for i := 0; i < 32; i++ {
			tooLarge = append(tooLarge, fmt.Sprintf("%03d", i)+strings.Repeat("x", 200))
		}
		cases = append(cases, tooLarge)
		for _, scopes := range cases {
			c := f.claims("tenant", "user", scopes)
			if _, err := f.verifier.Verify(context.Background(), f.sign(t, c, nil), auth.HTTP); err == nil {
				t.Fatal("bad scope set accepted")
			}
		}
		for _, field := range []string{"scope", "tenant_id", "user_id", "session_id", "audience"} {
			c := f.claims("tenant", "user", nil)
			c[field] = "override"
			if _, err := f.verifier.Verify(context.Background(), f.sign(t, c, nil), auth.HTTP); err == nil {
				t.Fatal("parallel claim representation")
			}
		}
		for _, value := range []any{"ops.read", []any{123}, nil} {
			c := f.claims("tenant", "user", nil)
			c["scopes"] = value
			if _, err := f.verifier.Verify(context.Background(), f.sign(t, c, nil), auth.HTTP); err == nil {
				t.Fatal("ambiguous scopes")
			}
		}
		c := f.claims("tenant", "user", []string{"cw.source.query:source:v1"})
		if _, err := f.verifier.Verify(context.Background(), f.sign(t, c, nil), auth.HTTP); err != nil {
			t.Fatal("canonical colon ID rejected")
		}
	})
	t.Run("AC05", func(t *testing.T) {
		// Production has verification imports, but no call to a signing primitive or local issuer.
		repo, err := os.OpenRoot("../..")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = repo.Close() }()
		for _, root := range []string{"internal", "cmd", "sdk"} {
			err := fs.WalkDir(repo.FS(), root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return nil
				}
				b, err := repo.ReadFile(path)
				if err != nil {
					return err
				}
				for _, bad := range []string{".SignedString(", "jwt.NewWithClaims(", "rsa.SignPKCS1v15(", "ecdsa.Sign("} {
					if strings.Contains(string(b), bad) {
						t.Errorf("production signer in %s", path)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		f := newTokenFixture(t)
		called := 0
		h := f.verifier.Middleware(auth.HTTP, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called++ }))
		for _, header := range []string{"", "Bearer", "Bearer ", "Basic secret", "Bearer a b", "Bearer a,b", "Bearer\tsecret"} {
			w := callProtected(t, h, "GET", "/", "", "", map[string]string{"Authorization": header, "Cookie": "access_token=not-authority"})
			if w.Code != 401 {
				t.Fatal("alternate credential accepted")
			}
		}
		req := httpRequestWithDuplicateHeader()
		w := newRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 401 || called != 0 {
			t.Fatal("duplicate Authorization accepted")
		}
	})
	t.Run("AC06", func(t *testing.T) {
		f := newTokenFixture(t)
		old := f.sign(t, f.claims("tenant", "user", nil), nil)
		if _, err := f.verifier.Verify(context.Background(), old, auth.HTTP); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := f.verifier.Verify(context.Background(), old, auth.HTTP); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if f.requests.Load() != 1 {
			t.Fatal("unbounded refresh on known key")
		}
		newer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		f.set(publicDocument(t, newer, "new-key"), 200)
		f.clock.Add(2)
		if !f.verifier.Check(context.Background()).Ready {
			t.Fatal("rotation failed")
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodES256, f.claims("tenant", "user", nil))
		tok.Header["kid"] = "new-key"
		fresh, err := tok.SignedString(newer)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.verifier.Verify(context.Background(), fresh, auth.HTTP); err != nil {
			t.Fatal(err)
		}
		if _, err = f.verifier.Verify(context.Background(), old, auth.HTTP); err == nil {
			t.Fatal("removed key accepted")
		}
		before := f.requests.Load()
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, _ = f.verifier.Verify(context.Background(), old, auth.HTTP) }()
		}
		wg.Wait()
		if f.requests.Load() != before {
			t.Fatal("unknown-kid network amplification")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err = f.verifier.Verify(ctx, fresh, auth.HTTP); err == nil {
			t.Fatal("cancelled verification succeeded")
		}
		for _, raw := range []string{`{"alg":"ES256","alg":"RS256"}`, `{"scopes":[null]}`, `{"x":1} {"x":2}`, `[]`, "{", strings.Repeat("[", 18) + "1" + strings.Repeat("]", 18)} {
			if _, err := auth.Object([]byte(raw), 4096); err == nil {
				t.Fatal("malformed JSON accepted")
			}
		}
	})
}
