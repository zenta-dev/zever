package redisclient

import (
	"errors"
	"sync"
	"testing"
)

// TestShared_invalidAddr_returnsError verifies a bad address fails before any
// client is built and returns no release closure.
func TestShared_invalidAddr_returnsError(t *testing.T) {
	t.Parallel()

	client, release, err := Shared(Options{Addr: "redis://"})
	if err == nil {
		t.Fatal("Shared() error = nil, want invalid-address error")
	}

	if !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("errors.Is(err, ErrInvalidAddress) = false (err = %v)", err)
	}

	if client != nil {
		t.Errorf("Shared() client = %v, want nil", client)
	}

	if release != nil {
		t.Error("Shared() release != nil, want nil")
	}
}

// TestToRedisOptions_urlUsernameWithoutPassword verifies a URL carrying a
// username but no password leaves Password empty.
func TestToRedisOptions_urlUsernameWithoutPassword(t *testing.T) {
	t.Parallel()

	got, err := toRedisOptions(Options{Addr: "redis://bob@h:6379"})
	if err != nil {
		t.Fatalf("toRedisOptions() error = %v", err)
	}

	if got.Password != "" {
		t.Errorf("Password = %q, want empty", got.Password)
	}
}

// TestShared_concurrentAcquireReleaseChurn exercises the refcounted registry
// under concurrent acquire/release on one key; the race detector must stay
// clean and every call must succeed.
func TestShared_concurrentAcquireReleaseChurn(t *testing.T) {
	t.Parallel()

	opts := Options{Addr: "127.0.0.1:30"}

	const (
		workers    = 16
		iterations = 8
	)

	var wg sync.WaitGroup

	errCh := make(chan error, workers)

	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range iterations {
				_, release, err := Shared(opts)
				if err != nil {
					errCh <- err

					return
				}

				if err := release(); err != nil {
					errCh <- err

					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Shared() churn error = %v", err)
	}
}
