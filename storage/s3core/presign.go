package s3core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awstransport "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/storage"
)

func isPublicUpload(pol storage.Policy) bool {
	return pol.Public(storage.PermWrite) && pol.Public(storage.PermUpdate)
}

func (c *Core) uploadPerm(ctx context.Context, bucket, key string, pol storage.Policy) storage.Perm {
	if isPublicUpload(pol) {
		return storage.PermWrite
	}

	if !pol.NeedsExistCheck() {
		return storage.PermWrite
	}

	exists, err := c.ExistsUnrestricted(ctx, bucket, key)

	return storage.SelectUploadPerm(pol, exists && err == nil)
}

// PresignUpload validates bucket, key, and ttl, then returns a PUT URL for upload.
// It returns a public static URL when uploads are public, otherwise it selects the write or update perm and enforces policy with FireAllow on success and FireDeny on denial.
// The presigned URL is issued via PresignPutObject.
func (c *Core) PresignUpload(
	ctx context.Context,
	bucket string,
	key string,
	contentType string,
	ttl time.Duration,
) (storage.PresignedURL, error) {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return storage.PresignedURL{}, fmt.Errorf("%s: %w", c.prefix, err)
	}

	if _, err := storage.PresignExpiry(ttl); err != nil {
		return storage.PresignedURL{}, fmt.Errorf("%s: %w", c.prefix, err)
	}

	if c.Configured() {
		pol := c.PolicyFor(bucket)

		if isPublicUpload(pol) {
			u, err := c.staticURL(bucket, key)
			if err != nil {
				return storage.PresignedURL{}, err
			}

			storage.FireAllow(ctx, storage.PermWrite, bucket, "", storage.ReasonOkPublic, pol.Version)

			return storage.PresignedURL{Method: "PUT", URL: u}, nil
		}

		perm := c.uploadPerm(ctx, bucket, key, pol)

		subject, ok := storage.SubjectFrom(ctx)
		if !ok || !pol.Allow(perm, subject, ok) {
			storage.FireDeny(ctx, perm, bucket, subject, storage.DenyReason(pol, perm, subject, ok), pol.Version)

			return storage.PresignedURL{}, storage.ErrForbidden
		}

		storage.FireAllow(ctx, perm, bucket, subject, storage.ReasonOk, pol.Version)
	}

	po := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}

	p, err := c.Presigner.PresignPutObject(ctx, po, s3.WithPresignExpires(ttl))
	if err != nil {
		return storage.PresignedURL{}, fmt.Errorf("%s: presign upload: %w", c.prefix, err)
	}

	return storage.PresignedURL{Method: "PUT", URL: p.URL}, nil
}

// PresignDownload validates bucket, key, and ttl, then returns a GET URL for download.
// It returns a public static URL when reads are public, otherwise it enforces read policy with FireAllow on success and FireDeny on denial.
// The presigned URL is issued via PresignGetObject.
func (c *Core) PresignDownload(ctx context.Context, bucket string, key string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return storage.PresignedURL{}, fmt.Errorf("%s: %w", c.prefix, err)
	}

	if _, err := storage.PresignExpiry(ttl); err != nil {
		return storage.PresignedURL{}, fmt.Errorf("%s: %w", c.prefix, err)
	}

	perm := storage.PermRead

	if c.Configured() {
		pol := c.PolicyFor(bucket)
		if pol.Public(perm) {
			u, err := c.staticURL(bucket, key)
			if err != nil {
				return storage.PresignedURL{}, err
			}

			storage.FireAllow(ctx, perm, bucket, "", storage.ReasonOkPublic, pol.Version)

			return storage.PresignedURL{Method: "GET", URL: u}, nil
		}

		subject, ok := storage.SubjectFrom(ctx)
		if !ok || !pol.Allow(perm, subject, ok) {
			storage.FireDeny(ctx, perm, bucket, subject, storage.DenyReason(pol, perm, subject, ok), pol.Version)

			return storage.PresignedURL{}, storage.ErrForbidden
		}

		storage.FireAllow(ctx, perm, bucket, subject, storage.ReasonOk, pol.Version)
	}

	goo := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}

	p, err := c.Presigner.PresignGetObject(ctx, goo, s3.WithPresignExpires(ttl))
	if err != nil {
		return storage.PresignedURL{}, fmt.Errorf("%s: presign download: %w", c.prefix, err)
	}

	return storage.PresignedURL{Method: "GET", URL: p.URL}, nil
}

// Delete validates bucket and key, enforces delete policy, then removes the object.
// It denies with FireDeny and ErrForbidden when the subject lacks delete permission.
// Delete failures from S3 are returned wrapped.
func (c *Core) Delete(ctx context.Context, bucket string, key string) error {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return fmt.Errorf("%s: %w", c.prefix, err)
	}

	if c.Configured() {
		pol := c.PolicyFor(bucket)
		subject, sok := storage.SubjectFrom(ctx)

		if !pol.Allow(storage.PermDelete, subject, sok) {
			storage.FireDeny(ctx, storage.PermDelete, bucket, subject, storage.DenyReason(pol, storage.PermDelete, subject, sok), pol.Version)

			return fmt.Errorf("%s: %w", c.prefix, storage.ErrForbidden)
		}
	}

	_, err := c.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("%s: delete object: %w", c.prefix, err)
	}

	return nil
}

// Exists validates bucket and key, then reports object existence without policy checks.
// It delegates to ExistsUnrestricted.
func (c *Core) Exists(ctx context.Context, bucket string, key string) (bool, error) {
	if err := storage.ValidateBucketKey(bucket, key); err != nil {
		return false, fmt.Errorf("%s: %w", c.prefix, err)
	}

	return c.ExistsUnrestricted(ctx, bucket, key)
}

// ExistsUnrestricted checks object existence via HeadObject without policy checks.
// It returns false and nil when HeadObject reports 404, and wraps any other error.
func (c *Core) ExistsUnrestricted(ctx context.Context, bucket, key string) (bool, error) {
	_, err := c.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	var respErr *awstransport.ResponseError
	if errors.As(err, &respErr) && respErr.Response.StatusCode == http.StatusNotFound {
		return false, nil
	}

	return false, fmt.Errorf("%s: head object: %w", c.prefix, err)
}
