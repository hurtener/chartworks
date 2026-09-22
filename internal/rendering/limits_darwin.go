//go:build darwin

package rendering

// Darwin rejects RLIMIT_AS, RLIMIT_DATA and RLIMIT_RSS updates with EINVAL.
// WorkerMain still applies Go's memory limit on developer machines; the reference
// Linux container additionally applies the hard address-space limit.
func applyMemoryLimit(int64) error {
	return nil
}
