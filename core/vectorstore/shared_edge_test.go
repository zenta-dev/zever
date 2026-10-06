package vectorstore

import (
	"errors"
	"strings"
	"sync"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
)

func TestRegisterShared_nilFactory_returnsNilFactory(t *testing.T) {
	t.Parallel()

	if err := RegisterShared(freshAdapter(), nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("RegisterShared(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegisterShared_duplicate_returnsDuplicateError(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	ok := func(coredb.DB, Options) (VectorStore, error) { return newStubStore(), nil }

	if err := RegisterShared(a, ok); err != nil {
		t.Fatalf("first RegisterShared err = %v", err)
	}

	if err := RegisterShared(a, ok); !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("second RegisterShared err = %v, want ErrDuplicateAdapter", err)
	}
}

func TestOpenShared_unknownAdapter(t *testing.T) {
	t.Parallel()

	got, err := OpenShared(Adapter("edge-nope"), nil, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("OpenShared unknown err = %v, want ErrUnknownAdapter", err)
	}

	if got != nil {
		t.Fatalf("OpenShared unknown value = %v, want nil", got)
	}
}

func TestOpenShared_invalidOptions(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := RegisterShared(a, func(coredb.DB, Options) (VectorStore, error) { return newStubStore(), nil }); err != nil {
		t.Fatalf("RegisterShared err = %v", err)
	}

	got, err := OpenShared(a, nil, Options{Dimension: -1})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("OpenShared invalid opts err = %v, want ErrInvalidOptions", err)
	}

	if got != nil {
		t.Fatalf("OpenShared invalid opts value = %v, want nil", got)
	}
}

func TestOpenShared_factoryError_wrappedWithPrefix(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	sentinel := errors.New("boom")

	if err := RegisterShared(a, func(coredb.DB, Options) (VectorStore, error) { return nil, sentinel }); err != nil {
		t.Fatalf("RegisterShared err = %v", err)
	}

	got, err := OpenShared(a, nil, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("OpenShared err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "vectorstore: open shared") {
		t.Fatalf("OpenShared err %q missing %q prefix", err.Error(), "vectorstore: open shared")
	}

	if got != nil {
		t.Fatalf("OpenShared error value = %v, want nil", got)
	}
}

func TestRegisterShared_OpenShared_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16

	var wg sync.WaitGroup
	errs := make(chan error, n)
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshAdapter()
			if err := RegisterShared(a, func(coredb.DB, Options) (VectorStore, error) { return newStubStore(), nil }); err != nil {
				errs <- err
				return
			}

			if _, err := OpenShared(a, nil, Options{}); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
