package cache

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Entry is one locally materialised object.
//
// Invariants (all under mu):
//   - file length always equals size
//   - a block is readable iff blocks.get(i) || i*bs >= objSize
//     (everything past the S3 object's content is a local hole, i.e. zeros)
type Entry struct {
	c    *Cache
	path string

	mu      sync.Mutex
	key     string
	file    *os.File
	size    int64 // current file length
	objSize int64 // bytes of this file that still live only in S3
	etag    string
	mtime   time.Time
	blocks  *bitmap

	dirty      bool
	dirtySince time.Time
	written    bool // has had data written since it was created or last uploaded
	newFile    bool // created locally, has never existed in S3
	deleted    bool
	refs       int
	used       time.Time
	readOff    int64     // roughly where readers are, so reclaim keeps that region
	trimOrder  time.Time // scratch used while sorting reclaim candidates

	fetchMu  sync.Mutex // serialises block faults for this entry
	prefetch struct {
		mu      sync.Mutex
		pending bool
	}
}

// Key returns the object key this entry is currently bound to.
func (e *Entry) Key() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.key
}

// Size returns the current file length.
func (e *Entry) Size() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.size
}

// Mtime returns the last modification time known for this entry.
func (e *Entry) Mtime() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mtime
}

// ETag returns the ETag of the object as last seen or written.
func (e *Entry) ETag() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.etag
}

// Dirty reports whether the entry has data not yet in S3.
func (e *Entry) Dirty() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dirty
}

// Untouched reports an entry that was created but never written to. The kernel
// sends a FLUSH immediately after CREATE, before the first write, so uploading
// on that flush would store every new file twice.
func (e *Entry) Untouched() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.written && e.size == 0
}

// Ref takes an additional reference (one per open file handle).
func (e *Entry) Ref() {
	e.mu.Lock()
	e.refs++
	e.used = time.Now()
	e.mu.Unlock()
}

// Unref releases a reference; the entry stays cached until evicted.
func (e *Entry) Unref() {
	e.mu.Lock()
	if e.refs > 0 {
		e.refs--
	}
	last := e.refs == 0
	deleted := e.deleted
	key := e.key
	e.mu.Unlock()
	if last && deleted {
		e.c.mu.Lock()
		if cur, ok := e.c.entries[key]; ok && cur == e {
			delete(e.c.entries, key)
		}
		e.c.mu.Unlock()
		e.destroy()
	}
}

// SetClean records that the on-disk contents now match the given S3 object.
func (e *Entry) SetClean(size int64, etag string, mtime time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.objSize = size
	e.etag = etag
	e.mtime = mtime
	e.dirty = false
	e.newFile = false
}

// SetMtime records a modification time (used by utimens).
func (e *Entry) SetMtime(t time.Time) {
	e.mu.Lock()
	e.mtime = t
	e.mu.Unlock()
}

// diskBytesLocked reports how much local disk this entry occupies: whole
// blocks for everything but the tail, which is only as long as the file is.
// Charging a full block for a ten-byte file would make the cache budget
// meaningless on a source tree. Caller holds e.mu.
func (e *Entry) diskBytesLocked() int64 {
	n := e.blocks.count()
	if n == 0 {
		return 0
	}
	total := int64(n) * e.c.bs
	if last := int(nblocks(e.size, e.c.bs)) - 1; last >= 0 && e.blocks.get(last) {
		total -= int64(last+1)*e.c.bs - e.size
	}
	if total < 0 {
		return 0
	}
	return total
}

func (e *Entry) destroy() {
	e.file.Close()
	_ = os.Remove(e.path)
}

// invalidateLocked drops all cached content and rebinds the entry to a new
// version of the object. Caller holds e.mu.
func (e *Entry) invalidateLocked(objSize int64, etag string, mtime time.Time) {
	e.blocks = newBitmap(int(nblocks(objSize, e.c.bs)))
	e.size, e.objSize = objSize, objSize
	e.etag, e.mtime = etag, mtime
	e.dirty, e.newFile, e.written = false, false, false
	_ = e.file.Truncate(objSize)
}

// blockPresentLocked reports whether block i can be read without S3.
func (e *Entry) blockPresentLocked(i int) bool {
	if int64(i)*e.c.bs >= e.objSize {
		return true // beyond the object's content: a local hole reads as zeros
	}
	return e.blocks.get(i)
}

// ensureLocked faults in every missing block covering [off, off+n).
// Caller must NOT hold e.mu; the entry lock is taken and released around I/O.
func (e *Entry) ensure(ctx context.Context, off, n int64) error {
	if n <= 0 {
		return nil
	}
	bs := e.c.bs

	e.mu.Lock()
	limit := e.objSize
	if off >= limit {
		e.mu.Unlock()
		return nil
	}
	if off+n > limit {
		n = limit - off
	}
	first := int(off / bs)
	last := int((off + n - 1) / bs)
	missing := false
	for i := first; i <= last; i++ {
		if !e.blockPresentLocked(i) {
			missing = true
			break
		}
	}
	key := e.key
	e.mu.Unlock()
	if !missing {
		return nil
	}

	e.fetchMu.Lock()
	defer e.fetchMu.Unlock()

	for i := first; i <= last; {
		e.mu.Lock()
		if int64(i)*bs >= e.objSize {
			e.mu.Unlock()
			break
		}
		if e.blockPresentLocked(i) {
			e.mu.Unlock()
			i++
			continue
		}
		// Extend the run of consecutive missing blocks so one GET covers them.
		j := i
		for j+1 <= last && !e.blockPresentLocked(j+1) && int64(j+1)*bs < e.objSize {
			j++
		}
		start := int64(i) * bs
		end := int64(j+1) * bs
		if end > e.objSize {
			end = e.objSize
		}
		objSizeSnapshot := e.objSize
		key = e.key
		e.mu.Unlock()

		got, err := e.c.opts.Fetch(ctx, key, start, end-start, &sectionWriter{f: e.file, off: start})
		if err != nil {
			return fmt.Errorf("fetching %s [%d,%d): %w", key, start, end, err)
		}
		if got != end-start {
			return fmt.Errorf("short read of %s [%d,%d): got %d bytes", key, start, end, got)
		}

		e.mu.Lock()
		// If the entry was invalidated or truncated while the GET was in
		// flight, discard the result rather than marking bogus blocks present.
		if e.objSize == objSizeSnapshot {
			before := e.diskBytesLocked()
			e.c.blocksFetched.Add(int64(e.blocks.setRange(i, j)))
			e.c.account(e.diskBytesLocked() - before)
		}
		e.mu.Unlock()
		i = j + 1
	}
	return nil
}

// ReadAt reads from the local file, faulting in whatever is missing.
func (e *Entry) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	e.mu.Lock()
	size := e.size
	e.used = time.Now()
	e.mu.Unlock()
	if off >= size {
		return 0, io.EOF
	}
	n := int64(len(p))
	if off+n > size {
		n = size - off
		p = p[:n]
	}
	if err := e.ensure(ctx, off, n); err != nil {
		return 0, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.readOff = off
	read, err := e.file.ReadAt(p, off)
	if err == io.EOF && read > 0 {
		err = nil
	}
	return read, err
}

// Prefetch asynchronously faults in [off, off+n) for sequential readers.
func (e *Entry) Prefetch(off, n int64) {
	if n <= 0 {
		return
	}
	e.prefetch.mu.Lock()
	if e.prefetch.pending {
		e.prefetch.mu.Unlock()
		return
	}
	e.prefetch.pending = true
	e.prefetch.mu.Unlock()

	// Hold a reference for the duration: readahead outlives the read that
	// triggered it, and the file could otherwise be closed and destroyed
	// (by an unlink, or by cache reclaim) while the fetch is in flight.
	e.Ref()

	go func() {
		defer func() {
			e.Unref()
			e.prefetch.mu.Lock()
			e.prefetch.pending = false
			e.prefetch.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := e.ensure(ctx, off, n); err != nil {
			e.c.opts.Log("cache: readahead for %s failed: %v", e.Key(), err)
		}
	}()
}

// WriteAt writes into the local file and marks the entry dirty.
func (e *Entry) WriteAt(ctx context.Context, p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	bs := e.c.bs
	// A partial write to a block that is only in S3 needs a read-modify-write,
	// so fault in the two edge blocks first.
	if off%bs != 0 {
		if err := e.ensure(ctx, (off/bs)*bs, bs); err != nil {
			return 0, err
		}
	}
	if end := off + int64(len(p)); end%bs != 0 {
		if err := e.ensure(ctx, (end/bs)*bs, bs); err != nil {
			return 0, err
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	before := e.diskBytesLocked()
	n, err := e.file.WriteAt(p, off)
	if n > 0 {
		first := int(off / bs)
		last := int((off + int64(n) - 1) / bs)
		e.blocks.setRange(first, last)
		if end := off + int64(n); end > e.size {
			e.size = end
		}
		e.c.account(e.diskBytesLocked() - before)
		if !e.dirty {
			e.dirty = true
			e.dirtySince = time.Now()
		}
		e.written = true
		e.mtime = time.Now()
		e.used = e.mtime
	}
	return n, err
}

// Truncate resizes the file, keeping the "hole reads as zeros" invariant.
func (e *Entry) Truncate(ctx context.Context, size int64) error {
	if size < 0 {
		return fmt.Errorf("negative size")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	before := e.diskBytesLocked()
	if err := e.file.Truncate(size); err != nil {
		return err
	}
	if size < e.objSize {
		// Discard S3 content past the new end so a later extend reads zeros.
		e.objSize = size
		e.blocks.clearFrom(int(nblocks(size, e.c.bs)))
	}
	e.size = size
	e.c.account(e.diskBytesLocked() - before)
	if !e.dirty {
		e.dirty = true
		e.dirtySince = time.Now()
	}
	e.written = true
	e.mtime = time.Now()
	e.used = e.mtime
	return nil
}

// Flush uploads the entry if it has local changes. It is a no-op for clean or
// deleted entries. The entry is locked for the duration of the upload, so a
// concurrent write to the same file waits rather than tearing the object.
func (e *Entry) Flush(ctx context.Context) error {
	if e.c.opts.ReadOnly {
		// A read-only mount must never write to the bucket, not even to save
		// data recovered from a previous session. It stays on local disk,
		// still marked dirty, for a later read-write mount to pick up.
		return nil
	}
	e.mu.Lock()
	if !e.dirty || e.deleted {
		e.mu.Unlock()
		return nil
	}
	size, objSize, key := e.size, e.objSize, e.key
	e.mu.Unlock()

	// The whole object is rewritten, so every byte must be local first.
	if err := e.ensure(ctx, 0, objSize); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.dirty || e.deleted {
		return nil
	}
	size = e.size
	if err := e.file.Truncate(size); err != nil {
		return err
	}
	if err := e.file.Sync(); err != nil {
		return err
	}
	body := io.NewSectionReader(e.file, 0, size)
	etag, err := e.c.opts.Upload(ctx, key, body, size)
	if err != nil {
		return err
	}
	now := time.Now()
	created := e.newFile
	before := e.diskBytesLocked()
	e.etag = etag
	e.objSize = size
	e.blocks.setRange(0, int(nblocks(size, e.c.bs))-1)
	e.c.account(e.diskBytesLocked() - before)
	e.dirty = false
	e.newFile = false
	e.written = false
	e.mtime = now
	e.c.uploads.Add(1)
	if e.c.opts.OnUploaded != nil {
		e.c.opts.OnUploaded(key, size, etag, now, created)
	}
	return nil
}

// trim gives disk back by punching cached blocks out of a file that is still
// open. Whole-entry eviction cannot touch an open file, so without this a
// single large file read in one pass would grow past the cache budget without
// limit. Only clean entries are trimmed: every block of a clean entry also
// exists in S3, so dropping one costs a refetch and nothing else.
//
// Blocks near where readers currently are, and the readahead window ahead of
// them, are kept; everything else is fair game, oldest offsets first, which is
// the right order for the sequential reads that cause this in the first place.
func (e *Entry) trim(need int64) int64 {
	if need <= 0 {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dirty || e.newFile || e.deleted {
		return 0
	}
	bs := e.c.bs
	keep := e.c.opts.Readahead
	if keep < bs {
		keep = bs
	}
	keepLo := int((e.readOff - bs) / bs)
	keepHi := int((e.readOff + keep) / bs)

	var freed int64
	for i := 0; i < e.blocks.n && freed < need; i++ {
		if i >= keepLo && i <= keepHi {
			continue
		}
		if !e.blocks.get(i) {
			continue
		}
		off := int64(i) * bs
		length := bs
		if off+length > e.size {
			length = e.size - off
		}
		if length <= 0 {
			continue
		}
		if err := punchHole(e.file, off, length); err != nil {
			e.c.opts.Log("cache: cannot reclaim space inside %s: %v", e.key, err)
			break
		}
		e.blocks.clear(i)
		freed += length
	}
	if freed > 0 {
		e.c.account(-freed)
	}
	return freed
}

// sectionWriter adapts an *os.File into a sequential io.Writer at a fixed
// offset, so a ranged GET body can be streamed straight into the cache file.
type sectionWriter struct {
	f   *os.File
	off int64
}

func (w *sectionWriter) Write(p []byte) (int, error) {
	n, err := w.f.WriteAt(p, w.off)
	w.off += int64(n)
	return n, err
}
