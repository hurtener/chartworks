package migrationapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/store"
)

type migrationHTTPFixture struct {
	verifier *auth.Verifier
	server   *httptest.Server
	private  *ecdsa.PrivateKey
	issuer   string
	now      time.Time
}

func newMigrationHTTPFixture(t *testing.T) *migrationHTTPFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "migration-test", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	f := &migrationHTTPFixture{private: key, now: time.Now().Truncate(time.Second)}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(document) }))
	t.Cleanup(f.server.Close)
	f.issuer = f.server.URL + "/issuer"
	cfg := config.Defaults().Auth
	cfg.Issuer, cfg.JWKSURL, cfg.Audience = f.issuer, f.server.URL+"/keys", ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	cfg.RefreshInterval, cfg.JWKSMaxStale = config.Duration(time.Second), config.Duration(10*time.Second)
	f.verifier, err = auth.New(cfg, f.server.Client(), func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.verifier.Close)
	return f
}

func (f *migrationHTTPFixture) token(t *testing.T, scopes ...string) string {
	t.Helper()
	sort.Strings(scopes)
	now := f.now.Unix()
	claims := jwt.MapClaims{"iss": f.issuer, "aud": "chartworks:http", "sub": "operator", "tenant": "tenant", "user": "operator", "session": "session", "iat": now, "nbf": now - 1, "exp": now + 300, "scopes": scopes}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "migration-test"
	signed, err := token.SignedString(f.private)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func migrationHTTPManifest() migration.Manifest {
	hash := strings.Repeat("a", 64)
	evidence := make([]migration.Evidence, 0, 63)
	for _, group := range []struct {
		prefix string
		count  int
	}{{"B", 20}, {"R", 16}, {"Q", 10}, {"N", 16}} {
		for i := 1; i <= group.count; i++ {
			feature := fmt.Sprintf("%s%02d", group.prefix, i)
			evidence = append(evidence, migration.Evidence{Feature: feature, OwnerFeature: feature, Disposition: "required", Outcome: "passed", EvidenceType: "live", Reference: "ref-" + feature, Source: "evaluation", SourceVersion: hash, EvidenceHash: hash, ComparisonHash: hash, Engine: "postgres", Dialect: "postgres", SourceSnapshot: hash, SourceRevision: 1})
		}
	}
	evidence = append(evidence, migration.Evidence{Feature: "Q11", OwnerFeature: "EVAL-01", Disposition: "excluded", Outcome: "unsupported", EvidenceType: "operator", Reference: "discard", Source: "synthetic", SourceVersion: hash, EvidenceHash: hash})
	return migration.Manifest{Version: migration.ManifestVersion, Batch: "batch", Cohort: "cohort", SourceSnapshot: hash, Engine: "postgres", Dialect: "postgres", Mappings: []migration.Mapping{{Kind: migration.KindSource, ExternalRef: "source", Destination: "target", Revision: 1}}, Objects: []migration.Object{{Kind: migration.KindSource, ExternalRef: "source", Revision: 1, PayloadVersion: "v1", Payload: `{"engine":"postgres","dialect":"postgres","snapshot":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","context":"target:v1","revision":1}`, Lifecycle: "private_draft", Private: true, Origin: "synthetic"}}, Fields: []migration.FieldDisposition{{Path: "source.engine", Status: "retained"}, {Path: "source.dialect", Status: "retained"}, {Path: "source.snapshot", Status: "retained"}, {Path: "source.context", Status: "retained"}, {Path: "source.revision", Status: "retained"}}, Evidence: evidence}
}

func TestHTTPRoutesUseVerifiedAuthorityAndTypedService(t *testing.T) {
	f := newMigrationHTTPFixture(t)
	adapter := migration.AdapterFuncs{ValidateFunc: func(context.Context, identity.Envelope, migration.Object, migration.Mapping) error { return nil }, ApplyFunc: func(context.Context, identity.Envelope, migration.Object, migration.Mapping, string) (string, error) {
		return "target", nil
	}}
	service, err := migration.New(migration.NewMemoryRepository(nil), map[migration.Kind]migration.Adapter{migration.KindSource: adapter}, nil, migration.EvidenceVerifierFunc(func(context.Context, identity.Envelope, migration.Evidence) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(418) })
	handler := Handler(f.verifier, service, fallback)
	token := f.token(t, "migration.read", "migration.write", "migration.cutover", "migration.erase", "ops.read", "cw.tenant.read:tenant", "cw.tenant.write:tenant", "cw.tenant.erase:tenant", "cw.tenant.export:tenant")
	manifest := migrationHTTPManifest()
	dry, _ := json.Marshal(migration.DryRunRequest{Manifest: manifest})
	imp, _ := json.Marshal(migration.ImportRequest{Manifest: manifest})
	tests := []struct {
		path   string
		body   []byte
		status int
	}{
		{"/v1/migrations/dry-runs", dry, 200},
		{"/v1/migrations/imports", imp, 200},
		{"/v1/migrations/resume", []byte(`{"batch":"missing","expected_revision":1}`), 404},
		{"/v1/migrations/exports", []byte(`{"batch":"batch","limit":1}`), 200},
		{"/v1/migrations/cutovers", []byte(`{"batch":"missing","expected_generation":0,"route":"route","operator_reference":"drill"}`), 404},
		{"/v1/migrations/rollbacks", []byte(`{"cohort":"missing","expected_generation":1,"operator_reference":"drill","irreversible_effects":[]}`), 404},
		{"/v1/migrations/erasures", []byte(`{"batch":"missing","limit":1}`), 404},
	}
	for _, test := range tests {
		req := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(test.body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != test.status || strings.Contains(w.Body.String(), "PRIVATE") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: status=%d body=%s", test.path, w.Code, w.Body.String())
		}
	}
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/migrations/dry-runs", bytes.NewReader(dry)))
	if unauthorized.Code != 401 {
		t.Fatal("missing bearer accepted")
	}
	outside := httptest.NewRecorder()
	handler.ServeHTTP(outside, httptest.NewRequest(http.MethodGet, "/outside", nil))
	if outside.Code != 418 {
		t.Fatal("unrelated route did not fall through")
	}
}

func TestHTTPFailureAndDecodeAreClosed(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{access.ErrUnauthenticated, 401}, {access.ErrForbidden, 403}, {access.ErrNotFound, 404}, {migration.ErrNotFound, 404}, {store.ErrConflict, 409}, {migration.ErrLimit, 413}, {migration.ErrNotReady, 422}, {migration.ErrInvalid, 400}, {store.ErrInvalid, 400}, {context.Canceled, 504}, {errors.New("PRIVATE"), 503}} {
		w := httptest.NewRecorder()
		failure(w, test.err)
		if w.Code != test.status || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatal("unsafe failure mapping")
		}
	}
	registry, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	d, _, _ := registry.Match(http.MethodPost, "/v1/migrations/resume")
	for _, contentType := range []string{"text/plain", "application/json; charset=utf-8"} {
		req := httptest.NewRequest(http.MethodPost, d.Path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", contentType)
		if decode(httptest.NewRecorder(), req, d, &migration.ResumeRequest{}) == nil {
			t.Fatal("noncanonical content type accepted")
		}
	}
	if Handler(nil, nil, http.NotFoundHandler()) == nil {
		t.Fatal("nil dependencies produced no handler")
	}
}
