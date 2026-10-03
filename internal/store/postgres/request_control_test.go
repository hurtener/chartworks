package postgres

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

// Request controls use a real verifier and signed fixture authority. Raw store
// coordinates and serialized leases never construct an invocation in these tests.
type requestControlAuthority struct {
	key      *ecdsa.PrivateKey
	verifier *auth.Verifier
	issuer   string
	now      atomic.Int64
}

func newRequestControlAuthority(t *testing.T) *requestControlAuthority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "request-control", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(server.Close)
	f := &requestControlAuthority{key: key, issuer: server.URL + "/issuer"}
	f.now.Store(time.Now().Unix())
	cfg := config.Defaults().Auth
	cfg.Issuer, cfg.JWKSURL = f.issuer, server.URL+"/keys"
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	cfg.ClockSkew = 0
	f.verifier, err = auth.New(cfg, server.Client(), func() time.Time { return time.Unix(f.now.Load(), 0) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.verifier.Close)
	return f
}
func (f *requestControlAuthority) envelope(t *testing.T, tenant, actor, session string, scopes ...string) identity.Envelope {
	t.Helper()
	now := f.now.Load()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": f.issuer, "aud": "chartworks:http", "tenant": tenant, "user": actor, "sub": actor, "session": session, "iat": now, "exp": now + 300, "scopes": append([]string{}, scopes...)})
	token.Header["kid"] = "request-control"
	signed, err := token.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	e, err := f.verifier.Verify(context.Background(), signed, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func requestControlTestDB(t *testing.T) *DB {
	t.Helper()
	opts := Defaults()
	opts.MaxConns = 1 // Previously valid minimum must remain usable.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := Open(ctx, planLockTestDatabaseURL(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	// Bootstrap migrations use the normal setup budget; exercised request work
	// has a short bound so a starvation regression fails promptly.
	db.timeout = 400 * time.Millisecond
	scope, _ := store.NewScope("tenant", "actor")
	if _, err := db.SetPolicy(ctx, scope, 0, store.Policy{AuditDays: 30, OperationHours: 24}); err != nil {
		t.Fatal(err)
	}
	return db
}
func holdRequestOrdinaryPool(t *testing.T, db *DB) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() { once.Do(conn.Release) }
	t.Cleanup(release)
	if db.pool.Stat().AcquiredConns() != db.pool.Config().MaxConns {
		t.Fatal("ordinary pool is not saturated")
	}
	// A deadline failure while the connection remains held proves contention;
	// elapsed sleeps and scheduling guesses are not the saturation oracle.
	ordinary, stop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer stop()
	scope, _ := store.NewScope("tenant", "actor")
	if _, err := db.Policy(ordinary, scope); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ordinary work was not blocked: %v", err)
	}
	return release
}
func TestRequestControlSurvivesOrdinaryPoolSaturation(t *testing.T) {
	for _, saturateBeforeClaim := range []bool{true, false} {
		name := "pulse_and_failure"
		if saturateBeforeClaim {
			name = "claim"
		}
		t.Run(name, func(t *testing.T) {
			db := requestControlTestDB(t)
			signer := newRequestControlAuthority(t)
			e := signer.envelope(t, "tenant", "actor", "session", "sources.upload", "cw.source.write:source")
			limits := jobs.Defaults()
			limits.MaxAttempts = 1
			runner, err := jobs.NewRequestRunner(db, limits)
			if err != nil {
				t.Fatal(err)
			}
			input := jobs.RequestInput{Kind: "upload.load", Target: "source", InputHash: strings.Repeat("a", 64)}
			task, err := runner.Admit(context.Background(), e, "request-control", input)
			if err != nil {
				t.Fatal(err)
			}
			var release func()
			if saturateBeforeClaim {
				release = holdRequestOrdinaryPool(t, db)
				defer release()
			}
			failed := errors.New("synthetic request handler failure")
			called := false
			result, err := runner.Run(context.Background(), e, task, 3*time.Second, func(ctx context.Context, i jobs.Invocation) error {
				called = true
				if !saturateBeforeClaim {
					release = holdRequestOrdinaryPool(t, db)
				}
				state, err := db.PulseRequest(ctx, i, true, limits.Lease)
				if err != nil || state != "running" {
					t.Errorf("live request pulse starved behind ordinary work: state=%q err=%v", state, err)
				}
				return failed
			})
			if release != nil {
				release()
			}
			if !called || !errors.Is(err, failed) || result.State != "failed" || result.Code != "attempt_failed" {
				t.Fatalf("request control/final failure starved: called=%t state=%q code=%q err=%v", called, result.State, result.Code, err)
			}
			if db.pool.Stat().AcquiredConns() != 0 {
				t.Fatal("ordinary connection leaked")
			}
		})
	}
}
