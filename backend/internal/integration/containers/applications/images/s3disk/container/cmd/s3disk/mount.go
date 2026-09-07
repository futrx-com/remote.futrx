package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"s3disk/internal/config"
	"s3disk/internal/ctl"
	"s3disk/internal/s3fs"
)

// readyFD is the pipe the daemonised child uses to tell its parent that the
// mount is up (or why it is not).
const readyFD = 3

type mountFlags struct {
	fs  *flag.FlagSet
	cfg *config.Config

	cacheSize   string
	blockSize   string
	readahead   string
	fileMode    string
	dirMode     string
	uid         string
	gid         string
	partSize    string
	multipart   string
	optionList  string
	noDaemonize bool
}

func newMountFlags() *mountFlags {
	cfg := config.Default()
	m := &mountFlags{fs: flag.NewFlagSet("mount", flag.ContinueOnError), cfg: cfg}
	f := m.fs

	f.StringVar(&cfg.Endpoint, "endpoint", envOr("S3DISK_ENDPOINT", envOr("AWS_ENDPOINT_URL", "")),
		"S3 endpoint URL (for MinIO, Ceph, R2, Wasabi, …)")
	f.StringVar(&cfg.Region, "region", envOr("AWS_REGION", envOr("AWS_DEFAULT_REGION", cfg.Region)), "S3 region")
	f.StringVar(&cfg.Profile, "profile", os.Getenv("AWS_PROFILE"), "shared-credentials profile to use")
	f.BoolVar(&cfg.PathStyle, "path-style", envBool("S3DISK_PATH_STYLE", false), "use path-style addressing (required by most S3-compatible servers)")
	f.BoolVar(&cfg.Checksums, "checksums", false, "send CRC checksums with uploads (some S3-compatible servers reject them)")
	f.StringVar(&cfg.StorageClass, "storage-class", "", "storage class for new objects (STANDARD, STANDARD_IA, …)")
	f.StringVar(&cfg.SSE, "sse", "", "server-side encryption (AES256 or aws:kms)")
	f.StringVar(&cfg.KMSKeyID, "kms-key-id", "", "KMS key id when --sse=aws:kms")
	f.StringVar(&cfg.ACL, "acl", "", "canned ACL for new objects")
	f.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "retry attempts per S3 request")
	f.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "timeout for a single S3 request")

	f.StringVar(&cfg.CacheDir, "cache-dir", os.Getenv("S3DISK_CACHE_DIR"), "local cache directory (default: ~/.cache/s3disk/BUCKET)")
	f.StringVar(&m.cacheSize, "cache-size", "8G", "maximum disk used by cached file data")
	f.StringVar(&m.blockSize, "block-size", "4M", "granularity of ranged reads")
	f.StringVar(&m.readahead, "readahead", "16M", "how far ahead to prefetch for sequential readers")
	f.BoolVar(&cfg.PersistCache, "persist-cache", true, "keep the cache across mounts and recover unsaved writes")

	f.StringVar(&m.uid, "uid", "", "owner of files without stored ownership (default: current user)")
	f.StringVar(&m.gid, "gid", "", "group of files without stored ownership (default: current group)")
	f.StringVar(&m.fileMode, "file-mode", "0644", "mode for files without stored permissions")
	f.StringVar(&m.dirMode, "dir-mode", "0755", "mode for directories without stored permissions")

	f.DurationVar(&cfg.StatTTL, "stat-ttl", cfg.StatTTL, "how long attributes are cached")
	f.DurationVar(&cfg.ListTTL, "list-ttl", cfg.ListTTL, "how long directory listings are cached")
	f.DurationVar(&cfg.NegativeTTL, "negative-ttl", cfg.NegativeTTL, "how long 'file does not exist' is cached")
	f.DurationVar(&cfg.KernelTTL, "kernel-ttl", cfg.KernelTTL, "how long the kernel may cache entries and attributes")
	f.StringVar(&cfg.AttrMode, "attr-mode", cfg.AttrMode,
		"'full' reads POSIX modes and symlinks from object metadata; 'fast' skips the per-object HEAD")
	f.IntVar(&cfg.AttrWorkers, "attr-workers", cfg.AttrWorkers, "parallel HEAD requests used to fill in a listing")
	f.BoolVar(&cfg.SyncMetadata, "sync-metadata", true, "write chmod/chown/utimens back to S3")
	f.BoolVar(&cfg.Exclusive, "exclusive", envBool("S3DISK_EXCLUSIVE", false),
		"this mount is the only writer for the bucket/prefix: cached metadata never expires, "+
			"and a listed directory answers 'no such file' without asking S3")

	f.DurationVar(&cfg.DirtyTimeout, "dirty-timeout", cfg.DirtyTimeout, "upload a file that has been dirty this long even if still open")
	f.StringVar(&m.multipart, "multipart-threshold", "16M", "object size above which multipart upload is used")
	f.StringVar(&m.partSize, "part-size", "16M", "multipart upload part size")
	f.IntVar(&cfg.UploadConcurrency, "upload-concurrency", cfg.UploadConcurrency, "parallel part uploads for one object")
	f.IntVar(&cfg.UploadWorkers, "upload-workers", cfg.UploadWorkers, "objects uploaded at once during write-back")
	f.BoolVar(&cfg.AsyncWriteback, "async-writeback", false,
		"let close() return before the upload finishes; the file is stored within --dirty-timeout, "+
			"and recovered from the local cache if the mount dies first")

	f.BoolVar(&cfg.ReadOnly, "read-only", false, "mount read-only")
	f.BoolVar(&cfg.AllowOther, "allow-other", envBool("S3DISK_ALLOW_OTHER", false), "let other users access the mount")
	f.BoolVar(&cfg.Foreground, "foreground", false, "stay in the foreground instead of daemonising")
	f.BoolVar(&cfg.Debug, "debug", envBool("S3DISK_DEBUG", false), "log filesystem activity")
	f.BoolVar(&cfg.FuseDebug, "debug-fuse", false, "log raw FUSE traffic (very verbose)")
	f.StringVar(&cfg.LogFile, "log-file", os.Getenv("S3DISK_LOG_FILE"), "write logs here (default: stderr, or CACHE_DIR/s3disk.log when daemonised)")
	f.StringVar(&m.optionList, "o", "", "comma-separated options, for mount(8) and fstab compatibility")

	f.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: s3disk mount s3://BUCKET[/PREFIX] MOUNTPOINT [options]\n\nOptions:\n")
		f.PrintDefaults()
	}
	return m
}

// explicit reports whether the user actually passed a flag, as opposed to it
// carrying its default.
func (m *mountFlags) explicit(name string) bool {
	found := false
	m.fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// finish resolves the human-friendly flag values into the config.
func (m *mountFlags) finish() error {
	cfg := m.cfg
	var err error
	if cfg.CacheSize, err = config.ParseSize(m.cacheSize); err != nil {
		return err
	}
	if cfg.BlockSize, err = config.ParseSize(m.blockSize); err != nil {
		return err
	}
	if cfg.Readahead, err = config.ParseSize(m.readahead); err != nil {
		return err
	}
	if cfg.PartSize, err = config.ParseSize(m.partSize); err != nil {
		return err
	}
	if cfg.MultipartThreshold, err = config.ParseSize(m.multipart); err != nil {
		return err
	}
	if cfg.FileMode, err = config.ParseMode(m.fileMode); err != nil {
		return err
	}
	if cfg.DirMode, err = config.ParseMode(m.dirMode); err != nil {
		return err
	}
	if m.uid != "" {
		if cfg.UID, err = config.ParseOwner(m.uid, false); err != nil {
			return err
		}
	}
	if m.gid != "" {
		if cfg.GID, err = config.ParseOwner(m.gid, true); err != nil {
			return err
		}
	}
	// An exclusive mount owns its data, so the kernel may hold on to entries and
	// attributes much longer than the few seconds that are safe when another
	// client could be writing.
	if cfg.Exclusive && !m.explicit("kernel-ttl") {
		cfg.KernelTTL = time.Minute
	}

	cfg.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	cfg.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	cfg.SessionToken = os.Getenv("AWS_SESSION_TOKEN")
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		// Fall back to the SDK's own chain (profile, IMDS, web identity).
		cfg.AccessKey, cfg.SecretKey, cfg.SessionToken = "", "", ""
	}
	return nil
}

// applyOptions maps `-o key=value,flag` onto the same flags, so every option
// is usable from fstab and from `mount -t fuse.s3disk`.
func (m *mountFlags) applyOptions(list string) error {
	if list == "" {
		return nil
	}
	for _, opt := range splitOptions(list) {
		if opt == "" {
			continue
		}
		name, value, hasValue := strings.Cut(opt, "=")
		name = strings.TrimSpace(name)
		switch name {
		// Options the kernel/mount(8) passes that we accept and ignore.
		case "rw", "auto", "noauto", "user", "users", "nouser", "_netdev", "defaults",
			"nofail", "exec", "noexec", "suid", "nosuid", "dev", "nodev", "atime", "noatime":
			continue
		case "ro":
			name, value, hasValue = "read-only", "true", true
		}
		if m.fs.Lookup(name) == nil {
			return fmt.Errorf("unknown option %q in -o", name)
		}
		if !hasValue {
			value = "true"
		}
		if err := m.fs.Set(name, value); err != nil {
			return fmt.Errorf("option %s: %w", name, err)
		}
	}
	return nil
}

// splitOptions splits on commas that are not inside a quoted value.
func splitOptions(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote != 0 && c == inQuote:
			inQuote = 0
		case inQuote == 0 && (c == '\'' || c == '"'):
			inQuote = c
		case inQuote == 0 && c == ',':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	out = append(out, cur.String())
	return out
}

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

// daemonize re-executes this binary in the background and waits for the child
// to report that the mount is ready, so the shell prompt returns only when the
// filesystem is actually usable.
func daemonize(cfg *config.Config, args []string) int {
	exe, err := os.Executable()
	if err != nil {
		return fatalf("%v", err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return fatalf("%v", err)
	}
	cmd := exec.Command(exe, append([]string{"mount"}, args...)...)
	cmd.Env = append(os.Environ(), "S3DISK_DAEMON=1")
	cmd.Stdin = nil
	cmd.Stdout = nil
	// The daemon must not hold the caller's stderr open: it outlives this
	// process, and anything capturing our output (a command substitution, a
	// pipeline) would block forever waiting for that descriptor to close.
	// Point it at the log file instead, so panics and stray output survive.
	if logFile := openDaemonLog(cfg); logFile != nil {
		cmd.Stderr = logFile
		defer logFile.Close()
	}
	cmd.ExtraFiles = []*os.File{w}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fatalf("%v", err)
	}
	w.Close()

	// The child writes either "READY" or the reason it could not start.
	msg, _ := io.ReadAll(r)
	text := strings.TrimSpace(string(msg))
	if text == readyMessage {
		return 0
	}
	if text != "" {
		fmt.Fprintln(os.Stderr, text)
	} else {
		fmt.Fprintf(os.Stderr, "s3disk: the mount process exited during startup; see %s\n", daemonLogPath(cfg))
	}
	_ = cmd.Wait()
	return 1
}

func daemonLogPath(cfg *config.Config) string {
	if cfg.LogFile != "" {
		return cfg.LogFile
	}
	return filepath.Join(cfg.CacheDir, "s3disk.log")
}

func openDaemonLog(cfg *config.Config) *os.File {
	path := daemonLogPath(cfg)
	if path == "-" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil
	}
	return f
}

// readyMessage is the token a daemonised child sends once the mount works.
const readyMessage = "READY"

// reportReady tells the parent process that the filesystem is usable.
func reportReady() {
	if os.Getenv("S3DISK_DAEMON") == "" {
		return
	}
	if pipe := os.NewFile(readyFD, "ready"); pipe != nil {
		fmt.Fprint(pipe, readyMessage)
		pipe.Close()
	}
}

// startupError reports a failure that happened before the mount came up. When
// daemonised the message travels back over the ready pipe, so the user sees the
// real reason on their terminal rather than only in a log file.
func startupError(err error) int {
	msg := fmt.Sprintf("s3disk: %v", err)
	if os.Getenv("S3DISK_DAEMON") != "" {
		if pipe := os.NewFile(readyFD, "ready"); pipe != nil {
			fmt.Fprint(pipe, msg)
			pipe.Close()
		}
	}
	fmt.Fprintln(os.Stderr, msg)
	return 1
}

func setupLogging(cfg *config.Config) (*log.Logger, func(), error) {
	out := io.Writer(os.Stderr)
	closer := func() {}
	path := cfg.LogFile
	if path == "" && os.Getenv("S3DISK_DAEMON") != "" {
		if err := os.MkdirAll(cfg.CacheDir, 0700); err == nil {
			path = filepath.Join(cfg.CacheDir, "s3disk.log")
		}
	}
	if path != "" && path != "-" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, nil, fmt.Errorf("opening log file %s: %w", path, err)
		}
		out = f
		closer = func() { f.Close() }
	}
	return log.New(out, "s3disk: ", log.LstdFlags|log.Lmsgprefix), closer, nil
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}
