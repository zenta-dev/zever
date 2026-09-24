package permission

// Adapter identifies a registered permission backend.
type Adapter int

const (
	// Noop selects the allow/deny-all permission adapter.
	Noop Adapter = iota
	// RBAC selects the role-based permission adapter.
	RBAC
	// Casbin selects the Casbin-backed permission adapter.
	Casbin
)

// String returns the canonical lowercase name of the adapter.
func (a Adapter) String() string {
	switch a {
	case Noop:
		return "noop"
	case RBAC:
		return "rbac"
	case Casbin:
		return "casbin"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "noop":
		return Noop, nil
	case "rbac":
		return RBAC, nil
	case "casbin":
		return Casbin, nil
	default:
		return Noop, &InvalidAdapterError{Adapter: s}
	}
}
