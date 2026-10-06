package redis_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/session"
)

func TestEdgeCreate_NegativeTTL_UsesDefault(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	before := time.Now()
	s, err := st.Create(ctx, -time.Hour)
	if err != nil {
		t.Fatalf("Create(negative ttl) err = %v", err)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}

	if got.ExpiresAt.Sub(before) < session.DefaultTTL-time.Minute ||
		got.ExpiresAt.Sub(before) > session.DefaultTTL+time.Minute {
		t.Fatalf("Create(negative ttl) ExpiresAt = %v, want ~now+%v", got.ExpiresAt, session.DefaultTTL)
	}
}

func TestEdgeSave_NilData_NoError(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	id := session.NewID()
	if err := st.Save(ctx, session.Session{ID: id}); err != nil {
		t.Fatalf("Save(nil Data) err = %v", err)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get(nil Data) err = %v", err)
	}
	if len(got.Data) != 0 {
		t.Fatalf("Get(nil Data).Data = %v, want empty", got.Data)
	}
}

func TestEdgeLargeData_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	blob := strings.Repeat("r", 1<<20)
	s.Data["blob"] = blob

	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	gotBlob, ok := got.Data["blob"].(string)
	if !ok {
		t.Fatalf("blob type = %T, want string", got.Data["blob"])
	}

	if gotBlob != blob {
		t.Fatalf("blob round trip mismatch: got %d bytes, want %d", len(gotBlob), len(blob))
	}
}

func TestEdgeSave_ConcurrentSameID(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			cp := s.Clone()
			cp.Data = map[string]any{"i": i}

			if serr := st.Save(ctx, cp); serr != nil {
				t.Errorf("goroutine %d: Save err = %v", i, serr)
			}
		}(i)
	}

	wg.Wait()

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after concurrent Save err = %v", err)
	}
	if got.ID != s.ID {
		t.Fatalf("Get ID = %q, want %q", got.ID, s.ID)
	}
}

func TestEdgeBoundary_ManySessions(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))

	const n = 256

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s, err := st.Create(ctx, time.Hour)
		if err != nil {
			t.Fatalf("Create[%d] err = %v", i, err)
		}

		ids = append(ids, s.ID)
	}

	for i, id := range ids {
		if _, err := st.Get(ctx, id); err != nil {
			t.Fatalf("Get[%d] err = %v", i, err)
		}
	}
}

func TestEdgeGet_AfterFlush_ErrNotFound(t *testing.T) {
	t.Parallel()

	server := testServer(t)
	opts := optionsFor(t, server)
	st := newTestStore(t, opts)
	ctx := t.Context()

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	server.FlushAll()

	if _, err := st.Get(ctx, s.ID); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get after flush err = %v, want ErrNotFound", err)
	}
}

// TestEdgeSave_overExpiredRecord proves Save against an expired record
// writes fresh under the store default TTL, ignoring any caller-provided
// ExpiresAt.
func TestEdgeSave_overExpiredRecord(t *testing.T) {
	t.Parallel()

	server := testServer(t)
	ctx := t.Context()
	st := newTestStore(t, optionsFor(t, server))

	s, err := st.Create(ctx, time.Second)
	if err != nil {
		t.Fatalf("Create err = %v, want nil", err)
	}

	server.FastForward(2 * time.Second)

	if _, getErr := st.Get(ctx, s.ID); !errors.Is(getErr, session.ErrNotFound) {
		t.Fatalf("Get after expiry err = %v, want ErrNotFound", getErr)
	}

	before := time.Now()

	if saveErr := st.Save(ctx, s); saveErr != nil {
		t.Fatalf("Save over expired err = %v, want nil", saveErr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get after Save-over-expired err = %v, want nil", err)
	}

	if got.ExpiresAt.Sub(before) < session.DefaultTTL-time.Minute {
		t.Fatalf("Save-over-expired ExpiresAt = %v, want ~now+%v (fresh default)", got.ExpiresAt, session.DefaultTTL)
	}
}

func TestEdgeClose_ConcurrentIdempotent(t *testing.T) {
	t.Parallel()

	st := newTestStore(t, testOptions(t))

	const n = 16

	var wg sync.WaitGroup

	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs[i] = st.Close()
		}(i)
	}

	wg.Wait()

	for i, cerr := range errs {
		if cerr != nil {
			t.Errorf("Close[%d] = %v, want nil", i, cerr)
		}
	}
}
