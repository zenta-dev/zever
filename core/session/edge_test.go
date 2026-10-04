package session

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

func TestOpenShared_success(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := RegisterShared(a, func(coredb.DB, Options) (Store, error) { return newStubStore(), nil }); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	s, err := OpenShared(a, nil, Options{})
	if err != nil {
		t.Fatalf("OpenShared(%v) error = %v", a, err)
	}
	if s == nil {
		t.Fatal("OpenShared returned nil Store")
	}
}

func TestRegisterShared_nilFactory(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := RegisterShared(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("RegisterShared(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegisterShared_duplicate(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	stub := func(coredb.DB, Options) (Store, error) { return newStubStore(), nil }
	if err := RegisterShared(a, stub); err != nil {
		t.Fatalf("first RegisterShared(%v) error = %v", a, err)
	}
	if err := RegisterShared(a, stub); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second RegisterShared(%v) err = %v, want ErrDuplicate", a, err)
	}
}

func TestOpenShared_unknown(t *testing.T) {
	t.Parallel()

	_, err := OpenShared(freshAdapter(), nil, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("OpenShared(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func TestOpenShared_factoryError_wrapped(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	sentinel := errors.New("shared boom")
	if err := RegisterShared(a, func(coredb.DB, Options) (Store, error) { return nil, sentinel }); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	_, err := OpenShared(a, nil, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("OpenShared err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "session: open shared") {
		t.Fatalf("OpenShared err %q missing %q", err.Error(), "session: open shared")
	}
}

func TestValidateID_length_boundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"63 chars", strings.Repeat("a", 63), true},
		{"64 lowercase hex", strings.Repeat("ab12", 16), false},
		{"65 chars", strings.Repeat("a", 65), true},
		{"uppercase hex", strings.Repeat("A", 64), true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateID(tc.id)
			if tc.wantErr && !errors.Is(err, ErrInvalidID) {
				t.Fatalf("ValidateID(len=%d) err = %v, want ErrInvalidID", len(tc.id), err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateID(len=%d) err = %v, want nil", len(tc.id), err)
			}
		})
	}
}

func TestNewSession_negative_ttl_no_expiry(t *testing.T) {
	t.Parallel()

	s := NewSession(NewID(), -time.Hour)
	if !s.ExpiresAt.IsZero() {
		t.Fatalf("negative ttl ExpiresAt = %v, want zero", s.ExpiresAt)
	}
}

func TestStore_concurrent_create_get(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newStubStore()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			created, err := st.Create(ctx, time.Hour)
			if err != nil {
				t.Errorf("Create() error = %v", err)
				return
			}
			if _, err := st.Get(ctx, created.ID); err != nil {
				t.Errorf("Get(%s) error = %v", created.ID, err)
			}
		}()
	}

	wg.Wait()
}

func TestRegister_Open_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshAdapter()
			if err := Register(a, func(Options) (Store, error) { return newStubStore(), nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
