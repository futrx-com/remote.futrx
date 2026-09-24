package workspace

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	MaxArchiveBytes       = int64(1 << 30)
	MaxConcurrentArchives = 2
)

// Spooler bounds ZIP construction before any success response is returned.
// Files live in the application's DataDir rather than the host temp directory,
// so ownership and uninstall cleanup stay with this installed instance.
type Spooler struct {
	directory string
	slots     chan struct{}
	maxBytes  int64
}

type SpooledArchive struct {
	file    *os.File
	size    int64
	release func()
	once    sync.Once
}

func NewSpooler(directory string, maxConcurrent int, maxBytes int64) (*Spooler, error) {
	if maxConcurrent < 1 || maxBytes < 1 {
		return nil, errors.New("invalid archive spool limits")
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
	return &Spooler{
		directory: directory,
		slots:     make(chan struct{}, maxConcurrent),
		maxBytes:  maxBytes,
	}, nil
}

func (s *Spooler) Prepare(ctx context.Context, writeArchive func(io.Writer) error) (*SpooledArchive, error) {
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	var temporary *os.File
	prepared := false
	defer func() {
		if prepared {
			return
		}
		if temporary != nil {
			_ = temporary.Close()
			_ = os.Remove(temporary.Name())
		}
		s.release()
	}()

	temporary, err := os.CreateTemp(s.directory, "workspace-archive-*.zip")
	if err != nil {
		return nil, err
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
	prepared = true
	return &SpooledArchive{file: temporary, size: info.Size(), release: s.release}, nil
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
		a.release()
	})
	return closeErr
}

func (s *Spooler) acquire(ctx context.Context) error {
	select {
	case s.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Spooler) release() { <-s.slots }

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
