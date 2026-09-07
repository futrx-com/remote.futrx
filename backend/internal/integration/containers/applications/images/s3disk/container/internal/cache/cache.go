// Package cache implements the local disk cache that turns S3 objects into
// randomly readable, randomly writable files.
//
// Every open object is backed by a sparse file on local disk. Reads fault in
// missing regions with ranged GETs at block granularity; writes land in the
// local file and mark it dirty. A dirty file is uploaded again in full when it
// is closed, fsync'd, or has been idle for longer than the dirty timeout.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Fetcher downloads [off, off+length) of an object into dst.
type Fetcher func(ctx context.Context, key string, off, length int64, dst io.Writer) (int64, error)

// Uploader stores the full body of an object and returns its new ETag.
type Uploader func(ctx context.Context, key string, body io.ReadSeeker, size int64) (etag string, err error)

// Options configures a Cache.
type Options struct {
	Dir string
	// Identity names the bucket and prefix this cache belongs to. Cache keys
	// are bucket-relative, so a directory reused for a different mount would
	// otherwise serve one bucket's data for another bucket's keys.
	Identity     string
	BlockSize    int64
	MaxBytes     int64
	Readahead    int64
	DirtyTimeout time.Duration
	Persist      bool
	// AsyncWriteback lets close(2) return before the object has been stored,
	// leaving the upload to the background flusher. A build that writes
	// hundreds of files then stops blocking on a round trip per file; the cost
	// is that a file is durable a few seconds after close rather than at it.
	AsyncWriteback bool
	// UploadWorkers bounds how many write-backs run at once.
	UploadWorkers int
	// ReadOnly stops the cache from ever writing to S3. Locally recovered data
	// stays dirty on disk so that a later read-write mount can upload it.
	ReadOnly bool
	Fetch    Fetcher
	Upload   Uploader
	// OnUploaded reports a completed write-back. created distinguishes a file
	// that had never existed in S3 from an overwrite, because only the former
	// changes what its directory contains.
	OnUploaded func(key string, size int64, etag string, mtime time.Time, created bool)
	Log        func(format string, args ...any)
}

// Stats is a snapshot of cache activity, surfaced by `s3disk status`.
type Stats struct {
	Entries     int   `json:"entries"`
	Dirty       int   `json:"dirty"`
	Bytes       int64 `json:"bytes"`
	MaxBytes    int64 `json:"max_bytes"`
	Hits        int64 `json:"hits"`
	Misses      int64 `json:"misses"`
	Evictions   int64 `json:"evictions"`
	Uploads     int64 `json:"uploads"`
	BlocksFetch int64 `json:"blocks_fetched"`
}

// Cache owns every locally materialised object.
type Cache struct {
	opts Options
	dir  string
	bs   int64

	mu      sync.Mutex
	entries map[string]*Entry
	closed  bool

	// bytes is the local disk currently held by cached blocks. It is atomic so
	// it can be adjusted while an entry lock is held, without ever taking the
	// cache lock in the reverse order.
	bytes    atomic.Int64
	evicting atomic.Bool

	hits, misses, evictions, uploads, blocksFetched atomic.Int64

	flushStop chan struct{}
	flushDone chan struct{}
}

// indexSaveInterval bounds how often the cache index is written while files are
// dirty. The index is what lets a killed mount recover unsaved writes, so it
// has to be refreshed as work happens, not only at shutdown.
const indexSaveInterval = 5 * time.Second

// New opens (or creates) a cache directory.
func New(opts Options) (*Cache, error) {
	if opts.BlockSize <= 0 {
		opts.BlockSize = 4 << 20
	}
	if opts.Log == nil {
		opts.Log = func(string, ...any) {}
	}
	c := &Cache{
		opts:      opts,
		dir:       opts.Dir,
		bs:        opts.BlockSize,
		entries:   make(map[string]*Entry),
		flushStop: make(chan struct{}),
		flushDone: make(chan struct{}),
	}
	if err := os.MkdirAll(filepath.Join(c.dir, "data"), 0700); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}
	if opts.Persist {
		if err := c.loadIndex(); err != nil {
			c.opts.Log("cache: discarding unusable index: %v", err)
			c.wipe()
		}
	} else {
		c.wipe()
	}
	go c.flushLoop()
	return c, nil
}

// BlockSize is the granularity of read faults.
func (c *Cache) BlockSize() int64 { return c.bs }

func (c *Cache) pathFor(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, "data", h[:2], h)
}

// Open returns the entry for key, creating it if needed. The caller owns one
// reference and must call Unref. objSize/etag describe the object currently in
// S3; if the ETag moved under a clean entry, the cached blocks are dropped.
func (c *Cache) Open(key string, objSize int64, etag string, mtime time.Time) (*Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		e.mu.Lock()
		if e.dirty || e.newFile {
			// Local writes win; they will overwrite whatever is in S3.
			e.refs++
			e.used = time.Now()
			e.mu.Unlock()
			c.hits.Add(1)
			return e, nil
		}
		if etag != "" && e.etag != "" && e.etag == etag {
			e.refs++
			e.used = time.Now()
			e.mu.Unlock()
			c.hits.Add(1)
			return e, nil
		}
		// Stale: the object changed in S3 behind our back.
		c.bytes.Add(-e.diskBytesLocked())
		e.invalidateLocked(objSize, etag, mtime)
		e.refs++
		e.used = time.Now()
		e.mu.Unlock()
		c.misses.Add(1)
		return e, nil
	}
	e, err := c.newEntryLocked(key, objSize, etag, mtime, false)
	if err != nil {
		return nil, err
	}
	c.misses.Add(1)
	e.refs++
	return e, nil
}

// Create makes an empty, fully present, dirty entry for a brand new file.
func (c *Cache) Create(key string) (*Entry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		e.mu.Lock()
		c.bytes.Add(-e.diskBytesLocked())
		e.invalidateLocked(0, "", time.Now())
		e.size, e.objSize = 0, 0
		e.newFile, e.dirty, e.written = true, true, false
		e.dirtySince = time.Now()
		_ = e.file.Truncate(0)
		e.refs++
		e.used = time.Now()
		e.mu.Unlock()
		return e, nil
	}
	e, err := c.newEntryLocked(key, 0, "", time.Now(), true)
	if err != nil {
		return nil, err
	}
	e.refs++
	return e, nil
}

func (c *Cache) newEntryLocked(key string, objSize int64, etag string, mtime time.Time, fresh bool) (*Entry, error) {
	p := c.pathFor(key)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	e := &Entry{
		c:       c,
		key:     key,
		file:    f,
		path:    p,
		size:    objSize,
		objSize: objSize,
		etag:    etag,
		mtime:   mtime,
		blocks:  newBitmap(int(nblocks(objSize, c.bs))),
		used:    time.Now(),
	}
	if fresh {
		e.size, e.objSize = 0, 0
		e.newFile, e.dirty, e.written = true, true, false
		e.dirtySince = time.Now()
	}
	if err := f.Truncate(e.size); err != nil {
		f.Close()
		return nil, err
	}
	c.entries[key] = e
	return e, nil
}

// Lookup returns an existing entry without taking a reference.
func (c *Cache) Lookup(key string) *Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[key]
}

// Rename moves cached data with a renamed object so the cache stays warm.
//
// The backing file has to move too: its name is derived from the object key,
// and leaving it behind would alias the entry onto whatever file is created at
// the old key next. (That is exactly the "write lockfile, rename over the
// original" pattern git and most editors use.)
func (c *Cache) Rename(oldKey, newKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[oldKey]
	if !ok {
		return
	}
	if victim, ok := c.entries[newKey]; ok && victim != e {
		c.dropLocked(victim, newKey)
	}
	newPath := c.pathFor(newKey)
	if err := os.MkdirAll(filepath.Dir(newPath), 0700); err != nil {
		c.dropLocked(e, oldKey) // fall back to refetching from S3
		return
	}
	if err := os.Rename(e.path, newPath); err != nil {
		c.opts.Log("cache: could not move cached data for %s: %v", oldKey, err)
		c.dropLocked(e, oldKey)
		return
	}
	delete(c.entries, oldKey)
	e.mu.Lock()
	e.key = newKey
	e.path = newPath
	e.mu.Unlock()
	c.entries[newKey] = e
}

// Remove drops an entry and its local data (used after unlink).
func (c *Cache) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok {
		c.dropLocked(e, key)
	}
}

// dropLocked detaches an entry from the cache. Data still open by a file
// handle is kept alive until the last reference goes away, mirroring the POSIX
// rule that an unlinked file stays readable while it is open.
func (c *Cache) dropLocked(e *Entry, key string) {
	delete(c.entries, key)
	e.mu.Lock()
	c.bytes.Add(-e.diskBytesLocked())
	e.deleted = true
	e.dirty = false
	refs := e.refs
	e.mu.Unlock()
	if refs == 0 {
		e.destroy()
	}
}

// RemovePrefix drops every cached entry under a key prefix (recursive delete).
func (c *Cache) RemovePrefix(prefix string) {
	c.mu.Lock()
	keys := make([]string, 0)
	for k := range c.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	c.mu.Unlock()
	for _, k := range keys {
		c.Remove(k)
	}
}

// DirtyKeys lists objects with unsaved local changes.
func (c *Cache) DirtyKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for k, e := range c.entries {
		e.mu.Lock()
		if e.dirty {
			out = append(out, k)
		}
		e.mu.Unlock()
	}
	return out
}

// FlushAll uploads every dirty entry. Returns the first error encountered.
func (c *Cache) FlushAll(ctx context.Context) error {
	c.mu.Lock()
	list := make([]*Entry, 0, len(c.entries))
	for _, e := range c.entries {
		list = append(list, e)
	}
	c.mu.Unlock()

	workers := c.opts.UploadWorkers
	if workers < 1 {
		workers = 1
	}
	if workers > len(list) {
		workers = len(list)
	}
	if workers == 0 {
		return nil
	}
	queue := make(chan *Entry)
	errs := make(chan error, len(list))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range queue {
				if err := e.Flush(ctx); err != nil {
					errs <- err
				}
			}
		}()
	}
	for _, e := range list {
		queue <- e
	}
	close(queue)
	wg.Wait()
	close(errs)
	return <-errs
}

// Bytes reports the local disk currently used by cached data. Unlike Stats it
// takes no locks, so it is safe on hot paths such as statfs.
func (c *Cache) Bytes() int64 { return c.bytes.Load() }

// Stats snapshots counters for the status endpoint.
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	n, dirty := len(c.entries), 0
	for _, e := range c.entries {
		e.mu.Lock()
		if e.dirty {
			dirty++
		}
		e.mu.Unlock()
	}
	c.mu.Unlock()
	bytes := c.bytes.Load()
	return Stats{
		Entries: n, Dirty: dirty, Bytes: bytes, MaxBytes: c.opts.MaxBytes,
		Hits: c.hits.Load(), Misses: c.misses.Load(), Evictions: c.evictions.Load(),
		Uploads: c.uploads.Load(), BlocksFetch: c.blocksFetched.Load(),
	}
}

// Close flushes dirty data, persists the index and releases all files.
func (c *Cache) Close(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	close(c.flushStop)
	<-c.flushDone

	err := c.FlushAll(ctx)
	if c.opts.Persist {
		if serr := c.saveIndex(); serr != nil && err == nil {
			err = serr
		}
	}
	c.mu.Lock()
	for _, e := range c.entries {
		e.file.Close()
	}
	c.mu.Unlock()
	return err
}

// flushLoop writes back files that have been dirty and idle for a while, so a
// long-running editor or build does not sit on unsaved data.
func (c *Cache) flushLoop() {
	defer close(c.flushDone)
	if c.opts.ReadOnly || c.opts.DirtyTimeout <= 0 {
		<-c.flushStop
		return
	}
	interval := c.opts.DirtyTimeout / 2
	if interval > time.Second {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	var lastSave time.Time
	hadDirty := false
	for {
		select {
		case <-c.flushStop:
			return
		case <-t.C:
			cutoff := time.Now().Add(-c.opts.DirtyTimeout)
			c.mu.Lock()
			var due []*Entry
			dirty := 0
			for _, e := range c.entries {
				e.mu.Lock()
				if e.dirty {
					dirty++
					if e.dirtySince.Before(cutoff) {
						due = append(due, e)
					}
				}
				e.mu.Unlock()
			}
			c.mu.Unlock()

			// Persist the index while writes are outstanding, so a mount that is
			// killed can pick them up again on the next start.
			if c.opts.Persist && (dirty > 0 || hadDirty) && time.Since(lastSave) >= indexSaveInterval {
				if err := c.saveIndex(); err != nil {
					c.opts.Log("cache: could not save index: %v", err)
				}
				lastSave = time.Now()
			}
			hadDirty = dirty > 0

			c.flushBatch(due)
		}
	}
}

// flushBatch uploads entries concurrently. Write-back is round-trip bound, so
// doing it one at a time makes a build that writes many files as slow as the
// sum of its uploads rather than the slowest few.
func (c *Cache) flushBatch(due []*Entry) {
	workers := c.opts.UploadWorkers
	if workers < 1 {
		workers = 1
	}
	if workers > len(due) {
		workers = len(due)
	}
	if workers == 0 {
		return
	}
	queue := make(chan *Entry)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range queue {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				if err := e.Flush(ctx); err != nil {
					c.opts.Log("cache: background flush of %s failed: %v", e.Key(), err)
				}
				cancel()
			}
		}()
	}
	for _, e := range due {
		queue <- e
	}
	close(queue)
	wg.Wait()
}

// AsyncWriteback reports whether close(2) may return before the upload.
func (c *Cache) AsyncWriteback() bool { return c.opts.AsyncWriteback }

// evict drops clean, unreferenced entries, least recently used first, until the
// cache is back under budget. Candidates are collected and sorted once, rather
// than rescanning for each victim, so reclaiming space on a large tree does not
// degrade into a quadratic scan.
func (c *Cache) evict() {
	if c.opts.MaxBytes <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bytes.Load() <= c.opts.MaxBytes {
		return
	}

	type candidate struct {
		key   string
		entry *Entry
		used  time.Time
	}
	candidates := make([]candidate, 0, len(c.entries))
	for k, e := range c.entries {
		e.mu.Lock()
		evictable := e.refs == 0 && !e.dirty && !e.newFile && e.blocks.count() > 0
		used := e.used
		e.mu.Unlock()
		if evictable {
			candidates = append(candidates, candidate{k, e, used})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].used.Before(candidates[j].used) })

	target := c.opts.MaxBytes * 9 / 10
	for _, cand := range candidates {
		if c.bytes.Load() <= target {
			return
		}
		c.dropLocked(cand.entry, cand.key)
		c.evictions.Add(1)
	}

	// Nothing left to drop whole. Reclaim ranges from files that are still open
	// but clean, so a large file being read cannot grow past the budget.
	if c.bytes.Load() <= target {
		return
	}
	open := make([]*Entry, 0, len(c.entries))
	for _, e := range c.entries {
		e.mu.Lock()
		trimmable := !e.dirty && !e.newFile && e.blocks.count() > 0
		used := e.used
		e.mu.Unlock()
		if trimmable {
			e.trimOrder = used
			open = append(open, e)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].trimOrder.Before(open[j].trimOrder) })
	for _, e := range open {
		over := c.bytes.Load() - target
		if over <= 0 {
			return
		}
		if e.trim(over) > 0 {
			c.evictions.Add(1)
		}
	}
}

// account adjusts the disk-usage counter and kicks off eviction when the cache
// is over budget. Safe to call with an entry lock held.
func (c *Cache) account(delta int64) {
	if delta == 0 {
		return
	}
	total := c.bytes.Add(delta)
	if c.opts.MaxBytes <= 0 || total <= c.opts.MaxBytes {
		return
	}
	if c.evicting.CompareAndSwap(false, true) {
		go func() {
			defer c.evicting.Store(false)
			c.evict()
		}()
	}
}

func (c *Cache) wipe() {
	_ = os.RemoveAll(filepath.Join(c.dir, "data"))
	_ = os.Remove(filepath.Join(c.dir, "index.json"))
	_ = os.MkdirAll(filepath.Join(c.dir, "data"), 0700)
}

func nblocks(size, bs int64) int64 {
	if size <= 0 {
		return 0
	}
	return (size + bs - 1) / bs
}
