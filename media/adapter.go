package media

// Adapter identifies the media backend implementation.
type Adapter int

const (
	// Local selects the local-filesystem media backend.
	Local Adapter = iota
	// S3 selects the S3 media backend.
	S3
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Local:
		return "local"
	case S3:
		return "s3"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "local":
		return Local, nil
	case "s3":
		return S3, nil
	default:
		return Local, &InvalidAdapterError{Adapter: s}
	}
}
