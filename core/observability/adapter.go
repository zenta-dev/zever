package observability

// Adapter identifies a registered observability backend.

type Adapter string

const (
	// Noop selects the discard-all observability adapter.
	Noop Adapter = "noop"
	// Stdout selects the human-readable stdout observability adapter.
	Stdout Adapter = "stdout"
	// OTLP selects the OpenTelemetry OTLP observability adapter.
	OTLP Adapter = "otlp"
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
