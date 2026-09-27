package observability

import "context"

// requestIDKey is the context key for request correlation IDs.
type requestIDKey struct{}

// WithRequestID attaches a request correlation ID to ctx.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the request correlation ID in ctx, if any.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// MapCarrier is a map-backed propagation carrier.
type MapCarrier map[string]string

// Get returns the value for key, or empty string.
func (c MapCarrier) Get(key string) string {
	return c[key]
}

// Set stores value for key.
func (c MapCarrier) Set(key, value string) {
	c[key] = value
}

// Keys returns all keys in the carrier.
func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
