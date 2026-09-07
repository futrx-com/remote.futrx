package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"s3disk/internal/config"
	"s3disk/internal/ctl"
	"s3disk/internal/s3fs"
)

func runMount(args []string) int {
	m := newMountFlags()
	if err := m.fs.Parse(permute(m.fs, args)); err != nil {
		return 2
	}
	rest := m.fs.Args()
	if len(rest) < 2 {
		m.fs.Usage()
		return 2
	}
	bucket, prefix, err := config.ParseS3URL(rest[0])
	if err != nil {
		return fatalf("%v", err)
	}
	mountpoint, err := filepath.Abs(rest[1])
	if err != nil {
		return fatalf("%v", err)
	}
	m.cfg.Bucket, m.cfg.Prefix, m.cfg.Mountpoint = bucket, prefix, mountpoint

	if err := m.applyOptions(m.optionList); err != nil {
		return fatalf("%v", err)
	}
	if err := m.finish(); err != nil {
		return fatalf("%v", err)
	}
	if err := m.cfg.Validate(); err != nil {
		return fatalf("%v", err)
	}
	return mountAndServe(m.cfg, args)
}

// runMountHelper implements the mount(8) helper calling convention.
func runMountHelper(args []string) int {
	// mount.s3disk SPEC DIR [-sfnv] [-o options]
	var spec, dir, opts string
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 < len(args) {
				i++
				opts = args[i]
			}
		case "-s", "-f", "-n", "-v":
			// mount(8) housekeeping flags; nothing to do.
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mount.s3disk BUCKET[:PREFIX] MOUNTPOINT [-o options]")
		return 2
	}
	spec, dir = positional[0], positional[1]
	forwarded := []string{spec, dir}
	if opts != "" {
		forwarded = append(forwarded, "-o", opts)
	}
	return runMount(forwarded)
}

// mountAndServe daemonises if asked, then mounts and serves until unmounted.
func mountAndServe(cfg *config.Config, origArgs []string) int {
	if !cfg.Foreground && os.Getenv("S3DISK_DAEMON") == "" {
		return daemonize(cfg, origArgs)
	}

	logger, closeLog, err := setupLogging(cfg)
	if err != nil {
		return startupError(err)
	}
	defer closeLog()
	logf := func(format string, args ...any) { logger.Printf(format, args...) }
	debugf := logf
	if !cfg.Debug {
		debugf = func(string, ...any) {}
	}

	if err := checkMountpoint(cfg.Mountpoint); err != nil {
		return startupError(err)
	}

	ctx := context.Background()
	fsys, err := s3fs.New(ctx, cfg, debugf)
	if err != nil {
		return startupError(err)
	}

	// Recovery uploads data left over from a previous mount, so it must not run
	// on a read-only mount: that would write to a bucket the user asked us not
	// to touch.
	if cfg.ReadOnly {
		if pending := fsys.Cache().DirtyKeys(); len(pending) > 0 {
			logf("warning: %d unsaved file(s) from a previous mount are being left alone "+
				"because this mount is read-only; remount read-write to recover them", len(pending))
		}
	} else if n, err := fsys.Recover(ctx); err != nil {
		logf("warning: could not recover unsaved writes: %v", err)
	} else if n > 0 {
		logf("recovered %d unsaved file(s) from the previous mount", n)
	}

	server, err := s3fs.Mount(fsys)
	if err != nil {
		_ = fsys.Close(ctx)
		return startupError(err)
	}

	sockPath := ctl.SocketPath(cfg.CacheDir)
	control, err := ctl.Serve(sockPath, ctl.Handler{
		Status:  func() any { return fsys.Status() },
		Sync:    func(ctx context.Context) error { return fsys.Sync(ctx) },
		Refresh: fsys.Refresh,
		Umount:  func() error { return server.Unmount() },
	})
	if err != nil {
		logf("warning: control socket unavailable: %v", err)
	}
	_ = ctl.Register(ctl.Entry{
		Mountpoint: cfg.Mountpoint, Bucket: cfg.Bucket, Prefix: cfg.Prefix,
		CacheDir: cfg.CacheDir, Socket: sockPath, PID: os.Getpid(), Started: time.Now(),
	})

	logf("mounted s3://%s on %s (cache %s)", cfg.Describe(), cfg.Mountpoint, cfg.CacheDir)
	reportReady()

	// Unmount cleanly on SIGINT/SIGTERM so buffered writes are not lost.
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		logf("received %s, flushing and unmounting", sig)
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		if err := fsys.Sync(flushCtx); err != nil {
			logf("warning: flush before unmount failed: %v", err)
		}
		cancel()
		for i := 0; i < 20; i++ {
			if err := server.Unmount(); err == nil {
				return
			} else if i == 0 {
				logf("mount is busy, retrying unmount")
			}
			time.Sleep(500 * time.Millisecond)
		}
		logf("could not unmount %s; forcing exit", cfg.Mountpoint)
		os.Exit(1)
	}()

	server.Wait()

	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := fsys.Close(shutdown); err != nil {
		logf("warning: %v", err)
	}
	_ = control.Close()
	_ = ctl.Deregister(cfg.Mountpoint)
	logf("unmounted %s", cfg.Mountpoint)
	return 0
}

// checkMountpoint makes the usual mistakes fail with a clear message instead of
// a confusing FUSE error, and clears away the stale mount a killed s3disk
// leaves behind so that remounting just works.
func checkMountpoint(path string) error {
	st, err := os.Stat(path)
	if errors.Is(err, syscall.ENOTCONN) {
		// "transport endpoint is not connected": a FUSE mount whose server died.
		if uerr := unmountPath(path, true); uerr != nil {
			return fmt.Errorf("%s is a stale mount left by a previous s3disk and could not be cleared: %w", path, uerr)
		}
		st, err = os.Stat(path)
	}
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("creating mountpoint %s: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("mountpoint %s: %w", path, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("mountpoint %s is not a directory", path)
	}
	if mounted, err := s3fs.IsMounted(path); err == nil && mounted {
		return fmt.Errorf("%s is already a mountpoint", path)
	}
	return nil
}
