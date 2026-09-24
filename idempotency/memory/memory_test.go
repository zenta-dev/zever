package memory_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/idempotency"
	"github.com/zenta-dev/zever/idempotency/memory"
)

func newStore(t *testing.T) idempotency.Store {
	t.Helper()

	s, err := memory.New(idempotency.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func TestClaimCompleteReplay(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()
	fp := []byte("fp1")

	out, err := s.Begin(ctx, "key-claim", idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if out.Replay {
		t.Fatal("first Begin must not replay")
	}

	if cerr := s.Complete(ctx, "key-claim", fp, []byte("result")); cerr != nil {
		t.Fatalf("Complete: %v", cerr)
	}

	out, err = s.Begin(ctx, "key-claim", idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("replay Begin: %v", err)
	}

	if !out.Replay || string(out.Result) != "result" {
		t.Fatalf("replay = %v %q, want replay result", out.Replay, out.Result)
	}
}

func TestSecondClaimantInProgress(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Begin(ctx, "key-race", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("first Begin: %v", err)
	}

	if _, err := s.Begin(ctx, "key-race", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInProgress) {
		t.Fatalf("second Begin = %v, want ErrInProgress", err)
	}
}

func TestSingleWinner64Goroutines(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	var winners atomic.Int64

	var wg sync.WaitGroup

	errs := make([]error, 64)
	outs := make([]idempotency.Outcome, 64)

	for i := range 64 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			out, err := s.Begin(ctx, "key-contention", idempotency.BeginOptions{})
			outs[i] = out
			errs[i] = err

			if err == nil && !out.Replay {
				winners.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if got := winners.Load(); got != 1 {
		t.Fatalf("winners = %d, want exactly 1", got)
	}

	for i := range 64 {
		if errs[i] == nil {
			continue
		}

		if !errors.Is(errs[i], idempotency.ErrInProgress) {
			t.Fatalf("loser err = %v, want ErrInProgress", errs[i])
		}
	}
}

func TestFingerprintMismatchOnDone(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Begin(ctx, "key-fpdone", idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Complete(ctx, "key-fpdone", []byte("a"), []byte("orig")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if _, err := s.Begin(ctx, "key-fpdone", idempotency.BeginOptions{Fingerprint: []byte("b")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("mismatch Begin = %v, want ErrKeyMismatch", err)
	}

	out, err := s.Begin(ctx, "key-fpdone", idempotency.BeginOptions{Fingerprint: []byte("a")})
	if err != nil {
		t.Fatalf("original Begin: %v", err)
	}

	if !out.Replay || string(out.Result) != "orig" {
		t.Fatalf("original not preserved: %+v", out)
	}
}

func TestFingerprintMismatchOnPending(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Begin(ctx, "key-fppend", idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	_, err := s.Begin(ctx, "key-fppend", idempotency.BeginOptions{Fingerprint: []byte("b")})
	if !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("pending mismatch = %v, want ErrKeyMismatch (not InProgress)", err)
	}
}

func TestNilFingerprintBackCompat(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Begin(ctx, "key-nilfp", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Complete(ctx, "key-nilfp", nil, []byte("r")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	out, err := s.Begin(ctx, "key-nilfp", idempotency.BeginOptions{})
	if err != nil {
		t.Fatalf("replay Begin: %v", err)
	}

	if !out.Replay || string(out.Result) != "r" {
		t.Fatalf("nil-fp replay = %+v, want replay", out)
	}
}

func TestFingerprintVsEmptyMismatch(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Begin(ctx, "key-fpempty", idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Complete(ctx, "key-fpempty", []byte("a"), []byte("r")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if _, err := s.Begin(ctx, "key-fpempty", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("fp-vs-empty = %v, want ErrKeyMismatch", err)
	}

	if _, err := s.Begin(ctx, "key-fpempty2", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("empty Begin: %v", err)
	}

	if err := s.Complete(ctx, "key-fpempty2", nil, []byte("r")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if _, err := s.Begin(ctx, "key-fpempty2", idempotency.BeginOptions{Fingerprint: []byte("x")}); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("empty-vs-fp = %v, want ErrKeyMismatch", err)
	}
}

func TestCompleteWithoutBeginUpserts(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if err := s.Complete(ctx, "key-fresh", []byte("fp"), []byte("side-effect")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	out, err := s.Begin(ctx, "key-fresh", idempotency.BeginOptions{Fingerprint: []byte("fp")})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if !out.Replay || string(out.Result) != "side-effect" {
		t.Fatalf("upsert replay = %+v, want replay", out)
	}
}

func TestExpiryReclaim(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()
	ttl := 20 * time.Millisecond

	if _, err := s.Begin(ctx, "key-expire", idempotency.BeginOptions{TTL: ttl}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// Poll re-claim until the 20ms record expires; a live record reports
	// ErrInProgress, an expired one re-claims cleanly without replay.
	eventually(t, 2*time.Second, func() bool {
		out, err := s.Begin(ctx, "key-expire", idempotency.BeginOptions{TTL: ttl})
		return err == nil && !out.Replay
	}, "idempotency record expiry")
}

func TestForget(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if err := s.Forget(ctx, "key-unknown"); err != nil {
		t.Fatalf("Forget unknown = %v, want nil", err)
	}

	fp := []byte("fp")

	if _, err := s.Begin(ctx, "key-forget", idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Complete(ctx, "key-forget", fp, []byte("r")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if err := s.Forget(ctx, "key-forget"); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	out, err := s.Begin(ctx, "key-forget", idempotency.BeginOptions{Fingerprint: fp})
	if err != nil {
		t.Fatalf("re-claim after Forget: %v", err)
	}

	if out.Replay {
		t.Fatal("after Forget must not replay")
	}
}

func TestCopyIndependence(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	fp := []byte("fp-copy")
	res := []byte("result-copy")

	if _, err := s.Begin(ctx, "key-copy", idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	fp[0] = 'X'

	if err := s.Complete(ctx, "key-copy", []byte("fp-copy"), res); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	res[0] = 'X'

	out, err := s.Begin(ctx, "key-copy", idempotency.BeginOptions{Fingerprint: []byte("fp-copy")})
	if err != nil {
		t.Fatalf("replay Begin: %v", err)
	}

	if string(out.Result) != "result-copy" {
		t.Fatalf("aliased result = %q", out.Result)
	}

	out.Result[0] = 'X'

	out2, err := s.Begin(ctx, "key-copy", idempotency.BeginOptions{Fingerprint: []byte("fp-copy")})
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}

	if string(out2.Result) != "result-copy" {
		t.Fatalf("replay mutated store = %q", out2.Result)
	}
}

func TestAfterClose(t *testing.T) {
	t.Parallel()

	s, err := memory.New(idempotency.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("Begin after close = %v, want ErrClosed", err)
	}

	if err := s.Complete(ctx, "k", nil, []byte("r")); !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("Complete after close = %v, want ErrClosed", err)
	}

	if err := s.Forget(ctx, "k"); !errors.Is(err, idempotency.ErrClosed) {
		t.Fatalf("Forget after close = %v, want ErrClosed", err)
	}
}

func TestInvalidKeys(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	keys := []string{
		"",
		strings.Repeat("k", idempotency.MaxKeyLen+1),
		"bad\rkey",
		"bad\nkey",
		"bad\x00key",
	}

	for _, k := range keys {
		if _, err := s.Begin(ctx, k, idempotency.BeginOptions{}); !errors.Is(err, idempotency.ErrInvalidKey) {
			t.Errorf("Begin(%q) = %v, want ErrInvalidKey", k, err)
		}

		if err := s.Complete(ctx, k, nil, []byte("r")); !errors.Is(err, idempotency.ErrInvalidKey) {
			t.Errorf("Complete(%q) = %v, want ErrInvalidKey", k, err)
		}

		if err := s.Forget(ctx, k); !errors.Is(err, idempotency.ErrInvalidKey) {
			t.Errorf("Forget(%q) = %v, want ErrInvalidKey", k, err)
		}
	}
}

func TestInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := memory.New(idempotency.Options{TTL: -time.Second})
	if err == nil {
		t.Fatal("New(negative TTL) = nil, want error")
	}

	if !errors.Is(err, idempotency.ErrInvalidOptions) {
		t.Errorf("New(negative TTL) = %v, want ErrInvalidOptions", err)
	}

	if !strings.HasPrefix(err.Error(), "memory: ") {
		t.Errorf("New(negative TTL) = %q, want memory: prefix", err)
	}
}

func TestContextCancelled(t *testing.T) {
	t.Parallel()

	s := newStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := s.Begin(ctx, "k", idempotency.BeginOptions{}); !errors.Is(err, context.Canceled) {
		t.Errorf("Begin cancelled = %v, want context.Canceled", err)
	}

	if err := s.Complete(ctx, "k", nil, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Complete cancelled = %v, want context.Canceled", err)
	}

	if err := s.Forget(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Errorf("Forget cancelled = %v, want context.Canceled", err)
	}
}

func TestCompleteMismatchPreservesOriginal(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Begin(ctx, "key-nocobber", idempotency.BeginOptions{Fingerprint: []byte("a")}); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if err := s.Complete(ctx, "key-nocobber", []byte("WRONG"), []byte("evil")); !errors.Is(err, idempotency.ErrKeyMismatch) {
		t.Fatalf("Complete mismatch = %v, want ErrKeyMismatch", err)
	}

	if err := s.Complete(ctx, "key-nocobber", []byte("a"), []byte("good")); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	out, err := s.Begin(ctx, "key-nocobber", idempotency.BeginOptions{Fingerprint: []byte("a")})
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	if !out.Replay || string(out.Result) != "good" {
		t.Fatalf("record clobbered: %+v", out)
	}
}

func TestFingerprintTooLarge_rejected(t *testing.T) {
	t.Parallel()

	s := newStore(t)
	ctx := context.Background()
	big := make([]byte, 5000)

	if _, err := s.Begin(ctx, "key-bigfp", idempotency.BeginOptions{Fingerprint: big}); !errors.Is(err, idempotency.ErrFingerprintTooLarge) {
		t.Fatalf("Begin oversize fp = %v, want ErrFingerprintTooLarge", err)
	}

	if err := s.Complete(ctx, "key-bigfp", big, []byte("r")); !errors.Is(err, idempotency.ErrFingerprintTooLarge) {
		t.Fatalf("Complete oversize fp = %v, want ErrFingerprintTooLarge", err)
	}
}
