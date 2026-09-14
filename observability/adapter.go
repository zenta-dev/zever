package observability

// Adapter identifies a registered observability backend.
type Adapter int

const (
	// Noop selects the discard-all observability adapter.
	Noop Adapter = iota
	// Stdout selects the human-readable stdout observability adapter.
	Stdout
	// OTLP selects the OpenTelemetry OTLP observability adapter.
	OTLP
)

// String returns the canonical lowercase name of the adapter.
func (a Adapter) String() string {
	switch a {
	case Noop:
		return "noop"
	case Stdout:
		return "stdout"
	case OTLP:
		return "otlp"
	default:
		return "unknown"
	}
}

// ParseAdapter converts a canonical adapter name into an Adapter.
func ParseAdapter(adapter string) (Adapter, error) {
	switch adapter {
	case "noop":
		return Noop, nil
	case "stdout":
		return Stdout, nil
	case "otlp":
		return OTLP, nil
	default:
		return Noop, &InvalidAdapterError{Adapter: adapter}
	}
}
