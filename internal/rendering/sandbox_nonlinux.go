//go:build !linux

package rendering

import (
	"context"
	"os"
	"os/exec"
)

func sandboxSupported(mode string) error {
	if mode != "development" {
		return ErrInvalid
	}
	return nil
}
func sandboxCommand(ctx context.Context, path, mode string) (*exec.Cmd, func(), error) {
	if mode != "development" {
		return nil, func() {}, ErrInvalid
	}
	dir, err := os.MkdirTemp("", "chartworks-render-dev-")
	if err != nil {
		return nil, func() {}, ErrWorker
	}
	cmd := exec.CommandContext(ctx, path, "--sealed-render-worker")
	cmd.Dir = dir
	return cmd, func() { _ = os.RemoveAll(dir) }, nil
}
