// Package s3fs implements the FUSE filesystem: it maps VFS operations onto S3
// objects, using a local block cache for data and a TTL cache for metadata.
package s3fs

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"s3disk/internal/cache"
	"s3disk/internal/config"
	"s3disk/internal/s3io"
)

// FS is the shared state behind every node in one mount.
type FS struct {
	cfg   *config.Config
	s3    *s3io.Client
	cache *cache.Cache
	attrs *attrCache
	dirs  *dirCache
	log   func(string, ...any)

	// pending tracks names that exist only in the local cache because their
	// object has not been uploaded yet; readdir merges them into listings.
	pendingMu sync.Mutex
	pending   map[string]map[string]bool // parent dir path -> name

	started time.Time
	ops     atomic.Int64
	root    *Node
	server  *fuse.Server
}

// New builds the filesystem and its cache. Nothing is mounted yet.
func New(ctx context.Context, cfg *config.Config, logf func(string, ...any)) (*FS, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	client, err := s3io.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := client.CheckAccess(ctx); err != nil {
		return nil, err
	}
	f := &FS{
		cfg:     cfg,
		s3:      client,
		attrs:   newAttrCache(cfg),
		dirs:    newDirCache(cfg.ListTTL, cfg.Exclusive),
		log:     logf,
		pending: make(map[string]map[string]bool),
		started: time.Now(),
	}
	f.cache, err = cache.New(cache.Options{
		Dir:            cfg.CacheDir,
		Identity:       cfg.Identity(),
		BlockSize:      cfg.BlockSize,
		MaxBytes:       cfg.CacheSize,
		Readahead:      cfg.Readahead,
		DirtyTimeout:   cfg.DirtyTimeout,
		AsyncWriteback: cfg.AsyncWriteback,
		UploadWorkers:  cfg.UploadWorkers,
		Persist:        cfg.PersistCache,
		ReadOnly:       cfg.ReadOnly,
		Fetch:          f.fetchRange,
		Upload:         f.uploadObject,
		OnUploaded:     f.onUploaded,
		Log:            logf,
	})
	if err != nil {
		return nil, err
	}
	f.root = &Node{fsys: f}
	return f, nil
}

// Root returns the root node for mounting.
func (f *FS) Root() *Node { return f.root }

// Cache exposes the block cache (used by the control socket and shutdown).
func (f *FS) Cache() *cache.Cache { return f.cache }

// Recover re-uploads anything left dirty by a previous mount.
func (f *FS) Recover(ctx context.Context) (int, error) {
	return f.cache.Recover(ctx, f.log)
}

func (f *FS) fetchRange(ctx context.Context, key string, off, length int64, dst io.Writer) (int64, error) {
	return f.s3.GetRange(ctx, key, off, length, dst)
}

// uploadObject is the cache's write-back hook: it attaches the current POSIX
// metadata for the path and PUTs the whole object.
func (f *FS) uploadObject(ctx context.Context, key string, body io.ReadSeeker, size int64) (string, error) {
	p := f.s3.PathOf(key)
	a, _, ok := f.attrs.get(p)
	if !ok || a == nil {
		a = &Attr{Mode: syscall.S_IFREG | f.cfg.FileMode, Uid: f.cfg.UID, Gid: f.cfg.GID, Mtime: time.Now()}
	}
	now := time.Now()
	meta := metaFor(&Attr{
		Mode: a.Mode, Uid: a.Uid, Gid: a.Gid,
		Mtime: pick(a.Mtime, now), Atime: pick(a.Atime, now), Ctime: pick(a.Ctime, now),
	})
	obj, err := f.s3.Put(ctx, s3io.PutInput{
		Key: key, Body: body, Size: size, Meta: meta,
		ContentType: contentTypeFor(p),
	})
	if err != nil {
		return "", err
	}
	return obj.ETag, nil
}

// onUploaded promotes a local-only path to a normal cached one.
func (f *FS) onUploaded(key string, size int64, etag string, mtime time.Time, created bool) {
	p := f.s3.PathOf(key)
	if a, _, ok := f.attrs.get(p); ok && a != nil {
		na := *a
		na.Size = size
		na.ETag = etag
		f.attrs.unstick(p, &na)
	}
	dir := path.Dir(p)
	f.clearPending(dir, path.Base(p))
	if created {
		// A listing taken while this file was still local-only does not contain
		// it, and the file has just stopped being local-only — so without this
		// the file would vanish from readdir once its pending marker cleared.
		// Recording the name keeps the listing usable; dropping it would cost
		// a re-listing for every file a build writes.
		f.dirs.add(dir, fuse.DirEntry{Name: path.Base(p), Mode: syscall.S_IFREG})
	}
}

func pick(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}

// ---------------------------------------------------------------- pending set

func (f *FS) markPending(dir, name string) {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	m, ok := f.pending[dir]
	if !ok {
		m = make(map[string]bool)
		f.pending[dir] = m
	}
	m[name] = true
}

func (f *FS) clearPending(dir, name string) {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	if m, ok := f.pending[dir]; ok {
		delete(m, name)
		if len(m) == 0 {
			delete(f.pending, dir)
		}
	}
}

func (f *FS) pendingIn(dir string) []string {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	m := f.pending[dir]
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	return out
}

func (f *FS) clearPendingPrefix(prefix string) {
	f.pendingMu.Lock()
	defer f.pendingMu.Unlock()
	for dir := range f.pending {
		if dir == prefix || strings.HasPrefix(dir, prefix+"/") {
			delete(f.pending, dir)
		}
	}
}

// -------------------------------------------------------------------- stat

// dirAttr synthesises attributes for a directory with no marker object.
func (f *FS) dirAttr() *Attr {
	now := f.started
	return &Attr{
		Mode: syscall.S_IFDIR | f.cfg.DirMode, Size: 4096,
		Uid: f.cfg.UID, Gid: f.cfg.GID,
		Mtime: now, Atime: now, Ctime: now,
	}
}

// knownAbsent answers "this path does not exist" without asking S3.
//
// It only applies to an exclusive mount: the parent directory has been listed
// in full, nothing outside this mount can add to it, and every local change
// invalidates the listing. That makes a cached listing authoritative, which
// matters because tools resolving imports and include paths generate far more
// misses than hits.
func (f *FS) knownAbsent(p string) bool {
	if !f.cfg.Exclusive {
		return false
	}
	dir, name := parentOf(p), baseOf(p)
	entries, ok := f.dirs.get(dir)
	if !ok {
		return false
	}
	for _, e := range entries {
		if e.Name == name {
			return false
		}
	}
	// A file created here but not uploaded yet is not in the S3 listing.
	for _, pending := range f.pendingIn(dir) {
		if pending == name {
			return false
		}
	}
	return true
}

// stat resolves a path to attributes, consulting the attribute cache, then S3.
func (f *FS) stat(ctx context.Context, p string) (*Attr, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." {
		return f.dirAttr(), nil
	}
	if a, negative, ok := f.attrs.get(p); ok {
		if negative {
			return nil, syscall.ENOENT
		}
		return a, nil
	}

	if f.knownAbsent(p) {
		f.attrs.putNegative(p)
		return nil, syscall.ENOENT
	}

	// A file that is open and dirty is authoritative locally.
	if e := f.cache.Lookup(f.s3.Key(p)); e != nil && e.Dirty() {
		a := &Attr{
			Mode: syscall.S_IFREG | f.cfg.FileMode, Size: e.Size(),
			Uid: f.cfg.UID, Gid: f.cfg.GID, Mtime: e.Mtime(), Atime: e.Mtime(), Ctime: e.Mtime(),
		}
		f.attrs.putSticky(p, a)
		return a, nil
	}

	obj, err := f.s3.Head(ctx, f.s3.Key(p))
	if err == nil {
		a := f.attrFromObject(obj, false)
		f.attrs.put(p, a)
		return a, nil
	}
	if !errors.Is(err, s3io.ErrNotFound) {
		return nil, err
	}

	// Not a plain object: it may still be a directory, either an explicit
	// "path/" marker or an implicit prefix with children.
	a, err := f.probeDir(ctx, p)
	if err != nil {
		return nil, err
	}
	if a == nil {
		f.attrs.putNegative(p)
		return nil, syscall.ENOENT
	}
	f.attrs.put(p, a)
	return a, nil
}

// probeDir reports directory attributes for p, or nil if p is not a directory.
func (f *FS) probeDir(ctx context.Context, p string) (*Attr, error) {
	dirKey := f.s3.DirKey(p)
	hasMarker := false
	found := false
	err := f.s3.ListAll(ctx, dirKey, 2, func(key string, size int64) bool {
		found = true
		if key == dirKey {
			hasMarker = true
			return true // keep looking for a real child
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	if !found {
		// A prefix with only sub-prefixes still lists under ListAll, so a
		// negative here really means "nothing under this path".
		return nil, nil
	}
	a := f.dirAttr()
	if hasMarker && f.cfg.AttrMode == config.AttrFull {
		if obj, err := f.s3.Head(ctx, dirKey); err == nil {
			a = f.attrFromObject(obj, true)
		}
	}
	return a, nil
}

// -------------------------------------------------------------------- listing

// listDir returns the children of a directory, merging S3 listing results with
// locally created entries that have not been uploaded yet.
func (f *FS) listDir(ctx context.Context, p string) ([]fuse.DirEntry, error) {
	p = strings.TrimPrefix(p, "/")
	if ents, ok := f.dirs.get(p); ok {
		return f.mergeLocal(p, ents), nil
	}
	dirKey := f.s3.DirKey(p)
	var raw []s3io.ListEntry
	err := f.s3.List(ctx, dirKey, func(e s3io.ListEntry) bool {
		raw = append(raw, e)
		return true
	})
	if err != nil {
		return nil, err
	}

	if f.cfg.AttrMode == config.AttrFull {
		f.prefetchAttrs(ctx, p, raw)
	}

	ents := make([]fuse.DirEntry, 0, len(raw))
	for _, e := range raw {
		child := join(p, e.Name)
		var a *Attr
		if cached, negative, ok := f.attrs.get(child); ok && !negative {
			a = cached
		} else {
			a = &Attr{
				Mode: syscall.S_IFREG | f.cfg.FileMode, Size: e.Size,
				Uid: f.cfg.UID, Gid: f.cfg.GID,
				Mtime: e.Modified, Atime: e.Modified, Ctime: e.Modified, ETag: e.ETag,
			}
			if e.IsDir {
				a = f.dirAttr()
			}
			f.attrs.put(child, a)
		}
		ents = append(ents, fuse.DirEntry{Name: e.Name, Mode: a.Mode & syscall.S_IFMT})
	}
	f.dirs.put(p, ents)
	return f.mergeLocal(p, ents), nil
}

// prefetchAttrs issues parallel HEADs so `ls -l` shows real modes, ownership
// and symlinks instead of the mount defaults.
func (f *FS) prefetchAttrs(ctx context.Context, dir string, raw []s3io.ListEntry) {
	type job struct {
		path  string
		key   string
		isDir bool
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	workers := f.cfg.AttrWorkers
	if workers > len(raw) {
		workers = len(raw)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				obj, err := f.s3.Head(ctx, j.key)
				if err != nil {
					// A markerless directory keeps the mount defaults, and a
					// file that cannot be HEADed keeps what the listing gave.
					continue
				}
				f.attrs.put(j.path, f.attrFromObject(obj, j.isDir))
			}
		}()
	}
	for _, e := range raw {
		child := join(dir, e.Name)
		if _, _, ok := f.attrs.get(child); ok {
			continue
		}
		key := f.s3.Key(child)
		if e.IsDir {
			key += "/"
		}
		jobs <- job{path: child, key: key, isDir: e.IsDir}
	}
	close(jobs)
	wg.Wait()
}

// mergeLocal adds not-yet-uploaded entries and refreshes sizes from the cache.
func (f *FS) mergeLocal(dir string, ents []fuse.DirEntry) []fuse.DirEntry {
	extra := f.pendingIn(dir)
	if len(extra) == 0 {
		return ents
	}
	seen := make(map[string]bool, len(ents))
	for _, e := range ents {
		seen[e.Name] = true
	}
	// Copy: ents belongs to the listing cache, and appending to it would write
	// into a backing array another reader may be using.
	out := make([]fuse.DirEntry, len(ents), len(ents)+len(extra))
	copy(out, ents)
	for _, name := range extra {
		if seen[name] {
			continue
		}
		mode := uint32(syscall.S_IFREG)
		if a, negative, ok := f.attrs.get(join(dir, name)); ok && !negative {
			mode = a.Mode & syscall.S_IFMT
		}
		out = append(out, fuse.DirEntry{Name: name, Mode: mode})
	}
	return out
}

// invalidateDir drops cached listings for a directory.
func (f *FS) invalidateDir(p string) { f.dirs.invalidate(strings.TrimPrefix(p, "/")) }

// ------------------------------------------------------------------ helpers

func join(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return dir + "/" + name
}

func parentOf(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return ""
	}
	return p[:i]
}

// toErrno maps S3 and local errors onto errno values the kernel understands.
func (f *FS) toErrno(op string, err error) syscall.Errno {
	if err == nil {
		return 0
	}
	if errors.Is(err, s3io.ErrNotFound) {
		return syscall.ENOENT
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	if errors.Is(err, context.Canceled) {
		return syscall.EINTR
	}
	if errors.Is(err, context.DeadlineExceeded) {
		f.log("%s: timed out: %v", op, err)
		return syscall.ETIMEDOUT
	}
	f.log("%s: %v", op, err)
	return syscall.EIO
}

func (f *FS) readOnly() syscall.Errno {
	if f.cfg.ReadOnly {
		return syscall.EROFS
	}
	return 0
}

// Status is the JSON payload served by the control socket.
type Status struct {
	Bucket     string      `json:"bucket"`
	Prefix     string      `json:"prefix"`
	Mountpoint string      `json:"mountpoint"`
	CacheDir   string      `json:"cache_dir"`
	Uptime     string      `json:"uptime"`
	ReadOnly   bool        `json:"read_only"`
	Exclusive  bool        `json:"exclusive"`
	Ops        int64       `json:"fuse_ops"`
	AttrCached int         `json:"attr_cached"`
	Cache      cache.Stats `json:"cache"`
	S3         S3Stats     `json:"s3"`
	DirtyFiles []string    `json:"dirty_files,omitempty"`
}

// S3Stats counts requests issued since mount.
type S3Stats struct {
	Heads     int64 `json:"heads"`
	Lists     int64 `json:"lists"`
	Gets      int64 `json:"gets"`
	Puts      int64 `json:"puts"`
	Copies    int64 `json:"copies"`
	Deletes   int64 `json:"deletes"`
	Errors    int64 `json:"errors"`
	BytesDown int64 `json:"bytes_down"`
	BytesUp   int64 `json:"bytes_up"`
}

// Status snapshots the mount for `s3disk status`.
func (f *FS) Status() Status {
	st := &f.s3.Stats
	return Status{
		Bucket:     f.cfg.Bucket,
		Prefix:     f.cfg.Prefix,
		Mountpoint: f.cfg.Mountpoint,
		CacheDir:   f.cfg.CacheDir,
		Uptime:     time.Since(f.started).Round(time.Second).String(),
		ReadOnly:   f.cfg.ReadOnly,
		Exclusive:  f.cfg.Exclusive,
		Ops:        f.ops.Load(),
		AttrCached: f.attrs.len(),
		Cache:      f.cache.Stats(),
		S3: S3Stats{
			Heads: st.Heads.Load(), Lists: st.Lists.Load(), Gets: st.Gets.Load(),
			Puts: st.Puts.Load(), Copies: st.Copies.Load(), Deletes: st.Deletes.Load(),
			Errors: st.Errors.Load(), BytesDown: st.BytesDown.Load(), BytesUp: st.BytesUp.Load(),
		},
		DirtyFiles: f.cache.DirtyKeys(),
	}
}

// Refresh forgets everything this mount learned from S3 and tells the kernel to
// drop what it cached, so changes made to the bucket by anything else become
// visible immediately. Unsaved local writes are kept.
//
// An exclusive mount never expires metadata on its own, so this is the escape
// hatch for the occasional out-of-band change (someone seeding or editing the
// bucket directly).
func (f *FS) Refresh() {
	f.attrs.refresh()
	f.dirs.clear()
	f.invalidateKernel()
}

// invalidateKernel drops the kernel's dentry and page caches for everything it
// currently holds for this mount.
func (f *FS) invalidateKernel() {
	if f.server == nil {
		return
	}
	type item struct {
		parent *fs.Inode
		name   string
	}
	var items []item
	var walk func(n *fs.Inode, depth int)
	walk = func(n *fs.Inode, depth int) {
		if depth > 128 {
			return
		}
		for name, child := range n.Children() {
			items = append(items, item{n, name})
			if child.IsDir() {
				walk(child, depth+1)
			}
		}
	}
	walk(f.root.EmbeddedInode(), 0)

	// Notify outside the walk: these calls re-enter the filesystem.
	for _, it := range items {
		_ = it.parent.NotifyEntry(it.name)
	}
}

// SetServer records the running FUSE server, needed for cache invalidation.
func (f *FS) SetServer(s *fuse.Server) { f.server = s }

// Sync flushes every dirty file to S3.
func (f *FS) Sync(ctx context.Context) error { return f.cache.FlushAll(ctx) }

// Close flushes and shuts the cache down.
func (f *FS) Close(ctx context.Context) error { return f.cache.Close(ctx) }
