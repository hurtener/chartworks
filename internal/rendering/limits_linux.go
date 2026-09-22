//go:build linux

package rendering

import "golang.org/x/sys/unix"

func applyMemoryLimit(memory int64) error {
	limit := uint64(memory)
	return unix.Setrlimit(unix.RLIMIT_AS, &unix.Rlimit{Cur: limit, Max: limit})
}
