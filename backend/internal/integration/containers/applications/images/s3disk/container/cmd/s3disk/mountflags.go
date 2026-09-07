package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"s3disk/internal/config"
)

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
	f.StringVar(&m.cacheSize, "cache-size", config.FormatSize(cfg.CacheSize), "maximum disk used by cached file data")
	f.StringVar(&m.blockSize, "block-size", config.FormatSize(cfg.BlockSize), "granularity of ranged reads")
	f.StringVar(&m.readahead, "readahead", config.FormatSize(cfg.Readahead), "how far ahead to prefetch for sequential readers")
	f.BoolVar(&cfg.PersistCache, "persist-cache", cfg.PersistCache, "keep the cache across mounts and recover unsaved writes")

	f.StringVar(&m.uid, "uid", "", "owner of files without stored ownership (default: current user)")
	f.StringVar(&m.gid, "gid", "", "group of files without stored ownership (default: current group)")
	f.StringVar(&m.fileMode, "file-mode", config.FormatMode(cfg.FileMode), "mode for files without stored permissions")
	f.StringVar(&m.dirMode, "dir-mode", config.FormatMode(cfg.DirMode), "mode for directories without stored permissions")

	f.DurationVar(&cfg.StatTTL, "stat-ttl", cfg.StatTTL, "how long attributes are cached")
	f.DurationVar(&cfg.ListTTL, "list-ttl", cfg.ListTTL, "how long directory listings are cached")
	f.DurationVar(&cfg.NegativeTTL, "negative-ttl", cfg.NegativeTTL, "how long 'file does not exist' is cached")
	f.DurationVar(&cfg.KernelTTL, "kernel-ttl", cfg.KernelTTL, "how long the kernel may cache entries and attributes")
	f.StringVar(&cfg.AttrMode, "attr-mode", cfg.AttrMode,
		"'full' reads POSIX modes and symlinks from object metadata; 'fast' skips the per-object HEAD")
	f.IntVar(&cfg.AttrWorkers, "attr-workers", cfg.AttrWorkers, "parallel HEAD requests used to fill in a listing")
	f.BoolVar(&cfg.SyncMetadata, "sync-metadata", cfg.SyncMetadata, "write chmod/chown/utimens back to S3")
	f.BoolVar(&cfg.Exclusive, "exclusive", envBool("S3DISK_EXCLUSIVE", false),
		"this mount is the only writer for the bucket/prefix: cached metadata never expires, "+
			"and a listed directory answers 'no such file' without asking S3")

	f.DurationVar(&cfg.DirtyTimeout, "dirty-timeout", cfg.DirtyTimeout, "upload a file that has been dirty this long even if still open")
	f.StringVar(&m.multipart, "multipart-threshold", config.FormatSize(cfg.MultipartThreshold), "object size above which multipart upload is used")
	f.StringVar(&m.partSize, "part-size", config.FormatSize(cfg.PartSize), "multipart upload part size")
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
