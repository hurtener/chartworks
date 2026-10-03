//go:build linux && renderer_integration

package rendering

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/rendering/containment"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/chartfixtures"
)

// This is a deployment qualification gate, never a skip or a host-provisioning
// script. The operator must supply an already authorized, delegated hierarchy.
func TestRendererKernelMemoryContract(t *testing.T) {
	root := os.Getenv("CHARTWORKS_TEST_RENDER_CGROUP_ROOT")
	if root == "" {
		t.Fatal("charged-memory qualification requires a preconfigured cgroup root")
	}
	f, err := containment.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Run("non-page-aligned-budget", func(t *testing.T) {
		requested := int64(32<<20) + 1
		g, err := containment.New(root, requested)
		if err != nil {
			t.Fatal("page-granular admission", err)
		}
		defer func() {
			if err := g.Close(); err != nil {
				t.Error("cleanup", err)
			}
		}()
		value, err := os.ReadFile(filepath.Join("/proc/self/fd", strconv.Itoa(g.FD()), "memory.max"))
		if err != nil {
			t.Fatal("effective memory limit", err)
		}
		effective, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
		page := int64(os.Getpagesize())
		if err != nil || effective != requested-requested%page || effective > requested {
			t.Fatal("kernel budget widened or unexpected", effective, err)
		}
	})
	binary := filepath.Join(t.TempDir(), "probe")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./testdata/memory_probe.go")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("probe compile: %v %s", err, out)
	}
	t.Run("startup-after-kill-probe", func(t *testing.T) {
		// New verifies cgroup.kill on a separate empty sibling. On kernels with
		// the CLONE_INTO_CGROUP kill_seq regression, probing the launch leaf
		// itself instead makes even this healthy child die before it can reply.
		for i := 0; i < 2; i++ {
			g, err := containment.New(root, 1<<30)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			cmd, cleanup, err := sandboxCommand(ctx, binary, "linux_namespaces")
			if err == nil {
				err = bindMemoryGroup(cmd, g)
			}
			var stdout, stderr bytes.Buffer
			if err == nil {
				cmd.Args = append(cmd.Args, "healthy")
				cmd.Env = []string{"GOMAXPROCS=1"}
				cmd.Stdin = strings.NewReader("ping\n")
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				cmd.Cancel = g.Kill
				cmd.WaitDelay = time.Second
				err = cmd.Run()
			}
			killed, eventErr := g.OOMKilled()
			closeErr := g.Close()
			cleanup()
			cancel()
			if err != nil || stdout.String() != "alive\n" || stderr.Len() != 0 || eventErr != nil || killed || closeErr != nil {
				t.Fatalf("fresh worker %d: run=%v stdout=%q stderr=%q oom=%v accounting=%v cleanup=%v", i, err, stdout.String(), stderr.String(), killed, eventErr, closeErr)
			}
		}
	})
	t.Run("revoked-controls", func(t *testing.T) {
		// A privileged manager can bypass revoked mode bits and mask this bug.
		status, err := os.ReadFile("/proc/self/status")
		if err != nil || os.Geteuid() == 0 {
			t.Fatal("revoked-control qualification requires an unprivileged manager")
		}
		for _, line := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(line, "CapEff:") {
				bits, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
				if err != nil || bits&((1<<1)|(1<<2)) != 0 {
					t.Fatal("DAC override masks revoked-control qualification")
				}
			}
		}
		g, err := containment.New(root, 1<<30)
		if err != nil {
			t.Fatal(err)
		}
		closed := false
		defer func() {
			if !closed {
				if err := g.Close(); err != nil {
					t.Error(err)
				}
			}
		}()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd, cleanup, err := sandboxCommand(ctx, binary, "linux_namespaces")
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		if err := bindMemoryGroup(cmd, g); err != nil {
			t.Fatal(err)
		}
		cmd.Args = append(cmd.Args, "revoke-controls")
		cmd.Env = []string{"GOMAXPROCS=1"}
		cmd.Cancel = g.Kill
		cmd.WaitDelay = time.Second
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		line, readErr := bufio.NewReader(out).ReadString('\n')
		cancel()
		runErr := cmd.Wait()
		if readErr != nil || line != "controls-revoked\n" || runErr == nil {
			t.Fatal("mode-change/cancellation boundary", line, readErr, runErr)
		}
		if killed, err := g.OOMKilled(); err != nil || killed {
			t.Fatal("revoked accounting", killed, err)
		}
		if err := g.Close(); err != nil {
			t.Fatal("revoked cleanup", err)
		}
		closed = true
		// Subsequent admission must still be possible after the hostile leaf is gone.
		healthy, err := containment.New(root, 1<<30)
		if err != nil {
			t.Fatal("subsequent admission", err)
		}
		if err := healthy.Close(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("independent-jobs", func(t *testing.T) {
		healthy, err := containment.New(root, 1<<30)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := healthy.Close(); err != nil {
				t.Error(err)
			}
		}()
		hungry, err := containment.New(root, 1<<30)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := hungry.Close(); err != nil {
				t.Error(err)
			}
		}()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		a, cleanA, err := sandboxCommand(ctx, binary, "linux_namespaces")
		if err != nil {
			t.Fatal(err)
		}
		defer cleanA()
		b, cleanB, err := sandboxCommand(ctx, binary, "linux_namespaces")
		if err != nil {
			t.Fatal(err)
		}
		defer cleanB()
		if bindMemoryGroup(a, healthy) != nil || bindMemoryGroup(b, hungry) != nil {
			t.Fatal("group binding")
		}
		a.Args = append(a.Args, "healthy")
		b.Args = append(b.Args, "anonymous")
		a.Env = []string{"GOMAXPROCS=1"}
		b.Env = []string{"GOMAXPROCS=1"}
		a.Cancel = healthy.Kill
		b.Cancel = hungry.Kill
		a.WaitDelay = time.Second
		b.WaitDelay = time.Second
		in, err := a.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		out, err := a.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = healthy.Kill(); _ = a.Wait() }()
		if err := b.Run(); err == nil {
			t.Fatal("over-budget job survived")
		}
		killed, err := hungry.OOMKilled()
		if err != nil || !killed {
			t.Fatal("no OOM receipt", err)
		}
		if _, err := in.Write([]byte("ping\n")); err != nil {
			t.Fatal("independent worker lost", err)
		}
		in.Close()
		line, err := bufio.NewReader(out).ReadString('\n')
		if err != nil || line != "alive\n" {
			t.Fatal("independent worker affected", line, err)
		}
		if err := a.Wait(); err != nil {
			t.Fatal(err)
		}
		killed, err = healthy.OOMKilled()
		if err != nil || killed {
			t.Fatal("cross-job OOM", err)
		}
	})
	for _, mode := range []string{"worker-limits", "tamper", "address", "anonymous", "descendants", "file", "pipes", "sleep"} {
		t.Run(mode, func(t *testing.T) {
			g, err := containment.New(root, 1<<30)
			if err != nil {
				t.Fatal(err)
			}
			closed := false
			defer func() {
				if !closed {
					if err := g.Close(); err != nil {
						t.Error("cleanup", err)
					}
				}
			}()
			duration := 30 * time.Second
			if mode == "sleep" {
				duration = 200 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), duration)
			defer cancel()
			cmd, cleanup, err := sandboxCommand(ctx, binary, "linux_namespaces")
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			if err := bindMemoryGroup(cmd, g); err != nil {
				t.Fatal(err)
			}
			cmd.Args = append(cmd.Args, mode)
			cmd.Env = []string{"RENDER_MEMORY_CONTRACT=" + containment.ContractVersion, "GOMAXPROCS=1", "GOMEMLIMIT=536870912B"}
			cmd.WaitDelay = 500 * time.Millisecond
			cmd.Cancel = func() error { return g.Kill() }
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			runErr := cmd.Run()
			killed, eventErr := g.OOMKilled()
			if err := g.Close(); err != nil {
				t.Fatal("cleanup", err)
			}
			closed = true
			if eventErr != nil {
				t.Fatal(eventErr)
			}
			switch mode {
			case "worker-limits", "tamper", "address":
				if runErr != nil || !strings.Contains(stdout.String(), mode+"-denied") {
					t.Fatalf("boundary: %v %s %s", runErr, stdout.String(), stderr.String())
				}
			case "anonymous", "descendants", "file":
				if runErr == nil || !killed {
					t.Fatalf("kernel OOM enforcement absent: %v %s", runErr, stderr.String())
				}
			case "pipes":
				// PID-namespace init exit may close descendant pipes immediately;
				// otherwise WaitDelay must bound them. Both paths must clean the group.
				if runErr != nil && !errors.Is(runErr, exec.ErrWaitDelay) {
					t.Fatalf("descendant pipe deadline: %v", runErr)
				}
			case "sleep":
				if !errors.Is(ctx.Err(), context.DeadlineExceeded) || runErr == nil {
					t.Fatal("cancellation", runErr)
				}
			}
		})
	}
}

func TestRendererKernelStableCatalog(t *testing.T) {
	root := os.Getenv("CHARTWORKS_TEST_RENDER_CGROUP_ROOT")
	if root == "" {
		t.Fatal("stable renderer qualification requires an operator-provisioned cgroup root")
	}
	binary := filepath.Join(t.TempDir(), "worker")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/chartworks-renderer")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("worker build: %v %s", err, out)
	}
	opts := Options{WorkerVersion: "kernel-v2", ThemeVersion: "theme-v1", MaxTime: 30 * time.Second, MaxMemoryBytes: 1 << 30, MaxInputBytes: 4 << 20, MaxOutputBytes: 4 << 20, MaxConcurrent: 2, MaxWidgets: 100, Retention: time.Hour, Isolation: "linux_namespaces", CgroupRoot: root}
	p, err := NewProcess(binary, opts)
	if err != nil {
		t.Fatal(err)
	}
	for batch := 0; batch < 50; batch++ {
		var wg sync.WaitGroup
		for slot := 0; slot < 2; slot++ {
			i := batch*2 + slot
			wg.Add(1)
			go func() {
				defer wg.Done()
				view := tableView()
				if i%14 != 13 {
					g := chartfixtures.Produce(sceneCatalog[i%14], "binding")
					view.Output = &reporting.ViewerOutput{ID: "chart", Kind: "chart", State: "succeeded", RetainedDigest: strings.Repeat("a", 64), Chart: g.Output}
				}
				format := []string{"html", "svg", "png"}[i%3]
				theme := []string{"light", "dark"}[i%2]
				request := Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: "run", Output: view.Output.ID, Limit: 100}, Format: format, Theme: theme, Width: 800, Height: 420}
				expected, err := renderSealedContext(t.Context(), request, view, 4<<20)
				if err != nil {
					t.Errorf("fixture %d: %v", i, err)
					return
				}
				got, err := p.Process(t.Context(), SealedWork{Version: WorkerProtocolVersion, Request: request, View: view})
				if err != nil || got.Digest != expected.Digest || got.Content != expected.Content {
					t.Errorf("isolated render %d (%s): %v", i, format, err)
				}
			}()
		}
		wg.Wait()
	}
	if p.failed.Load() {
		t.Fatal("supervisor cleanup failure")
	}
}
