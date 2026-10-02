// Package permissiontest provides the conformance kit third-party permission
// checkers run to prove backend parity.
package permissiontest

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/permission"
)

// kitSubject is the canonical allowlisted actor the kit authorizes.
func kitSubject() permission.Subject {
	return permission.Subject{ID: "alice", Roles: []string{"admin"}}
}

// kitResource is the canonical resource the kit checks against.
func kitResource() permission.Resource {
	return permission.Resource{Type: "doc", ID: "r1"}
}

// Conformance verifies factory-built checkers seeded with the canonical
// policy (allow admin doc.read, deny admin doc.delete) implement the
// permission.Checker contract: allow, deny-override, role/no-rule
// default-deny, and nil-ctx-as-Background. Each subtest takes a fresh
// instance from factory so cases stay isolated. Denials surface as
// Decision{Allowed:false} with a nil error; only infrastructure failure
// may return an error (callers deny on err != nil, fail-closed). Tests
// never touch the network.
//
// Reason strings differ by backend (explicit_deny vs implicit_deny for
// deny-all backends), so the kit asserts Allowed and error parity only;
// reasons stay covered by adapter unit tests.
func Conformance(t *testing.T, factory func(t *testing.T) permission.Checker) {
	t.Helper()

	t.Run("Allow", func(t *testing.T) { conformanceAllow(t, factory) })
	t.Run("DenyOverride", func(t *testing.T) { conformanceDenyOverride(t, factory) })
	t.Run("DefaultDeny", func(t *testing.T) { conformanceDefaultDeny(t, factory) })
	t.Run("NilCtx", func(t *testing.T) { conformanceNilCtx(t, factory) })
}

func conformanceAllow(t *testing.T, factory func(t *testing.T) permission.Checker) {
	t.Helper()

	c := factory(t)
	// The canonical allow rule grants: both rule-indexed backends (rbac
	// matchRule) and the Casbin seeded policy (g(alice,admin) grouping
	// with obj "*" wildcard) resolve admin doc.read to allow.
	decision, err := c.Can(t.Context(), kitSubject(), "doc.read", kitResource())
	if err != nil {
		t.Fatalf("Can() error = %v, want nil", err)
	}
	if !decision.Allowed {
		t.Error("Can(admin, doc.read) = denied, want allow")
	}
	if decision.Reason != "allow" {
		t.Errorf("Can() Reason = %q, want allow", decision.Reason)
	}
}

func conformanceDenyOverride(t *testing.T, factory func(t *testing.T) permission.Checker) {
	t.Helper()

	c := factory(t)
	// Deny wins over any allow: rbac checks deny rules first and Casbin
	// runs a deny-override effect (some-allow && !some-deny). Fail-closed
	// either way: denial is Allowed:false with nil error.
	decision, err := c.Can(t.Context(), kitSubject(), "doc.delete", kitResource())
	if err != nil {
		t.Fatalf("Can() error = %v, want nil", err)
	}
	if decision.Allowed {
		t.Error("Can(admin, doc.delete) = allowed, want deny-override")
	}
}

func conformanceDefaultDeny(t *testing.T, factory func(t *testing.T) permission.Checker) {
	t.Helper()

	c := factory(t)

	cases := map[string]struct {
		subject permission.Subject
		action  string
	}{
		// Deny cases use mallory, an identity outside every adapter's
		// role allowlist. This is deliberate: the rbac adapter
		// evaluates asserted Subject.Roles per request, while the
		// casbin embedded path seeds Options.Roles as persistent
		// groupings, so an allowlisted identity keeps its roles even
		// when asserting none (current behavior, reported as a
		// contract ambiguity -- the kit pins only the agreed deny
		// outcome for non-allowlisted identities).
		//
		// A role with no rules grants nothing: rbac finds no matching
		// rule and Casbin matches no policy, both implicit_deny.
		"NoRuleRole": {subject: permission.Subject{ID: "mallory", Roles: []string{"viewer"}}, action: "doc.read"},
		// An action with no rules grants nothing on either backend.
		"NoRuleAction": {subject: kitSubject(), action: "doc.unknown"},
		// A subject asserting no roles matches no role rule: rbac
		// matchRule requires a role hit and Casbin groupings never
		// match an empty role set.
		"NoRoles": {subject: permission.Subject{ID: "mallory"}, action: "doc.read"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			decision, err := c.Can(t.Context(), tc.subject, tc.action, kitResource())
			if err != nil {
				t.Fatalf("Can() error = %v, want nil", err)
			}
			if decision.Allowed {
				t.Errorf("Can(%v, %q) = allowed, want default-deny", tc.subject, tc.action)
			}
			if decision.Reason != "implicit_deny" {
				t.Errorf("Can() Reason = %q, want implicit_deny", decision.Reason)
			}
		})
	}
}

func conformanceNilCtx(t *testing.T, factory func(t *testing.T) permission.Checker) {
	t.Helper()

	c := factory(t)
	// A nil ctx is treated as context.Background (no deadline to
	// observe), matching the rbac and casbin adapters' current code.
	// Never mint a fresh root from a possibly-cancelled parent: nil
	// means "no ctx", not "cancelled".
	var nilCtx context.Context
	decision, err := c.Can(nilCtx, kitSubject(), "doc.read", kitResource())
	if err != nil {
		t.Fatalf("Can(nil ctx) error = %v, want nil", err)
	}
	if !decision.Allowed {
		t.Error("Can(nil ctx, admin, doc.read) = denied, want allow (Background parity)")
	}
}

// ConformanceCancelledCtx verifies fail-closed behavior under a cancelled
// context: either a non-nil error or a denial. Backends that observe the
// context (casbin checks ctx.Err before touching the enforcer) return an
// error; backends that ignore it but deny anyway (noop) deny. Wiring this
// is per-adapter: the rbac adapter is ctx-insensitive and still allows on
// a cancelled ctx, so it documents that exemption instead of running this.
func ConformanceCancelledCtx(t *testing.T, factory func(t *testing.T) permission.Checker) {
	t.Helper()

	c := factory(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	decision, err := c.Can(ctx, kitSubject(), "doc.read", kitResource())
	if err == nil && decision.Allowed {
		t.Error("Can(cancelled ctx, admin, doc.read) = allowed with nil error, want fail-closed (error or deny)")
	}
}

// ConformanceDenyAll verifies deny-all backends (noop): every check,
// including the canonical allow action and nil/cancelled contexts,
// denies with implicit_deny and a nil error. It is the parity twin of
// Conformance for backends with no allow path.
func ConformanceDenyAll(t *testing.T, factory func(t *testing.T) permission.Checker) {
	t.Helper()

	t.Run("CanonicalAllowDenied", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		// noop.Can unconditionally returns implicit_deny regardless of
		// subject, action, or resource.
		decision, err := c.Can(t.Context(), kitSubject(), "doc.read", kitResource())
		if err != nil {
			t.Fatalf("Can() error = %v, want nil", err)
		}
		if decision.Allowed {
			t.Error("Can(admin, doc.read) = allowed, want deny-all")
		}
		if decision.Reason != "implicit_deny" {
			t.Errorf("Can() Reason = %q, want implicit_deny", decision.Reason)
		}
	})
	t.Run("UnknownDenied", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		decision, err := c.Can(t.Context(), kitSubject(), "doc.unknown", kitResource())
		if err != nil {
			t.Fatalf("Can() error = %v, want nil", err)
		}
		if decision.Allowed {
			t.Error("Can(admin, doc.unknown) = allowed, want deny-all")
		}
	})
	t.Run("NilCtxDenied", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		// noop ignores ctx entirely, so nil is as denied as Background.
		var nilCtx context.Context
		decision, err := c.Can(nilCtx, kitSubject(), "doc.read", kitResource())
		if err != nil {
			t.Fatalf("Can(nil ctx) error = %v, want nil", err)
		}
		if decision.Allowed {
			t.Error("Can(nil ctx) = allowed, want deny-all")
		}
	})
	t.Run("CancelledCtxDenied", func(t *testing.T) {
		t.Helper()
		c := factory(t)
		// noop ignores ctx entirely, so cancellation cannot open a
		// hole: still denied, fail-closed trivially.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		decision, err := c.Can(ctx, kitSubject(), "doc.read", kitResource())
		if err != nil {
			t.Fatalf("Can(cancelled ctx) error = %v, want nil", err)
		}
		if decision.Allowed {
			t.Error("Can(cancelled ctx) = allowed, want deny-all")
		}
	})
}
