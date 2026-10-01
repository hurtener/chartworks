//go:build ignore

// Synthetic adversarial worker used only by the opt-in kernel qualification.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hurtener/chartworks/internal/rendering"
	"golang.org/x/sys/unix"
)

func fatal(v any) { fmt.Fprintln(os.Stderr, v); os.Exit(2) }
func main() {
	if len(os.Args) != 3 {
		fatal("mode")
	}
	mode := os.Args[2]
	if mode == "worker-limits" {
		if rendering.WorkerMain(strings.NewReader("invalid"), io.Discard, 1024, 1024, 1<<30) == nil {
			fatal("invalid input accepted")
		}
		var as, locked syscall.Rlimit
		if syscall.Getrlimit(syscall.RLIMIT_AS, &as) != nil || as.Cur != 3<<30 || as.Max != 3<<30 || syscall.Getrlimit(unix.RLIMIT_MEMLOCK, &locked) != nil || locked.Cur != 0 || locked.Max != 0 || debug.SetMemoryLimit(-1) != 1<<29 {
			fatal("worker limits not installed")
		}
		fmt.Println("worker-limits-denied")
		return
	}

	if err := syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: 3 << 30, Max: 3 << 30}); err != nil {
		fatal(err)
	}
	if mode == "healthy" {
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() || scanner.Text() != "ping" {
			fatal("health input")
		}
		fmt.Println("alive")
		return
	}
	if mode == "child" {
		allocate(300 << 20)
		time.Sleep(time.Minute)
		return
	}
	if mode == "sleep" {
		time.Sleep(time.Minute)
		return
	}
	if mode == "pipes" {
		c := exec.Command("/worker", "--sealed-render-worker", "sleep")
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if c.Start() != nil {
			fatal("child start")
		}
		return
	}
	if mode == "anonymous" {
		allocate(1200 << 20)
		fatal("over budget allocation survived")
	}
	if mode == "descendants" {
		for i := 0; i < 4; i++ {
			c := exec.Command("/worker", "--sealed-render-worker", "child")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if c.Start() != nil {
				fatal("child start")
			}
		}
		time.Sleep(time.Minute)
		fatal("aggregate budget survived")
	}
	if mode == "file" {
		if os.Mkdir("/data", 0700) != nil || syscall.Mount("tmpfs", "/data", "tmpfs", 0, "size=2G") != nil {
			fatal("tmpfs mount")
		}
		f, e := os.Create("/data/pages")
		if e != nil {
			fatal(e)
		}
		b := make([]byte, 1<<20)
		for i := 0; i < 1200; i++ {
			if _, e = f.Write(b); e != nil {
				fatal(e)
			}
		}
		fatal("file charge survived")
	}
	if mode == "address" {
		_, _, e := syscall.Syscall6(syscall.SYS_MMAP, 0, 3<<30, syscall.PROT_NONE, syscall.MAP_PRIVATE|syscall.MAP_ANON, ^uintptr(0), 0)
		if e != syscall.ENOMEM {
			fatal("virtual ceiling not enforced")
		}
		fmt.Println("address-denied")
		return
	}
	if mode == "revoke-controls" {
		if os.Mkdir("/cg", 0700) != nil || syscall.Mount("none", "/cg", "cgroup2", 0, "") != nil {
			fatal("cgroup mount")
		}
		for _, name := range []string{"cgroup.kill", "cgroup.events", "memory.events"} {
			if err := os.Chmod("/cg/"+name, 0); err != nil {
				fatal("controller chmod")
			}
		}
		if err := os.Chmod("/cg", 0); err != nil {
			fatal("directory chmod")
		}
		fmt.Println("controls-revoked")
		time.Sleep(time.Minute)
		return
	}
	if mode != "tamper" {
		fatal("unknown")
	}
	if os.Mkdir("/proc", 0700) != nil || syscall.Mount("proc", "/proc", "proc", 0, "") != nil {
		fatal("proc mount")
	}
	b, e := os.ReadFile("/proc/self/cgroup")
	if e != nil || strings.TrimSpace(string(b)) != "0::/" {
		fatal("wrong cgroup namespace")
	}
	var limit syscall.Rlimit
	if syscall.Getrlimit(syscall.RLIMIT_AS, &limit) != nil || limit.Cur != 3<<30 || limit.Max != 3<<30 {
		fatal("address limit")
	}
	entries, e := os.ReadDir("/proc/self/fd")
	if e != nil {
		fatal(e)
	}
	for _, entry := range entries {
		fd, _ := strconv.Atoi(entry.Name())
		var fs syscall.Statfs_t
		if fd > 2 && syscall.Fstatfs(fd, &fs) == nil && fs.Type == 0x63677270 {
			fatal("leaked controller descriptor")
		}
	}
	if os.Mkdir("/cg", 0700) != nil || syscall.Mount("none", "/cg", "cgroup2", 0, "") != nil {
		fatal("cgroup mount")
	}
	for name, value := range map[string]string{"memory.max": "max", "memory.swap.max": "max", "memory.oom.group": "0", "cgroup.max.descendants": "max", "cgroup.max.depth": "max", "pids.max": "max", "cpu.max": "max 100000"} {
		_ = os.Chmod("/cg/"+name, 0666)
		if os.WriteFile("/cg/"+name, []byte(value), 0600) == nil {
			fatal("controller rewrite")
		}
	}
	if os.Mkdir("/cg/escape", 0700) == nil {
		fatal("descendant created")
	}
	if syscall.Unshare(syscall.CLONE_NEWCGROUP) == nil {
		if os.WriteFile("/cg/memory.max", []byte("max"), 0600) == nil {
			fatal("nested namespace rewrite")
		}
	}
	b, e = os.ReadFile("/cg/memory.max")
	if e != nil || strings.TrimSpace(string(b)) != "1073741824" {
		fatal("limit changed")
	}
	fmt.Println("tamper-denied")
}
func allocate(n int) {
	b, e := syscall.Mmap(-1, 0, n, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if e != nil {
		fatal(e)
	}
	for i := 0; i < len(b); i += 4096 {
		b[i] = 1
	}
	if b[len(b)-1] != 0 {
		fatal("unexpected page")
	}
	time.Sleep(time.Minute)
}
