package rbac_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/permission/rbac"
)

func TestCan_matrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rules    []permission.Rule
		subject  permission.Subject
		action   string
		resource permission.Resource
		allowed  bool
		reason   string
	}{
		{
			name:     "exact allow",
			rules:    []permission.Rule{{Role: "admin", Action: "read"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  true,
			reason:   "allow",
		},
		{
			name:     "wildcard action",
			rules:    []permission.Rule{{Role: "admin", Action: "*"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "delete",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  true,
			reason:   "allow",
		},
		{
			name:     "wildcard role authenticated",
			rules:    []permission.Rule{{Role: "*", Action: "read"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"viewer"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  true,
			reason:   "allow",
		},
		{
			name:     "wildcard role anonymous empty ID denied",
			rules:    []permission.Rule{{Role: "*", Action: "read"}},
			subject:  permission.Subject{ID: "", Roles: []string{"viewer"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "wildcard role anonymous no roles denied",
			rules:    []permission.Rule{{Role: "*", Action: "read"}},
			subject:  permission.Subject{ID: "u1", Roles: nil},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "wildcard role fully anonymous denied",
			rules:    []permission.Rule{{Role: "*", Action: "read"}},
			subject:  permission.Subject{},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "owned only match",
			rules:    []permission.Rule{{Role: "editor", Action: "write", OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u1"}},
			allowed:  true,
			reason:   "allow",
		},
		{
			name:     "owned only mismatch",
			rules:    []permission.Rule{{Role: "editor", Action: "write", OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u2"}},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "owned only missing attr",
			rules:    []permission.Rule{{Role: "editor", Action: "write", OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name: "explicit deny wins over allow",
			rules: []permission.Rule{
				{Role: "admin", Action: "delete"},
				{Role: "admin", Action: "delete", Effect: permission.Deny},
			},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "delete",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "explicit_deny",
		},
		{
			name:     "deny wildcard action blocks exact allow",
			rules:    []permission.Rule{{Role: "admin", Action: "read"}, {Role: "admin", Action: "*", Effect: permission.Deny}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "explicit_deny",
		},
		{
			name:     "explicit deny reason",
			rules:    []permission.Rule{{Role: "viewer", Action: "read", Effect: permission.Deny}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"viewer"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "explicit_deny",
		},
		{
			name:     "empty rules deny all",
			rules:    nil,
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "unknown action deny",
			rules:    []permission.Rule{{Role: "admin", Action: "read"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "role mismatch deny",
			rules:    []permission.Rule{{Role: "admin", Action: "read"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"viewer"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := rbac.New(permission.Options{Rules: tt.rules})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			got, err := c.Can(context.Background(), tt.subject, tt.action, tt.resource)
			if err != nil {
				t.Fatalf("Can() error = %v", err)
			}
			if got.Allowed != tt.allowed {
				t.Errorf("Can() Allowed = %v, want %v", got.Allowed, tt.allowed)
			}
			if got.Reason != tt.reason {
				t.Errorf("Can() Reason = %q, want %q", got.Reason, tt.reason)
			}
		})
	}
}

func TestCan_nilContext_treatedAsBackground(t *testing.T) {
	t.Parallel()

	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{{Role: "admin", Action: "read"}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := c.Can(nil, permission.Subject{ID: "u1", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc"}) //nolint:staticcheck // exercising nil-ctx robustness
	if err != nil {
		t.Fatalf("Can() error = %v", err)
	}
	if !got.Allowed || got.Reason != "allow" {
		t.Errorf("Can() = %+v, want {Allowed:true Reason:allow}", got)
	}
}

func TestNew_invalidOptions_returnsError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts permission.Options
	}{
		{name: "empty role", opts: permission.Options{Rules: []permission.Rule{{Role: "", Action: "read"}}}},
		{name: "empty action", opts: permission.Options{Rules: []permission.Rule{{Role: "admin", Action: ""}}}},
		{name: "bad effect", opts: permission.Options{Rules: []permission.Rule{{Role: "admin", Action: "read", Effect: "maybe"}}}},
		{name: "owned-only without attr", opts: permission.Options{Rules: []permission.Rule{{Role: "admin", Action: "read", OwnedOnly: true}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := rbac.New(tc.opts); err == nil {
				t.Errorf("New() error = nil, want non-nil")
			}
		})
	}
}

func BenchmarkCan_allow(b *testing.B) {
	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{{Role: "admin", Action: "read"}},
	})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	sub := permission.Subject{ID: "u1", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "d1"}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Can(ctx, sub, "read", res); err != nil {
			b.Fatalf("Can() error = %v", err)
		}
	}
}
