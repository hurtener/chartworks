//go:build !linux

package rendering

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestProductionIsolationFailsClosedOffLinux(t *testing.T) {
	o := Options{WorkerVersion: "v", ThemeVersion: "t", MaxTime: time.Second, MaxMemoryBytes: 64 << 20, MaxInputBytes: 1 << 20, MaxOutputBytes: 1 << 20, MaxConcurrent: 1, MaxWidgets: 1, Retention: time.Hour, Isolation: "linux_namespaces"}
	if _, err := NewProcess(os.Args[0], o); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
