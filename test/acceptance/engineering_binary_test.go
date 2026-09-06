package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

// binaryClient qualifies the shipping composition root rather than reconstructing
// its service graph in the test. Both database roles and JWT verification remain
// the actual isolated PostgreSQL/JWKS fixtures, not mocked source/model adapters.
func (f *engineeringFixture) binaryClient(t *testing.T) *sdk.Client {
	t.Helper()
	directory := t.TempDir()
	binary := filepath.Join(directory, "chartworks")
	buildContext, stopBuild := context.WithTimeout(context.Background(), 90*time.Second)
	defer stopBuild()
	// #nosec G204 -- fixed build command; the output is a test-created temporary path.
	build := exec.CommandContext(buildContext, "go", "build", "-o", binary, "./cmd/chartworks")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile engineering binary: %v: %s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(directory, "fixture-ca.pem")
	if err = os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.token.server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	values := f.values
	values.Auth = f.token.cfg
	values.Server.Listen = address
	values.Store.DSN = "env:CHARTWORKS_STORE_URL"
	configuration, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(directory, "config.json")
	if err = os.WriteFile(configPath, configuration, 0600); err != nil {
		t.Fatal(err)
	}
	runContext, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	// #nosec G204 -- executable was built above; no user-selected command or arguments.
	command := exec.CommandContext(runContext, binary, "serve", "--config", configPath)
	read, hasRead := f.lookup("CHARTWORKS_SOURCE_READ")
	write, hasWrite := f.lookup("CHARTWORKS_SOURCE_WRITE")
	if !hasRead || !hasWrite {
		t.Fatal("isolated workspace credentials missing")
	}
	command.Env = append(os.Environ(), "CHARTWORKS_STORE_URL="+f.dsn, "CHARTWORKS_SOURCE_READ="+read, "CHARTWORKS_SOURCE_WRITE="+write, "SSL_CERT_FILE="+root)
	output := &engineeringProcessOutput{}
	command.Stdout, command.Stderr = output, output
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	finished := false
	t.Cleanup(func() {
		if finished {
			return
		}
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-wait:
			if err != nil {
				t.Errorf("engineering binary shutdown failed: %v: %s", err, output.String())
			}
		case <-time.After(10 * time.Second):
			_ = command.Process.Kill()
			<-wait
			t.Error("engineering service failed to join before shutdown")
		}
	})
	httpClient := &http.Client{Timeout: 10 * time.Second}
	base := "http://" + address
	ready := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-wait:
			finished = true
			t.Fatalf("engineering binary exited before readiness: %v: %s", err, output.String())
		default:
		}
		response, err := httpClient.Get(base + "/readyz")
		if err == nil {
			_ = response.Body.Close()
			ready = response.StatusCode == http.StatusOK
		}
		if ready {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("engineering binary did not become ready: %s", output.String())
	}
	client, err := sdk.New(base, httpClient, func(context.Context) (string, error) {
		return f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), f.e.Scopes()), nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// The process can still be writing while a readiness failure is reported. Keep
// diagnostic capture bounded and synchronized; never race on bytes.Buffer.
type engineeringProcessOutput struct {
	mu sync.Mutex
	buffer bytes.Buffer
}

func (b *engineeringProcessOutput) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	length := len(data)
	left := (32 << 10) - b.buffer.Len()
	if left > 0 {
		_, _ = b.buffer.Write(data[:min(left, len(data))])
	}
	return length, nil
}

func (b *engineeringProcessOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
