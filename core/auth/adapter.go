package auth

// Adapter identifies the authentication backend implementation.

type Adapter string

const (
	// JWT is the JWT authentication adapter.
	JWT Adapter = "jwt"
	// Session is the session-backed authentication adapter.
	Session Adapter = "session"
	// OIDC is the OpenID Connect authentication adapter.
	OIDC Adapter = "oidc"
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
