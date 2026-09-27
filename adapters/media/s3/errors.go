package s3

import (
	"github.com/zenta-dev/zever/shared/s3opts"
)

// ErrMissingBucket is returned when the S3 bucket name is empty.
var ErrMissingBucket = s3opts.ErrMissingBucket

// ErrMissingCredentials is returned when the access key or secret is empty.
var ErrMissingCredentials = s3opts.ErrMissingCredentials
