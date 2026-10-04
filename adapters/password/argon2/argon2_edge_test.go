package argon2

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestHash_contextIgnored(t *testing.T) {
	t.Parallel()

	h := testHasher(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := h.Hash(ctx, "secret"); err != nil {
		t.Errorf("Hash(canceled ctx) = %v, want nil (derivation is CPU-bound)", err)
	}
	if _, err := h.Hash(nil, "secret"); err != nil { //nolint:staticcheck // deliberately exercises nil-context handling
		t.Errorf("Hash(nil ctx) = %v, want nil", err)
	}
}

func TestHash_distinctSalts(t *testing.T) {
	t.Parallel()

	h := testHasher(t)
	ctx := t.Context()

	first, err := h.Hash(ctx, "same-password")
	if err != nil {
		t.Fatalf("Hash() = %v", err)
	}
	second, err := h.Hash(ctx, "same-password")
	if err != nil {
		t.Fatalf("Hash() = %v", err)
	}
	if first == second {
		t.Error("two hashes of the same password are identical, want distinct random salts")
	}

	for i, hash := range []string{first, second} {
		ok, err := h.Verify(ctx, hash, "same-password")
		if err != nil {
			t.Fatalf("Verify(hash %d) = %v", i, err)
		}
		if !ok {
			t.Errorf("Verify(hash %d) = false, want true", i)
		}
	}
}

func TestHash_concurrentSafe(t *testing.T) {
	t.Parallel()

	h := testHasher(t)
	const workers = 16

	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, err := h.Hash(t.Context(), "secret")
			if err != nil {
				errs[w] = err
				return
			}
			ok, err := h.Verify(t.Context(), hash, "secret")
			if err != nil {
				errs[w] = err
				return
			}
			if !ok {
				errs[w] = errors.New("verify returned false")
			}
		}()
	}
	wg.Wait()
	for w, err := range errs {
		if err != nil {
			t.Errorf("worker %d = %v, want nil", w, err)
		}
	}
}
