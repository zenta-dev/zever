package rbac_test

import (
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
