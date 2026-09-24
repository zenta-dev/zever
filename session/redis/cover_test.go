package redis_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/session"
)

// plantRaw writes a raw wire record directly, bypassing the store, for
// expiry/corruption scenarios the public API cannot construct.
func plantRaw(t *testing.T, prefix, id, raw string) {
	t.Helper()

	if err := rawClient(t).Set(t.Context(), prefix+":"+id, raw, time.Hour).Err(); err != nil {
		t.Fatalf("plant: %v", err)
	}
}

func wireJSON(t *testing.T, expiresAt time.Time) string {
	t.Helper()

	var exp int64
	if !expiresAt.IsZero() {
		exp = expiresAt.UnixNano()
	}
	b, err := json.Marshal(map[string]any{
		"data":       map[string]any{"v": 1},
		"created_at": time.Now().Add(-time.Hour).UnixNano(),
		"updated_at": time.Now().Add(-time.Hour).UnixNano(),
		"expires_at": exp,
	})
	if err != nil {
		t.Fatalf("marshal wire: %v", err)
	}
	return string(b)
}

func TestCoverGetAppExpiredDeletes(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	id := session.NewID()

	// Record alive server-side (1h TTL) but expired by app clock.
	plantRaw(t, opts.Redis.Prefix, id, wireJSON(t, time.Now().Add(-time.Hour)))

	if _, err := st.Get(ctx, id); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("Get(app-expired) = %v, want ErrNotFound", err)
	}
	if n, _ := rawClient(t).Exists(ctx, opts.Redis.Prefix+":"+id).Result(); n != 0 {
		t.Fatal("expired key not deleted best-effort")
	}
}

func TestCoverSaveOverCorruptUpsertsFresh(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	id := session.NewID()

	plantRaw(t, opts.Redis.Prefix, id, "{corrupt")

	s := session.Session{ID: id, Data: map[string]any{"v": 2}}
	if err := st.Save(ctx, s); err != nil {
		t.Fatalf("Save over corrupt: %v", err)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after resave: %v", err)
	}
	if got.Data["v"] != float64(2) {
		t.Errorf("resaved data = %v, want 2", got.Data["v"])
	}
}

func TestCoverSaveZeroExpiryPersists(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	id := session.NewID()

	// Existing record with no absolute expiry: Save must persist
	// without expiry rather than inventing one.
	plantRaw(t, opts.Redis.Prefix, id, wireJSON(t, time.Time{}))

	s := session.Session{ID: id, Data: map[string]any{"v": 3}}
	if err := st.Save(ctx, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ttl, err := rawClient(t).TTL(ctx, opts.Redis.Prefix+":"+id).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl != -1 {
		t.Errorf("TTL = %v, want persist (-1)", ttl)
	}
}

func TestCoverSaveOverExpiredUpsertsFresh(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	st := newTestStore(t, opts)
	id := session.NewID()

	// Live server-side but expired by app clock: Save treats as
	// missing and writes a fresh default expiry.
	plantRaw(t, opts.Redis.Prefix, id, wireJSON(t, time.Now().Add(-time.Hour)))

	before := time.Now()
	if err := st.Save(ctx, session.Session{ID: id, Data: map[string]any{"v": 1}}); err != nil {
		t.Fatalf("Save over expired: %v", err)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after resave: %v", err)
	}
	if !got.ExpiresAt.After(before) {
		t.Errorf("ExpiresAt = %v, want fresh expiry after %v", got.ExpiresAt, before)
	}
}

func TestCoverSaveFreshIDStampsTimes(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := newTestStore(t, testOptions(t))
	id := session.NewID()

	before := time.Now()
	if err := st.Save(ctx, session.Session{ID: id, Data: map[string]any{"a": 1}}); err != nil {
		t.Fatalf("Save fresh ID: %v", err)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want >= %v", got.CreatedAt, before)
	}
	if !got.ExpiresAt.After(got.CreatedAt) {
		t.Errorf("ExpiresAt = %v, want after CreatedAt", got.ExpiresAt)
	}
}

func TestCoverSaveEncodeFails(t *testing.T) {
	t.Parallel()

	st := newTestStore(t, testOptions(t))

	s := session.Session{ID: session.NewID(), Data: map[string]any{"f": func() {}}}
	if err := st.Save(t.Context(), s); err == nil {
		t.Fatal("Save(func data) = nil, want encode error")
	}
}
