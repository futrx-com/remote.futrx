//go:build linux

package cache

import (
	"os"

	"golang.org/x/sys/unix"
)

// punchHole releases the disk backing [off, off+length) while leaving the file
// length unchanged, so the region reads back as zeros and can be refetched.
func punchHole(f *os.File, off, length int64) error {
	return unix.Fallocate(int(f.Fd()),
		unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, off, length)
}
