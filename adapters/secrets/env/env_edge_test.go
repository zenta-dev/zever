package env

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// TestConcurrentGetList exercises Get and List from many goroutines; the
// adapter is stateless apart from its immutable prefix. t.Setenv is
// incompatible with t.Parallel, so this test runs serially.
func TestConcurrentGetList(t *testing.T) {
	t.Setenv("TCONC_GREETING", "hello")

	a, err := New(Options{Prefix: "TCONC_"})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	ctx := t.Context()

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			if i%2 == 0 {
				val, gerr := a.Get(ctx, "GREETING")
				if gerr != nil {
					t.Errorf("Get() err = %v, want nil", gerr)
					return
				}

				if string(val) != "hello" {
					t.Errorf("Get() = %q, want %q", val, "hello")
				}

				return
			}

			if _, lerr := a.List(ctx); lerr != nil {
				t.Errorf("List() err = %v, want nil", lerr)
			}
		}(i)
	}

	wg.Wait()
}

// TestGetMissingNameNotListed verifies a missing name reports ErrNotFound and
// is absent from List, guarding the boundary between the two read paths.
func TestGetMissingNameNotListed(t *testing.T) {
	t.Parallel()

	a, err := New(Options{Prefix: "TABSENT_"})
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}

	if _, getErr := a.Get(t.Context(), "NOPE"); !errors.Is(getErr, secrets.ErrNotFound) {
		t.Fatalf("Get(missing) err = %v, want ErrNotFound", getErr)
	}

	keys, err := a.List(t.Context())
	if err != nil {
		t.Fatalf("List() err = %v, want nil", err)
	}

	for _, k := range keys {
		if k == "NOPE" {
			t.Fatalf("List() = %v, must not contain missing name", keys)
		}
	}
}
