package s3opts

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/core/storage"
)

// Move validates both source and destination bucket/keys, treats self-move as a no-op, and returns ErrNotFound when the source is missing.
// It requires read plus delete on the source and write or update on the destination, copies the object then deletes the source, and fires allow decisions on success.
func (c *Core) Move(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	if err := storage.ValidateBucketKey(srcBucket, srcKey); err != nil {
		return fmt.Errorf("%s: %w", c.prefix, err)
	}

	if err := storage.ValidateBucketKey(dstBucket, dstKey); err != nil {
		return fmt.Errorf("%s: %w", c.prefix, err)
	}

	if srcBucket == dstBucket && srcKey == dstKey {
		return nil
	}

	exists, err := c.ExistsUnrestricted(ctx, srcBucket, srcKey)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("%s: %w", c.prefix, storage.ErrNotFound)
	}

	var (
		srcPol  storage.Policy
		dstPol  storage.Policy
		dstPerm storage.Perm
	)

	subject, sok := storage.SubjectFrom(ctx)

	if c.Configured() {
		srcPol = c.PolicyFor(srcBucket)

		if !srcPol.Allow(storage.PermRead, subject, sok) {
			storage.FireDeny(ctx, storage.PermRead, srcBucket, subject, storage.DenyReason(srcPol, storage.PermRead, subject, sok), srcPol.Version)

			return fmt.Errorf("%s: %w", c.prefix, storage.ErrForbidden)
		}

		if !srcPol.Allow(storage.PermDelete, subject, sok) {
			storage.FireDeny(ctx, storage.PermDelete, srcBucket, subject, storage.DenyReason(srcPol, storage.PermDelete, subject, sok), srcPol.Version)

			return fmt.Errorf("%s: %w", c.prefix, storage.ErrForbidden)
		}

		dstPol = c.PolicyFor(dstBucket)
		dstPerm = storage.PermWrite

		if dstPol.NeedsExistCheck() {
			var dstExists bool
			dstExists, err = c.ExistsUnrestricted(ctx, dstBucket, dstKey)
			if err != nil {
				return err
			}

			if dstExists {
				dstPerm = storage.PermUpdate
			}
		}

		if !dstPol.Allow(dstPerm, subject, sok) {
			storage.FireDeny(ctx, dstPerm, dstBucket, subject, storage.DenyReason(dstPol, dstPerm, subject, sok), dstPol.Version)

			return fmt.Errorf("%s: %w", c.prefix, storage.ErrForbidden)
		}
	}

	_, err = c.Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(url.PathEscape(srcBucket) + "/" + EscapeKey(srcKey)),
	})
	if err != nil {
		return fmt.Errorf("%s: copy object: %w", c.prefix, err)
	}

	_, err = c.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(srcBucket),
		Key:    aws.String(srcKey),
	})
	if err != nil {
		return fmt.Errorf("%s: move copied but delete source failed: %w", c.prefix, err)
	}

	if c.Configured() {
		storage.FireAllow(ctx, storage.PermRead, srcBucket, subject, storage.ReasonOk, srcPol.Version)
		storage.FireAllow(ctx, storage.PermDelete, srcBucket, subject, storage.ReasonOk, srcPol.Version)
		storage.FireAllow(ctx, dstPerm, dstBucket, subject, storage.ReasonOk, dstPol.Version)
	}

	return nil
}
