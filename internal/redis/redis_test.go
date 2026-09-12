package redis

import (
	"sync"
	"testing"
)

func resetRedis(t *testing.T) {
	t.Helper()

	if err := Close(); err != nil {
		t.Fatalf("Close() setup error = %v", err)
	}

	t.Cleanup(func() {
		if err := Close(); err != nil {
			t.Errorf("Close() cleanup error = %v", err)
		}
	})
}

func TestNew_sameOptions_reusesClient(t *testing.T) {
	resetRedis(t)

	opts := Options{Addr: "localhost:6379"}

	first, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	second, err := New(opts)
	if err != nil {
		t.Fatalf("New() second error = %v", err)
	}

	if first != second {
		t.Error("New() with same opts returned different client, want reused instance")
	}
}

func TestNew_whitespaceEquivalentOptions_reusesClient(t *testing.T) {
	resetRedis(t)

	first, err := New(Options{Addr: "localhost:6379", Password: "pw"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	second, err := New(Options{Addr: "  localhost:6379 ", Password: " pw "})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if first != second {
		t.Error("New() with whitespace-equivalent opts returned different client, want reuse")
	}
}

func TestNew_differentOptions_replacesClient(t *testing.T) {
	resetRedis(t)

	first, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	second, err := New(Options{Addr: "localhost:6380"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if first == second {
		t.Error("New() with different opts returned same client, want replacement")
	}
}

func TestNew_invalidAddr_returnsError(t *testing.T) {
	resetRedis(t)

	if _, err := New(Options{Addr: "redis://"}); err == nil {
		t.Fatal("New() = nil error, want missing-host error")
	}
}

func TestClose_noInstance_returnsNil(t *testing.T) {
	resetRedis(t)

	if err := Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

func TestClose_afterNew_resetsSingleton(t *testing.T) {
	resetRedis(t)

	first, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err = Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second, err := New(Options{Addr: "localhost:6379"})
	if err != nil {
		t.Fatalf("New() after Close error = %v", err)
	}

	if first == second {
		t.Error("New() after Close returned same client, want fresh instance")
	}
}

func TestNew_concurrentSameOptions_safe(t *testing.T) {
	resetRedis(t)

	opts := Options{Addr: "localhost:6379"}

	const workers = 20

	clients := make([]any, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()
			c, err := New(opts)
			clients[i] = c
			errs[i] = err
		}()
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("New() worker %d error = %v", i, err)
		}
	}

	for i := 1; i < workers; i++ {
		if clients[i] != clients[0] {
			t.Fatalf("New() worker %d got different client, want single reused instance", i)
		}
	}
}
