package acceptance

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/foundation"
	"github.com/hurtener/chartworks/internal/releasegate"
	"github.com/hurtener/chartworks/internal/securityapi"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/internal/telemetry"
	"github.com/hurtener/chartworks/test/support"
)

func phase25Release(t *testing.T) (string, releasegate.Bundle, bool) {
	t.Helper()
	if os.Getenv("CHARTWORKS_RELEASE_MODE") != "1" {
		return "", releasegate.Bundle{}, false
	}
	dir := os.Getenv("CHARTWORKS_RELEASE_EVIDENCE_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("release mode requires an absolute external evidence directory")
	}
	bundle, err := releasegate.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, bundle, true
}

// AC03 is deliberately absent until the final_stress profile has been run
// against the selected post-Phase-34 release head. The strict phase runner
// rejects this parent in the interim.
func TestPhase25(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newTokenFixture(t)
		service, handler, _ := protectedFixture(t, f)
		if _, err := service.Configure(context.Background(), f.envelope(t, "tenant", "operator", operationalScopes("tenant")...), 0, store.Policy{AuditDays: 7, OperationHours: 24}); err != nil {
			t.Fatal(err)
		}
		good := f.sign(t, f.claims("tenant", "user", []string{"ops.read", "cw.tenant.read:tenant"}), nil)
		if got := callProtected(t, handler, "GET", "/v1/retention-policy", good, "", nil).Code; got != http.StatusOK {
			t.Fatalf("verified Pengui-shaped bearer rejected: %d", got)
		}
		for _, bearer := range []string{"", "local", f.sign(t, jwt.MapClaims{"iss": "https://local.invalid", "tenant": "tenant", "user": "user", "session": "s", "scopes": []string{"ops.read"}, "iat": f.clock.Load(), "exp": f.clock.Load() + 300, "aud": "chartworks:http"}, nil)} {
			if got := callProtected(t, handler, "GET", "/v1/retention-policy", bearer, "", nil).Code; got != http.StatusUnauthorized {
				t.Fatalf("non-Pengui or missing bearer accepted: %d", got)
			}
		}
		for _, path := range []string{"/auth/token", "/v1/bootstrap", "/v1/grants", "/v1/users", "/v1/roles", "/v1/embed-tokens"} {
			if got := callProtected(t, handler, "POST", path, good, `{}`, nil).Code; got != http.StatusNotFound {
				t.Fatalf("local issuer/grant route survived: %s: %d", path, got)
			}
		}
		lookup := func(name string) (string, bool) {
			return "postgres://fixture:fixture@localhost/fixture?sslmode=disable", name == "CHARTWORKS_STORE_URL"
		}
		base := `{"auth":{"issuer":"https://pengui.example.test","jwks_url":"https://pengui.example.test/keys","audience":"chartworks:http"}}`
		if _, err := config.Load(strings.NewReader(base), lookup, config.Overrides{}); err != nil {
			t.Fatal("Pengui verifier configuration rejected", err)
		}
		for _, retired := range []string{`"mode":"local"`, `"signing_secret":"fixture"`, `"bootstrap_admin":"actor"`, `"local_issuer":"https://local.invalid"`} {
			bad := strings.Replace(base, `"audience":"chartworks:http"`, `"audience":"chartworks:http",`+retired, 1)
			if _, err := config.Load(strings.NewReader(bad), lookup, config.Overrides{}); err == nil {
				t.Fatal("retired local authority configuration accepted", retired)
			}
		}
		if dir, bundle, release := phase25Release(t); release {
			if err := releasegate.VerifyRecords(dir, bundle, "authority", []string{"pengui"}); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("AC02", func(t *testing.T) {
		dsn := support.Database(t)
		primary := support.Open(t, dsn) // fresh migration, not a pre-migrated store
		if err := primary.Check(context.Background()); err != nil {
			t.Fatal("fresh PostgreSQL 17 store unready", err)
		}
		f := newTokenFixture(t)
		policy, err := securityapi.New(primary)
		if err != nil {
			t.Fatal(err)
		}
		e := f.envelope(t, "tenant", "operator", operationalScopes("tenant")...)
		if _, err = policy.Configure(context.Background(), e, 0, store.Policy{AuditDays: 13, OperationHours: 24}); err != nil {
			t.Fatal("store write before backup failed", err)
		}
		restoredDSN := support.Database(t)
		dump := filepath.Join(t.TempDir(), "metadata.dump")
		for _, args := range [][]string{{"pg_dump", "--format=custom", "--no-owner", "--no-privileges", "--file", dump, dsn}, {"pg_restore", "--no-owner", "--no-privileges", "--dbname", restoredDSN, dump}} {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			cmd := exec.CommandContext(ctx, args[0], args[1:]...)
			output, runErr := cmd.CombinedOutput()
			cancel()
			if runErr != nil {
				t.Fatalf("PostgreSQL backup/restore failed: %s: %v", args[0], runErr)
			}
			_ = output // raw utility output can contain host details; never print it.
		}
		opts := postgres.Defaults()
		opts.MigrationPolicy = "check"
		restored, err := postgres.Open(context.Background(), restoredDSN, opts)
		if err != nil {
			t.Fatal("restored release schema rejected", err)
		}
		defer restored.Close()
		if err := restored.Check(context.Background()); err != nil {
			t.Fatal("restored store unready", err)
		}
		restoredPolicy, err := securityapi.New(restored)
		if err != nil {
			t.Fatal(err)
		}
		got, err := restoredPolicy.Policy(context.Background(), e)
		if err != nil || got.AuditDays != 13 {
			t.Fatal("restored policy differed", err)
		}
		f.clock.Store(time.Now().Unix())
		phase25ReadyLifecycle(t, dsn, f)
		old := f.sign(t, f.claims("tenant", "operator", nil), nil)
		if _, err := f.verifier.Verify(context.Background(), old, auth.HTTP); err != nil {
			t.Fatal(err)
		}
		newer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		f.set(publicDocument(t, newer, "rotated-key"), http.StatusOK)
		f.clock.Add(2)
		if !f.verifier.Check(context.Background()).Ready {
			t.Fatal("rotated trusted JWKS unready")
		}
		rotated := jwt.NewWithClaims(jwt.SigningMethodES256, f.claims("tenant", "operator", nil))
		rotated.Header["kid"] = "rotated-key"
		fresh, err := rotated.SignedString(newer)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.verifier.Verify(context.Background(), fresh, auth.HTTP); err != nil {
			t.Fatal("new Pengui key rejected", err)
		}
		if _, err = f.verifier.Verify(context.Background(), old, auth.HTTP); err == nil {
			t.Fatal("removed verification key still accepted")
		}
		if dir, bundle, release := phase25Release(t); release {
			if err := releasegate.VerifyRecords(dir, bundle, "operation", releasegate.Operations); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("AC04", func(t *testing.T) {
		if dir, bundle, release := phase25Release(t); release {
			inventory, err := releasegate.VerifyInventory(dir, bundle.Head, bundle.Inventory)
			if err != nil {
				t.Fatal(err)
			}
			if err = releasegate.VerifyRecords(dir, bundle, "engine", releasegate.Engines); err != nil {
				t.Fatal(err)
			}
			ids := make([]string, len(inventory.Cohorts))
			for i, cohort := range inventory.Cohorts {
				ids[i] = cohort.ID
			}
			if err = releasegate.VerifyRecords(dir, bundle, "cohort", ids); err != nil {
				t.Fatal(err)
			}
			return
		}
		// The development lane proves fixture and unknown observations cannot
		// close the live engine gate; the release lane above requires real logs.
		if err := releasegate.VerifyRecords(t.TempDir(), releasegate.Bundle{Head: strings.Repeat("a", 40)}, "engine", releasegate.Engines); !errors.Is(err, releasegate.ErrEvidence) {
			t.Fatal("missing real engine evidence passed", err)
		}
	})

	t.Run("AC05", func(t *testing.T) {
		root := filepath.Join("..", "..")
		digest, err := postgres.SchemaDigest()
		if err != nil || digest == "" {
			t.Fatal("compiled migration digest unavailable", err)
		}
		var output bytes.Buffer
		if code := foundation.Command(context.Background(), []string{"schema-manifest"}, nil, &output, io.Discard, foundation.Build{Commit: "candidate-head"}, nil); code != 0 {
			t.Fatal("released binary schema-manifest command unavailable", code)
		}
		var manifest struct {
			Commit         string `json:"commit"`
			MigrationCount int    `json:"migration_count"`
			SHA256         string `json:"sha256"`
		}
		if json.Unmarshal(output.Bytes(), &manifest) != nil || manifest.Commit != "candidate-head" || manifest.MigrationCount < 1 || manifest.SHA256 != digest {
			t.Fatal("compiled schema metadata differs", output.String())
		}
		files, err := releasegate.SourceFileDigests(root)
		if err != nil || len(files) < 40 {
			t.Fatal("active documentation/schema inventory incomplete", err)
		}
		if dir, bundle, release := phase25Release(t); release {
			if err := releasegate.VerifyArtifacts(root, dir, bundle); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("AC06", func(t *testing.T) {
		root := filepath.Join("..", "..")
		if err := releasegate.VerifyClosure(root, nil, false); err != nil {
			t.Fatal("active row mapping invalid", err)
		}
		if _, _, release := phase25Release(t); release {
			var receipts map[string][]string
			if err := json.Unmarshal([]byte(os.Getenv("CHARTWORKS_RELEASE_PHASE_RECEIPTS")), &receipts); err != nil {
				t.Fatal("strict runner receipts missing", err)
			}
			if err := releasegate.VerifyClosure(root, receipts, true); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func phase25ReadyLifecycle(t *testing.T, dsn string, f *tokenFixture) {
	t.Helper()
	db := support.Open(t, dsn)
	lookup := func(name string) (string, bool) { return dsn, name == "CHARTWORKS_STORE_URL" }
	input := `{"auth":{"issuer":"` + f.cfg.Issuer + `","jwks_url":"` + f.cfg.JWKSURL + `","audiences":{"http":"chartworks:http","mcp":"chartworks:mcp"},"refresh_interval":"100ms","jwks_max_stale":"10s"}}`
	cfg, err := config.Load(strings.NewReader(input), lookup, config.Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := telemetry.New(io.Discard, "json", false)
	if err != nil {
		t.Fatal(err)
	}
	var storeUp, keysUp atomic.Bool
	storeUp.Store(true)
	keysUp.Store(true)
	for restart := 0; restart < 2; restart++ {
		server, err := foundation.NewServer(cfg, reporter, func(ctx context.Context) foundation.Dependency {
			return foundation.Dependency{Ready: storeUp.Load() && db.Check(ctx) == nil}
		}, func(ctx context.Context) foundation.Dependency {
			if !keysUp.Load() {
				return foundation.Dependency{}
			}
			return f.verifier.Check(ctx)
		})
		if err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- server.Serve(ctx, listener) }()
		url := "http://" + listener.Addr().String() + "/readyz"
		phase25WaitReady(t, url, http.StatusOK)
		if restart == 0 {
			keysUp.Store(false)
			phase25WaitReady(t, url, http.StatusServiceUnavailable)
			keysUp.Store(true)
			storeUp.Store(false)
			phase25WaitReady(t, url, http.StatusServiceUnavailable)
			storeUp.Store(true)
			phase25WaitReady(t, url, http.StatusOK)
		}
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal("shutdown failed", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("shutdown did not join dependency monitors")
		}
	}
}

func phase25WaitReady(t *testing.T, url string, expected int) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == expected {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("readiness did not reach HTTP %d", expected)
}
