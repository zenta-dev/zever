package casbin

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/permission"
)

func allowOpts() permission.Options {
	return permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "read"},
		},
		Roles: map[string][]string{
			"alice": {"admin"},
		},
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	cases := map[string]permission.Options{
		"empty rule role": {
			Rules: []permission.Rule{{Action: "read"}},
		},
		"bad rule effect": {
			Rules: []permission.Rule{{Role: "admin", Action: "read", Effect: "maybe"}},
		},
		"model without policy": {
			ModelPath: "model.conf",
		},
		"empty role name": {
			Roles: map[string][]string{"": {"admin"}},
		},
	}

	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := New(opts); err == nil {
				t.Fatalf("New(%q) = nil error, want validation error", name)
			}
		})
	}
}

func TestCanAllowViaSeededRule(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can: %v", err)
	}

	if !d.Allowed || d.Reason != "allow" {
		t.Fatalf("Can = %+v, want {Allowed:true Reason:allow}", d)
	}
}

func TestCanDenyOverrideWins(t *testing.T) {
	t.Parallel()

	opts := permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "write"},
			{Role: "admin", Action: "write", Effect: permission.Deny},
		},
		Roles: map[string][]string{
			"alice": {"admin"},
		},
	}

	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "alice", Roles: []string{"admin"}}, "write", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can: %v", err)
	}

	if d.Allowed || d.Reason != "explicit_deny" {
		t.Fatalf("Can = %+v, want {Allowed:false Reason:explicit_deny}", d)
	}
}

func TestCanImplicitDenyWithoutRule(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "alice", Roles: []string{"admin"}}, "delete", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can: %v", err)
	}

	if d.Allowed || d.Reason != "implicit_deny" {
		t.Fatalf("Can = %+v, want {Allowed:false Reason:implicit_deny}", d)
	}
}

func TestCanPrivEscBlocked(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// bob claims admin but allowlist only grants alice -> admin.
	d, err := c.Can(t.Context(), permission.Subject{ID: "bob", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can: %v", err)
	}

	if d.Allowed {
		t.Fatalf("Can = %+v, want denied privilege escalation", d)
	}
}

func TestCanNilContext(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	d, err := c.Can(nil, permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"}) //nolint:staticcheck // exercising nil-ctx robustness
	if err != nil {
		t.Fatalf("Can(nil ctx): %v", err)
	}

	if !d.Allowed {
		t.Fatalf("Can(nil ctx) = %+v, want allowed", d)
	}
}

func TestCanConcurrentDistinctSubjects(t *testing.T) {
	t.Parallel()

	roles := make(map[string][]string, 20)
	for i := range 20 {
		roles[fmt.Sprintf("user-%d", i)] = []string{"admin"}
	}

	opts := permission.Options{
		Rules: []permission.Rule{{Role: "admin", Action: "read"}},
		Roles: roles,
	}

	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var wg sync.WaitGroup

	errs := make([]error, 20)

	for i := range 20 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			id := fmt.Sprintf("user-%d", i)
			d, err := c.Can(t.Context(), permission.Subject{ID: id, Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})

			if err != nil {
				errs[i] = err
				return
			}

			if !d.Allowed {
				errs[i] = fmt.Errorf("Can(%s) = %+v, want allowed", id, d)
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
}

const fileModel = `[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (p.obj == "*" || r.obj == p.obj) && g(r.sub, p.sub) && r.act == p.act
`

func TestNewFromModelPolicyFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	modelPath := filepath.Join(dir, "model.conf")
	policyPath := filepath.Join(dir, "policy.csv")

	if err := os.WriteFile(modelPath, []byte(fileModel), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}

	policy := "p, admin, *, read, allow\n\ng, bob, admin\n"
	if err := os.WriteFile(policyPath, []byte(policy), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	c, err := New(permission.Options{ModelPath: modelPath, PolicyPath: policyPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sub := permission.Subject{ID: "bob"}
	res := permission.Resource{Type: "doc", ID: "1"}

	// Twice: persistent file grouping must survive cleanup after first Can.
	for i := range 2 {
		d, err := c.Can(t.Context(), sub, "read", res)
		if err != nil {
			t.Fatalf("Can #%d: %v", i, err)
		}

		if !d.Allowed {
			t.Fatalf("Can #%d = %+v, want allowed via file policy", i, d)
		}
	}
}
