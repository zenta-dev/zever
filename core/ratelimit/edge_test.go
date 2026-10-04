package ratelimit

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestAdapter_Parse_custom_nonempty(t *testing.T) {
	t.Parallel()

	got, err := ParseAdapter("custom")
	if err != nil {
		t.Fatalf("ParseAdapter(custom) error = %v", err)
	}
	if got != Adapter("custom") {
		t.Fatalf("ParseAdapter(custom) = %v, want custom", got)
	}
}

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()

	_, err := Open(Memory, Options{Rate: 0, Burst: 1})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open(invalid options) err = %v, want ErrInvalidOptions", err)
	}
}

func TestValidateKey_boundary_lengths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"one byte", "a", false},
		{"max len", strings.Repeat("a", MaxKeyLen), false},
		{"max len plus one", strings.Repeat("a", MaxKeyLen+1), true},
		{"empty", "", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateKey(tc.key)
			if tc.wantErr && !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("ValidateKey(len=%d) err = %v, want ErrInvalidKey", len(tc.key), err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateKey(len=%d) err = %v, want nil", len(tc.key), err)
			}
		})
	}
}

func TestDecision_zero(t *testing.T) {
	t.Parallel()

	var d Decision
	if d.Allowed || d.RetryAfter != 0 || d.Remaining != 0 {
		t.Fatalf("zero Decision = %+v, want zero", d)
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

			a := freshAdapter()
			if err := Register(a, func(Options) (Limiter, error) { return stubLimiter{}, nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{Rate: 1, Burst: 1}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}
