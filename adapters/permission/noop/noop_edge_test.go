package noop_test

import (
	"sync"
	"testing"

	"github.com/zenta-dev/zever/adapters/permission/noop"
	"github.com/zenta-dev/zever/core/permission"
)

func TestCan_nilContext_denies(t *testing.T) {
	t.Parallel()

	c, err := noop.New(permission.Options{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	d, err := c.Can(nil, permission.Subject{ID: "u1"}, "read", permission.Resource{Type: "doc", ID: "1"}) //nolint:staticcheck // deliberately exercises nil-context handling
	if err != nil {
		t.Fatalf("Can(nil) = %v, want nil", err)
	}
	if d.Allowed || d.Reason != "implicit_deny" {
		t.Errorf("Can(nil) = %+v, want implicit_deny", d)
	}
}

func TestCan_concurrentSafe(t *testing.T) {
	t.Parallel()

	c, err := noop.New(permission.Options{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if d, err := c.Can(t.Context(), permission.Subject{ID: "u"}, "read", permission.Resource{Type: "doc"}); err != nil || d.Allowed {
				t.Errorf("Can() = (%+v, %v), want denied nil error", d, err)
			}
		}()
	}
	wg.Wait()
}

func TestRegister_wiresNoopFactory(t *testing.T) {
	noop.Register()

	c, err := permission.Open(permission.Noop, permission.Options{})
	if err != nil {
		t.Fatalf("Open(noop) = %v, want nil", err)
	}
	d, err := c.Can(t.Context(), permission.Subject{ID: "u1"}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err != nil {
		t.Fatalf("Can() = %v, want nil", err)
	}
	if d.Allowed || d.Reason != "implicit_deny" {
		t.Errorf("Can() = %+v, want implicit_deny", d)
	}
}
