package kvstore

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

// mustStore opens a fresh file-backed sqlite DB per test and wraps it in a
// Store. Fresh files (never ":memory:" shared cache) keep fixed keys
// isolated across reruns and parallel tests.
func mustStore(t *testing.T) *Store {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kv.db")

	db, err := dbsqlite.New(coredb.Options{Path: path})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = db.Close(t.Context()) })

	s, err := New(db, "")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	return s
}

// expireKey rewrites a record's expiry directly: deterministic, no sleep.
func expireKey(t *testing.T, s *Store, key string, exp time.Time) {
	t.Helper()

	n, err := orm.UpdateTable(s.tbl).Where(s.cKey.Eq(key)).
		Set(orm.Set(s.cExpires, exp)).Exec(t.Context(), s.db)
	if err != nil {
		t.Fatalf("expire %q: %v", key, err)
	}

	if n != 1 {
		t.Fatalf("expire %q affected %d rows, want 1", key, n)
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "k1", []byte("v1"), time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, ok, err := s.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !ok {
		t.Fatal("Get ok = false, want true")
	}

	if string(got) != "v1" {
		t.Fatalf("Get = %q, want %q", got, "v1")
	}
}

func TestGetMiss(t *testing.T) {
	t.Parallel()

	s := mustStore(t)

	if _, ok, err := s.Get(t.Context(), "missing"); err != nil || ok {
		t.Fatalf("Get = (%v, %v), want (nil, false)", ok, err)
	}
}

func TestSetOverwrite(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "k", []byte("old"), time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if err := s.Set(ctx, "k", []byte("new"), time.Hour); err != nil {
		t.Fatalf("Set overwrite failed: %v", err)
	}

	got, ok, err := s.Get(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want value", ok, err)
	}

	if string(got) != "new" {
		t.Fatalf("Get = %q, want %q", got, "new")
	}
}

func TestNoExpiryNeverSwept(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "immortal", []byte("v"), 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if n, err := s.SweepExpired(ctx); err != nil || n != 0 {
		t.Fatalf("SweepExpired = (%d, %v), want (0, nil)", n, err)
	}

	if _, ok, err := s.Get(ctx, "immortal"); err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want hit", ok, err)
	}
}

func TestLazyExpiryOnRead(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "gone", []byte("v"), time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	expireKey(t, s, "gone", time.Now().Add(-time.Hour))

	if _, ok, err := s.Get(ctx, "gone"); err != nil || ok {
		t.Fatalf("Get expired = (%v, %v), want miss", ok, err)
	}

	// Lazy delete on read: the row is gone, so a sweep finds nothing.
	if n, err := s.SweepExpired(ctx); err != nil || n != 0 {
		t.Fatalf("SweepExpired = (%d, %v), want (0, nil)", n, err)
	}
}

func TestSweepExpired(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	for _, k := range []string{"e1", "e2", "live", "immortal"} {
		if err := s.Set(ctx, k, []byte("v"), time.Hour); err != nil {
			t.Fatalf("Set %q failed: %v", k, err)
		}
	}

	if err := s.Set(ctx, "immortal2", []byte("v"), 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	past := time.Now().Add(-time.Hour)
	expireKey(t, s, "e1", past)
	expireKey(t, s, "e2", past)

	n, err := s.SweepExpired(ctx)
	if err != nil {
		t.Fatalf("SweepExpired failed: %v", err)
	}

	if n != 2 {
		t.Fatalf("SweepExpired = %d, want 2", n)
	}

	for _, k := range []string{"live", "immortal", "immortal2"} {
		if _, ok, err := s.Get(ctx, k); err != nil || !ok {
			t.Fatalf("Get %q = (%v, %v), want hit", k, ok, err)
		}
	}

	if _, ok, err := s.Get(ctx, "e1"); err != nil || ok {
		t.Fatalf("Get e1 = (%v, %v), want miss", ok, err)
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "k", []byte("v"), time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, ok, err := s.Get(ctx, "k"); err != nil || ok {
		t.Fatalf("Get after Delete = (%v, %v), want miss", ok, err)
	}

	// Missing keys are idempotent.
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete missing failed: %v", err)
	}
}

func TestGetWithExpiry(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()
	before := time.Now()

	if err := s.Set(ctx, "timed", []byte("v"), time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if err := s.Set(ctx, "immortal", []byte("v"), 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	_, exp, ok, err := s.GetWithExpiry(ctx, "timed")
	if err != nil || !ok {
		t.Fatalf("GetWithExpiry = (%v, %v), want hit", ok, err)
	}

	if exp.Before(before.Add(59*time.Minute)) || exp.After(time.Now().Add(time.Hour+time.Minute)) {
		t.Fatalf("expiresAt = %v, want ~now+1h", exp)
	}

	_, exp, ok, err = s.GetWithExpiry(ctx, "immortal")
	if err != nil || !ok {
		t.Fatalf("GetWithExpiry = (%v, %v), want hit", ok, err)
	}

	if !exp.IsZero() {
		t.Fatalf("expiresAt = %v, want zero (no expiry)", exp)
	}

	if _, _, ok, err := s.GetWithExpiry(ctx, "missing"); err != nil || ok {
		t.Fatalf("GetWithExpiry missing = (%v, %v), want miss", ok, err)
	}
}

func TestSetNX(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	won, err := s.SetNX(ctx, "nx", []byte("v1"), time.Hour)
	if err != nil || !won {
		t.Fatalf("SetNX new = (%v, %v), want (true, nil)", won, err)
	}

	won, err = s.SetNX(ctx, "nx", []byte("v2"), time.Hour)
	if err != nil || won {
		t.Fatalf("SetNX existing = (%v, %v), want (false, nil)", won, err)
	}

	got, ok, err := s.Get(ctx, "nx")
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want hit", ok, err)
	}

	if string(got) != "v1" {
		t.Fatalf("Get = %q, want %q (first write wins)", got, "v1")
	}

	if _, err := s.SetNX(ctx, "", []byte("v"), time.Hour); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("SetNX empty = %v, want ErrInvalidKey", err)
	}
}

func TestInvalidKey(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	cases := []struct {
		name string
		key  string
	}{
		{"empty get", ""},
		{"too long get", strings.Repeat("k", MaxKeyLen+1)},
	}

	for _, tc := range cases {
		if _, _, err := s.Get(ctx, tc.key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%s: Get err = %v, want ErrInvalidKey", tc.name, err)
		}

		if err := s.Set(ctx, tc.key, []byte("v"), time.Hour); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%s: Set err = %v, want ErrInvalidKey", tc.name, err)
		}

		if err := s.Delete(ctx, tc.key); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%s: Delete err = %v, want ErrInvalidKey", tc.name, err)
		}
	}
}

func TestNewInvalid(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, ""); err == nil {
		t.Error("New(nil) = nil, want error")
	}

	path := filepath.Join(t.TempDir(), "kv.db")

	db, err := dbsqlite.New(coredb.Options{Path: path})
	if err != nil {
		t.Fatalf("sqlite New failed: %v", err)
	}

	t.Cleanup(func() { _ = db.Close(t.Context()) })

	if _, err := New(db, "no-dashes!"); !errors.Is(err, ErrInvalidTable) {
		t.Errorf("New bad table = %v, want ErrInvalidTable", err)
	}
}

// stubDB fails checkDialect with an unlisted dialect name.
type stubDB struct{ coredb.DB }

func (stubDB) Dialect() string { return "mysql" }

func (stubDB) Ping(context.Context) error { return nil }

func TestUnsupportedDialect(t *testing.T) {
	t.Parallel()

	s := &Store{
		db:        stubDB{},
		tbl:       orm.NewTable[kvRow]("kv", kvColumns),
		cKey:      orm.NewColumn[kvRow, string]("kv", "key"),
		cValue:    orm.NewColumn[kvRow, []byte]("kv", "value"),
		cExpires:  orm.NewColumn[kvRow, time.Time]("kv", "expires_at"),
		tableName: "kv",
	}
	ctx := t.Context()

	if _, _, err := s.Get(ctx, "k"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Errorf("Get err = %v, want ErrUnsupportedByDialect", err)
	}

	if err := s.Set(ctx, "k", []byte("v"), time.Hour); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Errorf("Set err = %v, want ErrUnsupportedByDialect", err)
	}

	if _, err := s.SetNX(ctx, "k", []byte("v"), time.Hour); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Errorf("SetNX err = %v, want ErrUnsupportedByDialect", err)
	}

	if _, _, _, err := s.GetWithExpiry(ctx, "k"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Errorf("GetWithExpiry err = %v, want ErrUnsupportedByDialect", err)
	}

	if err := s.Delete(ctx, "k"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Errorf("Delete err = %v, want ErrUnsupportedByDialect", err)
	}

	if _, err := s.SweepExpired(ctx); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Errorf("SweepExpired err = %v, want ErrUnsupportedByDialect", err)
	}

	if _, err := s.AddDelta(ctx, "k", 1); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Errorf("AddDelta err = %v, want ErrUnsupportedByDialect", err)
	}
}

func TestAddDeltaMissingStartsAtOne(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	n, err := s.AddDelta(ctx, "n", 1)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	if n != 1 {
		t.Fatalf("AddDelta = %d, want 1", n)
	}

	got, ok, err := s.Get(ctx, "n")
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want hit", ok, err)
	}

	if string(got) != "1" {
		t.Fatalf("Get = %q, want 1", got)
	}
}

func TestAddDeltaRoundTrip(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if _, err := s.AddDelta(ctx, "n", 1); err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	if _, err := s.AddDelta(ctx, "n", 1); err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	n, err := s.AddDelta(ctx, "n", -1)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	if n != 1 {
		t.Fatalf("AddDelta = %d, want 1", n)
	}

	got, ok, err := s.Get(ctx, "n")
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want hit", ok, err)
	}

	if string(got) != "1" {
		t.Fatalf("Get = %q, want 1", got)
	}
}

func TestAddDeltaFreshDecrement(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	n, err := s.AddDelta(ctx, "fresh", -1)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	if n != -1 {
		t.Fatalf("AddDelta = %d, want -1", n)
	}

	got, ok, err := s.Get(ctx, "fresh")
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want hit", ok, err)
	}

	if string(got) != "-1" {
		t.Fatalf("Get = %q, want -1", got)
	}
}

func TestAddDeltaBaseValue(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "base", []byte("41"), 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	n, err := s.AddDelta(ctx, "base", 1)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	if n != 42 {
		t.Fatalf("AddDelta = %d, want 42", n)
	}

	got, ok, err := s.Get(ctx, "base")
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want hit", ok, err)
	}

	if string(got) != "42" {
		t.Fatalf("Get = %q, want 42", got)
	}
}

func TestAddDeltaNonNumeric(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "bad", []byte("abc"), 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if _, err := s.AddDelta(ctx, "bad", 1); !errors.Is(err, ErrInvalidInteger) {
		t.Fatalf("AddDelta err = %v, want ErrInvalidInteger", err)
	}

	got, ok, err := s.Get(ctx, "bad")
	if err != nil || !ok {
		t.Fatalf("Get = (%v, %v), want hit", ok, err)
	}

	if string(got) != "abc" {
		t.Fatalf("Get = %q, want abc (untouched)", got)
	}
}

func TestAddDeltaPreservesExpiry(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "timed", []byte("10"), time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	_, before, ok, err := s.GetWithExpiry(ctx, "timed")
	if err != nil || !ok {
		t.Fatalf("GetWithExpiry = (%v, %v), want hit", ok, err)
	}

	if _, derr := s.AddDelta(ctx, "timed", 5); derr != nil {
		t.Fatalf("AddDelta failed: %v", derr)
	}

	got, after, ok, err := s.GetWithExpiry(ctx, "timed")
	if err != nil || !ok {
		t.Fatalf("GetWithExpiry = (%v, %v), want hit", ok, err)
	}

	if string(got) != "15" {
		t.Fatalf("Get = %q, want 15", got)
	}

	if !after.Equal(before) {
		t.Fatalf("expiresAt changed: before %v, after %v", before, after)
	}
}

func TestAddDeltaExpiredAsMissing(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if err := s.Set(ctx, "gone", []byte("99"), time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	expireKey(t, s, "gone", time.Now().Add(-time.Hour))

	n, err := s.AddDelta(ctx, "gone", 5)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	if n != 5 {
		t.Fatalf("AddDelta = %d, want 5 (expired base 0)", n)
	}

	got, exp, ok, err := s.GetWithExpiry(ctx, "gone")
	if err != nil || !ok {
		t.Fatalf("GetWithExpiry = (%v, %v), want hit", ok, err)
	}

	if string(got) != "5" {
		t.Fatalf("Get = %q, want 5", got)
	}

	if !exp.IsZero() {
		t.Fatalf("expiresAt = %v, want zero (expired reset)", exp)
	}
}

func TestAddDeltaInvalidKey(t *testing.T) {
	t.Parallel()

	s := mustStore(t)
	ctx := t.Context()

	if _, err := s.AddDelta(ctx, "", 1); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("AddDelta empty = %v, want ErrInvalidKey", err)
	}

	if _, err := s.AddDelta(ctx, strings.Repeat("k", MaxKeyLen+1), 1); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("AddDelta too long = %v, want ErrInvalidKey", err)
	}

	if err := ctx.Err(); err != nil {
		t.Fatalf("ctx cancelled: %v", err)
	}
}
