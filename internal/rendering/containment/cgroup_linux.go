//go:build linux

package containment

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// Group owns one fresh leaf. No directory or controller descriptor is inherited
// by the worker; FD is used only by clone3's atomic CLONE_INTO_CGROUP placement.
type Group struct {
	root, dir *os.File
	name      string
	// Opened in the manager namespace before launch, never inherited. Retaining
	// controller descriptors prevents same-UID chmod from disabling cleanup.
	kill, events, memoryEvents *os.File
	mu                         sync.Mutex
}

func OpenRoot(path string) (*os.File, error) {
	oom, err := os.ReadFile("/proc/self/oom_score_adj")
	score, parseErr := strconv.ParseInt(strings.TrimSpace(string(oom)), 10, 32)
	if err != nil || parseErr != nil || score <= -1000 || score > 1000 {
		return nil, ErrUnavailable
	}
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrUnavailable
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return nil, ErrUnavailable
	}
	f := os.NewFile(uintptr(fd), "render-cgroup-root")
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
		}
	}()
	var fs unix.Statfs_t
	var st unix.Stat_t
	if unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.CGROUP2_SUPER_MAGIC || unix.Fstat(fd, &st) != nil || st.Mode&0022 != 0 || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) {
		return nil, ErrUnavailable
	}
	info, err := os.ReadFile("/proc/self/fdinfo/" + strconv.Itoa(fd))
	if err != nil {
		return nil, ErrUnavailable
	}
	mounts, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, ErrUnavailable
	}
	defer mounts.Close()
	if !namespaceDelegated(string(info), mounts) {
		return nil, ErrUnavailable
	}
	controllers, e1 := readControl(f, "cgroup.controllers")
	enabled, e2 := readControl(f, "cgroup.subtree_control")
	kind, e3 := readControl(f, "cgroup.type")
	if e1 != nil || e2 != nil || e3 != nil || !word(controllers, "memory") || !word(enabled, "memory") || !word(controllers, "hugetlb") || !word(enabled, "hugetlb") || !word(controllers, "pids") || !word(enabled, "pids") || !word(controllers, "cpu") || !word(enabled, "cpu") || kind != "domain" {
		return nil, ErrUnavailable
	}
	ok = true
	return f, nil
}

// namespaceDelegated checks the actual descriptor's mount, not merely kernel
// feature availability or a similarly named path. nsdelegate must be enabled by
// the trusted deployment; this application never remounts or enables controllers.
func namespaceDelegated(fdinfo string, mounts io.Reader) bool {
	id := ""
	for _, line := range strings.Split(fdinfo, "\n") {
		p := strings.Fields(line)
		if len(p) == 2 && p[0] == "mnt_id:" {
			id = p[1]
		}
	}
	if id == "" {
		return false
	}
	s := bufio.NewScanner(io.LimitReader(mounts, 1<<20))
	for s.Scan() {
		parts := strings.Split(s.Text(), " - ")
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 6 || left[0] != id || len(right) != 3 || right[0] != "cgroup2" {
			continue
		}
		return word(strings.ReplaceAll(right[2], ",", " "), "nsdelegate")
	}
	return false
}
func word(s, w string) bool {
	for _, v := range strings.Fields(s) {
		if v == w {
			return true
		}
	}
	return false
}

func New(rootPath string, memory int64) (*Group, error) {
	if !ValidBudget(memory) {
		return nil, ErrUnavailable
	}
	// memory.max is page-granular. Program an already aligned value so kernel
	// rounding can neither widen the caller's budget nor fail exact readback.
	memory = pageBudget(memory, int64(os.Getpagesize()))
	if !ValidBudget(memory) {
		return nil, ErrUnavailable
	}
	root, err := OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		_ = root.Close()
		return nil, ErrUnavailable
	}
	name := "render-" + hex.EncodeToString(random[:])
	if unix.Mkdirat(int(root.Fd()), name, 0700) != nil {
		_ = root.Close()
		return nil, ErrUnavailable
	}
	fd, err := unix.Openat(int(root.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		cleanupErr := unix.Unlinkat(int(root.Fd()), name, unix.AT_REMOVEDIR)
		_ = root.Close()
		if cleanupErr != nil {
			return nil, errors.Join(ErrUnavailable, ErrCleanup)
		}
		return nil, ErrUnavailable
	}
	g := &Group{root: root, dir: os.NewFile(uintptr(fd), "render-cgroup"), name: name}
	limits := [][2]string{{"memory.max", strconv.FormatInt(memory, 10)}, {"memory.swap.max", "0"}, {"memory.oom.group", "1"}, {"pids.max", "64"}, {"cpu.max", "100000 100000"}, {"cgroup.max.descendants", "0"}, {"cgroup.max.depth", "0"}}
	for _, limit := range limits {
		if writeControl(g.dir, limit[0], limit[1]) != nil {
			return nil, g.discard()
		}
		got, e := readControl(g.dir, limit[0])
		if e != nil || got != limit[1] {
			return nil, g.discard()
		}
	}
	entries, e := g.dir.ReadDir(-1)
	if e != nil {
		return nil, g.discard()
	}
	huge := 0
	hugeFile := regexp.MustCompile(`^hugetlb\.[0-9]+[KMG]B\.(rsvd\.)?max$`)
	for _, entry := range entries {
		if hugeFile.MatchString(entry.Name()) {
			huge++
			if writeControl(g.dir, entry.Name(), "0") != nil {
				return nil, g.discard()
			}
			got, e := readControl(g.dir, entry.Name())
			if e != nil || got != "0" {
				return nil, g.discard()
			}
		}
	}
	if huge == 0 || g.openControls() != nil || g.Kill() != nil {
		return nil, g.discard()
	}
	stats, e := readControl(g.dir, "cgroup.stat")
	if e != nil || !zeroDescendants(stats) {
		return nil, g.discard()
	}
	events, e := readControl(g.dir, "cgroup.events")
	if e != nil || !unpopulated(events) {
		return nil, g.discard()
	}
	procs, e := readControl(g.dir, "cgroup.procs")
	if e != nil || procs != "" {
		return nil, g.discard()
	}
	return g, nil
}

func pageBudget(memory, page int64) int64 {
	if memory <= 0 || page <= 0 {
		return 0
	}
	return memory - memory%page
}

// discard is only used before New returns: no child has ever been launched.
func (g *Group) discard() error {
	defer g.closeControls()
	defer g.dir.Close()
	defer g.root.Close()
	if unix.Unlinkat(int(g.root.Fd()), g.name, unix.AT_REMOVEDIR) != nil {
		return errors.Join(ErrUnavailable, ErrCleanup)
	}
	return ErrUnavailable
}
func (g *Group) openControls() (err error) {
	if g.kill, err = openControl(g.dir, "cgroup.kill", unix.O_WRONLY); err != nil {
		return err
	}
	if g.events, err = openControl(g.dir, "cgroup.events", unix.O_RDONLY); err != nil {
		return err
	}
	g.memoryEvents, err = openControl(g.dir, "memory.events", unix.O_RDONLY)
	return err
}

func (g *Group) closeControls() {
	for _, f := range []*os.File{g.kill, g.events, g.memoryEvents} {
		if f != nil {
			_ = f.Close()
		}
	}
}

func (g *Group) Kill() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return writeControlFile(g.kill, "1")
}

func (g *Group) OOMKilled() (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	v, err := readControlFile(g.memoryEvents)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(v, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "oom_kill" {
			n, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return false, ErrUnavailable
			}
			return n > 0, nil
		}
	}
	return false, ErrUnavailable
}
func (g *Group) FD() int { return int(g.dir.Fd()) }

// Close terminates all processes before removing the leaf. It uses a separate
// short cleanup bound even if the render's context was canceled. Failure must
// poison that supervisor; a successful output cannot conceal failed cleanup.
func (g *Group) Close() error {
	if g == nil || g.dir == nil {
		return ErrUnavailable
	}
	defer g.root.Close()
	defer g.dir.Close()
	g.mu.Lock()
	defer g.mu.Unlock()
	defer g.closeControls()
	if writeControlFile(g.kill, "1") != nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		events, err := readControlFile(g.events)
		if err != nil {
			return ErrUnavailable
		}
		if unpopulated(events) {
			if unix.Unlinkat(int(g.root.Fd()), g.name, unix.AT_REMOVEDIR) != nil {
				return ErrUnavailable
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrUnavailable
		case <-tick.C:
		}
	}
}
func zeroDescendants(stats string) bool {
	for _, line := range strings.Split(stats, "\n") {
		if strings.TrimSpace(line) == "nr_descendants 0" {
			return true
		}
	}
	return false
}
func unpopulated(events string) bool {
	for _, line := range strings.Split(events, "\n") {
		if strings.TrimSpace(line) == "populated 0" {
			return true
		}
	}
	return false
}
func openControl(dir *os.File, name string, mode int) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, mode|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	return os.NewFile(uintptr(fd), "render-control"), nil
}
func readControl(dir *os.File, name string) (string, error) {
	f, err := openControl(dir, name, unix.O_RDONLY)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return readControlFile(f)
}
func readControlFile(f *os.File) (string, error) {
	if f == nil {
		return "", ErrUnavailable
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", ErrUnavailable
	}
	v, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(v) > 4096 {
		return "", ErrUnavailable
	}
	return strings.TrimSpace(string(v)), nil
}
func writeControl(dir *os.File, name, value string) error {
	f, err := openControl(dir, name, unix.O_WRONLY)
	if err != nil {
		return ErrUnavailable
	}
	defer f.Close()
	return writeControlFile(f, value)
}
func writeControlFile(f *os.File, value string) error {
	if f == nil {
		return ErrUnavailable
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ErrUnavailable
	}
	n, err := f.WriteString(value)
	if err != nil || n != len(value) {
		return ErrUnavailable
	}
	return nil
}
