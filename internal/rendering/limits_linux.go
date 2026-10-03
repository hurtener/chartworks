//go:build linux

package rendering

import "golang.org/x/sys/unix"

func applyMemoryLimit(memory int64) error {
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return err
	}
	limit := uint64(memory)
	return unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: limit, Max: limit})
}
