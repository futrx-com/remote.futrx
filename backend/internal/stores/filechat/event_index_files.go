package filechat

import (
	"fmt"
	"os"
)

const chatEventIndexFileMode = 0o600

var chatEventIndexFileSuffixes = [...]string{"", "-wal", "-shm"}

func createPrivateIndexFile(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("chat event index is not a regular file: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, chatEventIndexFileMode)
	if err != nil {
		return fmt.Errorf("open chat event index: %w", err)
	}
	closeErr := file.Close()
	if err := os.Chmod(path, chatEventIndexFileMode); err != nil {
		return fmt.Errorf("set chat event index permissions: %w", err)
	}
	return closeErr
}

func removeChatEventIndexFiles(path string) error {
	for _, suffix := range chatEventIndexFileSuffixes {
		candidate := path + suffix
		info, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse to remove non-regular chat index file: %s", candidate)
		}
		if err := os.Remove(candidate); err != nil {
			return err
		}
	}
	return nil
}

func (index *chatEventIndex) restrictFiles() error {
	for _, suffix := range chatEventIndexFileSuffixes {
		err := os.Chmod(index.path+suffix, chatEventIndexFileMode)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
