package render

import (
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestSelectModsExtendedLockModes pins the exact text of the Postgres-only
// lock modes and the OF table list.
func TestSelectModsExtendedLockModes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		mods SelectModifiers
		want string
	}{
		{"for-no-key-update", SelectModifiers{Lock: LockForNoKeyUpdate}, `SELECT "id" FROM "widgets" FOR NO KEY UPDATE`},
		{"for-key-share", SelectModifiers{Lock: LockForKeyShare}, `SELECT "id" FROM "widgets" FOR KEY SHARE`},
		{"for-update-of", SelectModifiers{Lock: LockForUpdate, LockOf: []string{"widgets"}}, `SELECT "id" FROM "widgets" FOR UPDATE OF "widgets"`},
		{"for-update-of-multi", SelectModifiers{Lock: LockForUpdate, LockOf: []string{"widgets", "orders"}}, `SELECT "id" FROM "widgets" FOR UPDATE OF "widgets", "orders"`},
		{"for-update-of-nowait", SelectModifiers{Lock: LockForUpdate, LockOf: []string{"widgets"}, NoWait: true}, `SELECT "id" FROM "widgets" FOR UPDATE OF "widgets" NOWAIT`},
		{"for-no-key-update-skip-locked", SelectModifiers{Lock: LockForNoKeyUpdate, SkipLocked: true}, `SELECT "id" FROM "widgets" FOR NO KEY UPDATE SKIP LOCKED`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, args, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0, tc.mods)
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

// TestSelectModsLockOfWithoutModeErrors proves an OF list with no lock mode
// fails closed rather than rendering a dangling clause.
func TestSelectModsLockOfWithoutModeErrors(t *testing.T) {
	t.Parallel()
	if _, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{LockOf: []string{"widgets"}}); err == nil {
		t.Fatalf("SelectMods(LockOf without Lock) err = nil, want an error")
	}
}

// TestSelectModsExtendedLockShapeCache proves the new lock modes and the OF
// list are part of the shape-cache key.
func TestSelectModsExtendedLockShapeCache(t *testing.T) {
	d := postgres.New()

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForNoKeyUpdate})
	})

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForUpdate, LockOf: []string{"widgets"}})
	})

	resetShapeCache()

	a, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForNoKeyUpdate})
	b, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForKeyShare})
	c, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForUpdate, LockOf: []string{"widgets"}})
	plain, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Lock: LockForUpdate})

	if a == b || a == c || a == plain || b == c || b == plain || c == plain {
		t.Fatalf("extended lock shapes collided:\n a=%q\n b=%q\n c=%q\n plain=%q", a, b, c, plain)
	}
}

// TestLockingDialectStillSatisfied guards the pre-existing capability
// interface while the new Postgres-only modes gate on ExtendedLockingDialect.
func TestLockingDialectStillSatisfied(_ *testing.T) {
	var (
		_ dialect.LockingDialect = sqlite.New()
		_ dialect.LockingDialect = postgres.New()
	)
}
