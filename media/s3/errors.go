package s3

import "errors"

// ErrMissingBucket is returned when the S3 bucket name is empty.
var ErrMissingBucket = errors.New("s3: bucket is required")

// ErrMissingCredentials is returned when the access key or secret is empty.
var ErrMissingCredentials = errors.New("s3: access key and secret are required")
