package sessiontest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/session"
)

// stubStore is a scriptable session.Store double with a small in-memory
// record map and TTL expiry. Per-method error fields drive each
// conformance failure branch.
type stubStore struct {
	mu sync.Mutex

	records    map[string]session.Session
	createErr  error
	zeroExpiry bool
	getErr     error
	saveErr    error
	deleteErr  error
	closeErr   error
	closeAg    error
	closes     int
	closed     bool
	// neverExpire disables TTL expiry so polls time out.
	neverExpire bool
}

func healthyStubStore() *stubStore {
	return &stubStore{records: map[string]session.Session{}}
}

func (s *stubStore) Create(_ context.Context, ttl time.Duration) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return session.Session{}, session.ErrClosed
	}
	if s.createErr != nil {
		return session.Session{}, s.createErr
	}
	sess := session.NewSession(session.NewID(), ttl)
	if s.zeroExpiry {
		sess.ExpiresAt = time.Time{}
	}
	if sess.Data == nil {
		sess.Data = map[string]any{}
	}
	s.records[sess.ID] = sess.Clone()
	return sess, nil
}

func (s *stubStore) Get(_ context.Context, id string) (session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return session.Session{}, session.ErrClosed
	}
	if err := session.ValidateID(id); err != nil {
		return session.Session{}, err
	}
	if s.getErr != nil {
		return session.Session{}, s.getErr
	}
	rec, ok := s.records[id]
	if !ok {
		return session.Session{}, session.ErrNotFound
	}
	if !s.neverExpire && rec.Expired(time.Now()) {
		delete(s.records, id)
		return session.Session{}, session.ErrNotFound
	}
	return rec.Clone(), nil
}

func (s *stubStore) Save(_ context.Context, sess session.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return session.ErrClosed
	}
	if err := session.ValidateID(sess.ID); err != nil {
		return err
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	sess.UpdatedAt = time.Now()
	s.records[sess.ID] = sess.Clone()
	return nil
}

func (s *stubStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return session.ErrClosed
	}
	if err := session.ValidateID(id); err != nil {
		return err
	}
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.records, id)
	return nil
}

func (s *stubStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closes++
	if s.closes > 1 {
		return s.closeAg
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

func TestCheckCreateGetSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkCreateGet(t.Context(), healthyStubStore()))
}

func TestCheckCreateGetFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("sessiontest: boom")

	t.Run("create error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.createErr = boom

		if err := checkCreateGet(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkCreateGet() = %v, want wrap of boom", err)
		}
	})

	t.Run("zero expiry", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.zeroExpiry = true

		err := checkCreateGet(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "ExpiresAt is zero") {
			t.Fatalf("checkCreateGet() = %v, want zero-expiry error", err)
		}
	})

	t.Run("get error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.getErr = boom

		err := checkCreateGet(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Get() error") {
			t.Fatalf("checkCreateGet() = %v, want Get error", err)
		}
	})

	t.Run("id mismatch and unknown found", func(t *testing.T) {
		t.Parallel()

		stub := &mismatchStore{stub: healthyStubStore()}

		err := checkCreateGet(t.Context(), stub)
		if err == nil {
			t.Fatal("checkCreateGet(mismatch) = nil, want joined errors")
		}
		for _, want := range []string{"Get().ID", "Get(unknown)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkCreateGet() = %v, want containing %q", err, want)
			}
		}
	})
}

// mismatchStore returns a session with a different ID and resolves
// unknown IDs, tripping both soft-mismatch branches.
type mismatchStore struct {
	stub *stubStore
}

func (m *mismatchStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	return m.stub.Create(ctx, ttl)
}

func (m *mismatchStore) Get(ctx context.Context, id string) (session.Session, error) {
	got, stubErr := m.stub.Get(ctx, id)
	if stubErr != nil {
		// Report unknown IDs as found. The stub error is
		// intentionally swallowed: a found session is the violation.
		return session.Session{ID: id, Data: map[string]any{}}, nil //nolint:nilerr // proves Get(unknown) violation
	}
	got.ID = session.NewID()
	return got, nil
}

func (m *mismatchStore) Save(ctx context.Context, s session.Session) error {
	return m.stub.Save(ctx, s)
}

func (m *mismatchStore) Delete(ctx context.Context, id string) error {
	return m.stub.Delete(ctx, id)
}

func (m *mismatchStore) Close() error { return m.stub.Close() }

func TestCheckSaveSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkSave(t.Context(), healthyStubStore()))
}

func TestCheckSaveFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("sessiontest: boom")

	t.Run("create error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.createErr = boom

		if err := checkSave(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkSave() = %v, want wrap of boom", err)
		}
	})

	t.Run("save error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.saveErr = boom

		if err := checkSave(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkSave() = %v, want wrap of boom", err)
		}
	})

	t.Run("get error", func(t *testing.T) {
		t.Parallel()

		stub := &failGetAfterSaveStore{stub: healthyStubStore(), err: boom}

		err := checkSave(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Get() error") {
			t.Fatalf("checkSave() = %v, want Get error", err)
		}
	})

	t.Run("data and id mismatch", func(t *testing.T) {
		t.Parallel()

		stub := &dropDataStore{stub: healthyStubStore()}

		err := checkSave(t.Context(), stub)
		if err == nil {
			t.Fatal("checkSave(drop-data) = nil, want joined errors")
		}
		for _, want := range []string{"Data[user]", "Get().ID"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkSave() = %v, want containing %q", err, want)
			}
		}
	})
}

// failGetAfterSaveStore fails Get calls after the first Create+Save.
type failGetAfterSaveStore struct {
	stub *stubStore
	err  error
}

func (f *failGetAfterSaveStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	return f.stub.Create(ctx, ttl)
}

func (f *failGetAfterSaveStore) Save(ctx context.Context, s session.Session) error {
	return f.stub.Save(ctx, s)
}

func (f *failGetAfterSaveStore) Get(context.Context, string) (session.Session, error) {
	return session.Session{}, f.err
}

func (f *failGetAfterSaveStore) Delete(ctx context.Context, id string) error {
	return f.stub.Delete(ctx, id)
}

func (f *failGetAfterSaveStore) Close() error { return f.stub.Close() }

// dropDataStore drops Data and swaps the ID on Get.
type dropDataStore struct {
	stub *stubStore
}

func (d *dropDataStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	return d.stub.Create(ctx, ttl)
}

func (d *dropDataStore) Save(ctx context.Context, s session.Session) error {
	return d.stub.Save(ctx, s)
}

func (d *dropDataStore) Get(ctx context.Context, id string) (session.Session, error) {
	got, err := d.stub.Get(ctx, id)
	if err != nil {
		return got, err
	}
	got.Data = map[string]any{}
	got.ID = session.NewID()
	return got, nil
}

func (d *dropDataStore) Delete(ctx context.Context, id string) error {
	return d.stub.Delete(ctx, id)
}

func (d *dropDataStore) Close() error { return d.stub.Close() }

func TestCheckDeleteSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkDelete(t.Context(), healthyStubStore()))
}

func TestCheckDeleteFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("sessiontest: boom")

	t.Run("delete missing error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.deleteErr = boom

		if err := checkDelete(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkDelete() = %v, want wrap of boom", err)
		}
	})

	t.Run("create error", func(t *testing.T) {
		t.Parallel()

		stub := &failSecondDeleteStore{stub: healthyStubStore(), failOn: 0, err: boom}
		stub.stub.createErr = boom

		if err := checkDelete(t.Context(), stub); !errors.Is(err, boom) {
			t.Fatalf("checkDelete() = %v, want wrap of boom", err)
		}
	})

	t.Run("delete error", func(t *testing.T) {
		t.Parallel()

		stub := &failSecondDeleteStore{stub: healthyStubStore(), failOn: 2, err: boom}

		err := checkDelete(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Delete() error") {
			t.Fatalf("checkDelete() = %v, want Delete error", err)
		}
	})

	t.Run("get after delete found and delete again error", func(t *testing.T) {
		t.Parallel()

		stub := &noDeleteStore{stub: healthyStubStore(), deleteAgainErr: boom}

		err := checkDelete(t.Context(), stub)
		if err == nil {
			t.Fatal("checkDelete(no-delete) = nil, want joined errors")
		}
		for _, want := range []string{"after Delete", "Delete(again)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkDelete() = %v, want containing %q", err, want)
			}
		}
	})
}

// failSecondDeleteStore fails the nth Delete call.
type failSecondDeleteStore struct {
	stub   *stubStore
	failOn int
	calls  int
	err    error
}

func (f *failSecondDeleteStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	return f.stub.Create(ctx, ttl)
}

func (f *failSecondDeleteStore) Get(ctx context.Context, id string) (session.Session, error) {
	return f.stub.Get(ctx, id)
}

func (f *failSecondDeleteStore) Save(ctx context.Context, s session.Session) error {
	return f.stub.Save(ctx, s)
}

func (f *failSecondDeleteStore) Delete(ctx context.Context, id string) error {
	f.calls++
	if f.calls == f.failOn {
		return f.err
	}
	return f.stub.Delete(ctx, id)
}

func (f *failSecondDeleteStore) Close() error { return f.stub.Close() }

// noDeleteStore keeps records on Delete and fails the second Delete.
type noDeleteStore struct {
	stub           *stubStore
	deleteAgainErr error
	calls          int
}

func (n *noDeleteStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	return n.stub.Create(ctx, ttl)
}

func (n *noDeleteStore) Get(ctx context.Context, id string) (session.Session, error) {
	return n.stub.Get(ctx, id)
}

func (n *noDeleteStore) Save(ctx context.Context, s session.Session) error {
	return n.stub.Save(ctx, s)
}

func (n *noDeleteStore) Delete(_ context.Context, _ string) error {
	n.calls++
	if n.calls <= 2 {
		return nil
	}
	return n.deleteAgainErr
}

func (n *noDeleteStore) Close() error { return n.stub.Close() }

func TestCheckTTLExpirySuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkTTLExpiry(t.Context(), healthyStubStore(), DefaultEntryTTL, DefaultExpiryTimeout))
}

func TestCheckTTLExpiryFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("sessiontest: boom")

	t.Run("create error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.createErr = boom

		if err := checkTTLExpiry(t.Context(), stub, time.Minute, time.Second); !errors.Is(err, boom) {
			t.Fatalf("checkTTLExpiry() = %v, want wrap of boom", err)
		}
	})

	t.Run("bad expiry and get before expiry", func(t *testing.T) {
		t.Parallel()

		stub := &failGetBeforeExpiryStore{stub: healthyStubStore(), err: boom}

		err := checkTTLExpiry(t.Context(), stub, time.Minute, time.Second)
		if err == nil {
			t.Fatal("checkTTLExpiry() = nil, want joined errors")
		}
	})

	t.Run("never expires", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.neverExpire = true

		err := checkTTLExpiry(t.Context(), stub, time.Hour, 60*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "condition not met") {
			t.Fatalf("checkTTLExpiry() = %v, want poll-timeout error", err)
		}
	})
}

// failGetBeforeExpiryStore has zero expiry and fails Get.
type failGetBeforeExpiryStore struct {
	stub *stubStore
	err  error
}

func (f *failGetBeforeExpiryStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	sess, err := f.stub.Create(ctx, ttl)
	if err != nil {
		return sess, err
	}
	sess.ExpiresAt = time.Time{}
	return sess, nil
}

func (f *failGetBeforeExpiryStore) Get(context.Context, string) (session.Session, error) {
	return session.Session{}, f.err
}

func (f *failGetBeforeExpiryStore) Save(ctx context.Context, s session.Session) error {
	return f.stub.Save(ctx, s)
}

func (f *failGetBeforeExpiryStore) Delete(ctx context.Context, id string) error {
	return f.stub.Delete(ctx, id)
}

func (f *failGetBeforeExpiryStore) Close() error { return f.stub.Close() }

func TestCheckOpenRegisterSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkOpenRegister(healthyStubStore()))
}

func TestCheckErrorsReportsAll(t *testing.T) {
	t.Parallel()

	mustPass(t, checkErrors(t.Context(), healthyStubStore()))

	err := checkErrors(t.Context(), &permissiveSessionStore{})
	if err == nil {
		t.Fatal("checkErrors(permissive) = nil, want joined errors")
	}

	for _, want := range []string{"Get(bogus)", "errors.As", "Delete(bogus)", "Save(bogus)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("checkErrors() = %v, want containing %q", err, want)
		}
	}
}

// permissiveSessionStore accepts bogus IDs everywhere.
type permissiveSessionStore struct{}

func (permissiveSessionStore) Create(_ context.Context, ttl time.Duration) (session.Session, error) {
	return session.NewSession(session.NewID(), ttl), nil
}

func (permissiveSessionStore) Get(_ context.Context, _ string) (session.Session, error) {
	return session.Session{ID: session.NewID()}, nil
}

func (permissiveSessionStore) Save(_ context.Context, _ session.Session) error { return nil }

func (permissiveSessionStore) Delete(_ context.Context, _ string) error { return nil }

func (permissiveSessionStore) Close() error { return nil }

func TestCheckCloseSuccess(t *testing.T) {
	t.Parallel()

	mustPass(t, checkClose(t.Context(), healthyStubStore()))
}

func TestCheckCloseFailures(t *testing.T) {
	t.Parallel()

	boom := errors.New("sessiontest: boom")

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

		stub := &neverClosesSessionStore{stub: healthyStubStore()}

		err := checkClose(t.Context(), stub)
		if err == nil {
			t.Fatal("checkClose(never-closes) = nil, want ErrClosed errors")
		}
		for _, want := range []string{"Create()", "Get()", "Save()", "Delete()"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("checkClose() = %v, want containing %q", err, want)
			}
		}
	})

	t.Run("second close error", func(t *testing.T) {
		t.Parallel()

		stub := healthyStubStore()
		stub.closeAg = boom

		err := checkClose(t.Context(), stub)
		if err == nil || !strings.Contains(err.Error(), "Close() second") {
			t.Fatalf("checkClose() = %v, want second-close error", err)
		}
	})
}

// neverClosesSessionStore reports nil from Close without marking closed.
type neverClosesSessionStore struct {
	stub *stubStore
}

func (n *neverClosesSessionStore) Create(ctx context.Context, ttl time.Duration) (session.Session, error) {
	return n.stub.Create(ctx, ttl)
}

func (n *neverClosesSessionStore) Get(ctx context.Context, id string) (session.Session, error) {
	return n.stub.Get(ctx, id)
}

func (n *neverClosesSessionStore) Save(ctx context.Context, s session.Session) error {
	return n.stub.Save(ctx, s)
}

func (n *neverClosesSessionStore) Delete(ctx context.Context, id string) error {
	return n.stub.Delete(ctx, id)
}

func (n *neverClosesSessionStore) Close() error { return nil }

func TestPollExpiryTimeout(t *testing.T) {
	t.Parallel()

	err := pollExpiry(t.Context(), 40*time.Millisecond, "never true", func(context.Context) bool {
		return false
	})
	if err == nil || !strings.Contains(err.Error(), "condition not met") {
		t.Fatalf("pollExpiry() = %v, want timeout error", err)
	}

	mustPass(t, pollExpiry(t.Context(), time.Second, "at once", func(context.Context) bool {
		return true
	}))
}

func TestCheckConcurrent(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := context.Background()

			if err := checkCreateGet(ctx, healthyStubStore()); err != nil {
				t.Errorf("checkCreateGet() = %v, want nil", err)
			}

			if err := checkDelete(ctx, healthyStubStore()); err != nil {
				t.Errorf("checkDelete() = %v, want nil", err)
			}

			if err := checkClose(ctx, healthyStubStore()); err != nil {
				t.Errorf("checkClose() = %v, want nil", err)
			}
		}()
	}

	wg.Wait()
}
