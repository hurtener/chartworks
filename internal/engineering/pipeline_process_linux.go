//go:build linux

package engineering

import (
	"os/exec"
	"syscall"
)

func configurePipelineProcess(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return nil
}
func validatePipelineTempFS(path string) error {
	var stat syscall.Statfs_t
	if syscall.Statfs(path, &stat) != nil || stat.Type != 0x01021994 || stat.Flags&8 != 0 {
		return ErrInvalid
	}
	blockSize := stat.Bsize
	if blockSize <= 0 {
		return ErrInvalid
	}
	bytesPerBlock := uint64(blockSize)
	// Compare required block counts without overflowing filesystem byte totals.
	if stat.Blocks < (512<<20+bytesPerBlock-1)/bytesPerBlock || stat.Bavail < (384<<20+bytesPerBlock-1)/bytesPerBlock {
		return ErrInvalid
	}
	return nil
}
