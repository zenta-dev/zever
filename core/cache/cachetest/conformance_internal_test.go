package cachetest

import (
	"context"
	"testing"
	"time"

	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
)

// expiryStubCache lags real expiry: Get reports "k" missing only after the
// first read, and Exists keeps reporting "k" present for two polls before
// clearing. That forces the TTL-expiry polls through their retry path
// (maybeFastForward + poll-again) instead of succeeding on the first try.
// It also implements FastForward so the virtual-clock branch is exercised.
type expiryStubCache struct {
	cache.Cache

	ffCalls     int
	getCalls    int
	existsCalls int
}

func (s *expiryStubCache) Get(ctx context.Context, key string) ([]byte, error) {
	if key == "k" {
		s.getCalls++

		if s.getCalls > 1 {
			return nil, cache.NotFoundError{Key: key}
		}
	}

	return s.Cache.Get(ctx, key)
}

func (s *expiryStubCache) Exists(ctx context.Context, key string) (bool, error) {
	if key == "k" {
		s.existsCalls++

		if s.existsCalls <= 2 {
			return true, nil
		}

		return false, nil
	}

	return s.Cache.Exists(ctx, key)
}

// FastForward records virtual-clock advances for the conformance poll loop.
func (s *expiryStubCache) FastForward(_ time.Duration) {
	s.ffCalls++
}

func TestConformanceTTLExpiryPollPaths(t *testing.T) {
	t.Parallel()

	inner, err := cachememory.New(cache.Options{})
	if err != nil {
		t.Fatalf("memory.New() error = %v", err)
	}

	t.Cleanup(func() { _ = inner.Close(t.Context()) })

	var stub *expiryStubCache

	conformanceTTLExpiry(t, func(t *testing.T) cache.Cache {
		t.Helper()

		stub = &expiryStubCache{Cache: inner}

		return stub
	})

	if stub == nil {
		t.Fatal("factory never ran")
	}

	if stub.ffCalls == 0 {
		t.Error("FastForward never ran during expiry polls")
	}
}
