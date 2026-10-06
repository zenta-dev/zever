package cdn

// Adapter names a CDN backend implementation.
type Adapter string

const (
	// AdapterCloudflare selects the Cloudflare CDN backend.
	AdapterCloudflare Adapter = "cloudflare"
)

// String returns the adapter name, or "unknown" for the zero value.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter converts a config adapter name into an Adapter.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
