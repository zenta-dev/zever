package noop_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/permission/noop"
)

func TestNew_valid_returnsChecker(t *testing.T) {
	t.Parallel()

	c, err := noop.New(permission.Options{})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if c == nil {
		t.Fatal("New() returned nil checker, want non-nil")
	}
}

func TestNew_invalidOptions_returnsError(t *testing.T) {
	t.Parallel()

	opts := permission.Options{
		Rules: []permission.Rule{{Role: "", Action: "read"}},
	}
	if _, err := noop.New(opts); err == nil {
		t.Fatal("New() error = nil, want non-nil for invalid options")
	}
}

func TestCan_alwaysDenies(t *testing.T) {
	t.Parallel()

	c, err := noop.New(permission.Options{})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	tests := []struct {
		name     string
		subject  permission.Subject
		action   string
		resource permission.Resource
	}{
		{
			name:     "admin read document",
			subject:  permission.Subject{ID: "alice", Roles: []string{"admin"}},
			action:   "read",
			resource: permission.Resource{Type: "document", ID: "doc-1"},
		},
		{
			name:     "user with attributes write file",
			subject:  permission.Subject{ID: "bob", Roles: []string{"user"}, Attributes: map[string]string{"dept": "eng"}},
			action:   "write",
			resource: permission.Resource{Type: "file", ID: "f-2", Attributes: map[string]string{"owner": "bob"}},
		},
		{
			name:     "empty subject action resource",
			subject:  permission.Subject{},
			action:   "",
			resource: permission.Resource{},
		},
		{
			name:     "empty action only",
			subject:  permission.Subject{ID: "carol"},
			action:   "",
			resource: permission.Resource{Type: "report", ID: "r-3"},
		},
		{
			name:     "delete with no roles",
			subject:  permission.Subject{ID: "dave"},
			action:   "delete",
			resource: permission.Resource{Type: "document", ID: "doc-9"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := c.Can(context.Background(), tt.subject, tt.action, tt.resource)
			if err != nil {
				t.Fatalf("Can() error = %v, want nil", err)
			}
			if got.Allowed {
				t.Errorf("Can() Allowed = true, want false")
			}
			if got.Reason != "implicit_deny" {
				t.Errorf("Can() Reason = %q, want %q", got.Reason, "implicit_deny")
			}
		})
	}
}

func TestCan_errorIsNil(t *testing.T) {
	t.Parallel()

	c, err := noop.New(permission.Options{})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	d, err := c.Can(context.Background(), permission.Subject{ID: "alice"}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can() error = %v, want nil", err)
	}
	if d.Allowed {
		t.Fatalf("Can() Allowed = true, want false (fail-closed shape)")
	}
}

func TestChecker_implementsInterface(t *testing.T) {
	t.Parallel()

	_ = mustNew(t)
}

func mustNew(t *testing.T) permission.Checker {
	t.Helper()

	c, err := noop.New(permission.Options{})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	return c
}
