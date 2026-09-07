package s3fs

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"

	"s3disk/internal/s3io"
)

// renameNoReplace is renameat2(2)'s RENAME_NOREPLACE; go-fuse only exports
// RENAME_EXCHANGE.
const renameNoReplace = 0x1

// Node is one path in the mounted tree. It carries no state of its own: the
// path comes from its position in the inode tree, and everything else lives in
// FS's caches, so renames stay consistent for free.
type Node struct {
	fs.Inode
	fsys *FS
}

// Compile-time assertions for the operations s3disk implements.
var (
	_ fs.NodeLookuper   = (*Node)(nil)
	_ fs.NodeGetattrer  = (*Node)(nil)
	_ fs.NodeSetattrer  = (*Node)(nil)
	_ fs.NodeReaddirer  = (*Node)(nil)
	_ fs.NodeOpener     = (*Node)(nil)
	_ fs.NodeCreater    = (*Node)(nil)
	_ fs.NodeMknoder    = (*Node)(nil)
	_ fs.NodeReader     = (*Node)(nil)
	_ fs.NodeWriter     = (*Node)(nil)
	_ fs.NodeFlusher    = (*Node)(nil)
	_ fs.NodeReleaser   = (*Node)(nil)
	_ fs.NodeFsyncer    = (*Node)(nil)
	_ fs.NodeUnlinker   = (*Node)(nil)
	_ fs.NodeRmdirer    = (*Node)(nil)
	_ fs.NodeMkdirer    = (*Node)(nil)
	_ fs.NodeRenamer    = (*Node)(nil)
	_ fs.NodeSymlinker  = (*Node)(nil)
	_ fs.NodeReadlinker = (*Node)(nil)
	_ fs.NodeStatfser   = (*Node)(nil)
)

func (n *Node) path() string { return n.Path(nil) }

func (n *Node) count() { n.fsys.ops.Add(1) }

// childInode reuses the inode already attached to this name so that repeated
// lookups of a path keep returning the same st_ino.
func (n *Node) childInode(ctx context.Context, name string, a *Attr) *fs.Inode {
	typ := a.Mode & syscall.S_IFMT
	if ch := n.GetChild(name); ch != nil && ch.Mode() == typ {
		return ch
	}
	return n.NewInode(ctx, &Node{fsys: n.fsys}, fs.StableAttr{Mode: typ})
}

// callerOwner returns the uid/gid of the process making the request, so files
// created inside the mount are owned by whoever created them.
func (n *Node) callerOwner(ctx context.Context) (uint32, uint32) {
	if caller, ok := fuse.FromContext(ctx); ok {
		return caller.Uid, caller.Gid
	}
	return n.fsys.cfg.UID, n.fsys.cfg.GID
}

// Lookup resolves one path component.
func (n *Node) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.count()
	p := join(n.path(), name)
	a, err := n.fsys.stat(ctx, p)
	if err != nil {
		return nil, n.fsys.toErrno("lookup "+p, err)
	}
	a.Fill(&out.Attr)
	return n.childInode(ctx, name, a), 0
}

// Getattr stats an open handle, or the node's path when there is none.
func (n *Node) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	n.count()
	if h, ok := fh.(*fileHandle); ok && h != nil {
		a, err := n.fsys.stat(ctx, h.path)
		if err != nil {
			return n.fsys.toErrno("getattr "+h.path, err)
		}
		// The open handle knows the authoritative length and mtime.
		local := *a
		local.Size = h.entry.Size()
		if mt := h.entry.Mtime(); !mt.IsZero() {
			local.Mtime = mt
		}
		local.Fill(&out.Attr)
		return 0
	}
	p := n.path()
	a, err := n.fsys.stat(ctx, p)
	if err != nil {
		return n.fsys.toErrno("getattr "+p, err)
	}
	a.Fill(&out.Attr)
	return 0
}

// Setattr implements chmod, chown, truncate and utimens.
func (n *Node) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return e
	}
	p := n.path()
	if h, ok := fh.(*fileHandle); ok && h != nil {
		p = h.path
	}
	a, err := n.fsys.stat(ctx, p)
	if err != nil {
		return n.fsys.toErrno("setattr "+p, err)
	}
	next := *a
	touched := false

	if sz, ok := in.GetSize(); ok && !a.IsDir() {
		// The entry must be referenced for the duration: an unreferenced clean
		// entry can be evicted at any moment, which would close the file under
		// us. Opening through the cache takes that reference atomically.
		entry, openedHere := (*cacheEntry)(nil), false
		if h, ok := fh.(*fileHandle); ok && h != nil {
			entry = h.entry
		} else {
			opened, oerr := n.fsys.cache.Open(n.fsys.s3.Key(p), a.Size, a.ETag, a.Mtime)
			if oerr != nil {
				return n.fsys.toErrno("truncate "+p, oerr)
			}
			defer opened.Unref()
			entry, openedHere = opened, true
		}
		if err := entry.Truncate(ctx, int64(sz)); err != nil {
			return n.fsys.toErrno("truncate "+p, err)
		}
		next.Size = int64(sz)
		next.Mtime = time.Now()
		touched = true
		n.fsys.attrs.putSticky(p, &next)
		// A truncate of a file nobody has open still has to reach S3; when a
		// handle is involved, its close will carry the change.
		if openedHere {
			if err := entry.Flush(ctx); err != nil {
				return n.fsys.toErrno("truncate flush "+p, err)
			}
		}
	}
	if m, ok := in.GetMode(); ok {
		next.Mode = (next.Mode & syscall.S_IFMT) | (m & 07777)
		touched = true
	}
	if uid, ok := in.GetUID(); ok {
		next.Uid = uid
		touched = true
	}
	if gid, ok := in.GetGID(); ok {
		next.Gid = gid
		touched = true
	}
	if mt, ok := in.GetMTime(); ok {
		next.Mtime = mt
		touched = true
	}
	if at, ok := in.GetATime(); ok {
		next.Atime = at
		touched = true
	}
	if !touched {
		a.Fill(&out.Attr)
		return 0
	}
	next.Ctime = time.Now()
	n.fsys.attrs.putSticky(p, &next)

	if err := n.fsys.persistMetadata(ctx, p, &next); err != nil {
		return n.fsys.toErrno("setattr persist "+p, err)
	}
	next.Fill(&out.Attr)
	return 0
}

// Readdir lists a directory.
func (n *Node) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	n.count()
	p := n.path()
	ents, err := n.fsys.listDir(ctx, p)
	if err != nil {
		return nil, n.fsys.toErrno("readdir "+p, err)
	}
	return fs.NewListDirStream(ents), 0
}

// Open opens an existing file.
func (n *Node) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	n.count()
	p := n.path()
	writing := flags&(syscall.O_WRONLY|syscall.O_RDWR|syscall.O_APPEND|syscall.O_TRUNC) != 0
	if writing {
		if e := n.fsys.readOnly(); e != 0 {
			return nil, 0, e
		}
	}
	a, err := n.fsys.stat(ctx, p)
	if err != nil {
		return nil, 0, n.fsys.toErrno("open "+p, err)
	}
	if a.IsDir() {
		return nil, 0, syscall.EISDIR
	}
	key := n.fsys.s3.Key(p)

	var entry *cacheEntry
	if flags&syscall.O_TRUNC != 0 {
		e, err := n.fsys.cache.Create(key)
		if err != nil {
			return nil, 0, n.fsys.toErrno("open "+p, err)
		}
		entry = e
		trunc := *a
		trunc.Size = 0
		trunc.Mtime = time.Now()
		n.fsys.attrs.putSticky(p, &trunc)
	} else {
		e, err := n.fsys.cache.Open(key, a.Size, a.ETag, a.Mtime)
		if err != nil {
			return nil, 0, n.fsys.toErrno("open "+p, err)
		}
		entry = e
	}
	h := &fileHandle{fsys: n.fsys, entry: entry, path: p, key: key, flags: flags}
	return h, n.fsys.openFlags(), 0
}

// Create makes a new file and returns an open handle to it.
func (n *Node) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return nil, nil, 0, e
	}
	dir := n.path()
	p := join(dir, name)
	if flags&syscall.O_EXCL != 0 {
		if _, err := n.fsys.stat(ctx, p); err == nil {
			return nil, nil, 0, syscall.EEXIST
		} else if !errors.Is(err, syscall.ENOENT) && !errors.Is(err, s3io.ErrNotFound) {
			return nil, nil, 0, n.fsys.toErrno("create "+p, err)
		}
	}
	key := n.fsys.s3.Key(p)
	entry, err := n.fsys.cache.Create(key)
	if err != nil {
		return nil, nil, 0, n.fsys.toErrno("create "+p, err)
	}
	uid, gid := n.callerOwner(ctx)
	now := time.Now()
	a := &Attr{
		Mode: syscall.S_IFREG | (mode & 07777), Size: 0,
		Uid: uid, Gid: gid, Mtime: now, Atime: now, Ctime: now,
	}
	n.fsys.attrs.putSticky(p, a)
	n.fsys.markPending(dir, name)
	n.fsys.dirs.add(dir, fuse.DirEntry{Name: name, Mode: syscall.S_IFREG})

	a.Fill(&out.Attr)
	child := n.childInode(ctx, name, a)
	h := &fileHandle{fsys: n.fsys, entry: entry, path: p, key: key, flags: flags}
	return child, h, n.fsys.openFlags(), 0
}

// Mknod supports tools that create regular files with mknod(2).
func (n *Node) Mknod(ctx context.Context, name string, mode uint32, dev uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	if mode&syscall.S_IFMT != syscall.S_IFREG && mode&syscall.S_IFMT != 0 {
		return nil, syscall.ENOTSUP // no device nodes, fifos or sockets in S3
	}
	child, fh, _, errno := n.Create(ctx, name, syscall.O_CREAT|syscall.O_WRONLY, mode|syscall.S_IFREG, out)
	if errno != 0 {
		return nil, errno
	}
	h := fh.(*fileHandle)
	if err := h.entry.Flush(ctx); err != nil {
		return nil, n.fsys.toErrno("mknod "+name, err)
	}
	h.entry.Unref()
	return child, 0
}

// Read serves a read from the local cache, faulting in missing blocks.
func (n *Node) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	n.count()
	h, ok := fh.(*fileHandle)
	if !ok || h == nil {
		return nil, syscall.EBADF
	}
	got, err := h.entry.ReadAt(ctx, dest, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, n.fsys.toErrno("read "+h.path, err)
	}
	h.noteRead(off, int64(got), n.fsys.cfg.Readahead)
	return fuse.ReadResultData(dest[:got]), 0
}

// Write stores data in the local cache; the object is uploaded on close.
func (n *Node) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return 0, e
	}
	h, ok := fh.(*fileHandle)
	if !ok || h == nil {
		return 0, syscall.EBADF
	}
	if h.flags&syscall.O_APPEND != 0 {
		off = h.entry.Size()
	}
	written, err := h.entry.WriteAt(ctx, data, off)
	if err != nil {
		return uint32(written), n.fsys.toErrno("write "+h.path, err)
	}
	n.fsys.noteWrite(h.path, h.entry.Size())
	return uint32(written), 0
}

// Flush uploads pending changes when a file descriptor is closed.
//
// The kernel also sends a flush right after create, before any data has been
// written; uploading there would store an empty object for every new file and
// double the request count. Such an entry is left for Release, which always
// follows, so an empty file still reaches S3 when it is closed.
func (n *Node) Flush(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	n.count()
	h, ok := fh.(*fileHandle)
	if !ok || h == nil {
		return 0
	}
	if h.entry.Untouched() {
		return 0
	}
	// With asynchronous write-back the entry stays dirty and the background
	// flusher stores it, in parallel with everything else closed around the
	// same time. It is already durable on local disk and recorded in the cache
	// index, so a mount that dies before the upload recovers it on the next
	// start.
	if n.fsys.cache.AsyncWriteback() {
		return 0
	}
	if err := h.entry.Flush(ctx); err != nil {
		return n.fsys.toErrno("flush "+h.path, err)
	}
	return 0
}

// Fsync forces a write-back. Unlike close, this is synchronous even with
// asynchronous write-back configured: a caller asking for fsync is asking for
// durability, not for speed.
func (n *Node) Fsync(ctx context.Context, fh fs.FileHandle, flags uint32) syscall.Errno {
	h, ok := fh.(*fileHandle)
	if !ok || h == nil {
		return 0
	}
	if err := h.entry.Flush(ctx); err != nil {
		return n.fsys.toErrno("fsync "+h.path, err)
	}
	return 0
}

// Release drops the handle's reference on the cached file, uploading anything
// still pending (in particular a file that was created but never written to).
func (n *Node) Release(ctx context.Context, fh fs.FileHandle) syscall.Errno {
	h, ok := fh.(*fileHandle)
	if !ok || h == nil {
		return 0
	}
	var err error
	if !n.fsys.cache.AsyncWriteback() {
		err = h.entry.Flush(ctx)
	}
	h.entry.Unref()
	if err != nil {
		return n.fsys.toErrno("release "+h.path, err)
	}
	return 0
}

// Mkdir creates a directory marker object.
func (n *Node) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return nil, e
	}
	dir := n.path()
	p := join(dir, name)
	if _, err := n.fsys.stat(ctx, p); err == nil {
		return nil, syscall.EEXIST
	}
	uid, gid := n.callerOwner(ctx)
	now := time.Now()
	a := &Attr{
		Mode: syscall.S_IFDIR | (mode & 07777), Size: 4096,
		Uid: uid, Gid: gid, Mtime: now, Atime: now, Ctime: now,
	}
	if _, err := n.fsys.s3.Put(ctx, s3io.PutInput{
		Key: n.fsys.s3.DirKey(p), Size: 0, Meta: metaFor(a),
		ContentType: "application/x-directory",
	}); err != nil {
		return nil, n.fsys.toErrno("mkdir "+p, err)
	}
	n.fsys.attrs.put(p, a)
	n.fsys.dirs.add(dir, fuse.DirEntry{Name: name, Mode: syscall.S_IFDIR})
	n.fsys.dirs.seed(p)
	a.Fill(&out.Attr)
	return n.childInode(ctx, name, a), 0
}

// Rmdir removes an empty directory.
func (n *Node) Rmdir(ctx context.Context, name string) syscall.Errno {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return e
	}
	dir := n.path()
	p := join(dir, name)
	a, err := n.fsys.stat(ctx, p)
	if err != nil {
		return n.fsys.toErrno("rmdir "+p, err)
	}
	if !a.IsDir() {
		return syscall.ENOTDIR
	}
	empty, err := n.fsys.dirEmpty(ctx, p)
	if err != nil {
		return n.fsys.toErrno("rmdir "+p, err)
	}
	if !empty {
		return syscall.ENOTEMPTY
	}
	if err := n.fsys.s3.Delete(ctx, n.fsys.s3.DirKey(p)); err != nil && !errors.Is(err, s3io.ErrNotFound) {
		return n.fsys.toErrno("rmdir "+p, err)
	}
	n.fsys.attrs.invalidatePrefix(p)
	n.fsys.attrs.putNegative(p)
	n.fsys.dirs.invalidatePrefix(p)
	n.fsys.dirs.remove(dir, name)
	return 0
}

// Unlink deletes a file.
func (n *Node) Unlink(ctx context.Context, name string) syscall.Errno {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return e
	}
	dir := n.path()
	p := join(dir, name)
	a, err := n.fsys.stat(ctx, p)
	if err != nil {
		return n.fsys.toErrno("unlink "+p, err)
	}
	if a.IsDir() {
		return syscall.EISDIR
	}
	key := n.fsys.s3.Key(p)
	// Drop local state first so an in-flight background flush cannot resurrect
	// the object after the delete.
	n.fsys.cache.Remove(key)
	if err := n.fsys.s3.Delete(ctx, key); err != nil && !errors.Is(err, s3io.ErrNotFound) {
		return n.fsys.toErrno("unlink "+p, err)
	}
	n.fsys.clearPending(dir, name)
	n.fsys.attrs.putNegative(p)
	n.fsys.dirs.remove(dir, name)
	return 0
}

// Symlink stores the link target as the object body, tagged S_IFLNK.
func (n *Node) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return nil, e
	}
	dir := n.path()
	p := join(dir, name)
	uid, gid := n.callerOwner(ctx)
	now := time.Now()
	a := &Attr{
		Mode: syscall.S_IFLNK | 0777, Size: int64(len(target)),
		Uid: uid, Gid: gid, Mtime: now, Atime: now, Ctime: now, Symlink: target,
	}
	if _, err := n.fsys.s3.Put(ctx, s3io.PutInput{
		Key: n.fsys.s3.Key(p), Body: strings.NewReader(target), Size: int64(len(target)),
		Meta: metaFor(a), ContentType: "application/x-symlink",
	}); err != nil {
		return nil, n.fsys.toErrno("symlink "+p, err)
	}
	n.fsys.attrs.put(p, a)
	n.fsys.dirs.add(dir, fuse.DirEntry{Name: name, Mode: syscall.S_IFLNK})
	a.Fill(&out.Attr)
	return n.childInode(ctx, name, a), 0
}

// Readlink returns a symlink target, fetching it once and caching it.
func (n *Node) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	n.count()
	p := n.path()
	a, err := n.fsys.stat(ctx, p)
	if err != nil {
		return nil, n.fsys.toErrno("readlink "+p, err)
	}
	if !a.IsLink() {
		return nil, syscall.EINVAL
	}
	if a.Symlink != "" {
		return []byte(a.Symlink), 0
	}
	body, err := n.fsys.s3.GetAll(ctx, n.fsys.s3.Key(p))
	if err != nil {
		return nil, n.fsys.toErrno("readlink "+p, err)
	}
	updated := *a
	updated.Symlink = string(body)
	n.fsys.attrs.put(p, &updated)
	return body, 0
}

// Rename moves a file or a whole subtree.
func (n *Node) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	n.count()
	if e := n.fsys.readOnly(); e != 0 {
		return e
	}
	if flags&fs.RENAME_EXCHANGE != 0 {
		return syscall.EINVAL // no atomic swap primitive exists in S3
	}
	np, ok := newParent.(*Node)
	if !ok {
		return syscall.EXDEV
	}
	oldPath := join(n.path(), name)
	newPath := join(np.path(), newName)
	if oldPath == newPath {
		return 0
	}
	if err := n.fsys.rename(ctx, oldPath, newPath, flags&renameNoReplace != 0); err != nil {
		return n.fsys.toErrno("rename "+oldPath, err)
	}
	return 0
}

// Statfs reports a large, mostly-empty filesystem: S3 has no fixed capacity,
// and tools refuse to write when free space looks like zero.
func (n *Node) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
	const (
		blockSize = 4096
		total     = uint64(1) << 48 // 256 TiB of headroom
	)
	used := uint64(n.fsys.cache.Bytes()) / blockSize
	out.Blocks = total / blockSize
	out.Bfree = out.Blocks - used
	out.Bavail = out.Bfree
	out.Files = 1 << 30
	out.Ffree = 1 << 30
	out.Bsize = blockSize
	out.Frsize = blockSize
	out.NameLen = 1024
	return 0
}

// openFlags controls whether the kernel may keep page cache across opens.
func (f *FS) openFlags() uint32 {
	if f.cfg.KernelTTL > 0 {
		return fuse.FOPEN_KEEP_CACHE
	}
	return 0
}

// noteWrite keeps the cached size in step with a file being written.
func (f *FS) noteWrite(p string, size int64) {
	a, negative, ok := f.attrs.get(p)
	if !ok || negative || a == nil {
		return
	}
	next := *a
	next.Size = size
	next.Mtime = time.Now()
	f.attrs.putSticky(p, &next)
}

// dirEmpty reports whether a directory has any children besides its marker.
func (f *FS) dirEmpty(ctx context.Context, p string) (bool, error) {
	dirKey := f.s3.DirKey(p)
	empty := true
	err := f.s3.ListAll(ctx, dirKey, 0, func(key string, size int64) bool {
		if key == dirKey {
			return true
		}
		empty = false
		return false
	})
	if err != nil {
		return false, err
	}
	if !empty {
		return false, nil
	}
	// Files created locally but not uploaded yet still count as children.
	return len(f.pendingIn(p)) == 0, nil
}

// persistMetadata writes POSIX attributes back to S3.
//
// For a file with pending writes this is a no-op: the write-back will carry the
// new metadata. Otherwise the object is copied onto itself with replaced
// metadata, which is how s3fs stores chmod/chown/utimens too.
func (f *FS) persistMetadata(ctx context.Context, p string, a *Attr) error {
	if !f.cfg.SyncMetadata {
		return nil
	}
	key := f.s3.Key(p)
	if a.IsDir() {
		key = f.s3.DirKey(p)
		_, err := f.s3.Put(ctx, s3io.PutInput{
			Key: key, Size: 0, Meta: metaFor(a), ContentType: "application/x-directory",
		})
		return err
	}
	if e := f.cache.Lookup(key); e != nil && e.Dirty() {
		return nil
	}
	err := f.s3.Copy(ctx, key, key, a.Size, metaFor(a), contentTypeFor(p))
	if errors.Is(err, s3io.ErrNotFound) {
		return nil // never uploaded yet; metadata rides along with the first PUT
	}
	return err
}

// rename moves oldPath to newPath, recursing for directories.
func (f *FS) rename(ctx context.Context, oldPath, newPath string, noReplace bool) error {
	a, err := f.stat(ctx, oldPath)
	if err != nil {
		return err
	}
	if noReplace {
		if _, err := f.stat(ctx, newPath); err == nil {
			return syscall.EEXIST
		}
	}
	if a.IsDir() {
		return f.renameDir(ctx, oldPath, newPath)
	}
	return f.renameFile(ctx, oldPath, newPath, a)
}

func (f *FS) renameFile(ctx context.Context, oldPath, newPath string, a *Attr) error {
	oldKey, newKey := f.s3.Key(oldPath), f.s3.Key(newPath)

	// Unwritten data must reach S3 before the server-side copy reads the object.
	if e := f.cache.Lookup(oldKey); e != nil && e.Dirty() {
		if err := e.Flush(ctx); err != nil {
			return err
		}
	}
	if err := f.s3.Copy(ctx, oldKey, newKey, a.Size, nil, ""); err != nil {
		return err
	}
	if err := f.s3.Delete(ctx, oldKey); err != nil && !errors.Is(err, s3io.ErrNotFound) {
		return err
	}
	f.cache.Rename(oldKey, newKey)

	moved := *a
	f.attrs.invalidate(oldPath)
	f.attrs.putNegative(oldPath)
	f.attrs.put(newPath, &moved)
	f.clearPending(parentOf(oldPath), baseOf(oldPath))
	f.dirs.remove(parentOf(oldPath), baseOf(oldPath))
	f.dirs.add(parentOf(newPath), fuse.DirEntry{Name: baseOf(newPath), Mode: moved.Mode & syscall.S_IFMT})
	return nil
}

// renameDir copies every object under the old prefix and deletes the originals.
// S3 has no directory primitive, so the cost is linear in the subtree size.
func (f *FS) renameDir(ctx context.Context, oldPath, newPath string) error {
	oldPrefix, newPrefix := f.s3.DirKey(oldPath), f.s3.DirKey(newPath)

	// Anything dirty underneath has to be uploaded before it can be copied.
	for _, k := range f.cache.DirtyKeys() {
		if strings.HasPrefix(k, oldPrefix) {
			if e := f.cache.Lookup(k); e != nil {
				if err := e.Flush(ctx); err != nil {
					return err
				}
			}
		}
	}

	type item struct {
		key  string
		size int64
	}
	var items []item
	if err := f.s3.ListAll(ctx, oldPrefix, 0, func(key string, size int64) bool {
		items = append(items, item{key, size})
		return true
	}); err != nil {
		return err
	}

	sem := make(chan struct{}, f.cfg.UploadConcurrency*2)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for _, it := range items {
		dst := newPrefix + strings.TrimPrefix(it.key, oldPrefix)
		wg.Add(1)
		sem <- struct{}{}
		go func(src, dst string, size int64) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := f.s3.Copy(ctx, src, dst, size, nil, ""); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}(it.key, dst, it.size)
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
	}

	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, it.key)
	}
	if err := f.s3.DeleteMulti(ctx, keys); err != nil {
		return err
	}
	for _, it := range items {
		f.cache.Rename(it.key, newPrefix+strings.TrimPrefix(it.key, oldPrefix))
	}

	f.attrs.invalidatePrefix(oldPath)
	f.attrs.invalidatePrefix(newPath)
	f.dirs.invalidatePrefix(oldPath)
	f.dirs.invalidatePrefix(newPath)
	f.clearPendingPrefix(oldPath)
	f.dirs.remove(parentOf(oldPath), baseOf(oldPath))
	f.dirs.add(parentOf(newPath), fuse.DirEntry{Name: baseOf(newPath), Mode: syscall.S_IFDIR})
	return nil
}

func baseOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
