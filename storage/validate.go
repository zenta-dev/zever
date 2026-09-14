package storage

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// MaxPresignTTL bounds presigned-URL lifetimes (7d is the S3 presign hard
// limit; the same bound applies to all backends for consistency).
const MaxPresignTTL = 7 * 24 * time.Hour

// PresignExpiry validates ttl and returns its unix expiry. Non-positive ttls
// (which would mint instantly-dead URLs) report ErrExpired; ttls beyond
// MaxPresignTTL are rejected.
func PresignExpiry(ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		return 0, fmt.Errorf("%w: presign ttl must be positive, got %s", ErrExpired, ttl)
	}

	if ttl > MaxPresignTTL {
		return 0, fmt.Errorf("storage: presign ttl %s exceeds max %s", ttl, MaxPresignTTL)
	}

	return time.Now().Add(ttl).Unix(), nil
}

// ValidateBaseURL checks that value is an http(s) URL without user info or
// whitespace. An empty value is valid (means unset).
func ValidateBaseURL(prefix, name, value string) error {
	if value == "" {
		return nil
	}

	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("[%s] option %q must be a valid URL: %w", prefix, name, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("[%s] option %q must have http or https scheme, got %q", prefix, name, u.Scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("[%s] option %q must have a host", prefix, name)
	}

	if u.User != nil {
		return fmt.Errorf("[%s] option %q must not contain user info", prefix, name)
	}

	if strings.Contains(value, " ") || strings.Contains(value, "\n") || strings.Contains(value, "\t") {
		return fmt.Errorf("[%s] option %q must not contain whitespace", prefix, name)
	}

	return nil
}

// ValidateBucketKey checks bucket and key, wrapping ErrInvalidBucket or
// ErrInvalidKey with the offending value.
func ValidateBucketKey(bucket, key string) error {
	if !ValidBucket(bucket) {
		return fmt.Errorf("%w: %q", ErrInvalidBucket, bucket)
	}

	if !ValidKey(key) {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}

	return nil
}
