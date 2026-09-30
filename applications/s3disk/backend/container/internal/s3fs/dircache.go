package s3fs

import (
	"strings"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// dirCache caches directory listings for a short TTL. Local mutations
// invalidate the affected directory immediately, so the TTL only governs how
// quickly writes made by *other* clients become visible.
type dirCache struct {
	mu  sync.RWMutex
	m   map[string]*dirCacheEntry
	ttl time.Duration
	// exclusive listings never expire: only this mount can change the bucket,
	// and every local mutation invalidates the directory it touched.
	exclusive bool
	maxSize   int
}

type dirCacheEntry struct {
	entries []fuse.DirEntry
	expires time.Time
}

func newDirCache(ttl time.Duration, exclusive bool) *dirCache {
	return &dirCache{
		m:         make(map[string]*dirCacheEntry),
		ttl:       ttl,
		exclusive: exclusive,
		maxSize:   20000,
	}
}

func (c *dirCache) get(p string) ([]fuse.DirEntry, bool) {
	c.mu.RLock()
	e, ok := c.m[p]
	c.mu.RUnlock()
	if !ok || (!c.exclusive && time.Now().After(e.expires)) {
		return nil, false
	}
	return e.entries, true
}

func (c *dirCache) put(p string, entries []fuse.DirEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > c.maxSize {
		c.trimLocked()
	}
	c.m[p] = &dirCacheEntry{entries: entries, expires: time.Now().Add(c.ttl)}
}

// trimLocked bounds the cache. Expired listings go first; if that is not enough
// (nothing expires in exclusive mode) arbitrary listings are dropped, which
// only costs a re-listing.
func (c *dirCache) trimLocked() {
	now := time.Now()
	if !c.exclusive {
		for k, v := range c.m {
			if now.After(v.expires) {
				delete(c.m, k)
			}
		}
	}
	for k := range c.m {
		if len(c.m) <= c.maxSize/2 {
			return
		}
		delete(c.m, k)
	}
}

// clear drops every cached listing.
func (c *dirCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = make(map[string]*dirCacheEntry)
}

// add records a child this mount just created. Updating the cached listing
// beats dropping it: a create is followed by more lookups in the same
// directory, and a dropped listing turns every one of those into a round trip.
// The mount is the only thing that can change the directory, so the updated
// listing is as correct as a re-read would be.
func (c *dirCache) add(p string, entry fuse.DirEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[p]
	if !ok {
		return // nothing cached for this directory; nothing to keep current
	}
	for _, existing := range e.entries {
		if existing.Name == entry.Name {
			return
		}
	}
	// Copy: a reader may still hold the slice this listing was returned as.
	next := make([]fuse.DirEntry, len(e.entries), len(e.entries)+1)
	copy(next, e.entries)
	c.m[p] = &dirCacheEntry{entries: append(next, entry), expires: e.expires}
}

// remove records a child this mount just deleted.
func (c *dirCache) remove(p, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[p]
	if !ok {
		return
	}
	next := make([]fuse.DirEntry, 0, len(e.entries))
	for _, existing := range e.entries {
		if existing.Name != name {
			next = append(next, existing)
		}
	}
	c.m[p] = &dirCacheEntry{entries: next, expires: e.expires}
}

// seed records that a directory this mount just created is empty. Every lookup
// inside it can then be answered locally until something is put there, which is
// what makes writing a tree of new files cheap.
func (c *dirCache) seed(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[p] = &dirCacheEntry{entries: []fuse.DirEntry{}, expires: time.Now().Add(c.ttl)}
}

func (c *dirCache) invalidate(p string) {
	c.mu.Lock()
	delete(c.m, p)
	c.mu.Unlock()
}

func (c *dirCache) invalidatePrefix(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.m {
		if k == p || strings.HasPrefix(k, p+"/") {
			delete(c.m, k)
		}
	}
}
