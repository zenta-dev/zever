package registry_test

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/shared/registry"
)

type adapter int

const (
	adapterA adapter = iota
	adapterB
)

// namedAdapter is a string adapter key with a String method, mirroring the
// core/<battery> Adapter types.
type namedAdapter string

// String returns the adapter name.
func (a namedAdapter) String() string { return string(a) }

type factory func() string

var (
	errNil     = errors.New("test: nil factory")
	errDup     = errors.New("test: duplicate registration")
	errUnknown = errors.New("test: unknown adapter")
)

type dupError struct{ Adapter adapter }

func (e *dupError) Error() string { return fmt.Sprintf("%s: %d", errDup, int(e.Adapter)) }
func (e *dupError) Unwrap() error { return errDup }

type unknownError struct{ Adapter adapter }

func (e *unknownError) Error() string { return fmt.Sprintf("%s: %d", errUnknown, int(e.Adapter)) }
func (e *unknownError) Unwrap() error { return errUnknown }

func newTestRegistry() *registry.Registry[adapter, factory] {
	return registry.New[adapter, factory](
		errNil,
		func(a adapter) error { return &dupError{Adapter: a} },
		func(a adapter) error { return &unknownError{Adapter: a} },
	)
}

func TestRegisterAndLookupRoundTrip(t *testing.T) {
	t.Parallel()

	r := newTestRegistry()
	want := "ok"

	if err := r.Register(adapterA, func() string { return want }); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	f, err := r.Lookup(adapterA)
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if got := f(); got != want {
		t.Fatalf("factory returned %q, want %q", got, want)
	}
}

func TestRegisterDuplicateGuard(t *testing.T) {
	t.Parallel()

	r := newTestRegistry()

	if err := r.Register(adapterA, func() string { return "a" }); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	err := r.Register(adapterA, func() string { return "b" })
	if err == nil {
		t.Fatal("second Register succeeded, want duplicate error")
	}

	if !errors.Is(err, errDup) {
		t.Fatalf("duplicate error does not unwrap to sentinel: %v", err)
	}

	// Original factory must win.
	f, lerr := r.Lookup(adapterA)
	if lerr != nil {
		t.Fatalf("Lookup failed: %v", lerr)
	}

	if got := f(); got != "a" {
		t.Fatalf("factory overwritten on duplicate: got %q, want %q", got, "a")
	}
}

func TestRegisterNilFactory(t *testing.T) {
	t.Parallel()

	r := newTestRegistry()

	if err := r.Register(adapterA, nil); !errors.Is(err, errNil) {
		t.Fatalf("Register(nil) = %v, want error wrapping %v", err, errNil)
	}
}

func TestLookupUnknown(t *testing.T) {
	t.Parallel()

	r := newTestRegistry()

	_, err := r.Lookup(adapterB)
	if err == nil {
		t.Fatal("Lookup of unregistered adapter succeeded, want unknown error")
	}

	if !errors.Is(err, errUnknown) {
		t.Fatalf("unknown error does not unwrap to sentinel: %v", err)
	}
}

func TestNamesListsRegistered(t *testing.T) {
	t.Parallel()

	r := newTestRegistry()

	if got := r.Names(); len(got) != 0 {
		t.Fatalf("Names on empty registry = %v, want empty", got)
	}

	if err := r.Register(adapterA, func() string { return "a" }); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(adapterB, func() string { return "b" }); err != nil {
		t.Fatal(err)
	}

	got := r.Names()
	if len(got) != 2 || !slices.Contains(got, adapterA) || !slices.Contains(got, adapterB) {
		t.Fatalf("Names = %v, want both registered adapters", got)
	}
}

func TestSortedNamesAreSortedStrings(t *testing.T) {
	t.Parallel()

	r := registry.New[namedAdapter, factory](
		errNil,
		func(namedAdapter) error { return &dupError{Adapter: adapterA} },
		func(namedAdapter) error { return &unknownError{Adapter: adapterA} },
	)

	for _, name := range []namedAdapter{"redis", "memory", "db"} {
		if err := r.Register(name, func() string { return string(name) }); err != nil {
			t.Fatal(err)
		}
	}

	got := registry.SortedNames(r)
	want := []string{"db", "memory", "redis"}
	if !slices.Equal(got, want) {
		t.Fatalf("SortedNames = %v, want %v", got, want)
	}
}

func TestConcurrentUse(t *testing.T) {
	t.Parallel()

	r := newTestRegistry()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			_ = r.Register(adapterA, func() string { return "a" })
			_, _ = r.Lookup(adapterA)
			_, _ = r.Lookup(adapterB)
		}()
	}

	wg.Wait()

	if _, err := r.Lookup(adapterA); err != nil {
		t.Fatalf("Lookup after concurrent use failed: %v", err)
	}
}

func TestNilHooks(t *testing.T) {
	t.Parallel()

	r := registry.New[adapter, factory](errNil, nil, nil)

	if err := r.Register(adapterA, func() string { return "a" }); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	err := r.Register(adapterA, func() string { return "b" })
	if err == nil {
		t.Fatal("duplicate with nil hook must return an error, got nil")
	}

	if !errors.Is(err, registry.ErrDuplicate) {
		t.Fatalf("duplicate error does not unwrap to ErrDuplicate: %v", err)
	}

	if _, err := r.Lookup(adapterB); err == nil {
		t.Fatal("unknown with nil hook must return an error, got nil")
	} else if !errors.Is(err, registry.ErrUnknown) {
		t.Fatalf("unknown error does not wrap ErrUnknown: %v", err)
	}
}

func TestNilErrNilFactory(t *testing.T) {
	t.Parallel()

	r := registry.New[adapter, factory](nil, nil, nil)

	if err := r.Register(adapterA, nil); !errors.Is(err, registry.ErrNilFactory) {
		t.Fatalf("Register(nil) = %v, want error wrapping %v", err, registry.ErrNilFactory)
	}
}

func TestNonFuncFactoryType(t *testing.T) {
	t.Parallel()

	r := registry.New[string, int](errNil, nil, nil)

	if err := r.Register("a", 42); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	v, err := r.Lookup("a")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}

	if v != 42 {
		t.Fatalf("Lookup returned %d, want 42", v)
	}
}

func TestInterfaceFactoryNil(t *testing.T) {
	t.Parallel()

	r := registry.New[string, any](errNil, nil, nil)

	if err := r.Register("a", nil); !errors.Is(err, errNil) {
		t.Fatalf("Register(nil) = %v, want %v", err, errNil)
	}
}
