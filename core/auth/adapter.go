package auth

// Adapter identifies the authentication backend implementation.
type Adapter int

const (
	// JWT is the JWT authentication adapter.
	JWT Adapter = iota
	// Session is the session-backed authentication adapter.
	Session
	// OIDC is the OpenID Connect authentication adapter.
	OIDC
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case JWT:
		return "jwt"
	case Session:
		return "session"
	case OIDC:
		return "oidc"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "jwt":
		return JWT, nil
	case "session":
		return Session, nil
	case "oidc":
		return OIDC, nil
	default:
		return JWT, &InvalidAdapterError{Adapter: s}
	}
}
