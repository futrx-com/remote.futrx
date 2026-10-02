package filechat

import (
	"fmt"
	"os"
)

const sqliteFileMode = 0o600

var sqliteFileSuffixes = [...]string{"", "-wal", "-shm"}

// createPrivateSQLiteFile creates an empty database file that only root can
// read and refuses symlinked or special paths.
func createPrivateSQLiteFile(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sqlite database is not a regular file: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, sqliteFileMode)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	closeErr := file.Close()
	if err := os.Chmod(path, sqliteFileMode); err != nil {
		return fmt.Errorf("set sqlite database permissions: %w", err)
	}
	return closeErr
}

// removeSQLiteFiles deletes a database and its WAL sidecars. Callers must only
// pass paths they own; non-regular files are refused rather than unlinked.
func removeSQLiteFiles(path string) error {
	for _, suffix := range sqliteFileSuffixes {
		candidate := path + suffix
		info, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse to remove non-regular sqlite file: %s", candidate)
		}
		if err := os.Remove(candidate); err != nil {
			return err
		}
	}
	return nil
}

// chmodPrivateSQLiteFiles reapplies owner-only permissions to a database and
// its sidecars after SQLite recreated them.
func chmodPrivateSQLiteFiles(path string) error {
	for _, suffix := range sqliteFileSuffixes {
		err := os.Chmod(path+suffix, sqliteFileMode)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
