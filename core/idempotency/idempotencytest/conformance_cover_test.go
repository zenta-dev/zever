package idempotencytest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// stubStore is a scriptable idempotency.Store double backed by a small
// record map. Per-method error fields drive each conformance failure
// branch; result Override hooks force copy-semantics violations.
type stubStore struct {
	mu sync.Mutex

	records     map[string]*stubRecord
	beginErr    map[string]error
	completeErr error
	forgetErr   map[string]error
	closeErr    error
	closeAgain  error
	closes      int
	closed      bool

	replayResult []byte
	forceReplay  *bool
}

type stubRecord struct {
	fingerprint []byte
	result      []byte
	done        bool
}

func healthyStubStore() *stubStore {
	return &stubStore{records: map[string]*stubRecord{}, beginErr: map[string]error{}, forgetErr: map[string]error{}}
}

func (s *stubStore) Begin(_ context.Context, key string, opts idempotency.BeginOptions) (idempotency.Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return idempotency.Outcome{}, idempotency.ErrClosed
	}
	if key == "" {
		return idempotency.Outcome{}, idempotency.InvalidKeyError{KeyLen: 0}
	}
	if err, ok := s.beginErr[key]; ok && err != nil {
		return idempotency.Outcome{}, err
	}
	if s.forceReplay != nil && *s.forceReplay {
		return idempotency.Outcome{Replay: true, Result: append([]byte(nil), s.replayResult...)}, nil
	}
	rec, ok := s.records[key]
	if !ok {
		s.records[key] = &stubRecord{fingerprint: append([]byte(nil), opts.Fingerprint...)}
		return idempotency.Outcome{}, nil
	}
	if !idempotency.FingerprintMatches(rec.fingerprint, opts.Fingerprint) {
		return idempotency.Outcome{}, idempotency.ErrKeyMismatch
	}
	if !rec.done {
		return idempotency.Outcome{}, idempotency.ErrInProgress
	}
	return idempotency.Outcome{Replay: true, Result: append([]byte(nil), rec.result...)}, nil
}

func (s *stubStore) Complete(_ context.Context, key string, fingerprint, result []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return idempotency.ErrClosed
	}
	if key == "" {
		return idempotency.InvalidKeyError{KeyLen: 0}
	}
	if s.completeErr != nil {
		return s.completeErr
	}
	rec, ok := s.records[key]
	if !ok {
		s.records[key] = &stubRecord{fingerprint: append([]byte(nil), fingerprint...), result: append([]byte(nil), result...), done: true}
		return nil
	}
	if !idempotency.FingerprintMatches(rec.fingerprint, fingerprint) {
		return idempotency.ErrKeyMismatch
	}
	rec.result = append([]byte(nil), result...)
	rec.done = true
	return nil
}

func (s *stubStore) Forget(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return idempotency.ErrClosed
	}
	if key == "" {
		return idempotency.InvalidKeyError{KeyLen: 0}
	}
	if err, ok := s.forgetErr[key]; ok && err != nil {
		return err
	}
	delete(s.records, key)
	return nil
}

func (s *stubStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++
	if s.closes > 1 {
		return s.closeAgain
	}
	if s.closeErr != nil {
		return s.closeErr
	}
	s.closed = true
	return nil
}

func mustPass(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("check err = %v, want nil", err)
	}
}

func TestCheckOpenRegisterSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkOpenRegister())
}

func TestCheckExecuteReplaySuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkExecuteReplay(t.Context(), healthyStubStore()))
}

func TestCheckExecuteReplayFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("idempotencytest: boom")

	cases := []struct {
		name     string
		setup    func(*stubStore)
		contains string
	}{
		{"begin error", func(s *stubStore) { s.beginErr["exec-01"] = boom }, "Begin() error"},
		{"already replay", func(s *stubStore) {
			t := true
			s.forceReplay = &t
			s.replayResult = []byte("x")
		}, "Begin() = "},
		{"complete error", func(s *stubStore) { s.completeErr = boom }, "Complete() error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubStore()
			tc.setup(stub)

			err := checkExecuteReplay(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkExecuteReplay() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkExecuteReplay() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckExecuteReplayBeginErrorWraps(t *testing.T) {
	t.Parallel()

	boom := errors.New("idempotencytest: boom")
	stub := healthyStubStore()
	stub.beginErr["exec-01"] = boom

	if err := checkExecuteReplay(t.Context(), stub); !errors.Is(err, boom) {
		t.Fatalf("checkExecuteReplay() = %v, want wrap of boom", err)
	}
}

func TestCheckExecuteReplayLateBranches(t *testing.T) {
	t.Parallel()

	t.Run("replay begin error", func(t *testing.T) {
		t.Parallel()

		stub := &failNthBeginStore{stub: healthyStubStore(), failOn: 2, err: errors.New("idempotencytest: replay boom")}

		err := checkExecuteReplay(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Begin(replay)") {
			t.Fatalf("checkExecuteReplay() = %v, want Begin(replay) error", err)
		}
	})

	t.Run("replay false", func(t *testing.T) {
		t.Parallel()

		stub := &alwaysMissStore{}

		err := checkExecuteReplay(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Replay = false") {
			t.Fatalf("checkExecuteReplay() = %v, want replay-false error", err)
		}
	})

	t.Run("replay result mismatch", func(t *testing.T) {
		t.Parallel()

		stub := &wrongResultStore{stub: healthyStubStore()}

		err := checkExecuteReplay(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "want stored copy") {
			t.Fatalf("checkExecuteReplay() = %v, want stored-copy error", err)
		}
	})

	t.Run("second replay begin error", func(t *testing.T) {
		t.Parallel()

		stub := &failNthBeginStore{stub: healthyStubStore(), failOn: 3, err: errors.New("idempotencytest: again boom")}

		err := checkExecuteReplay(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Begin(replay)") {
			t.Fatalf("checkExecuteReplay() = %v, want Begin(replay) error", err)
		}
	})
}

// failNthBeginStore fails the nth Begin call with err.
type failNthBeginStore struct {
	stub   *stubStore
	failOn int
	calls  int
	err    error
}

func (f *failNthBeginStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	f.calls++
	if f.calls == f.failOn {
		return idempotency.Outcome{}, f.err
	}
	return f.stub.Begin(ctx, k, o)
}

func (f *failNthBeginStore) Complete(ctx context.Context, k string, fp, r []byte) error {
	return f.stub.Complete(ctx, k, fp, r)
}

func (f *failNthBeginStore) Forget(ctx context.Context, k string) error {
	return f.stub.Forget(ctx, k)
}

func (f *failNthBeginStore) Close() error { return f.stub.Close() }

// wrongResultStore completes with a different result than asserted.
type wrongResultStore struct {
	stub *stubStore
}

func (w *wrongResultStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	return w.stub.Begin(ctx, k, o)
}

func (w *wrongResultStore) Complete(ctx context.Context, k string, _ []byte, _ []byte) error {
	return w.stub.Complete(ctx, k, []byte("req-v1"), []byte("other-result"))
}

func (w *wrongResultStore) Forget(ctx context.Context, k string) error {
	return w.stub.Forget(ctx, k)
}

func (w *wrongResultStore) Close() error { return w.stub.Close() }

func TestCheckExecuteReplayCopySemantics(t *testing.T) {
	t.Parallel()

	// A store returning the live slice (not a copy) trips the
	// stored-copy assertions after caller mutation.
	stub := &aliasingStore{stub: healthyStubStore()}

	err := checkExecuteReplay(t.Context(), stub)
	if err == nil {
		t.Fatal("checkExecuteReplay(aliasing) = nil, want stored-copy error")
	}
}

// aliasingStore returns the live result slice so caller mutation is
// visible on the next replay, breaking copy semantics.
type aliasingStore struct {
	stub *stubStore
	live []byte
}

func (a *aliasingStore) Begin(ctx context.Context, key string, opts idempotency.BeginOptions) (idempotency.Outcome, error) {
	out, err := a.stub.Begin(ctx, key, opts)
	if err != nil || !out.Replay {
		return out, err
	}
	a.live = out.Result
	return idempotency.Outcome{Replay: true, Result: a.live}, nil
}

func (a *aliasingStore) Complete(ctx context.Context, key string, fp, result []byte) error {
	a.live = result
	err := a.stub.Complete(ctx, key, fp, result)
	// Point the stored record at the caller slice.
	a.stub.mu.Lock()
	if rec, ok := a.stub.records[key]; ok {
		rec.result = result
	}
	a.stub.mu.Unlock()
	return err
}

func (a *aliasingStore) Forget(ctx context.Context, key string) error {
	return a.stub.Forget(ctx, key)
}

func (a *aliasingStore) Close() error { return a.stub.Close() }

func TestCheckInProgressSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkInProgress(t.Context(), healthyStubStore()))
}

func TestCheckInProgressFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("idempotencytest: boom")

	t.Run("first begin error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.beginErr["flight-01"] = boom

		if err := checkInProgress(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkInProgress() = %v, want wrap of boom", err)
		}
	})

	t.Run("second begin not in-progress", func(t *testing.T) {
		t.Parallel()

		// A store that always misses never reports in-progress.
		stub := &alwaysMissStore{}

		err := checkInProgress(t.Context(), stub)
		if err == nil {
			t.Fatal("checkInProgress(always-miss) = nil, want ErrInProgress error")
		}
		if !strings.Contains(err.Error(), "Begin(in-flight)") {
			t.Fatalf("checkInProgress() = %v, want in-flight error", err)
		}
	})
}

// alwaysMissStore never reserves, so the second Begin misses too.
type alwaysMissStore struct{}

func (alwaysMissStore) Begin(context.Context, string, idempotency.BeginOptions) (idempotency.Outcome, error) {
	return idempotency.Outcome{}, nil
}

func (alwaysMissStore) Complete(context.Context, string, []byte, []byte) error { return nil }

func (alwaysMissStore) Forget(context.Context, string) error { return nil }

func (alwaysMissStore) Close() error { return nil }

func TestCheckFingerprintMismatchSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkFingerprintMismatch(t.Context(), healthyStubStore()))
}

func TestCheckFingerprintMismatchFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("idempotencytest: boom")

	t.Run("first begin error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.beginErr["fp-01"] = boom

		if err := checkFingerprintMismatch(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkFingerprintMismatch() = %v, want wrap of boom", err)
		}
	})

	t.Run("no mismatch reported", func(t *testing.T) {
		t.Parallel()

		// A store ignoring fingerprints never reports mismatch.
		stub := &noFingerprintStore{records: map[string][]byte{}}

		err := checkFingerprintMismatch(t.Context(), stub)
		if err == nil {
			t.Fatal("checkFingerprintMismatch(no-fp) = nil, want mismatch error")
		}
		if !strings.Contains(err.Error(), "mismatch") {
			t.Fatalf("checkFingerprintMismatch() = %v, want mismatch error", err)
		}
	})

	t.Run("complete error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.completeErr = errors.New("idempotencytest: complete boom")

		err := checkFingerprintMismatch(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Complete() error") {
			t.Fatalf("checkFingerprintMismatch() = %v, want Complete error", err)
		}
	})

	t.Run("replay mismatch missing", func(t *testing.T) {
		t.Parallel()

		// Forgets the record before replay so the replay Begin misses
		// instead of reporting mismatch.
		stub := &forgetOnCompleteStore{stub: healthyStubStore()}

		err := checkFingerprintMismatch(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "replay mismatch") {
			t.Fatalf("checkFingerprintMismatch() = %v, want replay-mismatch error", err)
		}
	})

	t.Run("complete mismatch missing", func(t *testing.T) {
		t.Parallel()

		// Accepts every Complete so the final mismatch check finds nil.
		stub := &acceptCompleteStore{stub: healthyStubStore()}

		err := checkFingerprintMismatch(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Complete(mismatch)") {
			t.Fatalf("checkFingerprintMismatch() = %v, want Complete(mismatch) error", err)
		}
	})
}

// forgetOnCompleteStore drops the record on Complete, so the later
// replay Begin misses instead of reporting a fingerprint mismatch.
type forgetOnCompleteStore struct {
	stub *stubStore
}

func (f *forgetOnCompleteStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	return f.stub.Begin(ctx, k, o)
}

func (f *forgetOnCompleteStore) Complete(ctx context.Context, k string, fp, r []byte) error {
	if err := f.stub.Complete(ctx, k, fp, r); err != nil {
		return err
	}
	f.stub.mu.Lock()
	delete(f.stub.records, k)
	f.stub.mu.Unlock()
	return nil
}

func (f *forgetOnCompleteStore) Forget(ctx context.Context, k string) error {
	return f.stub.Forget(ctx, k)
}

func (f *forgetOnCompleteStore) Close() error { return f.stub.Close() }

// acceptCompleteStore accepts every Complete with nil, so a mismatched
// Complete never reports ErrKeyMismatch.
type acceptCompleteStore struct {
	stub *stubStore
}

func (a *acceptCompleteStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	return a.stub.Begin(ctx, k, o)
}

func (a *acceptCompleteStore) Complete(context.Context, string, []byte, []byte) error {
	return nil
}

func (a *acceptCompleteStore) Forget(ctx context.Context, k string) error {
	return a.stub.Forget(ctx, k)
}

func (a *acceptCompleteStore) Close() error { return a.stub.Close() }

// noFingerprintStore ignores fingerprints entirely.
type noFingerprintStore struct {
	mu      sync.Mutex
	records map[string][]byte
}

func (s *noFingerprintStore) Begin(_ context.Context, key string, _ idempotency.BeginOptions) (idempotency.Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if res, ok := s.records[key]; ok {
		return idempotency.Outcome{Replay: true, Result: res}, nil
	}
	s.records[key] = nil
	return idempotency.Outcome{}, nil
}

func (s *noFingerprintStore) Complete(_ context.Context, key string, _ []byte, result []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[key] = result
	return nil
}

func (s *noFingerprintStore) Forget(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.records, key)
	return nil
}

func (s *noFingerprintStore) Close() error { return nil }

func TestCheckForgetSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkForget(t.Context(), healthyStubStore()))
}

func TestCheckForgetFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("idempotencytest: boom")

	cases := []struct {
		name     string
		setup    func(*stubStore)
		contains string
	}{
		{"forget missing error", func(s *stubStore) { s.forgetErr["never-seen"] = boom }, "Forget(missing)"},
		{"begin error", func(s *stubStore) { s.beginErr["drop-01"] = boom }, "Begin() error"},
		{"forget error", func(s *stubStore) { s.forgetErr["drop-01"] = boom }, "Forget() error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := healthyStubStore()
			tc.setup(stub)

			err := checkForget(t.Context(), stub)
			if err == nil {
				t.Fatalf("checkForget() = nil, want error containing %q", tc.contains)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("checkForget() = %v, want containing %q", err, tc.contains)
			}
		})
	}
}

func TestCheckForgetReplayAfterForget(t *testing.T) {
	t.Parallel()

	// A store whose Forget is a no-op keeps replaying after forget.
	stub := healthyStubStore()
	if _, err := stub.Begin(t.Context(), "drop-01", idempotency.BeginOptions{}); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	stub.forgetErr["drop-01"] = nil
	delete(stub.forgetErr, "drop-01")
	// Simulate no-op forget by re-adding guard: use a wrapper.
	noop := &noopForgetStore{stub: stub}

	err := checkForget(t.Context(), noop)
	if err == nil {
		t.Fatal("checkForget(noop-forget) = nil, want replay error")
	}
}

func TestCheckForgetLateBranches(t *testing.T) {
	t.Parallel()

	t.Run("begin after forget error", func(t *testing.T) {
		t.Parallel()

		stub := &failNthBeginStore{stub: healthyStubStore(), failOn: 2, err: errors.New("idempotencytest: re-begin boom")}

		err := checkForget(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Begin(after forget)") {
			t.Fatalf("checkForget() = %v, want Begin(after forget) error", err)
		}
	})

	t.Run("replay after forget", func(t *testing.T) {
		t.Parallel()

		// Completes the reservation on first Begin and ignores Forget,
		// so the post-forget Begin replays instead of missing.
		stub := &completeOnBeginStore{stub: healthyStubStore()}

		err := checkForget(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Replay = true") {
			t.Fatalf("checkForget() = %v, want fresh-reservation error", err)
		}
	})

	t.Run("forget again error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.forgetErr["drop-01"] = errors.New("idempotencytest: forget-again boom")

		err := checkForget(t.Context(), stub)
		// The first Forget(drop-01) already fails, so this hits the
		// Forget() error branch; exercise forget-again via two-phase.
		if err == nil || !strings.Contains(err.Error(), "Forget() error") {
			t.Fatalf("checkForget() = %v, want Forget error", err)
		}
	})

	t.Run("forget again late error", func(t *testing.T) {
		t.Parallel()

		stub := &failSecondForgetStore{stub: healthyStubStore()}

		err := checkForget(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Forget(again)") {
			t.Fatalf("checkForget() = %v, want Forget(again) error", err)
		}
	})
}

// completeOnBeginStore completes each reservation immediately and
// ignores Forget, so post-forget Begin replays.
type completeOnBeginStore struct {
	stub *stubStore
}

func (c *completeOnBeginStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	out, err := c.stub.Begin(ctx, k, o)
	if err != nil || out.Replay {
		return out, err
	}
	_ = c.stub.Complete(ctx, k, o.Fingerprint, []byte("done"))
	return out, nil
}

func (c *completeOnBeginStore) Complete(ctx context.Context, k string, fp, r []byte) error {
	return c.stub.Complete(ctx, k, fp, r)
}

func (c *completeOnBeginStore) Forget(context.Context, string) error { return nil }

func (c *completeOnBeginStore) Close() error { return c.stub.Close() }

// failSecondForgetStore fails only the second Forget call.
type failSecondForgetStore struct {
	stub  *stubStore
	calls int
}

func (f *failSecondForgetStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	return f.stub.Begin(ctx, k, o)
}

func (f *failSecondForgetStore) Complete(ctx context.Context, k string, fp, r []byte) error {
	return f.stub.Complete(ctx, k, fp, r)
}

func (f *failSecondForgetStore) Forget(ctx context.Context, k string) error {
	f.calls++
	if f.calls == 3 {
		return errors.New("idempotencytest: forget-again boom")
	}
	return f.stub.Forget(ctx, k)
}

func (f *failSecondForgetStore) Close() error { return f.stub.Close() }

// noopForgetStore drops Forget calls so the record survives.
type noopForgetStore struct {
	stub *stubStore
}

func (n *noopForgetStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	return n.stub.Begin(ctx, k, o)
}

func (n *noopForgetStore) Complete(ctx context.Context, k string, fp, r []byte) error {
	return n.stub.Complete(ctx, k, fp, r)
}

func (n *noopForgetStore) Forget(context.Context, string) error { return nil }

func (n *noopForgetStore) Close() error { return n.stub.Close() }

func TestCheckInvalidKeyReportsAll(t *testing.T) {
	t.Parallel()

	err := checkInvalidKey(t.Context(), &permissiveStore{})
	if err == nil {
		t.Fatal("checkInvalidKey(permissive) = nil, want joined errors")
	}

	for _, want := range []string{"Begin(empty)", "Complete(empty)", "Forget(empty)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkInvalidKey() = %v, want containing %q", err, want)
		}
	}
}

// permissiveStore accepts empty keys on store methods (ValidateKey
// still uses the real package function, so its branch stays passing).
type permissiveStore struct{}

func (permissiveStore) Begin(context.Context, string, idempotency.BeginOptions) (idempotency.Outcome, error) {
	return idempotency.Outcome{}, nil
}

func (permissiveStore) Complete(context.Context, string, []byte, []byte) error { return nil }

func (permissiveStore) Forget(context.Context, string) error { return nil }

func (permissiveStore) Close() error { return nil }

func TestCheckCloseSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(t.Context(), healthyStubStore()))
}

func TestCheckCloseFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("idempotencytest: boom")

	t.Run("first close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.closeErr = boom

		if err := checkClose(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkClose() = %v, want wrap of boom", err)
		}
	})

	t.Run("post-close ops open", func(t *testing.T) {
		t.Parallel()

		// A store that stays open after Close trips every ErrClosed branch.
		stub := &neverClosesStore{stub: healthyStubStore()}

		err := checkClose(t.Context(), stub)
		if err == nil {
			t.Fatal("checkClose(never-closes) = nil, want ErrClosed errors")
		}
		for _, want := range []string{"Begin()", "Complete()", "Forget()"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkClose() = %v, want containing %q", err, want)
			}
		}
	})
}

// neverClosesStore reports nil from Close without marking closed.
type neverClosesStore struct {
	stub *stubStore
}

func (n *neverClosesStore) Begin(ctx context.Context, k string, o idempotency.BeginOptions) (idempotency.Outcome, error) {
	n.stub.mu.Lock()
	was := n.stub.closed
	n.stub.mu.Unlock()
	_ = was
	return n.stub.Begin(ctx, k, o)
}

func (n *neverClosesStore) Complete(ctx context.Context, k string, fp, r []byte) error {
	return n.stub.Complete(ctx, k, fp, r)
}

func (n *neverClosesStore) Forget(ctx context.Context, k string) error {
	return n.stub.Forget(ctx, k)
}

func (n *neverClosesStore) Close() error { return nil }

func TestCheckConcurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()

			if err := checkExecuteReplay(ctx, healthyStubStore()); err != nil {
				t.Errorf("checkExecuteReplay() = %v, want nil", err)
			}

			if err := checkForget(ctx, healthyStubStore()); err != nil {
				t.Errorf("checkForget() = %v, want nil", err)
			}

			if err := checkClose(ctx, healthyStubStore()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
