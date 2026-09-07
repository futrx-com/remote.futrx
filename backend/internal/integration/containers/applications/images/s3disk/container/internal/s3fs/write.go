package s3fs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"

	"s3disk/internal/s3io"
)

// What the filesystem does to S3 when the tree changes: keeping a cached
// attribute in step with a write, deciding whether a directory can be removed,
// writing POSIX metadata back, and moving objects for a rename. Node's
// operations decide when these run; this decides what they do.

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
	return len(f.pending.names(p)) == 0, nil
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
	f.pending.remove(parentOf(oldPath), baseOf(oldPath))
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
	f.pending.removePrefix(oldPath)
	f.dirs.remove(parentOf(oldPath), baseOf(oldPath))
	f.dirs.add(parentOf(newPath), fuse.DirEntry{Name: baseOf(newPath), Mode: syscall.S_IFDIR})
	return nil
}
