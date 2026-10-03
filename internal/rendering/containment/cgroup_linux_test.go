//go:build linux

package containment

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestContainmentMountAdmission(t *testing.T) {
	good := "41 20 0:29 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw,nsdelegate,memory_recursiveprot\n"
	for _, tc := range []struct {
		name, info, mount string
		want              bool
	}{
		{"exact", "mnt_id:\t41\n", good, true},
		{"feature-only", "mnt_id:\t41\n", strings.ReplaceAll(good, ",nsdelegate", ""), false},
		{"other-mount", "mnt_id:\t42\n", good, false},
		{"wrong-fs", "mnt_id:\t41\n", strings.ReplaceAll(good, "cgroup2", "tmpfs"), false},
		{"substring", "mnt_id:\t41\n", strings.ReplaceAll(good, "nsdelegate", "notnsdelegate"), false},
		{"missing-id", "", good, false},
		{"malformed", "mnt_id:\t41\n", "41 - cgroup2 cgroup rw,nsdelegate", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := namespaceDelegated(tc.info, strings.NewReader(tc.mount)); got != tc.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}
func TestContainmentRejectsUnenforcedRoots(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "relative", dir + "/../", dir, link, "/does-not-exist-render-cgroup"} {
		if f, err := OpenRoot(p); !errors.Is(err, ErrUnavailable) || f != nil {
			if f != nil {
				f.Close()
			}
			t.Fatalf("unenforced root admitted: %q %v", p, err)
		}
	}
	for _, budget := range []int64{-1, 0, 31 << 20, MaxChargedBytes + 1} {
		if g, err := New(dir, budget); g != nil || !errors.Is(err, ErrUnavailable) {
			t.Fatal("invalid budget admitted")
		}
	}
}
func TestContainmentControlFilesAreClosedAndBounded(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := filepath.Join(dir, "memory.max")
	if err := os.WriteFile(p, []byte("1073741824\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := readControl(f, "memory.max"); err != nil || got != "1073741824" {
		t.Fatal(got, err)
	}
	if err := writeControl(f, "memory.max", "0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 4097)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readControl(f, "memory.max"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("oversized controller admitted")
	}
	if err := os.Symlink(p, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := readControl(f, "link"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("symlink controller read")
	}
	if err := writeControl(f, "link", "0"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("symlink controller write")
	}
	if _, err := readControl(f, "missing"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestContainmentCleanupFailureIsDistinct(t *testing.T) {
	rootPath := t.TempDir()
	dirPath := filepath.Join(rootPath, "job")
	if err := os.Mkdir(dirPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "not-empty"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	root, _ := os.Open(rootPath)
	dir, _ := os.Open(dirPath)
	g := &Group{root: root, dir: dir, name: "job"}
	if err := g.discard(); !errors.Is(err, ErrCleanup) || !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if unpopulated("populated 1\nfrozen 0") || unpopulated("not_populated 0") || !unpopulated("populated 0\nfrozen 0") {
		t.Fatal("population check")
	}
}
func TestContainmentIndependentBudgets(t *testing.T) {
	if AddressSpaceBytes != 3<<30 || MaxChargedBytes != 1<<30 {
		t.Fatal("approved ceilings changed")
	}
	for _, n := range []int64{32 << 20, 64 << 20, 1 << 30} {
		if !ValidBudget(n) || HeapBytes(n) != n/2 {
			t.Fatal(n)
		}
	}
}

func TestContainmentPageGranularBudget(t *testing.T) {
	for _, page := range []int64{4096, 16384, 65536} {
		for _, requested := range []int64{32 << 20, (32 << 20) + 1, (64 << 20) + 4095, MaxChargedBytes - 1, MaxChargedBytes} {
			got := pageBudget(requested, page)
			if !ValidBudget(got) || got > requested || got%page != 0 || requested-got >= page {
				t.Fatalf("requested=%d page=%d effective=%d", requested, page, got)
			}
		}
	}
	for _, tc := range [][2]int64{{0, 4096}, {-1, 4096}, {32 << 20, 0}, {32 << 20, -1}} {
		if pageBudget(tc[0], tc[1]) != 0 {
			t.Fatal("invalid page budget admitted", tc)
		}
	}
}

func TestContainmentRetainedControlsSurviveModeChanges(t *testing.T) {
	dirPath := t.TempDir()
	for name, value := range map[string]string{"cgroup.kill": "0", "cgroup.events": "populated 0\n", "memory.events": "oom 0\noom_kill 1\n"} {
		if err := os.WriteFile(filepath.Join(dirPath, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := os.Open(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	g := &Group{dir: dir}
	if err := g.openControls(); err != nil {
		t.Fatal(err)
	}
	defer g.closeControls()
	for _, f := range []*os.File{g.kill, g.events, g.memoryEvents} {
		flags, err := unix.FcntlInt(f.Fd(), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("controller descriptor can cross exec", err)
		}
		if err := f.Chmod(0); err != nil {
			t.Fatal(err)
		}
	}
	if err := dir.Chmod(0); err != nil {
		t.Fatal(err)
	}
	defer dir.Chmod(0700)
	for i := 0; i < 2; i++ {
		if err := g.Kill(); err != nil {
			t.Fatal("kill reopened revoked path", err)
		}
		if killed, err := g.OOMKilled(); err != nil || !killed {
			t.Fatal("accounting reopened revoked path or lost offset", killed, err)
		}
		if value, err := readControlFile(g.events); err != nil || !unpopulated(value) {
			t.Fatal("cleanup reopened revoked path or lost offset", value, err)
		}
	}
	if err := writeControlFile(nil, "1"); err == nil {
		t.Fatal("missing kill descriptor accepted")
	}
	if _, err := readControlFile(nil); err == nil {
		t.Fatal("missing event descriptor accepted")
	}
}
