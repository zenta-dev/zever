package storage

import (
	"errors"
	"testing"
)

// TestOpenInvalidOptions pins that Open validates options before touching
// the registry: bad options fail even for a registered adapter.
func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	a := benchStorageAdapter()
	if err := Register(a, func(Options) (Storage, error) { return stubStorage{}, nil }); err != nil {
		t.Fatalf("Register(%v) error = %v", a, err)
	}

	if _, err := Open(a, Options{URLBase: "ftp://example.com"}); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(bad url_base) err = %v, want ErrInvalidOptions", err)
	}

	badPolicy := Options{Policy: &PolicyConfig{Buckets: map[BucketName]Policy{"bad/name": {}}}}
	if _, err := Open(a, badPolicy); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(bad policy) err = %v, want ErrInvalidOptions", err)
	}
}
