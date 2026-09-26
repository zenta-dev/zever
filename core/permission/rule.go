package permission

// Effect is the outcome a Rule prescribes on match.
type Effect string

const (
	// Allow grants access on match.
	Allow Effect = "allow"
	// Deny refuses access on match.
	Deny Effect = "deny"
)

// Rule is a single role/action authorization rule.
type Rule struct {
	// Role is the role this rule applies to.
	Role string
	// Action is the action this rule applies to.
	Action string
	// Effect is allow or deny; empty means Allow for back-compat.
	Effect Effect
	// OwnedOnly restricts the rule to resources owned by the subject.
	OwnedOnly bool
	// OwnedAttr is the resource attribute carrying the owner ID.
	OwnedAttr string
}

// effect returns the normalized effect, defaulting empty to Allow.
func (r Rule) effect() Effect {
	if r.Effect == "" {
		return Allow
	}
	return r.Effect
}

// Validate checks the rule for consistency.
func (r Rule) Validate() error {
	if r.Role == "" {
		return &InvalidOptionsError{Reason: "rule role must be non-empty"}
	}
	if len(r.Role) > 256 {
		return &InvalidOptionsError{Reason: "rule role must be at most 256 characters"}
	}
	if r.Action == "" {
		return &InvalidOptionsError{Reason: "rule action must be non-empty"}
	}
	if len(r.Action) > 256 {
		return &InvalidOptionsError{Reason: "rule action must be at most 256 characters"}
	}
	switch r.Effect {
	case "", Allow, Deny:
	default:
		return &InvalidOptionsError{Reason: "rule effect must be allow or deny"}
	}
	if r.OwnedOnly && r.OwnedAttr == "" {
		return &InvalidOptionsError{Reason: "rule owned-only requires owned attribute"}
	}
	return nil
}
