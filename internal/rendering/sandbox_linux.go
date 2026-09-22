//go:build linux

package rendering

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func sandboxSupported(mode string) error {
	if mode != "linux_namespaces" {
		return ErrInvalid
	}
	return nil
}
func sandboxCommand(ctx context.Context, path, mode string) (*exec.Cmd, func(), error) {
	if mode != "linux_namespaces" {
		return nil, func() {}, ErrInvalid
	}
	root, err := os.MkdirTemp("", "chartworks-render-")
	if err != nil {
		return nil, func() {}, ErrWorker
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	in, err := os.Open(path)
	if err != nil {
		cleanup()
		return nil, func() {}, ErrWorker
	}
	defer in.Close()
	linked, statErr := os.Lstat(path)
	opened, openStatErr := in.Stat()
	if statErr != nil || openStatErr != nil || linked.Mode()&os.ModeSymlink != 0 || !linked.Mode().IsRegular() || !os.SameFile(linked, opened) {
		cleanup()
		return nil, func() {}, ErrInvalid
	}
	worker := filepath.Join(root, "worker")
	out, err := os.OpenFile(worker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0500)
	if err != nil {
		cleanup()
		return nil, func() {}, ErrWorker
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil || closeErr != nil {
		cleanup()
		return nil, func() {}, ErrWorker
	}
	cmd := exec.CommandContext(ctx, "/worker", "--sealed-render-worker")
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: root, Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC | syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID, Unshareflags: syscall.CLONE_NEWNS, UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}}, GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}}, GidMappingsEnableSetgroups: false}
	return cmd, cleanup, nil
}
