package session

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)






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

