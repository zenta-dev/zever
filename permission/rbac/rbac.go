package rbac

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/permission"
)

// checker is an in-memory role-based permission.Checker.
type checker struct {
	allow map[string][]permission.Rule
	deny  map[string][]permission.Rule
}

var _ permission.Checker = (*checker)(nil)

// New builds a role-based Checker from opts.
// It validates opts before indexing rules by action.
func New(opts permission.Options) (permission.Checker, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("rbac: %w", err)
	}

	allow := make(map[string][]permission.Rule)
	deny := make(map[string][]permission.Rule)

	for _, r := range opts.Rules {
		if r.Effect == permission.Deny {
			deny[r.Action] = append(deny[r.Action], r)
		} else {
			allow[r.Action] = append(allow[r.Action], r)
		}
	}

	return &checker{allow: allow, deny: deny}, nil
}

// Can reports whether subject may perform action on resource.
// Deny rules win over allow rules. A nil ctx is treated as
// context.Background. Policy decisions never produce an error.
func (c *checker) Can(ctx context.Context, subject permission.Subject, action string, resource permission.Resource) (permission.Decision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = ctx

	for _, key := range []string{action, "*"} {
		for _, r := range c.deny[key] {
			if matchRule(subject, action, resource, r) {
				return permission.Decision{Allowed: false, Reason: "explicit_deny"}, nil
			}
		}
	}

	for _, key := range []string{action, "*"} {
		for _, r := range c.allow[key] {
			if matchRule(subject, action, resource, r) {
				return permission.Decision{Allowed: true, Reason: "allow"}, nil
			}
		}
	}

	return permission.Decision{Allowed: false, Reason: "implicit_deny"}, nil
}

// matchRule reports whether rule applies to the request.
func matchRule(subject permission.Subject, action string, resource permission.Resource, rule permission.Rule) bool {
	if rule.Action != "*" && rule.Action != action {
		return false
	}

	if rule.Role == "*" {
		if subject.ID == "" || len(subject.Roles) == 0 {
			return false
		}
	} else {
		found := false
		for _, r := range subject.Roles {
			if r == rule.Role {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if rule.OwnedOnly {
		v, ok := resource.Attributes[rule.OwnedAttr]
		if subject.ID == "" || !ok || v != subject.ID {
			return false
		}
	}

	return true
}
