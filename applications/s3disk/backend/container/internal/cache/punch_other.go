//go:build !linux

package cache

import (
	"errors"
	"os"
)

// punchHole is only implemented on Linux; elsewhere the cache reclaims whole
// files instead of ranges.
func punchHole(f *os.File, off, length int64) error {
	return errors.ErrUnsupported
}
