package s3io

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// maxSingleCopy is the largest object CopyObject accepts (5 GiB).
const maxSingleCopy = 5 << 30

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
	c.writeOptions().applyPut(up)
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

// writeOptions are the settings this mount puts on an object it creates.
//
// Three request types create objects — put, copy, and the multipart copy — and
// the SDK gives them no common type, so the configuration is read once here
// and each request applies what it carries. An unset option is the field's
// zero value, which is what "leave it out of the request" already was.
type writeOptions struct {
	storageClass types.StorageClass
	sse          types.ServerSideEncryption
	kmsKeyID     *string
	acl          types.ObjectCannedACL
}

func (c *Client) writeOptions() writeOptions {
	o := writeOptions{
		storageClass: types.StorageClass(c.cfg.StorageClass),
		sse:          types.ServerSideEncryption(c.cfg.SSE),
		acl:          types.ObjectCannedACL(c.cfg.ACL),
	}
	if c.cfg.KMSKeyID != "" {
		o.kmsKeyID = aws.String(c.cfg.KMSKeyID)
	}
	return o
}

func (o writeOptions) applyPut(in *s3.PutObjectInput) {
	in.StorageClass, in.ServerSideEncryption = o.storageClass, o.sse
	in.SSEKMSKeyId, in.ACL = o.kmsKeyID, o.acl
}

func (o writeOptions) applyCopy(in *s3.CopyObjectInput) {
	in.StorageClass, in.ServerSideEncryption = o.storageClass, o.sse
	in.SSEKMSKeyId, in.ACL = o.kmsKeyID, o.acl
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
	c.writeOptions().applyCopy(in)
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
	// The storage class is all a multipart copy carries: SSE, the KMS key and
	// the ACL reach a normal copy and not this one, which is a gap rather than
	// a decision, and changing it is a change of behaviour.
	create.StorageClass = c.writeOptions().storageClass
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

func url(bucket, key string) string { return bucket + "/" + key }
