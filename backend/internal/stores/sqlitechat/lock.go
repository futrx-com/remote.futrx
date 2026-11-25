package sqlitechat

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// The server and offline importer must never own the conversation store at
// the same time. Keep the descriptor open for the entire Store lifetime.
func acquireStoreLock(root string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(root, ".conversations.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("conversation storage is already open; stop the backend before migration: %w", err)
	}
	return f, nil
}
