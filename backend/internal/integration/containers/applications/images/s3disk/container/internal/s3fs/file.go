package s3fs

import (
	"sync"

	"s3disk/internal/cache"
)

// cacheEntry is an alias so node.go reads cleanly.
type cacheEntry = cache.Entry

// fileHandle is the per-open-file state. Several handles can share one cache
// entry; the entry holds the data and the reference count.
type fileHandle struct {
	fsys  *FS
	entry *cacheEntry
	path  string
	key   string
	flags uint32

	mu      sync.Mutex
	nextOff int64 // offset a sequential reader would use next
	seq     int   // consecutive sequential reads seen
}

// noteRead tracks sequential access and triggers readahead once a reader looks
// like it is streaming, which is what makes builds, greps and tars fast.
func (h *fileHandle) noteRead(off, n, readahead int64) {
	if readahead <= 0 || n <= 0 {
		return
	}
	h.mu.Lock()
	sequential := off == h.nextOff
	if sequential {
		h.seq++
	} else {
		h.seq = 0
	}
	h.nextOff = off + n
	seq := h.seq
	next := h.nextOff
	h.mu.Unlock()

	if seq >= 2 {
		h.entry.Prefetch(next, readahead)
	}
}
