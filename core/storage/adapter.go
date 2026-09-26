package storage

// Adapter identifies a storage backend.

type Adapter string

const (
	// AdapterLocal selects the local-filesystem backend.
	AdapterLocal Adapter = "local"
	// AdapterS3 selects the Amazon S3 backend.
	AdapterS3 Adapter = "s3"
	// AdapterR2 selects the Cloudflare R2 backend.
	AdapterR2 Adapter = "r2"
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter parses adapter name into an Adapter.
// Any non-empty name is accepted to allow custom adapters; empty fails.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), &InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
