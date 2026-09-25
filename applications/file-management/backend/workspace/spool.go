package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	MaxArchiveBytes       = int64(1 << 30)
	MaxConcurrentArchives = 2
)

const archiveSlotRetryInterval = 25 * time.Millisecond

// Spooler bounds ZIP construction before any success response is returned.
// ZIP files are created in the installed instance's DataDir and immediately
// unlinked while their descriptor stays open. That keeps a loose-chat archive
// rooted above DataDir from discovering and recursively ingesting its own
// growing spool. Slot locks live in the application's shared runtime directory,
// so the concurrency and aggregate-spool limits cover every global and project
// instance rather than resetting in each backend process.
type Spooler struct {
	directory     string
	lockDirectory string
	maxConcurrent int
	maxBytes      int64
}

type SpooledArchive struct {
	file    *os.File
	size    int64
	release func() error
	once    sync.Once
}

func NewSpooler(
	directory string,
	sharedRuntimeDirectory string,
	maxConcurrent int,
	maxBytes int64,
) (*Spooler, error) {
	if directory == "" || sharedRuntimeDirectory == "" || maxConcurrent < 1 || maxBytes < 1 {
		return nil, errors.New("invalid archive spool configuration")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		if !entry.IsDir() {
			_ = os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
	lockDirectory := filepath.Join(sharedRuntimeDirectory, "archive-spool")
	if err := os.MkdirAll(lockDirectory, 0o700); err != nil {
		return nil, err
	}
	return &Spooler{
		directory:     directory,
		lockDirectory: lockDirectory,
		maxConcurrent: maxConcurrent,
		maxBytes:      maxBytes,
	}, nil
}

func (s *Spooler) Prepare(
	ctx context.Context,
	writeArchive func(io.Writer) error,
) (archive *SpooledArchive, err error) {
	slot, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	var temporary *os.File
	defer func() {
		if archive != nil {
			return
		}
		var cleanupErr error
		if temporary != nil {
			cleanupErr = temporary.Close()
			removeErr := os.Remove(temporary.Name())
			if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, removeErr)
			}
		}
		err = errors.Join(err, cleanupErr, slot.release())
	}()

	temporary, err = os.CreateTemp(s.directory, "workspace-archive-*.zip")
	if err != nil {
		return nil, err
	}
	if err := os.Remove(temporary.Name()); err != nil {
		return nil, fmt.Errorf("unlink archive spool: %w", err)
	}
	bounded := &boundedContextWriter{ctx: ctx, destination: temporary, remaining: s.maxBytes}
	if err := writeArchive(bounded); err != nil {
		return nil, err
	}
	if bounded.writeErr != nil {
		return nil, bounded.writeErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	info, err := temporary.Stat()
	if err != nil {
		return nil, err
	}
	archive = &SpooledArchive{file: temporary, size: info.Size(), release: slot.release}
	return archive, nil
}

func (a *SpooledArchive) Size() int64                     { return a.size }
func (a *SpooledArchive) Content() io.ReadSeeker          { return a }
func (a *SpooledArchive) Read(buffer []byte) (int, error) { return a.file.Read(buffer) }
func (a *SpooledArchive) Seek(offset int64, whence int) (int64, error) {
	return a.file.Seek(offset, whence)
}

func (a *SpooledArchive) Close() error {
	var closeErr error
	a.once.Do(func() {
		closeErr = a.file.Close()
		removeErr := os.Remove(a.file.Name())
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			closeErr = errors.Join(closeErr, removeErr)
		}
		closeErr = errors.Join(closeErr, a.release())
	})
	return closeErr
}

type archiveSlot struct {
	lock *os.File
}

func (s *Spooler) acquire(ctx context.Context) (*archiveSlot, error) {
	ticker := time.NewTicker(archiveSlotRetryInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for index := 0; index < s.maxConcurrent; index++ {
			lock, err := os.OpenFile(
				filepath.Join(s.lockDirectory, fmt.Sprintf("slot-%d.lock", index)),
				os.O_CREATE|os.O_RDWR,
				0o600,
			)
			if err != nil {
				return nil, err
			}
			err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if err == nil {
				return &archiveSlot{lock: lock}, nil
			}
			if closeErr := lock.Close(); closeErr != nil {
				return nil, closeErr
			}
			if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
				return nil, err
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *archiveSlot) release() error {
	if s == nil || s.lock == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
	closeErr := s.lock.Close()
	s.lock = nil
	return errors.Join(unlockErr, closeErr)
}

type boundedContextWriter struct {
	ctx         context.Context
	destination io.Writer
	remaining   int64
	writeErr    error
}

func (w *boundedContextWriter) Write(buffer []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if err := w.ctx.Err(); err != nil {
		w.writeErr = err
		return 0, err
	}
	allowed := buffer
	exceedsLimit := int64(len(buffer)) > w.remaining
	if exceedsLimit {
		allowed = buffer[:int(w.remaining)]
	}
	n, err := w.destination.Write(allowed)
	w.remaining -= int64(n)
	if err != nil {
		w.writeErr = err
		return n, err
	}
	if n != len(allowed) {
		w.writeErr = io.ErrShortWrite
		return n, io.ErrShortWrite
	}
	if exceedsLimit {
		w.writeErr = ErrArchiveTooLarge
		return n, ErrArchiveTooLarge
	}
	return n, nil
}
