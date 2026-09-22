//go:build linux

package rendering

import (
	"context"
	"os"
	"syscall"
	"testing"
)

func TestLinuxSandboxIsolatesNetworkFilesystemAndProcesses(t *testing.T) {
	cmd, cleanup, err := sandboxCommand(context.Background(), os.Args[0], "linux_namespaces")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	attr := cmd.SysProcAttr
	if attr == nil || attr.Chroot == "" || cmd.Dir != "/" {
		t.Fatal("filesystem boundary missing")
	}
	for _, flag := range []uintptr{syscall.CLONE_NEWNET, syscall.CLONE_NEWNS, syscall.CLONE_NEWPID} {
		if attr.Cloneflags&flag == 0 {
			t.Fatalf("namespace flag missing: %x", flag)
		}
	}
	if cmd.Path != "/worker" {
		t.Fatal("worker did not move inside sealed root", cmd.Path)
	}
}
