package s3io

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

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
