package scheduler

import (
	"errors"
	"strings"
	"sync"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
)

func TestOpenShared_success(t *testing.T) {
	t.Parallel()

	a := benchAdapter()
	if err := RegisterShared(a, func(_ coredb.DB, _ Options) (Scheduler, error) { return fakeScheduler{}, nil }); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	s, err := OpenShared(a, nil, benchOptions())
	if err != nil {
		t.Fatalf("OpenShared(%v) error = %v", a, err)
	}
	if s == nil {
		t.Fatal("OpenShared returned nil Scheduler")
	}
}

func TestRegisterShared_nilFactory(t *testing.T) {
	t.Parallel()

	a := benchAdapter()
	if err := RegisterShared(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("RegisterShared(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegisterShared_duplicate(t *testing.T) {
	t.Parallel()

	a := benchAdapter()
	stub := func(coredb.DB, Options) (Scheduler, error) { return fakeScheduler{}, nil }
	if err := RegisterShared(a, stub); err != nil {
		t.Fatalf("first RegisterShared(%v) error = %v", a, err)
	}
	if err := RegisterShared(a, stub); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second RegisterShared(%v) err = %v, want ErrDuplicate", a, err)
	}
}

func TestOpenShared_unknown(t *testing.T) {
	t.Parallel()

	_, err := OpenShared(benchAdapter(), nil, benchOptions())
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("OpenShared(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func TestOpenShared_invalidOptions(t *testing.T) {
	t.Parallel()

	a := benchAdapter()
	if err := RegisterShared(a, func(coredb.DB, Options) (Scheduler, error) { return fakeScheduler{}, nil }); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	_, err := OpenShared(a, nil, Options{})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("OpenShared(invalid opts) err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpenShared_factoryError_wrapped(t *testing.T) {
	t.Parallel()

	a := benchAdapter()
	sentinel := errors.New("shared boom")
	if err := RegisterShared(a, func(coredb.DB, Options) (Scheduler, error) { return nil, sentinel }); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	_, err := OpenShared(a, nil, benchOptions())
	if !errors.Is(err, sentinel) {
		t.Fatalf("OpenShared err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "scheduler: open shared") {
		t.Fatalf("OpenShared err %q missing %q", err.Error(), "scheduler: open shared")
	}
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := benchAdapter()
			if err := Register(a, func(Options) (Scheduler, error) { return fakeScheduler{}, nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, benchOptions()); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
