// Package config holds the mount-time configuration for an s3disk filesystem.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// AttrMode controls how much POSIX metadata s3disk recovers from S3.
const (
	// AttrFull issues a HEAD per object so mode bits, ownership and symlinks
	// survive a round trip through S3. Interoperable with s3fs-fuse.
	AttrFull = "full"
	// AttrFast derives attributes from LIST results only. No per-object HEAD,
	// so listing huge trees is fast, but stored mode bits and symlinks that
	// this process did not create are not visible.
	AttrFast = "fast"
)

// Config is the full set of knobs for one mount.
type Config struct {
	// Target
	Bucket     string
	Prefix     string // normalised: "" or "some/prefix/"
	Mountpoint string

	// S3 endpoint / auth
	Endpoint       string
	Region         string
	Profile        string
	PathStyle      bool
	AccessKey      string
	SecretKey      string
	SessionToken   string
	Checksums      bool // send CRC checksums (off by default: many S3 clones reject them)
	StorageClass   string
	SSE            string
	KMSKeyID       string
	ACL            string
	MaxRetries     int
	RequestTimeout time.Duration

	// Local cache
	CacheDir     string
	CacheSize    int64
	BlockSize    int64
	Readahead    int64
	PersistCache bool

	// Ownership / permissions presented to the kernel
	UID      uint32
	GID      uint32
	FileMode uint32
	DirMode  uint32

	// Caching TTLs
	StatTTL     time.Duration
	ListTTL     time.Duration
	NegativeTTL time.Duration
	KernelTTL   time.Duration

	// Metadata behaviour
	AttrMode     string
	AttrWorkers  int
	SyncMetadata bool // push chmod/chown/utimens back to S3 via metadata rewrite

	// Exclusive marks this mount as the only writer for its bucket/prefix,
	// which is the normal case when a container owns its own bucket or folder.
	// Cached metadata then never goes stale on its own: only this mount can
	// change anything, and it updates its caches as it does. Metadata TTLs stop
	// applying, and a fully listed directory can answer "no such file" locally.
	Exclusive bool

	// Write-back
	AsyncWriteback     bool
	UploadWorkers      int
	DirtyTimeout       time.Duration
	MultipartThreshold int64
	PartSize           int64
	UploadConcurrency  int

	// Mount behaviour
	ReadOnly   bool
	AllowOther bool
	Foreground bool
	Debug      bool
	FuseDebug  bool
	LogFile    string
	ListLimit  int32
}

// Default returns a configuration tuned for interactive/agent workloads:
// a project tree that is read constantly and written in small bursts.
func Default() *Config {
	return &Config{
		Region:             "us-east-1",
		MaxRetries:         5,
		RequestTimeout:     60 * time.Second,
		CacheSize:          8 << 30, // 8 GiB
		BlockSize:          4 << 20, // 4 MiB
		Readahead:          16 << 20,
		PersistCache:       true,
		FileMode:           0644,
		DirMode:            0755,
		StatTTL:            5 * time.Second,
		ListTTL:            5 * time.Second,
		NegativeTTL:        5 * time.Second,
		KernelTTL:          time.Second,
		AttrMode:           AttrFull,
		AttrWorkers:        32,
		SyncMetadata:       true,
		DirtyTimeout:       3 * time.Second,
		UploadWorkers:      16,
		MultipartThreshold: 16 << 20,
		PartSize:           16 << 20,
		UploadConcurrency:  4,
		ListLimit:          1000,
		UID:                uint32(os.Getuid()),
		GID:                uint32(os.Getgid()),
	}
}

// Validate checks the configuration and normalises derived defaults.
func (c *Config) Validate() error {
	if c.Bucket == "" {
		return fmt.Errorf("no bucket specified")
	}
	if c.Mountpoint == "" {
		return fmt.Errorf("no mountpoint specified")
	}
	switch c.AttrMode {
	case AttrFull, AttrFast:
	default:
		return fmt.Errorf("invalid --attr-mode %q (want %q or %q)", c.AttrMode, AttrFull, AttrFast)
	}
	if c.BlockSize < 64<<10 {
		return fmt.Errorf("--block-size must be at least 64K")
	}
	if c.PartSize < 5<<20 {
		return fmt.Errorf("--part-size must be at least 5M (S3 minimum part size)")
	}
	if c.AttrWorkers < 1 {
		c.AttrWorkers = 1
	}
	if c.UploadConcurrency < 1 {
		c.UploadConcurrency = 1
	}
	if c.UploadWorkers < 1 {
		c.UploadWorkers = 1
	}
	if c.CacheDir == "" {
		c.CacheDir = DefaultCacheDir(c.Bucket, c.Prefix)
	}
	return nil
}

// Identity names the exact bucket and prefix a cache belongs to, so a cache
// directory reused for a different mount is discarded instead of mixing keys
// from two buckets.
func (c *Config) Identity() string { return c.Bucket + "/" + c.Prefix }

// DefaultCacheDir derives a per-bucket cache location under XDG_CACHE_HOME.
func DefaultCacheDir(bucket, prefix string) string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			base = home + "/.cache"
		} else {
			base = "/var/cache"
		}
	}
	name := bucket
	if prefix != "" {
		name += "_" + strings.ReplaceAll(strings.TrimSuffix(prefix, "/"), "/", "_")
	}
	return base + "/s3disk/" + name
}

// MaxReadAhead returns the kernel readahead window, capped to what FUSE
// accepts. Zero leaves the kernel default in place.
func (c *Config) MaxReadAhead() int {
	const maxKernelReadahead = 1 << 20
	if c.Readahead <= 0 {
		return 0
	}
	if c.Readahead > maxKernelReadahead {
		return maxKernelReadahead
	}
	return int(c.Readahead)
}

// Describe renders the source in "bucket/prefix" form for logs and /proc/mounts.
func (c *Config) Describe() string {
	if c.Prefix == "" {
		return c.Bucket
	}
	return c.Bucket + "/" + strings.TrimSuffix(c.Prefix, "/")
}
