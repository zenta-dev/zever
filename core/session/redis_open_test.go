package session

import (
	"errors"
	"testing"
)

// redisOpenStub is process-shared so repeat runs (-count=2) rewire the same
// factory instance; Register tolerates the resulting duplicate.
var redisOpenStub = newStubStore()

// TestParseAdapter_redis_reachesOpen pins the redis adapter wiring:
// ParseAdapter("redis") must resolve to Redis and Open must dispatch to
// the registered factory.
func TestParseAdapter_redis_reachesOpen(t *testing.T) {
	a, err := ParseAdapter("redis")
	if err != nil {
		t.Fatalf("ParseAdapter(redis) err = %v, want nil", err)
	}

	if a != Redis {
		t.Fatalf("ParseAdapter(redis) = %v, want Redis", a)
	}

	if got := a.String(); got != "redis" {
		t.Fatalf("Redis.String() = %q, want %q", got, "redis")
	}

	want := redisOpenStub
	if regErr := Register(a, func(Options) (Store, error) { return want, nil }); regErr != nil {
		var dup *DuplicateError
		if !errors.As(regErr, &dup) {
			t.Fatalf("Register(Redis) err = %v, want nil", regErr)
		}
		// Duplicate means an earlier run in this process already wired the
		// same shared stub (e.g. -count=2); the identity check below holds.
	}

	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open(Redis) err = %v, want nil", err)
	}

	if got != want {
		t.Fatal("Open(Redis) did not return factory store")
	}
}
