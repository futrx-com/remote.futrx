// Package s3io wraps the AWS S3 API with the path-oriented surface the filesystem needs.
package s3io

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"futrx.local/catalog/applications/s3disk/backend/container/internal/config"
)

// Object is the subset of object state the filesystem cares about.
type Object struct {
	Key      string
	Size     int64
	ETag     string
	Modified time.Time
	Meta     map[string]string // user metadata, lower-cased keys
}

// ListEntry is one child of a directory listing.
type ListEntry struct {
	Name     string // basename, no trailing slash
	IsDir    bool
	Size     int64
	ETag     string
	Modified time.Time
}

// Stats counts the S3 traffic a mount has generated.
type Stats struct {
	Heads, Lists, Gets, Puts, Copies, Deletes, Errors atomic.Int64
	BytesDown, BytesUp                                atomic.Int64
}

// Client is a bucket+prefix scoped S3 accessor.
type Client struct {
	api      *s3.Client
	uploader *manager.Uploader
	cfg      *config.Config
	bucket   string
	prefix   string
	Stats    Stats
}

// New builds a client from the ambient AWS credential chain, overridden by any
// explicit endpoint/keys in cfg.
func New(ctx context.Context, cfg *config.Config) (*Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithRetryMaxAttempts(cfg.MaxRetries),
	}
	if cfg.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(cfg.Profile))
	}
	if cfg.AccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken)))
	}
	if !cfg.Checksums {
		// Many S3-compatible servers reject the CRC trailers the v2 SDK adds by
		// default; only send them when the object format requires it.
		opts = append(opts,
			awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
			awsconfig.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
		)
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS configuration: %w", err)
	}
	awsCfg.HTTPClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        256,
			MaxIdleConnsPerHost: 128,
			MaxConnsPerHost:     0,
			IdleConnTimeout:     90 * time.Second,
			ForceAttemptHTTP2:   true,
		},
	}

	api := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})

	c := &Client{api: api, cfg: cfg, bucket: cfg.Bucket, prefix: cfg.Prefix}
	c.uploader = manager.NewUploader(api, func(u *manager.Uploader) {
		u.PartSize = cfg.PartSize
		u.Concurrency = cfg.UploadConcurrency
	})
	return c, nil
}

// Bucket returns the bucket this client is scoped to.
func (c *Client) Bucket() string { return c.bucket }

// Key maps a filesystem path (no leading slash) to a full object key.
func (c *Client) Key(path string) string { return c.prefix + strings.TrimPrefix(path, "/") }

// DirKey maps a directory path to its "prefix/" form.
func (c *Client) DirKey(path string) string {
	k := c.Key(path)
	if k == "" || strings.HasSuffix(k, "/") {
		return k
	}
	return k + "/"
}

// PathOf is the inverse of Key: it strips the mount prefix from an object key.
func (c *Client) PathOf(key string) string { return strings.TrimPrefix(key, c.prefix) }

func (c *Client) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	if c.cfg.RequestTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, c.cfg.RequestTimeout)
}

// CheckAccess verifies the bucket exists and the credentials can read it.
func (c *Client) CheckAccess(ctx context.Context) error {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	_, err := c.api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.bucket),
		Prefix:  aws.String(c.prefix),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return fmt.Errorf("cannot access s3://%s/%s: %w", c.bucket, c.prefix, err)
	}
	return nil
}
