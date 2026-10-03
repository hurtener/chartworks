//go:build linux

package acceptance

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/rendering"
)

func phase32Options() rendering.Options {
	return rendering.Options{WorkerVersion: "worker-v1", ThemeVersion: "theme-v1", MaxTime: 5 * time.Second, MaxMemoryBytes: 1 << 30, MaxInputBytes: 4 << 20, MaxOutputBytes: 4 << 20, MaxConcurrent: 2, MaxWidgets: 100, Retention: time.Hour, Isolation: "linux_namespaces", CgroupRoot: os.Getenv("CHARTWORKS_TEST_RENDER_CGROUP_ROOT")}
}
func phase32RepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func phase32StaticBuild(t *testing.T, output string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"build", "-o", output}, args...)...)
	cmd.Dir = phase32RepoRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("static probe build: %v: %s", err, body)
	}
}
func mustExecutable32(t *testing.T) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "chartworks-renderer")
	phase32StaticBuild(t, output, "./cmd/chartworks-renderer")
	return output
}
func phase32Probe(t *testing.T, kind, _ string) string {
	t.Helper()
	switch kind {
	case "crash", "sleep", "large", "clean":
	default:
		t.Fatal("unknown probe")
	}
	output := filepath.Join(t.TempDir(), kind)
	// Source stays tracked inside the module for internal imports and read-only
	// source qualification. Only the executable is written to the test tempdir.
	phase32StaticBuild(t, output, "./test/acceptance/testdata/phase32_probes/"+kind+".go")
	return output
}

func TestPhase32ProbeBinaries(t *testing.T) {
	for _, kind := range []string{"crash", "sleep", "large", "clean"} {
		t.Run(kind, func(t *testing.T) {
			binary := phase32Probe(t, kind, "")
			relative, relativeErr := filepath.Rel(phase32RepoRoot(t), binary)
			if relativeErr != nil || !filepath.IsAbs(binary) || filepath.IsLocal(relative) {
				t.Fatal("probe output must be outside the source tree")
			}
			cmd := exec.CommandContext(t.Context(), binary)
			cmd.Env = []string{"GOMAXPROCS=1", "RENDER_MEMORY_CONTRACT=charged-memory-v1"}
			cmd.Stdin = bytes.NewBufferString("{}")
			start := time.Now()
			body, err := cmd.Output()
			switch kind {
			case "crash", "clean":
				want := 9
				if kind == "clean" {
					want = 1 // Clean environment reaches the invalid-input rejection.
				}
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != want || len(body) != 0 {
					t.Fatal("probe exit contract changed", err, len(body))
				}
			case "sleep":
				if err != nil || time.Since(start) < 2*time.Second || len(body) != 0 {
					t.Fatal("probe sleep contract changed", err, len(body))
				}
			case "large":
				if err != nil || !bytes.Equal(body, make([]byte, 4096)) {
					t.Fatal("probe output contract changed", err, len(body))
				}
			}
			if kind == "clean" {
				for _, environment := range [][]string{{"GOMAXPROCS=1", "SECRET_CANARY=must-not-cross"}, {"GOMAXPROCS=2"}, {}} {
					cmd := exec.CommandContext(t.Context(), binary)
					cmd.Env = environment
					body, err := cmd.Output()
					var exit *exec.ExitError
					if !errors.As(err, &exit) || exit.ExitCode() != 7 || len(body) != 0 {
						t.Fatal("probe clean-environment guard changed", err, len(body))
					}
				}
			}
		})
	}
}
