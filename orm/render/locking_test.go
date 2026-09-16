package render

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestSelectModsDistinct pins the exact text DISTINCT prepends to the
// projection on each dialect, and proves the plain Select entry point (the
// zero modifiers path) still renders no DISTINCT.
func TestSelectModsDistinct(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		d    dialect.Dialect
		want string
	}{
		{"postgres", postgres.New(), `SELECT DISTINCT "id", "name" FROM "widgets"`},
		{"sqlite", sqlite.New(), `SELECT DISTINCT "id", "name" FROM "widgets"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, args, err := SelectMods(tc.d, "widgets", []string{"id", "name"}, Node{}, nil, 0, 0, SelectModifiers{Distinct: true})
			if err != nil {
				t.Fatalf("SelectMods: %v", err)
			}

			if got != tc.want {
				t.Fatalf("query = %q, want %q", got, tc.want)
			}

			if len(args) != 0 {
				t.Fatalf("args = %#v, want none", args)
			}
		})
	}

	// The plain Select path renders no DISTINCT: zero modifiers is the
	// exactly-old behavior.
	plain, _, err := Select(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	if strings.Contains(plain, "DISTINCT") {
		t.Fatalf("plain Select rendered DISTINCT: %q", plain)
	}
}

// TestSelectModsLockForms pins every locking suffix form's exact text on
// Postgres.
func TestSelectModsLockForms(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		d    dialect.Dialect
		want string
	}{
		{"postgres/for-update", postgres.New(), `SELECT "id" FROM "widgets" FOR UPDATE`},
		{"postgres/for-share", postgres.New(), `SELECT "id" FROM "widgets" FOR SHARE`},
		{"postgres/for-update-nowait", postgres.New(), `SELECT "id" FROM "widgets" FOR UPDATE NOWAIT`},
		{"postgres/for-update-skip-locked", postgres.New(), `SELECT "id" FROM "widgets" FOR UPDATE SKIP LOCKED`},
		{"postgres/for-share-nowait", postgres.New(), `SELECT "id" FROM "widgets" FOR SHARE NOWAIT`},
		{"postgres/for-share-skip-locked", postgres.New(), `SELECT "id" FROM "widgets" FOR SHARE SKIP LOCKED`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mods := SelectModifiers{Lock: lockFromName(tc.name), NoWait: strings.HasSuffix(tc.name, "nowait"), SkipLocked: strings.HasSuffix(tc.name, "skip-locked")}

			got, args, err := SelectMods(tc.d, "widgets", []string{"id"}, Node{}, nil, 0, 0, mods)
			if err != nil {
				t.Fatalf("SelectMods: %v", err)
			}

			if got != tc.want {
				t.Fatalf("query = %q, want %q", got, tc.want)
			}

			if len(args) != 0 {
				t.Fatalf("args = %#v, want none", args)
			}
		})
	}
}

// lockFromName derives the lock mode from a "*/for-update" or "*/for-share"
// subtest name.
func lockFromName(name string) LockMode {
	if strings.Contains(name, "for-share") {
		return LockForShare
	}

	return LockForUpdate
}

// TestSelectModsPlaceholderNumbering proves the DISTINCT prefix and the
// locking suffix never disturb $N numbering: bound WHERE args remain $1..$n
// and the LIMIT/OFFSET args continue from there, with the lock clause
// binding nothing. DISTINCT and the lock are exercised separately because
// the combination is forbidden.
func TestSelectModsPlaceholderNumbering(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Table: "widgets", Column: "quantity", Op: OpGt, Value: int64(15)}

	got, args, err := SelectMods(
		postgres.New(), "widgets", []string{"id"},
		where, nil, 5, 10,
		SelectModifiers{Distinct: true},
	)
	if err != nil {
		t.Fatalf("SelectMods(DISTINCT): %v", err)
	}

	want := `SELECT DISTINCT "id" FROM "widgets" WHERE "quantity" > $1 LIMIT $2 OFFSET $3`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	if len(args) != 3 || args[0] != int64(15) || args[1] != 5 || args[2] != 10 {
		t.Fatalf("args = %#v, want [15 5 10]", args)
	}

	got, args, err = SelectMods(
		postgres.New(), "widgets", []string{"id"},
		where, []OrderTerm{{Column: "id"}}, 5, 10,
		SelectModifiers{Lock: LockForUpdate, SkipLocked: true},
	)
	if err != nil {
		t.Fatalf("SelectMods(lock): %v", err)
	}

	want = `SELECT "id" FROM "widgets" WHERE "quantity" > $1 ORDER BY "id" ASC LIMIT $2 OFFSET $3 FOR UPDATE SKIP LOCKED`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	if len(args) != 3 || args[0] != int64(15) || args[1] != 5 || args[2] != 10 {
		t.Fatalf("args = %#v, want [15 5 10]", args)
	}
}

// TestSelectModsDistinctWithLockErrors proves the renderer fails closed on
// the SQL-standard-forbidden DISTINCT + row-lock combination rather than
// emitting invalid SQL.
func TestSelectModsDistinctWithLockErrors(t *testing.T) {
	t.Parallel()
	_, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{Distinct: true, Lock: LockForUpdate})
	if err == nil {
		t.Fatalf("SelectMods(DISTINCT + FOR UPDATE) err = nil, want an error")
	}

	if !strings.Contains(err.Error(), "DISTINCT") {
		t.Fatalf("err = %v, want it to mention DISTINCT", err)
	}
}

// TestSelectModsMalformedLockRejected proves a lock modifier with no lock
// mode and an out-of-range lock mode both fail closed rather than rendering
// a silently-dropped clause.
func TestSelectModsMalformedLockRejected(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		mods SelectModifiers
	}{
		{"nowait-without-lock", SelectModifiers{NoWait: true}},
		{"skip-locked-without-lock", SelectModifiers{SkipLocked: true}},
		{"unknown-lock-mode", SelectModifiers{Lock: LockMode(99)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0, tc.mods); err == nil {
				t.Fatalf("SelectMods(%+v) err = nil, want an error", tc.mods)
			}
		})
	}
}

// TestSelectModsShapeCacheSeparatesDistinctAndLock proves DISTINCT and lock
// mode are part of the shape-cache key: each modifier shape is byte-stable
// across repeats (so it caches) yet distinct from the plain and sibling
// shapes (so no cache entry drops a modifier).
func TestSelectModsShapeCacheSeparatesDistinctAndLock(t *testing.T) {
	d := postgres.New()

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Distinct: true})
	})

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForUpdate})
	})

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForShare, SkipLocked: true})
	})

	resetShapeCache()

	plain, _, _ := Select(d, "widgets", []string{"id"}, Node{}, nil, 0, 0)
	distinct, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Distinct: true})
	update, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForUpdate})
	share, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForShare})

	if plain == distinct || plain == update || plain == share || distinct == update || distinct == share || update == share {
		t.Fatalf("modifier shapes collided on one cache entry:\n  plain=%q\n  distinct=%q\n  update=%q\n  share=%q", plain, distinct, update, share)
	}
}
