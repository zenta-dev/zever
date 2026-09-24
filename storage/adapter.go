package storage

// Adapter identifies a storage backend.
type Adapter uint8

// Adapter backend identifiers.
const (
	// AdapterLocal selects the local-filesystem backend.
	AdapterLocal Adapter = iota
	// AdapterS3 selects the Amazon S3 backend.
	AdapterS3
	// AdapterR2 selects the Cloudflare R2 backend.
	AdapterR2
)

// String returns the canonical adapter name.
func (a Adapter) String() string {
	switch a {
	case AdapterLocal:
		return "local"
	case AdapterS3:
		return "s3"
	case AdapterR2:
		return "r2"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "local":
		return AdapterLocal, nil
	case "s3":
		return AdapterS3, nil
	case "r2":
		return AdapterR2, nil
	default:
		return AdapterLocal, &InvalidAdapterError{Adapter: s}
	}
}
