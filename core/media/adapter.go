package media

// Adapter identifies the media backend implementation.

type Adapter string

const (
	// Local selects the local-filesystem media backend.
	Local Adapter = "local"
	// S3 selects the S3 media backend.
	S3 Adapter = "s3"
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
