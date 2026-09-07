package s3fs

import (
	"mime"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"

	"s3disk/internal/config"
	"s3disk/internal/s3io"
)

// Attr is the inode state s3disk tracks for one path.
//
// POSIX bits are round-tripped through S3 user metadata using the same keys as
// s3fs-fuse (mode/uid/gid/mtime/atime/ctime), so a bucket written by s3disk is
// readable by s3fs and vice versa. Objects without that metadata fall back to
// the mount's --file-mode/--dir-mode/--uid/--gid.
type Attr struct {
	Mode    uint32 // full mode including the file type bits
	Size    int64
	Uid     uint32
	Gid     uint32
	Mtime   time.Time
	Atime   time.Time
	Ctime   time.Time
	ETag    string
	Symlink string // target, for S_IFLNK
}

// IsDir reports whether this attribute describes a directory.
func (a *Attr) IsDir() bool { return a.Mode&syscall.S_IFMT == syscall.S_IFDIR }

// IsLink reports whether this attribute describes a symbolic link.
func (a *Attr) IsLink() bool { return a.Mode&syscall.S_IFMT == syscall.S_IFLNK }

// Fill copies the attribute into a FUSE reply.
func (a *Attr) Fill(out *fuse.Attr) {
	out.Mode = a.Mode
	out.Size = uint64(a.Size)
	out.Blocks = uint64((a.Size + 511) / 512)
	out.Owner.Uid = a.Uid
	out.Owner.Gid = a.Gid
	out.Nlink = 1
	if a.IsDir() {
		out.Nlink = 2
	}
	out.Blksize = 4096
	setTime(&out.Mtime, &out.Mtimensec, a.Mtime)
	setTime(&out.Atime, &out.Atimensec, a.Atime)
	setTime(&out.Ctime, &out.Ctimensec, a.Ctime)
}

func setTime(sec *uint64, nsec *uint32, t time.Time) {
	if t.IsZero() {
		t = time.Unix(0, 0)
	}
	*sec = uint64(t.Unix())
	*nsec = uint32(t.Nanosecond())
}

// attrFromObject reconstructs POSIX attributes from an S3 object.
func (f *FS) attrFromObject(o *s3io.Object, isDir bool) *Attr {
	a := &Attr{
		Size:  o.Size,
		Uid:   f.cfg.UID,
		Gid:   f.cfg.GID,
		Mtime: o.Modified,
		ETag:  o.ETag,
	}
	if isDir {
		a.Mode = syscall.S_IFDIR | f.cfg.DirMode
		a.Size = 4096
	} else {
		a.Mode = syscall.S_IFREG | f.cfg.FileMode
	}

	if m := o.Meta; m != nil {
		if v, ok := m["uid"]; ok {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil {
				a.Uid = uint32(n)
			}
		}
		if v, ok := m["gid"]; ok {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil {
				a.Gid = uint32(n)
			}
		}
		if v, ok := m["mode"]; ok {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil && n&uint64(syscall.S_IFMT) != 0 {
				// s3fs stores the complete mode word, file type included.
				a.Mode = uint32(n)
				if a.IsDir() {
					a.Size = 4096
				}
			} else if err == nil {
				a.Mode = (a.Mode & syscall.S_IFMT) | (uint32(n) & 07777)
			}
		}
		if t, ok := parseTime(m["mtime"]); ok {
			a.Mtime = t
		}
		if t, ok := parseTime(m["atime"]); ok {
			a.Atime = t
		}
		if t, ok := parseTime(m["ctime"]); ok {
			a.Ctime = t
		}
	}
	if a.Atime.IsZero() {
		a.Atime = a.Mtime
	}
	if a.Ctime.IsZero() {
		a.Ctime = a.Mtime
	}
	return a
}

// metaFor encodes POSIX attributes into S3 user metadata.
func metaFor(a *Attr) map[string]string {
	m := map[string]string{
		"mode":  strconv.FormatUint(uint64(a.Mode), 10),
		"uid":   strconv.FormatUint(uint64(a.Uid), 10),
		"gid":   strconv.FormatUint(uint64(a.Gid), 10),
		"mtime": strconv.FormatInt(a.Mtime.Unix(), 10),
		"atime": strconv.FormatInt(a.Atime.Unix(), 10),
		"ctime": strconv.FormatInt(a.Ctime.Unix(), 10),
	}
	return m
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	// s3fs writes whole seconds; tolerate "sec.nsec" too.
	if i := strings.IndexByte(s, '.'); i >= 0 {
		sec, err1 := strconv.ParseInt(s[:i], 10, 64)
		nsec, err2 := strconv.ParseInt(strings.TrimRight(s[i+1:], "0"), 10, 64)
		if err1 == nil {
			if err2 != nil {
				nsec = 0
			}
			return time.Unix(sec, nsec), true
		}
	}
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}

func contentTypeFor(name string) string {
	if ct := mime.TypeByExtension(strings.ToLower(path.Ext(name))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// attrCache is a TTL cache of path attributes, including negative entries.
//
// Entries the local mount authored are "sticky": they never expire on a timer,
// because they are the truth until the corresponding object is uploaded.
type attrCache struct {
	mu      sync.RWMutex
	m       map[string]*attrCacheEntry
	ttl     time.Duration
	negTTL  time.Duration
	maxSize int
	// exclusive means nothing but this mount can change the bucket, so entries
	// stay valid until a local operation invalidates them.
	exclusive bool
}

type attrCacheEntry struct {
	attr     *Attr
	negative bool
	sticky   bool
	expires  time.Time
}

func newAttrCache(cfg *config.Config) *attrCache {
	return &attrCache{
		m:         make(map[string]*attrCacheEntry),
		ttl:       cfg.StatTTL,
		negTTL:    cfg.NegativeTTL,
		maxSize:   200000,
		exclusive: cfg.Exclusive,
	}
}

// get returns (attr, negative, ok).
func (c *attrCache) get(p string) (*Attr, bool, bool) {
	c.mu.RLock()
	e, ok := c.m[p]
	c.mu.RUnlock()
	if !ok {
		return nil, false, false
	}
	if !e.sticky && !c.exclusive && time.Now().After(e.expires) {
		c.mu.Lock()
		if cur, ok := c.m[p]; ok && cur == e {
			delete(c.m, p)
		}
		c.mu.Unlock()
		return nil, false, false
	}
	return e.attr, e.negative, true
}

func (c *attrCache) put(p string, a *Attr) { c.set(p, a, false, false) }

// putSticky records an attribute that only exists locally so far.
func (c *attrCache) putSticky(p string, a *Attr) { c.set(p, a, false, true) }

func (c *attrCache) putNegative(p string) { c.set(p, nil, true, false) }

func (c *attrCache) set(p string, a *Attr, negative, sticky bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > c.maxSize {
		c.evictLocked()
	}
	ttl := c.ttl
	if negative {
		ttl = c.negTTL
	}
	c.m[p] = &attrCacheEntry{attr: a, negative: negative, sticky: sticky, expires: time.Now().Add(ttl)}
}

// unstick lets a previously local-only path fall back to normal TTL caching.
func (c *attrCache) unstick(p string, a *Attr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[p] = &attrCacheEntry{attr: a, expires: time.Now().Add(c.ttl)}
}

func (c *attrCache) invalidate(p string) {
	c.mu.Lock()
	delete(c.m, p)
	c.mu.Unlock()
}

// invalidatePrefix drops a whole subtree (used by recursive rename/delete).
func (c *attrCache) invalidatePrefix(p string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.m {
		if k == p || strings.HasPrefix(k, p+"/") {
			delete(c.m, k)
		}
	}
}

func (c *attrCache) evictLocked() {
	// Cheap bulk trim: drop everything that has already expired, and if that is
	// not enough, drop arbitrary non-sticky entries. Dropping a cached
	// attribute only costs a re-read; it is never a correctness problem.
	now := time.Now()
	if !c.exclusive {
		for k, e := range c.m {
			if !e.sticky && now.After(e.expires) {
				delete(c.m, k)
			}
		}
	}
	if len(c.m) <= c.maxSize {
		return
	}
	n := len(c.m) - c.maxSize/2
	for k, e := range c.m {
		if n <= 0 {
			return
		}
		if !e.sticky {
			delete(c.m, k)
			n--
		}
	}
}

// refresh drops everything the mount learned from S3, keeping only entries that
// exist solely in this mount (unsaved creates and writes).
func (c *attrCache) refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		if !e.sticky {
			delete(c.m, k)
		}
	}
}

func (c *attrCache) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.m)
}
