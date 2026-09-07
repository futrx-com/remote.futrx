package s3fs

import (
	"fmt"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// Mount attaches the filesystem to cfg.Mountpoint and returns the running
// FUSE server. The caller is responsible for calling Wait and Unmount.
func Mount(f *FS) (*fuse.Server, error) {
	cfg := f.cfg
	kernelTTL := cfg.KernelTTL
	negTTL := cfg.NegativeTTL
	if negTTL > kernelTTL && kernelTTL > 0 {
		negTTL = kernelTTL
	}
	if cfg.Exclusive {
		// Do not let the kernel cache "no such file". A negative dentry has no
		// inode behind it, so `s3disk refresh` cannot invalidate one, and it
		// would keep answering ENOENT for a path that now exists. Nothing is
		// lost: an exclusive mount already answers those lookups from a cached
		// listing without touching S3, which is where the cost actually is.
		negTTL = 0
	}

	mountOpts := fuse.MountOptions{
		FsName:        "s3disk#" + cfg.Describe(),
		Name:          "s3disk",
		AllowOther:    cfg.AllowOther,
		Debug:         cfg.FuseDebug,
		MaxWrite:      1 << 20,
		MaxReadAhead:  int(cfg.Readahead),
		MaxBackground: 64,
		// S3 stores no extended attributes, and answering every getxattr with a
		// round trip would make tools like ls and cp crawl.
		DisableXAttrs: true,
		// Mount with mount(2) when we have CAP_SYS_ADMIN (the common case in a
		// container) and fall back to the fusermount helper otherwise.
		DirectMount:          true,
		EnableSymlinkCaching: true,
		Options:              []string{},
	}
	if cfg.ReadOnly {
		mountOpts.Options = append(mountOpts.Options, "ro")
	}
	if cfg.MaxReadAhead() > 0 {
		mountOpts.MaxReadAhead = cfg.MaxReadAhead()
	}

	opts := &fs.Options{
		MountOptions:    mountOpts,
		EntryTimeout:    &kernelTTL,
		AttrTimeout:     &kernelTTL,
		NegativeTimeout: &negTTL,
		NullPermissions: true, // permissions come from S3 metadata, not from us
		UID:             cfg.UID,
		GID:             cfg.GID,
	}

	server, err := fs.Mount(cfg.Mountpoint, f.root, opts)
	if err != nil {
		return nil, fmt.Errorf("mounting %s: %w", cfg.Mountpoint, err)
	}
	f.SetServer(server)
	return server, nil
}

// WaitReady blocks until the mount answers a stat, so callers can report a
// mount as usable rather than merely started.
func WaitReady(mountpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if mounted, err := IsMounted(mountpoint); err == nil && mounted {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s to become ready", mountpoint)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
