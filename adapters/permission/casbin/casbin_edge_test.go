package casbin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// TestEdgeNew_modelPathNotExists proves a ModelPath pointing at a missing file
// fails closed at construction with a wrapped enforcer-creation error.
func TestEdgeNew_modelPathNotExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Options.Validate only checks existence, so the model file exists but is
	// unparseable: the failure must surface from enforcer creation.
	modelPath := filepath.Join(dir, "model.conf")
	if err := os.WriteFile(modelPath, []byte("not a casbin model"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}

	policyPath := filepath.Join(dir, "policy.csv")
	if err := os.WriteFile(policyPath, nil, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	_, err := New(permission.Options{
		ModelPath:  modelPath,
		PolicyPath: policyPath,
	})
	if err == nil {
		t.Fatal("New(missing model) = nil, want error")
	}
	if !strings.Contains(err.Error(), "casbin: creating enforcer") {
		t.Fatalf("New(missing model) err = %v, want enforcer-creation wrap", err)
	}
}

// TestEdgeCan_concurrentSameSubject proves concurrent Can calls for one
// subject share a single transient grouping via the ref-count path and all
// succeed under the race detector.
func TestEdgeCan_concurrentSameSubject(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}

	sub := permission.Subject{ID: "alice", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "1"}
	ctx := t.Context()

	var wg sync.WaitGroup

	errs := make([]error, 16)

	for i := range 16 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			d, err := c.Can(ctx, sub, "read", res)
			if err != nil {
				errs[i] = err
				return
			}
			if !d.Allowed {
				errs[i] = fmt.Errorf("Can = %+v, want allowed", d)
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// TestEdgeCan_emptyRolesSubject proves a subject with no roles and no
// persistent grouping is denied even when it claims nothing.
func TestEdgeCan_emptyRolesSubject(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New() = %v, want nil", err)
	}

	d, err := c.Can(t.Context(), permission.Subject{ID: "carol"}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can() = %v, want nil", err)
	}
	if d.Allowed || d.Reason != "implicit_deny" {
		t.Errorf("Can(no roles) = %+v, want implicit_deny", d)
	}
}
