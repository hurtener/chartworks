package rendering

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

// workerProcessError exposes only a closed failure class. Worker diagnostics may
// contain private input or runtime paths and must never cross this boundary.
func workerProcessError(err error, diagnostics []byte) error {
	if err == nil {
		return nil
	}
	reason := "process_exit"
	if errors.Is(err, os.ErrPermission) {
		reason = "launch_denied"
	} else {
		for _, prefix := range []string{"fatal error: runtime: cannot allocate memory\n", "fatal error: out of memory\n", "runtime: out of memory:"} {
			if bytes.HasPrefix(diagnostics, []byte(prefix)) {
				reason = "memory_budget"
				break
			}
		}
	}
	return fmt.Errorf("%w: %s", ErrWorker, reason)
}
