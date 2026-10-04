package casbin

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/permission"
)

func TestNew_duplicateRule_returnsError(t *testing.T) {
	t.Parallel()

	opts := permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "read"},
			{Role: "admin", Action: "read"},
		},
	}
	_, err := New(opts)
	if !errors.Is(err, ErrDuplicatePolicy) {
		t.Fatalf("New(duplicate) = %v, want ErrDuplicatePolicy", err)
	}

	var dup *DuplicatePolicyError
	if !errors.As(err, &dup) {
		t.Fatalf("err %T is not *DuplicatePolicyError", err)
	}
	if dup.Role != "admin" || dup.Action != "read" {
		t.Errorf("DuplicatePolicyError = %+v, want admin/read", dup)
	}
}

func TestCan_emptySubject_implicitDeny(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	d, err := c.Can(t.Context(), permission.Subject{}, "", permission.Resource{})
	if err != nil {
		t.Fatalf("Can(empty) = %v, want nil", err)
	}
	if d.Allowed || d.Reason != "implicit_deny" {
		t.Errorf("Can(empty) = %+v, want implicit_deny", d)
	}
}

func TestCan_unallowlistedRole_implicitDeny(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	d, err := c.Can(t.Context(), permission.Subject{ID: "bob", Roles: []string{"superadmin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can() = %v, want nil", err)
	}
	if d.Allowed {
		t.Errorf("Can(unallowlisted role) = %+v, want denied", d)
	}
}
