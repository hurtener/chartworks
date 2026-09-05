package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestCompiledAuthorityLifecycle(t *testing.T) {
	f := newTokenFixture(t)
	dsn := support.Database(t)
	directory := t.TempDir()
	binary := filepath.Join(directory, "chartworks")
	buildCtx, stopBuild := context.WithTimeout(context.Background(), 90*time.Second)
	defer stopBuild()
	// #nosec G204 -- fixed build command; output path is a test-created temporary directory.
	build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, "./cmd/chartworks")
	build.Dir = "../.."
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile test binary: %v: %s", err, data)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(directory, "root.pem")
	if err = os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.json")
	data, err := json.Marshal(map[string]any{"server": map[string]any{"listen": address}, "auth": f.cfg, "store": map[string]any{"dsn": "env:CHARTWORKS_STORE_URL"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// #nosec G204 -- executable was built above from the checked-out repository; no request-supplied path.
	cmd := exec.CommandContext(runCtx, binary, "serve", "--config", configPath)
	cmd.Env = append(os.Environ(), "CHARTWORKS_STORE_URL="+dsn, "SSL_CERT_FILE="+root)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	client := &http.Client{Timeout: time.Second}
	base := "http://" + address
	deadline := time.Now().Add(15 * time.Second)
	for {
		req, e := http.NewRequestWithContext(runCtx, http.MethodGet, base+"/readyz", nil)
		if e != nil {
			t.Fatal(e)
		}
		resp, e := client.Do(req)
		if e == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("compiled authority service did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	scopes := operationalScopes("binary-tenant")
	token := f.sign(t, f.claims("binary-tenant", "binary-user", scopes), nil)
	sdk, err := cw.New(base, client, func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	policy, err := sdk.SetRetentionPolicy(runCtx, 0, 7, 24)
	if err != nil || policy.Revision != 1 {
		t.Fatal("compiled protected mutation failed", err)
	}
	stored, err := sdk.RetentionPolicy(runCtx)
	if err != nil || stored != policy {
		t.Fatal("compiled protected read failed", err)
	}
	op, err := sdk.Sweep(runCtx, "binary-idempotent")
	if err != nil || op.Status != "succeeded" {
		t.Fatal("compiled sweep failed", err)
	}
	checks, err := sdk.Diagnostics(runCtx)
	if err != nil || len(checks) != 6 {
		t.Fatal("SDK registry diagnostics parity", err)
	}
	metrics, err := sdk.Metrics(runCtx)
	if err != nil || !strings.Contains(metrics, "chartworks_") {
		t.Fatal("protected metrics parity", err)
	}
	req, err := http.NewRequestWithContext(runCtx, http.MethodGet, base+"/v1/retention-policy", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Tenant-ID", "binary-tenant")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("headers authenticated compiled server")
	}
	denied := f.sign(t, f.claims("foreign", "foreign-user", []string{"ops.read", "cw.tenant.read:binary-tenant"}), nil)
	req.Header.Set("Authorization", "Bearer "+denied)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("foreign read reached compiled service")
	}
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal("compiled shutdown failed", err)
	}
	waited = true
	if strings.Contains(output.String(), token) || strings.Contains(output.String(), dsn) {
		t.Fatal("credential leak in compiled logs")
	}
	t.Log(fmt.Sprintf("compiled JWT -> scope -> PostgreSQL -> SDK and SIGTERM path passed; operations=%d", len(checks)))
}
