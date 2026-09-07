//go:build !linux

package engineering

import "os/exec"

// The reference managed runner requires Linux process groups and tmpfs.
func configurePipelineProcess(*exec.Cmd) error { return ErrUnavailable }
func validatePipelineTempFS(string) error      { return ErrUnavailable }
