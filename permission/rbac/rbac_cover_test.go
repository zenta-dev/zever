package rbac

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/permission"
)

func TestCover_MatchRule_ActionGate(t *testing.T) {
	t.Parallel()
	sub := permission.Subject{ID: "u1", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "d1"}
	if !matchRule(sub, "read", res, permission.Rule{Role: "admin", Action: "read"}) {
		t.Error("exact-action match = false, want true")
	}
	if !matchRule(sub, "delete", res, permission.Rule{Role: "admin", Action: "*"}) {
		t.Error("wildcard-action match = false, want true")
	}
	if matchRule(sub, "write", res, permission.Rule{Role: "admin", Action: "read"}) {
		t.Error("non-matching action match = true, want false")
	}
}

func TestCover_MatchRule_RoleWildcard(t *testing.T) {
	t.Parallel()
	res := permission.Resource{Type: "doc", ID: "d1"}
	cases := []struct {
		name string
		sub  permission.Subject
		want bool
	}{
		{"authenticated", permission.Subject{ID: "u1", Roles: []string{"viewer"}}, true},
		{"empty ID", permission.Subject{ID: "", Roles: []string{"viewer"}}, false},
		{"no roles", permission.Subject{ID: "u1", Roles: nil}, false},
		{"fully anonymous", permission.Subject{}, false},
	}
	for _, tc := range cases {
		if got := matchRule(tc.sub, "read", res, permission.Rule{Role: "*", Action: "read"}); got != tc.want {
			t.Errorf("%s: matchRule = %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestCover_MatchRule_OwnedOnly(t *testing.T) {
	t.Parallel()
	sub := permission.Subject{ID: "u1", Roles: []string{"editor"}}
	cases := []struct {
		name string
		res  permission.Resource
		want bool
	}{
		{"match", permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u1"}}, true},
		{"wrong owner", permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u2"}}, false},
		{"nil map", permission.Resource{Type: "doc", ID: "d1"}, false},
		{"missing key", permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"other": "u1"}}, false},
	}
	for _, tc := range cases {
		rule := permission.Rule{Role: "editor", Action: "write", OwnedOnly: true, OwnedAttr: "owner"}
		if got := matchRule(sub, "write", tc.res, rule); got != tc.want {
			t.Errorf("%s: matchRule = %v want %v", tc.name, got, tc.want)
		}
		deny := permission.Rule{Role: "editor", Action: "write", Effect: permission.Deny, OwnedOnly: true, OwnedAttr: "owner"}
		if got := matchRule(sub, "write", tc.res, deny); got != tc.want {
			t.Errorf("%s deny: matchRule = %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestCover_Can_Reasons(t *testing.T) {
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
			name:     "star action allows any",
			rules:    []permission.Rule{{Role: "admin", Action: "*"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "publish",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  true,
			reason:   "allow",
		},
		{
			name:     "action mismatch implicit deny",
			rules:    []permission.Rule{{Role: "admin", Action: "read"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "star role empty ID implicit deny",
			rules:    []permission.Rule{{Role: "*", Action: "read"}},
			subject:  permission.Subject{ID: "", Roles: []string{"viewer"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "star role no roles implicit deny",
			rules:    []permission.Rule{{Role: "*", Action: "read"}},
			subject:  permission.Subject{ID: "u1", Roles: nil},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "owned allow match",
			rules:    []permission.Rule{{Role: "editor", Action: "write", OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u1"}},
			allowed:  true,
			reason:   "allow",
		},
		{
			name:     "owned allow nil map mismatch",
			rules:    []permission.Rule{{Role: "editor", Action: "write", OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "owned allow wrong owner mismatch",
			rules:    []permission.Rule{{Role: "editor", Action: "write", OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u2"}},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name:     "owned deny match explicit deny",
			rules:    []permission.Rule{{Role: "editor", Action: "write", Effect: permission.Deny, OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u1"}},
			allowed:  false,
			reason:   "explicit_deny",
		},
		{
			name:     "owned deny mismatch implicit deny",
			rules:    []permission.Rule{{Role: "editor", Action: "write", Effect: permission.Deny, OwnedOnly: true, OwnedAttr: "owner"}},
			subject:  permission.Subject{ID: "u1", Roles: []string{"editor"}},
			action:   "write",
			resource: permission.Resource{Type: "doc", ID: "d1", Attributes: map[string]string{"owner": "u2"}},
			allowed:  false,
			reason:   "implicit_deny",
		},
		{
			name: "deny wins over allow",
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
			name: "wildcard deny beats exact allow",
			rules: []permission.Rule{
				{Role: "admin", Action: "read"},
				{Role: "admin", Action: "*", Effect: permission.Deny},
			},
			subject:  permission.Subject{ID: "u1", Roles: []string{"admin"}},
			action:   "read",
			resource: permission.Resource{Type: "doc", ID: "d1"},
			allowed:  false,
			reason:   "explicit_deny",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotChecker, err := New(permission.Options{Rules: tt.rules})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := gotChecker.Can(context.Background(), tt.subject, tt.action, tt.resource)
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

func TestCover_New_ValidatePassthrough(t *testing.T) {
	t.Parallel()
	_, err := New(permission.Options{Rules: []permission.Rule{{Role: "", Action: "read"}}})
	if err == nil {
		t.Fatal("New() error = nil, want non-nil")
	}
	if !errors.Is(err, permission.ErrInvalidOptions) {
		t.Errorf("New() err %v does not match ErrInvalidOptions", err)
	}
	if !strings.HasPrefix(err.Error(), "rbac: ") {
		t.Errorf("New() err %q missing %q prefix", err.Error(), "rbac: ")
	}
}

func TestCover_Can_NilCtx(t *testing.T) {
	t.Parallel()
	gotChecker, err := New(permission.Options{Rules: []permission.Rule{{Role: "admin", Action: "read"}}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	got, err := gotChecker.Can(nil, permission.Subject{ID: "u1", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc"}) //nolint:staticcheck // exercising nil-ctx robustness.
	if err != nil {
		t.Fatalf("Can() error = %v", err)
	}
	if !got.Allowed || got.Reason != "allow" {
		t.Errorf("Can() = %+v, want {Allowed:true Reason:allow}", got)
	}
}
