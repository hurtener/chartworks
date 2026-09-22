//go:build linux

package acceptance

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/rendering"
)

func phase32Options() rendering.Options {
	return rendering.Options{WorkerVersion: "worker-v1", ThemeVersion: "theme-v1", MaxTime: 5 * time.Second, MaxMemoryBytes: 1 << 30, MaxInputBytes: 4 << 20, MaxOutputBytes: 4 << 20, MaxConcurrent: 2, MaxWidgets: 100, Retention: time.Hour, Isolation: "linux_namespaces"}
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
	sourceDir, err := os.MkdirTemp(".", ".phase32-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sourceDir) })
	source := `package main
import("os";"time";"github.com/hurtener/chartworks/internal/rendering")
func main(){`
	switch kind {
	case "crash":
		source += `os.Exit(9)`
	case "sleep":
		source += `time.Sleep(2*time.Second)`
	case "large":
		source += `_,_=os.Stdout.Write(make([]byte,4096))`
	case "clean":
		source += `if os.Getenv("SECRET_CANARY")!=""{os.Exit(7)};if rendering.WorkerMain(os.Stdin,os.Stdout,4<<20,4<<20,1<<30)!=nil{os.Exit(1)}`
	default:
		t.Fatal("unknown probe")
	}
	source += `}`
	file := filepath.Join(sourceDir, "main.go")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	absoluteFile, err := filepath.Abs(file)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), kind)
	phase32StaticBuild(t, output, absoluteFile)
	return output
}
