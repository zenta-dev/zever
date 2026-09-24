package session

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var freshAdapterCounter int64

func freshAdapter() Adapter {
	return Adapter(1000 + atomic.AddInt64(&freshAdapterCounter, 1))
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		timer := time.NewTimer(time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s", msg)
		case <-timer.C:
		}
	}
}

func TestNewSession_fields_ttl_applied(t *testing.T) {
	t.Parallel()
	before := time.Now()
	s := NewSession(NewID(), time.Hour)
	after := time.Now()
	if s.ID == "" {
		t.Fatal("NewSession ID is empty")
	}
	if s.Data == nil {
		t.Fatal("NewSession Data is nil, want empty map")
	}
	if len(s.Data) != 0 {
		t.Fatalf("NewSession Data len = %d, want 0", len(s.Data))
	}
	if s.CreatedAt.Before(before) || s.CreatedAt.After(after) {
		t.Fatalf("NewSession CreatedAt = %v out of range", s.CreatedAt)
	}
	if s.UpdatedAt.Before(before) || s.UpdatedAt.After(after) {
		t.Fatalf("NewSession UpdatedAt = %v out of range", s.UpdatedAt)
	}
	if want := s.CreatedAt.Add(time.Hour); !s.ExpiresAt.Equal(want) {
		t.Fatalf("NewSession ExpiresAt = %v, want %v", s.ExpiresAt, want)
	}
}

func TestNewSession_zero_ttl_no_expiry(t *testing.T) {
	t.Parallel()
	s := NewSession(NewID(), 0)
	if !s.ExpiresAt.IsZero() {
		t.Fatalf("NewSession zero ttl ExpiresAt = %v, want zero", s.ExpiresAt)
	}
	if s.Expired(time.Now()) {
		t.Fatal("NewSession zero ttl must not be expired")
	}
}

func TestSession_Clone_independent(t *testing.T) {
	t.Parallel()
	s := NewSession(NewID(), time.Hour)
	s.Data["nested"] = map[string]any{"x": 1}
	s.Data["list"] = []any{1, "two"}
	s.Data["bytes"] = []byte{1, 2, 3}
	c := s.Clone()
	if c.ID != s.ID || !c.CreatedAt.Equal(s.CreatedAt) || !c.ExpiresAt.Equal(s.ExpiresAt) {
		t.Fatal("Clone lost identity/timestamp fields")
	}
	nested, ok := s.Data["nested"].(map[string]any)
	if !ok {
		t.Fatal("nested value lost map type")
	}
	nested["x"] = 999
	list, ok := s.Data["list"].([]any)
	if !ok {
		t.Fatal("list value lost slice type")
	}
	list[0] = 999
	by, ok := s.Data["bytes"].([]byte)
	if !ok {
		t.Fatal("bytes value lost type")
	}
	by[0] = 9
	s.Data["new"] = true
	if cn, ok := c.Data["nested"].(map[string]any); !ok || cn["x"] != 1 {
		t.Fatal("Clone shares nested map with original")
	}
	if cl, ok := c.Data["list"].([]any); !ok || cl[0] != 1 {
		t.Fatal("Clone shares slice with original")
	}
	if cb, ok := c.Data["bytes"].([]byte); !ok || cb[0] != 1 {
		t.Fatal("Clone shares bytes with original")
	}
	if _, ok := c.Data["new"]; ok {
		t.Fatal("Clone shares top-level map with original")
	}
}

func TestSession_Clone_nil_data(t *testing.T) {
	t.Parallel()
	s := Session{ID: NewID()}
	if c := s.Clone(); c.Data != nil {
		t.Fatalf("Clone nil Data = %v, want nil", c.Data)
	}
}

func TestSession_Expired_table(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cases := []struct {
		name    string
		session Session
		want    bool
	}{
		{"zero_never", Session{}, false},
		{"past_true", Session{ExpiresAt: now.Add(-time.Second)}, true},
		{"exact_true", Session{ExpiresAt: now}, true},
		{"future_false", Session{ExpiresAt: now.Add(time.Hour)}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.session.Expired(now); got != tc.want {
				t.Fatalf("Expired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRegister_nil_factory_fails(t *testing.T) {
	a := freshAdapter()
	err := Register(a, nil)
	if err == nil {
		t.Fatal("Register(nil) expected error, got nil")
	}
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_fails(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Store, error) { return newStubStore(), nil }); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	err := Register(a, func(Options) (Store, error) { return newStubStore(), nil })
	if err == nil {
		t.Fatal("duplicate Register expected error, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate Register err = %v, want ErrDuplicate", err)
	}
	var de *DuplicateError
	if !errors.As(err, &de) {
		t.Fatalf("duplicate Register err type = %T, want *DuplicateError", err)
	}
	if de.Adapter != a {
		t.Fatalf("DuplicateError.Adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_unknown_adapter_fails(t *testing.T) {
	a := freshAdapter()
	_, err := Open(a, Options{})
	if err == nil {
		t.Fatal("Open unknown expected error, got nil")
	}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("Open unknown err type = %T, want *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("UnknownAdapterError.Adapter = %v, want %v", ue.Adapter, a)
	}
}

func TestOpen_factory_error_wraps(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Store, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if err == nil {
		t.Fatal("Open expected factory error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrapped sentinel", err)
	}
	if !strings.HasPrefix(err.Error(), "session: open") {
		t.Fatalf("Open err = %q, want prefix %q", err.Error(), "session: open")
	}
}

func TestOpen_registered_success(t *testing.T) {
	a := freshAdapter()
	want := newStubStore()
	if err := Register(a, func(Options) (Store, error) { return want, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if got != want {
		t.Fatal("Open did not return factory store")
	}
}

func TestStore_Create_Get_roundtrip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := newStubStore()
	created, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if verr := ValidateID(created.ID); verr != nil {
		t.Fatalf("Create ID invalid: %v", verr)
	}
	got, err := st.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("Get ID = %q, want %q", got.ID, created.ID)
	}
}

func TestStore_Get_miss_fails(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := newStubStore()
	_, err := st.Get(ctx, NewID())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get miss err = %v, want ErrNotFound", err)
	}
}

func TestStore_Get_expired_fails(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := newStubStore()
	created, err := st.Create(ctx, time.Nanosecond)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	eventually(t, 2*time.Second, func() bool {
		_, err := st.Get(ctx, created.ID)
		return errors.Is(err, ErrNotFound)
	}, "session expiry")
}

func TestStore_Save_upsert_touches_updated(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := newStubStore()
	s := NewSession(NewID(), time.Hour)
	s.Data["k"] = "v"
	before := s.UpdatedAt
	// Ensure the clock advances past UpdatedAt so Save visibly touches it.
	eventually(t, 2*time.Second, func() bool { return time.Now().After(before) }, "clock advance")
	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}
	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after Save err = %v", err)
	}
	if got.Data["k"] != "v" {
		t.Fatalf("Get Data[k] = %v, want %q", got.Data["k"], "v")
	}
	if !got.UpdatedAt.After(before) {
		t.Fatal("Save did not touch UpdatedAt")
	}
}

func TestStore_Delete_idempotent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := newStubStore()
	created, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}
	if err := st.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete err = %v", err)
	}
	if err := st.Delete(ctx, created.ID); err != nil {
		t.Fatalf("second Delete err = %v, want nil", err)
	}
	if _, err := st.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete err = %v, want ErrNotFound", err)
	}
	if err := st.Delete(ctx, NewID()); err != nil {
		t.Fatalf("Delete unknown ID err = %v, want nil", err)
	}
}

func TestStore_Close_success(t *testing.T) {
	t.Parallel()
	if err := newStubStore().Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}
