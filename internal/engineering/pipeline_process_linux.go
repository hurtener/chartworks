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
	if syscall.Statfs(path, &stat) != nil || stat.Type != 0x01021994 || stat.Flags&8 != 0 || uint64(stat.Blocks)*uint64(stat.Bsize) < 512<<20 || uint64(stat.Bavail)*uint64(stat.Bsize) < 384<<20 {
		return ErrInvalid
	}
	return nil
}
