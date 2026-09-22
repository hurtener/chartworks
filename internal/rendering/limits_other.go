//go:build !darwin && !linux

package rendering

import "errors"

func applyMemoryLimit(int64) error {
	return errors.New("renderer hard memory limit is unsupported on this platform")
}
