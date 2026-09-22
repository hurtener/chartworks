//go:build !linux

package acceptance

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/rendering"
)

func phase32Options() rendering.Options {
	return rendering.Options{WorkerVersion: "worker-v1", ThemeVersion: "theme-v1", MaxTime: 5 * time.Second, MaxMemoryBytes: 1 << 30, MaxInputBytes: 4 << 20, MaxOutputBytes: 4 << 20, MaxConcurrent: 2, MaxWidgets: 100, Retention: time.Hour, Isolation: "development"}
}
func mustExecutable32(t *testing.T) string {
	t.Helper()
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func phase32Probe(t *testing.T, kind, worker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), kind)
	scripts := map[string]string{
		"crash": "#!/bin/sh\nexit 9\n",
		"sleep": "#!/bin/sh\n/bin/sleep 2\n",
		"large": "#!/bin/sh\n/usr/bin/head -c 4096 /dev/zero\n",
		"clean": fmt.Sprintf("#!/bin/sh\n[ -z \"$SECRET_CANARY\" ] || exit 7\nexec %q --sealed-render-worker\n", worker),
	}
	if err := os.WriteFile(path, []byte(scripts[kind]), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
