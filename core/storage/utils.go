package storage

import (
	"fmt"
	"path"
	"strings"
)

// BucketName is a validated bucket identifier.
type BucketName string

// String returns the string form of BucketName.
func (b BucketName) String() string {
	return string(b)
}

// Validate reports whether the bucket name is well-formed.
func (b BucketName) Validate() error {
	if !ValidBucket(string(b)) {
		return fmt.Errorf("%w: %q", ErrInvalidBucket, string(b))
	}

	return nil
}

// ValidKey reports whether key is a safe relative object path.
func ValidKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}

	clean := path.Clean(key)
	if clean != key {
		return false
	}

	switch clean {
	case ".", "..":
		return false
	}
	if clean[0] == '/' {
		return false
	}
	if strings.HasPrefix(clean, "../") {
		return false
	}

	for i := 0; i < len(clean); i++ {
		switch clean[i] {
		case 0:
			return false
		case '\\':
			return false
		}
	}

	return true
}

// ValidBucket reports whether bucket is a well-formed bucket name.
func ValidBucket(bucket string) bool {
	bucket = strings.TrimSpace(bucket)

	if bucket == "" || bucket == "." || bucket == ".." {
		return false
	}

	for i := 0; i < len(bucket); i++ {
		c := bucket[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '.':
		default:
			return false
		}
	}

	return true
}
