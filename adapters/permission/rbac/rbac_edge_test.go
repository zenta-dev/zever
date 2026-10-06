package rbac_test

import (
	"context"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/adapters/permission/rbac"
	"github.com/zenta-dev/zever/core/permission"
)

func TestCan_concurrentSafe(t *testing.T) {
	t.Parallel()

	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{{Role: "admin", Action: "read"}},
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	sub := permission.Subject{ID: "u1", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "d1"}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := c.Can(t.Context(), sub, "read", res)
			if err != nil || !d.Allowed {
				t.Errorf("Can() = (%+v, %v), want allowed nil error", d, err)
			}
		}()
	}
	wg.Wait()
}

func TestCan_duplicateAllowRules_stillAllow(t *testing.T) {
	t.Parallel()

	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "read"},
			{Role: "admin", Action: "read"},
		},
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "u1", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc"})
	if err != nil || !d.Allowed || d.Reason != "allow" {
		t.Errorf("Can() = (%+v, %v), want allow", d, err)
	}
}

func TestCan_denyForOneRoleDoesNotBlockOther(t *testing.T) {
	t.Parallel()

	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "read"},
			{Role: "banned", Action: "read", Effect: permission.Deny},
		},
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "u1", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc"})
	if err != nil || !d.Allowed {
		t.Errorf("Can(admin) = (%+v, %v), want allow", d, err)
	}

	d, err = c.Can(t.Context(), permission.Subject{ID: "u2", Roles: []string{"banned"}}, "read", permission.Resource{Type: "doc"})
	if err != nil || d.Allowed || d.Reason != "explicit_deny" {
		t.Errorf("Can(banned) = (%+v, %v), want explicit_deny", d, err)
	}
}

// TestEdgeCan_canceledContextStillDecides proves the checker consults no
// external state, so a canceled context does not change the decision (nil is
// treated as background; a canceled ctx is simply never observed).
func TestEdgeCan_canceledContextStillDecides(t *testing.T) {
	t.Parallel()

	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{{Role: "admin", Action: "read"}},
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d, err := c.Can(ctx, permission.Subject{ID: "u1", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc"})
	if err != nil {
		t.Fatalf("Can(canceled) = %v, want nil", err)
	}
	if !d.Allowed || d.Reason != "allow" {
		t.Errorf("Can(canceled) = %+v, want allow", d)
	}
}

// TestEdgeCan_ownedOnlyWildcardRole proves an OwnedOnly rule with a wildcard
// role still requires an authenticated subject whose ID matches the owner
// attribute.
func TestEdgeCan_ownedOnlyWildcardRole(t *testing.T) {
	t.Parallel()

	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{
			{Role: "*", Action: "write", OwnedOnly: true, OwnedAttr: "owner"},
		},
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	tests := []struct {
		name    string
		sub     permission.Subject
		res     permission.Resource
		allowed bool
		reason  string
	}{
		{
			name:    "owner match",
			sub:     permission.Subject{ID: "u1", Roles: []string{"editor"}},
			res:     permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u1"}},
			allowed: true,
			reason:  "allow",
		},
		{
			name:    "owner mismatch",
			sub:     permission.Subject{ID: "u1", Roles: []string{"editor"}},
			res:     permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u2"}},
			allowed: false,
			reason:  "implicit_deny",
		},
		{
			name:    "anonymous denied",
			sub:     permission.Subject{},
			res:     permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u1"}},
			allowed: false,
			reason:  "implicit_deny",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d, err := c.Can(t.Context(), tc.sub, "write", tc.res)
			if err != nil {
				t.Fatalf("Can() = %v, want nil", err)
			}
			if d.Allowed != tc.allowed || d.Reason != tc.reason {
				t.Errorf("Can() = %+v, want allowed=%v reason=%q", d, tc.allowed, tc.reason)
			}
		})
	}
}

// TestEdgeCan_duplicateDenyRules proves repeated deny rules behave like a
// single deny: explicit_deny, no error.
func TestEdgeCan_duplicateDenyRules(t *testing.T) {
	t.Parallel()

	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "delete"},
			{Role: "admin", Action: "delete", Effect: permission.Deny},
			{Role: "admin", Action: "delete", Effect: permission.Deny},
		},
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "u1", Roles: []string{"admin"}}, "delete", permission.Resource{Type: "doc"})
	if err != nil {
		t.Fatalf("Can() = %v, want nil", err)
	}
	if d.Allowed || d.Reason != "explicit_deny" {
		t.Errorf("Can() = %+v, want explicit_deny", d)
	}
}
