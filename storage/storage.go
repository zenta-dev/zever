package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/internal/registry"
)

// HTTPMethod is an HTTP verb used in presigned URLs.
type HTTPMethod string

const (
	// HTTPMethodGET marks a download URL.
	HTTPMethodGET HTTPMethod = "GET"
	// HTTPMethodPOST marks a POST upload URL.
	HTTPMethodPOST HTTPMethod = "POST"
	// HTTPMethodPUT marks a PUT upload URL.
	HTTPMethodPUT HTTPMethod = "PUT"
	// HTTPMethodPATCH marks a PATCH URL.
	HTTPMethodPATCH HTTPMethod = "PATCH"
	// HTTPMethodDELETE marks a DELETE URL.
	HTTPMethodDELETE HTTPMethod = "DELETE"
)

// PresignedURL is a minted method + URL pair granting temporary access.
type PresignedURL struct {
	// Method is the HTTP verb to use with URL.
	Method HTTPMethod
	// URL is the presigned or static URL.
	URL string
}

// Storage defines the backend contract for presigned uploads and downloads.
type Storage interface {
	// PresignUpload mints an upload URL for bucket/key with contentType valid for ttl.
	PresignUpload(
		ctx context.Context,
		bucket string,
		key string,
		contentType string,
		ttl time.Duration,
	) (PresignedURL, error)

	// PresignDownload mints a download URL for bucket/key valid for ttl.
	PresignDownload(
		ctx context.Context,
		bucket string,
		key string,
		ttl time.Duration,
	) (PresignedURL, error)

	// Exists reports whether an object is present under bucket/key.
	Exists(
		ctx context.Context,
		bucket string,
		key string,
	) (bool, error)

	// Delete removes the object under bucket/key.
	Delete(
		ctx context.Context,
		bucket string,
		key string,
	) error

	// Move relocates srcBucket/srcKey to dstBucket/dstKey, overwriting dst.
	// A missing source reports ErrNotFound; moving a key onto itself is a no-op.
	Move(
		ctx context.Context,
		srcBucket string,
		srcKey string,
		dstBucket string,
		dstKey string,
	) error

	// Close releases backend resources.
	Close(ctx context.Context) error

	// Name returns the canonical adapter name.
	Name() string
}

// Factory creates a Storage from the given Options.
type Factory func(opts Options) (Storage, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Storage for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Storage, error) {
	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", adapter, err)
	}

	return s, nil
}
