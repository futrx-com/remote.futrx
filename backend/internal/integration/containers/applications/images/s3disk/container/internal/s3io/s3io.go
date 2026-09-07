// Package s3io wraps the AWS S3 API with the small, path-oriented surface the
// filesystem needs: stat, list, ranged read, upload, copy and delete.
package s3io

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"s3disk/internal/config"
)

// ErrNotFound is returned for missing keys, mapped to ENOENT by the FS layer.
var ErrNotFound = errors.New("s3disk: object not found")

// maxSingleCopy is the largest object CopyObject accepts (5 GiB).
const maxSingleCopy = 5 << 30

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

// Head fetches object metadata for an exact key.
func (c *Client) Head(ctx context.Context, key string) (*Object, error) {
	rctx, cancel := c.ctx(ctx)
	defer cancel()
	c.Stats.Heads.Add(1)
	out, err := c.api.HeadObject(rctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, c.mapError(err)
	}
	o := &Object{Key: key, ETag: aws.ToString(out.ETag), Meta: lower(out.Metadata)}
	if out.ContentLength != nil {
		o.Size = *out.ContentLength
	}
	if out.LastModified != nil {
		o.Modified = *out.LastModified
	}
	return o, nil
}

// List enumerates one directory level below dirKey (which must end in "/" or be
// empty for the bucket root). fn is called for each child in listing order.
func (c *Client) List(ctx context.Context, dirKey string, fn func(ListEntry) bool) error {
	var token *string
	for {
		rctx, cancel := c.ctx(ctx)
		c.Stats.Lists.Add(1)
		out, err := c.api.ListObjectsV2(rctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.bucket),
			Prefix:            aws.String(dirKey),
			Delimiter:         aws.String("/"),
			MaxKeys:           aws.Int32(c.cfg.ListLimit),
			ContinuationToken: token,
		})
		cancel()
		if err != nil {
			return c.mapError(err)
		}
		for _, p := range out.CommonPrefixes {
			name := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(p.Prefix), dirKey), "/")
			if name == "" {
				continue
			}
			if !fn(ListEntry{Name: name, IsDir: true}) {
				return nil
			}
		}
		for _, o := range out.Contents {
			key := aws.ToString(o.Key)
			name := strings.TrimPrefix(key, dirKey)
			// The directory's own marker object, or a nested marker already
			// reported through CommonPrefixes.
			if name == "" || strings.Contains(name, "/") {
				continue
			}
			e := ListEntry{Name: name, ETag: aws.ToString(o.ETag)}
			if o.Size != nil {
				e.Size = *o.Size
			}
			if o.LastModified != nil {
				e.Modified = *o.LastModified
			}
			if !fn(e) {
				return nil
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return nil
		}
		token = out.NextContinuationToken
	}
}

// ListAll enumerates every key under a prefix, recursively. Used by directory
// rename, recursive delete and emptiness checks.
func (c *Client) ListAll(ctx context.Context, prefix string, limit int, fn func(key string, size int64) bool) error {
	var token *string
	n := 0
	for {
		rctx, cancel := c.ctx(ctx)
		c.Stats.Lists.Add(1)
		out, err := c.api.ListObjectsV2(rctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.bucket),
			Prefix:            aws.String(prefix),
			MaxKeys:           aws.Int32(c.cfg.ListLimit),
			ContinuationToken: token,
		})
		cancel()
		if err != nil {
			return c.mapError(err)
		}
		for _, o := range out.Contents {
			var size int64
			if o.Size != nil {
				size = *o.Size
			}
			if !fn(aws.ToString(o.Key), size) {
				return nil
			}
			n++
			if limit > 0 && n >= limit {
				return nil
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return nil
		}
		token = out.NextContinuationToken
	}
}

// Exists reports whether any key exists under prefix (implicit directory test).
func (c *Client) Exists(ctx context.Context, prefix string) (bool, error) {
	rctx, cancel := c.ctx(ctx)
	defer cancel()
	c.Stats.Lists.Add(1)
	out, err := c.api.ListObjectsV2(rctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(c.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return false, c.mapError(err)
	}
	return len(out.Contents) > 0 || len(out.CommonPrefixes) > 0, nil
}

// GetRange downloads [off, off+length) of an object into dst.
func (c *Client) GetRange(ctx context.Context, key string, off, length int64, dst io.Writer) (int64, error) {
	if length <= 0 {
		return 0, nil
	}
	rctx, cancel := c.ctx(ctx)
	defer cancel()
	c.Stats.Gets.Add(1)
	out, err := c.api.GetObject(rctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", off, off+length-1)),
	})
	if err != nil {
		return 0, c.mapError(err)
	}
	defer out.Body.Close()
	n, err := io.Copy(dst, out.Body)
	c.Stats.BytesDown.Add(n)
	if err != nil {
		c.Stats.Errors.Add(1)
		return n, fmt.Errorf("reading %s at %d: %w", key, off, err)
	}
	return n, nil
}

// GetAll downloads a whole object (used for symlink targets and small reads).
func (c *Client) GetAll(ctx context.Context, key string) ([]byte, error) {
	rctx, cancel := c.ctx(ctx)
	defer cancel()
	c.Stats.Gets.Add(1)
	out, err := c.api.GetObject(rctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, c.mapError(err)
	}
	defer out.Body.Close()
	b, err := io.ReadAll(out.Body)
	c.Stats.BytesDown.Add(int64(len(b)))
	return b, err
}

// PutInput describes a single upload.
type PutInput struct {
	Key         string
	Body        io.Reader
	Size        int64
	Meta        map[string]string
	ContentType string
}

// Put uploads an object, transparently switching to multipart for large bodies.
func (c *Client) Put(ctx context.Context, in PutInput) (*Object, error) {
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.Stats.Puts.Add(1)
	body := in.Body
	if body == nil {
		body = strings.NewReader("")
	}
	up := &s3.PutObjectInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(in.Key),
		Body:     body,
		Metadata: in.Meta,
	}
	if in.ContentType != "" {
		up.ContentType = aws.String(in.ContentType)
	}
	c.applyWriteOptions(up)
	out, err := c.uploader.Upload(rctx, up, func(u *manager.Uploader) {
		if in.Size >= 0 && in.Size < c.cfg.MultipartThreshold {
			u.PartSize = c.cfg.PartSize
			u.Concurrency = 1
		}
	})
	if err != nil {
		return nil, c.mapError(err)
	}
	if in.Size > 0 {
		c.Stats.BytesUp.Add(in.Size)
	}
	obj := &Object{Key: in.Key, Size: in.Size, ETag: aws.ToString(out.ETag), Modified: time.Now(), Meta: in.Meta}
	return obj, nil
}

func (c *Client) applyWriteOptions(in *s3.PutObjectInput) {
	if c.cfg.StorageClass != "" {
		in.StorageClass = types.StorageClass(c.cfg.StorageClass)
	}
	if c.cfg.SSE != "" {
		in.ServerSideEncryption = types.ServerSideEncryption(c.cfg.SSE)
	}
	if c.cfg.KMSKeyID != "" {
		in.SSEKMSKeyId = aws.String(c.cfg.KMSKeyID)
	}
	if c.cfg.ACL != "" {
		in.ACL = types.ObjectCannedACL(c.cfg.ACL)
	}
}

// Copy server-side copies src to dst. When meta is non-nil the copy replaces
// user metadata (this is how chmod/chown/utimens are persisted); otherwise the
// source metadata is preserved.
func (c *Client) Copy(ctx context.Context, src, dst string, size int64, meta map[string]string, contentType string) error {
	if size > maxSingleCopy {
		return c.copyMultipart(ctx, src, dst, size, meta, contentType)
	}
	rctx, cancel := c.ctx(ctx)
	defer cancel()
	c.Stats.Copies.Add(1)
	in := &s3.CopyObjectInput{
		Bucket:     aws.String(c.bucket),
		Key:        aws.String(dst),
		CopySource: aws.String(url(c.bucket, src)),
	}
	if meta != nil {
		in.Metadata = meta
		in.MetadataDirective = types.MetadataDirectiveReplace
		if contentType != "" {
			in.ContentType = aws.String(contentType)
		}
	}
	if c.cfg.StorageClass != "" {
		in.StorageClass = types.StorageClass(c.cfg.StorageClass)
	}
	if c.cfg.SSE != "" {
		in.ServerSideEncryption = types.ServerSideEncryption(c.cfg.SSE)
	}
	if c.cfg.KMSKeyID != "" {
		in.SSEKMSKeyId = aws.String(c.cfg.KMSKeyID)
	}
	if c.cfg.ACL != "" {
		in.ACL = types.ObjectCannedACL(c.cfg.ACL)
	}
	if _, err := c.api.CopyObject(rctx, in); err != nil {
		return c.mapError(err)
	}
	return nil
}

// copyMultipart handles objects above the 5 GiB CopyObject limit.
func (c *Client) copyMultipart(ctx context.Context, src, dst string, size int64, meta map[string]string, contentType string) error {
	create := &s3.CreateMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(dst),
		Metadata: meta,
	}
	if contentType != "" {
		create.ContentType = aws.String(contentType)
	}
	if c.cfg.StorageClass != "" {
		create.StorageClass = types.StorageClass(c.cfg.StorageClass)
	}
	mu, err := c.api.CreateMultipartUpload(ctx, create)
	if err != nil {
		return c.mapError(err)
	}
	partSize := c.cfg.PartSize
	if partSize < 64<<20 {
		partSize = 64 << 20 // fewer, larger parts for huge copies
	}
	var parts []types.CompletedPart
	for off, num := int64(0), int32(1); off < size; off, num = off+partSize, num+1 {
		end := off + partSize - 1
		if end >= size {
			end = size - 1
		}
		c.Stats.Copies.Add(1)
		out, err := c.api.UploadPartCopy(ctx, &s3.UploadPartCopyInput{
			Bucket:          aws.String(c.bucket),
			Key:             aws.String(dst),
			UploadId:        mu.UploadId,
			PartNumber:      aws.Int32(num),
			CopySource:      aws.String(url(c.bucket, src)),
			CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", off, end)),
		})
		if err != nil {
			_, _ = c.api.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket: aws.String(c.bucket), Key: aws.String(dst), UploadId: mu.UploadId})
			return c.mapError(err)
		}
		parts = append(parts, types.CompletedPart{ETag: out.CopyPartResult.ETag, PartNumber: aws.Int32(num)})
	}
	_, err = c.api.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(c.bucket),
		Key:             aws.String(dst),
		UploadId:        mu.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	return c.mapError(err)
}

// Delete removes a single key. Deleting a missing key is not an error.
func (c *Client) Delete(ctx context.Context, key string) error {
	rctx, cancel := c.ctx(ctx)
	defer cancel()
	c.Stats.Deletes.Add(1)
	_, err := c.api.DeleteObject(rctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	return c.mapError(err)
}

// DeleteMulti removes up to 1000 keys per request.
func (c *Client) DeleteMulti(ctx context.Context, keys []string) error {
	for len(keys) > 0 {
		batch := keys
		if len(batch) > 1000 {
			batch = batch[:1000]
		}
		keys = keys[len(batch):]
		objs := make([]types.ObjectIdentifier, 0, len(batch))
		for _, k := range batch {
			objs = append(objs, types.ObjectIdentifier{Key: aws.String(k)})
		}
		rctx, cancel := c.ctx(ctx)
		c.Stats.Deletes.Add(int64(len(batch)))
		_, err := c.api.DeleteObjects(rctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &types.Delete{Objects: objs, Quiet: aws.Bool(true)},
		})
		cancel()
		if err != nil {
			return c.mapError(err)
		}
	}
	return nil
}

// mapError normalises 404/NoSuchKey into ErrNotFound and counts failures.
func (c *Client) mapError(err error) error {
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return ErrNotFound
	}
	c.Stats.Errors.Add(1)
	return err
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var nf *types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}

func url(bucket, key string) string { return bucket + "/" + key }

func lower(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}
