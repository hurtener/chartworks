package rendering

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/rendering/containment"
)

func TestRendererRequiresChargedMemoryEnforcement(t *testing.T) {
	opts := Options{WorkerVersion: "v", ThemeVersion: "t", MaxTime: time.Second, MaxMemoryBytes: 1 << 30, MaxInputBytes: 1 << 20, MaxOutputBytes: 1 << 20, MaxConcurrent: 1, MaxWidgets: 1, Retention: time.Hour, Isolation: "linux_namespaces", CgroupRoot: t.TempDir()}
	if p, err := NewProcess(os.Args[0], opts); err == nil || p != nil {
		t.Fatal("ordinary directory admitted as kernel enforcement")
	}
	p := &Process{options: opts}
	p.failed.Store(true)
	work := SealedWork{Version: WorkerProtocolVersion, Request: Request{Format: "html", Theme: "light", Width: 800, Height: 400}, View: tableView()}
	if _, err := p.Process(t.Context(), work); !errors.Is(err, containment.ErrUnavailable) {
		t.Fatal("failed supervisor resumed", err)
	}
}

type cleanupFailureGroup struct {
	fd     int
	closed int
}

func (g *cleanupFailureGroup) FD() int                { return g.fd }
func (*cleanupFailureGroup) Kill() error              { return nil }
func (*cleanupFailureGroup) OOMKilled() (bool, error) { return false, nil }
func (g *cleanupFailureGroup) Close() error           { g.closed++; return containment.ErrCleanup }

func TestRendererCleanupFailureStopsAdmission(t *testing.T) {
	opts := Options{WorkerVersion: "v", ThemeVersion: "t", MaxTime: time.Second, MaxMemoryBytes: 1 << 30, MaxInputBytes: 1 << 20, MaxOutputBytes: 1 << 20, MaxConcurrent: 1, MaxWidgets: 1, Retention: time.Hour, Isolation: "linux_namespaces"}
	work := SealedWork{Version: WorkerProtocolVersion, Request: Request{Format: "html", Theme: "light", Width: 800, Height: 400}, View: tableView()}
	t.Run("setup", func(t *testing.T) {
		calls := 0
		p := &Process{options: opts, slots: make(chan struct{}, 1), newMemoryGroup: func(string, int64) (memoryGroup, error) {
			calls++
			return nil, errors.Join(containment.ErrUnavailable, containment.ErrCleanup)
		}}
		if _, err := p.Process(t.Context(), work); !errors.Is(err, containment.ErrCleanup) {
			t.Fatal(err)
		}
		if _, err := p.Process(t.Context(), work); !errors.Is(err, containment.ErrUnavailable) || calls != 1 {
			t.Fatal("abandoned setup was retried", err, calls)
		}
	})
	t.Run("terminal", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "invalid-worker")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err = f.WriteString("not an executable"); err != nil {
			t.Fatal(err)
		}
		g := &cleanupFailureGroup{fd: int(f.Fd())}
		calls := 0
		p := &Process{path: f.Name(), options: opts, slots: make(chan struct{}, 1), newMemoryGroup: func(string, int64) (memoryGroup, error) { calls++; return g, nil }}
		if _, err := p.Process(t.Context(), work); !errors.Is(err, containment.ErrCleanup) {
			t.Fatal("cleanup failure hidden")
		}
		if !p.failed.Load() || g.closed != 1 {
			t.Fatal("cleanup did not disable supervisor", g.closed)
		}
		if _, err := p.Process(t.Context(), work); !errors.Is(err, containment.ErrUnavailable) || calls != 1 {
			t.Fatal("failed cleanup retried", err, calls)
		}
	})
	t.Run("prelaunch", func(t *testing.T) {
		g := &cleanupFailureGroup{}
		p := &Process{path: "/nonexistent-renderer", options: opts, slots: make(chan struct{}, 1), newMemoryGroup: func(string, int64) (memoryGroup, error) { return g, nil }}
		out, err := p.Process(t.Context(), work)
		if !errors.Is(err, containment.ErrCleanup) || !p.failed.Load() || g.closed != 1 || out.Content != "" {
			t.Fatal("prelaunch cleanup failure hidden", err, g.closed)
		}
	})
}

func TestWorkerRejectsMissingMemoryContract(t *testing.T) {
	t.Setenv("RENDER_MEMORY_CONTRACT", "")
	if !errors.Is(WorkerMain(strings.NewReader("{}"), io.Discard, 1024, 1024, 1<<30), ErrInvalid) {
		t.Fatal("unpaired worker accepted")
	}
	if WorkerProtocolVersion != "chartworks-render-worker-v2" {
		t.Fatal("old protocol compatibility unexpectedly enabled")
	}
}
