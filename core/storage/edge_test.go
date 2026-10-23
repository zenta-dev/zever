package storage

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPresignExpiry_max_boundary(t *testing.T) {
	t.Parallel()

	if _, err := PresignExpiry(MaxPresignTTL); err != nil {
		t.Fatalf("PresignExpiry(MaxPresignTTL) error = %v, want nil", err)
	}
	if _, err := PresignExpiry(MaxPresignTTL + time.Nanosecond); !errors.Is(err, ErrPresignTTLExceeded) {
		t.Fatalf("PresignExpiry(max+1ns) err = %v, want ErrPresignTTLExceeded", err)
	}
}

func TestValidKey_additional_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want bool
	}{
		{"a//b", false},
		{"./a", false},
		{"a/", false},
		{"a/b/", false},
		{"a b", true},
		{strings.Repeat("a/", 1<<16) + "z", true},
		{"a\x7fb", false},
	}

	for _, tc := range cases {
		if got := ValidKey(tc.key); got != tc.want {
			t.Errorf("ValidKey(len=%d) = %v, want %v", len(tc.key), got, tc.want)
		}
	}
}

func TestPolicyAllow_unknown_perm(t *testing.T) {
	t.Parallel()

	pol := Policy{Read: Rule{Allow: []Subject{"alice"}}}
	if pol.Allow(Perm("bogus"), "alice", true) {
		t.Fatal("Allow(unknown perm) = true, want false")
	}
	if pol.Public(Perm("bogus")) {
		t.Fatal("Public(unknown perm) = true, want false")
	}
}

func TestDecisionHook_concurrent(t *testing.T) {
	var mu sync.Mutex
	count := 0

	SetDecisionHook(func(context.Context, Decision) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	defer SetDecisionHook(nil)

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			FireAllow(t.Context(), PermRead, "b", "alice", ReasonOk, "1")
		}()
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if count != n {
		t.Fatalf("hook fired %d times, want %d", count, n)
	}
}


func TestValidKeyRejectsControlCharacters(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"a\nb", "a\tb", "a\rb", "a\x7fb", "a\x01b", "a\x00b"} {
		if ValidKey(key) {
			t.Errorf("ValidKey(%q) = true, want false", key)
		}
	}
}
