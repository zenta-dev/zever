package render

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	postgresd "github.com/zenta-dev/zever/orm/dialect/postgres"
	sqlited "github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// renderTwice renders the same shape twice through fn and asserts the two
// (query, args) pairs are byte-identical -- the shape cache's zero-behavior-
// change contract -- AND that the second render was actually a cache hit.
func renderTwice(t *testing.T, fn func() (string, []any, error)) {
	t.Helper()

	resetShapeCache()

	q1, a1, err := fn()
	if err != nil {
		t.Fatalf("first render: %v", err)
	}

	hitsBefore := shapeCacheHits.Load()

	q2, a2, err := fn()
	if err != nil {
		t.Fatalf("second render: %v", err)
	}

	if q1 != q2 {
		t.Fatalf("cached text differs:\n  first:  %q\n  second: %q", q1, q2)
	}

	if !reflect.DeepEqual(a1, a2) {
		t.Fatalf("cached args differ:\n  first:  %#v\n  second: %#v", a1, a2)
	}

	if shapeCacheHits.Load() == hitsBefore {
		t.Fatalf("second render did not hit the shape cache (hits unchanged at %d)", hitsBefore)
	}
}

func nBinary(table, column string, op Op, value any) Node {
	return Node{Kind: KindBinary, Table: table, Column: column, Op: op, Value: value}
}

func nIn(table, column string, vs ...any) Node {
	return Node{Kind: KindIn, Table: table, Column: column, Value: vs}
}

func nBetween(table, column string, lo, hi any) Node {
	return Node{Kind: KindBetween, Table: table, Column: column, Value: [2]any{lo, hi}}
}

func nLike(table, column string, v any) Node {
	return Node{Kind: KindLike, Table: table, Column: column, Value: v}
}

func nCompound(op CompoundOp, children ...Node) Node {
	return Node{Kind: KindCompound, Compound: op, Children: children}
}

var whereShapes = []struct {
	name  string
	where Node
}{
	{"none", Node{}},
	{"eq", nBinary("users", "id", OpEq, int64(1))},
	{"neq", nBinary("users", "id", OpNeq, int64(1))},
	{"gt", nBinary("users", "age", OpGt, int64(30))},
	{"lte", nBinary("users", "age", OpLte, int64(30))},
	{"isnull", nBinary("users", "bio", OpIsNull, nil)},
	{"isnotnull", nBinary("users", "bio", OpIsNotNull, nil)},
	{"in1", nIn("users", "id", int64(1))},
	{"in3", nIn("users", "id", int64(1), int64(2), int64(3))},
	{"between", nBetween("users", "age", int64(20), int64(40))},
	{"like", nLike("users", "name", "a%")},
	{"and", nCompound(CompoundAnd, nBinary("users", "active", OpEq, true), nBinary("users", "age", OpGt, int64(30)))},
	{"or", nCompound(CompoundOr, nBinary("users", "active", OpEq, true), nBinary("users", "age", OpLt, int64(30)))},
	{"not", nCompound(CompoundNot, nBinary("users", "active", OpEq, false))},
	{"qualified-col", nBinary("users", "users.id", OpEq, int64(1))},
}

func dialects(t *testing.T) []dialect.Dialect {
	t.Helper()

	return []dialect.Dialect{sqlited.New(), postgresd.New()}
}

// TestSelectShapeCacheIsByteIdentical renders every representative WHERE/
// ORDER/LIMIT shape twice and proves the cached second render matches the
// first exactly, on every dialect.
func TestSelectShapeCacheIsByteIdentical(t *testing.T) {
	for _, d := range dialects(t) {
		for _, w := range whereShapes {
			for _, mod := range []struct {
				name   string
				order  []OrderTerm
				limit  int
				offset int
			}{
				{"plain", nil, 0, 0},
				{"limit", nil, 10, 0},
				{"limit-offset", nil, 10, 20},
				{"order", []OrderTerm{{Column: "id", Desc: true}}, 0, 0},
			} {
				w := w
				mod := mod

				t.Run(d.Name()+"/"+w.name+"/"+mod.name, func(t *testing.T) {
					renderTwice(t, func() (string, []any, error) {
						return Select(d, "users", []string{"id", "name", "age", "active"}, w.where, mod.order, mod.limit, mod.offset)
					})
				})
			}
		}
	}
}

// TestCountUpdateDeleteInsertShapeCacheIsByteIdentical extends the
// byte-identical contract to the other cached renderers: Count, Update,
// Delete, Insert and InsertMany.
func TestCountUpdateDeleteInsertShapeCacheIsByteIdentical(t *testing.T) {
	for _, d := range dialects(t) {
		t.Run(d.Name()+"/count", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Count(d, "users", nBinary("users", "active", OpEq, true))
			})
		})

		t.Run(d.Name()+"/count-none", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Count(d, "users", Node{})
			})
		})

		t.Run(d.Name()+"/update", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Update(d, "users",
					[]Assignment{{Column: "age", Value: int64(99)}, {Column: "name", Value: "x"}},
					nBinary("users", "id", OpEq, int64(7)), nil, 0, 0)
			})
		})

		t.Run(d.Name()+"/update-no-where", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Update(d, "users", []Assignment{{Column: "age", Value: int64(99)}}, Node{}, nil, 0, 0)
			})
		})

		t.Run(d.Name()+"/delete", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Delete(d, "users", nBinary("users", "id", OpEq, int64(7)), nil, 0, 0)
			})
		})

		t.Run(d.Name()+"/delete-no-where", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Delete(d, "users", Node{}, nil, 0, 0)
			})
		})

		t.Run(d.Name()+"/insert", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Insert(d, "users", []string{"id", "name"}, []any{int64(1), "x"})
			})
		})

		t.Run(d.Name()+"/insert-extra-values", func(t *testing.T) {
			// More values than columns: the renderer truncates to the
			// shorter side, so the cached hit must truncate identically.
			renderTwice(t, func() (string, []any, error) {
				return Insert(d, "users", []string{"id", "name"}, []any{int64(1), "x", "dropped"})
			})
		})

		t.Run(d.Name()+"/insert-default", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return Insert(d, "users", nil, nil)
			})
		})

		t.Run(d.Name()+"/insert-many", func(t *testing.T) {
			renderTwice(t, func() (string, []any, error) {
				return InsertMany(d, "users", []string{"id", "name"},
					[][]any{{int64(1), "a"}, {int64(2), "b"}, {int64(3), "c"}})
			})
		})
	}
}

// TestSelectDifferentShapesDoNotCollide proves two shapes that differ only
// in argument values (one shape, one cache key) share ONE cache entry,
// while structurally different shapes (different SQL text) never collide.
func TestSelectDifferentShapesDoNotCollide(t *testing.T) {
	d := sqlited.New()

	resetShapeCache()

	cols := []string{"id", "name"}

	eq1, _, err := Select(d, "users", cols, nBinary("users", "id", OpEq, int64(1)), nil, 0, 0)
	if err != nil {
		t.Fatalf("eq1: %v", err)
	}

	eq2, _, err := Select(d, "users", cols, nBinary("users", "id", OpEq, int64(2)), nil, 0, 0)
	if err != nil {
		t.Fatalf("eq2: %v", err)
	}

	// Same shape, different value: identical SQL text (one cache key).
	if eq1 != eq2 {
		t.Fatalf("same shape different value rendered different text:\n  %q\n  %q", eq1, eq2)
	}

	in, _, err := Select(d, "users", cols, nIn("users", "id", int64(1), int64(2)), nil, 0, 0)
	if err != nil {
		t.Fatalf("in: %v", err)
	}

	if in == eq1 {
		t.Fatalf("structurally different shapes collided on one cache entry")
	}

	other, _, err := Select(d, "users", cols, nBinary("users", "age", OpGt, int64(1)), nil, 0, 0)
	if err != nil {
		t.Fatalf("other: %v", err)
	}

	if other == eq1 {
		t.Fatalf("different column shapes collided on one cache entry")
	}
}

// TestUncacheableShapesBypassCache proves Raw/JSON/FTS predicates (and
// FTS order terms) render fresh every time -- never cached, never hit --
// while still rendering identically across repeats.
func TestUncacheableShapesBypassCache(t *testing.T) {
	for _, d := range dialects(t) {
		raw := Node{Kind: KindRaw, Value: RawExpr{Fragment: "x = ?", Args: []any{int64(1)}}}

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "users", []string{"id"}, raw, nil, 0, 0)
		})

		jsonNode := Node{
			Kind:   KindJSON,
			Table:  "users",
			Column: "attrs",
			Op:     OpEq,
			Value:  "a",
			JSON:   &JSONExpr{Op: JSONExtract, Steps: []JSONStep{{Key: "k"}}},
		}

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "users", []string{"id"}, jsonNode, nil, 0, 0)
		})

		ftsNode := Node{Kind: KindFTS, Table: "users", Column: "body", FTS: &FTSExpr{Op: FTSMatch, Query: "x"}}

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "users", []string{"id"}, ftsNode, nil, 0, 0)
		})

		ftsOrder := []OrderTerm{{Column: "body", FTS: &FTSExpr{Op: FTSRank, Query: "x"}}}

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "users", []string{"id"}, Node{}, ftsOrder, 0, 0)
		})
	}
}

// renderTwiceNoHit renders the same shape twice and proves both renders
// bypass the cache: identical output, but zero cache hits.
func renderTwiceNoHit(t *testing.T, fn func() (string, []any, error)) {
	t.Helper()

	resetShapeCache()

	q1, a1, err := fn()
	if err != nil {
		t.Fatalf("first render: %v", err)
	}

	q2, a2, err := fn()
	if err != nil {
		t.Fatalf("second render: %v", err)
	}

	if q1 != q2 {
		t.Fatalf("repeated render text differs:\n  first:  %q\n  second: %q", q1, q2)
	}

	if !reflect.DeepEqual(a1, a2) {
		t.Fatalf("repeated render args differ:\n  first:  %#v\n  second: %#v", a1, a2)
	}

	if shapeCacheHits.Load() != 0 {
		t.Fatalf("uncacheable shape produced %d cache hits, want 0", shapeCacheHits.Load())
	}
}

// TestShapeCacheIsBounded proves the cache never exceeds its capacity even
// under an unbounded stream of distinct shapes. Each iteration projects a
// different column, so every shape key is distinct and the cache must
// clear-on-overflow several times over.
func TestShapeCacheIsBounded(t *testing.T) {
	d := sqlited.New()

	resetShapeCache()

	for i := 0; i < shapeCacheCapacity*4; i++ {
		cols := []string{"id" + strconv.Itoa(i)}

		if _, _, err := Select(d, "users", cols, Node{}, nil, 0, 0); err != nil {
			t.Fatalf("shape %d: %v", i, err)
		}
	}

	if n := shapeCacheLen(); n > shapeCacheCapacity {
		t.Fatalf("cache holds %d entries, capacity %d", n, shapeCacheCapacity)
	}

	if n := shapeCacheLen(); n == 0 {
		t.Fatal("cache holds no entries, want the overflow survivor")
	}
}

// noArrowDialect is a dialect.Dialect reporting no JSON `->>` support, so
// the dialect key's capability flag renders "0".
type noArrowDialect struct{ dialect.Dialect }

func (noArrowDialect) SupportsJSONArrowText() bool { return false }

func TestWriteDialectKeyFlags(t *testing.T) {
	t.Parallel()

	var b strings.Builder

	writeDialectKey(&b, noArrowDialect{postgresd.New()})

	want := "postgres" + fsep + "0" + fsep + "0"
	if b.String() != want {
		t.Fatalf("key = %q, want %q", b.String(), want)
	}
}

func TestWriteFingerprintExprUncacheableKind(t *testing.T) {
	t.Parallel()

	var b strings.Builder

	writeFingerprintExpr(&b, Node{Kind: KindJSON})

	if !strings.HasPrefix(b.String(), "X"+fsep) {
		t.Fatalf("fingerprint = %q, want the uncacheable X marker", b.String())
	}
}

func TestInLenNonSlice(t *testing.T) {
	t.Parallel()

	if got := inLen("x"); got != 0 {
		t.Fatalf("inLen(string) = %d, want 0", got)
	}

	if got := inLen(nil); got != 0 {
		t.Fatalf("inLen(nil) = %d, want 0", got)
	}
}

func TestCountCollectedArgsShapes(t *testing.T) {
	t.Parallel()

	if got := countCollectedArgs(Node{Kind: KindIn, Value: "x"}); got != 0 {
		t.Fatalf("non-slice IN collects %d args, want 0", got)
	}

	if got := countCollectedArgs(Node{Kind: KindCompound, Compound: CompoundNot}); got != 0 {
		t.Fatalf("childless NOT collects %d args, want 0", got)
	}

	if got := countCollectedArgs(Node{Kind: KindJSON}); got != 0 {
		t.Fatalf("JSON collects %d args, want 0", got)
	}
}

func TestUpdateDeleteLimitOffsetCacheHit(t *testing.T) {
	// Serial: renderTwice asserts a cache hit, which concurrent cache
	// traffic could evict.
	t.Run("update", func(t *testing.T) {
		d := postgresd.New()

		renderTwice(t, func() (string, []any, error) {
			return Update(d, "users",
				[]Assignment{{Column: "age", Value: int64(99)}},
				nBinary("users", "id", OpEq, int64(7)), nil, 5, 10)
		})
	})

	t.Run("delete", func(t *testing.T) {
		d := postgresd.New()

		renderTwice(t, func() (string, []any, error) {
			return Delete(d, "users", nBinary("users", "id", OpEq, int64(7)), nil, 5, 10)
		})
	})
}

func TestCountUpdateDeleteUncacheableAndMissErrors(t *testing.T) {
	// Serial: renderTwiceNoHit asserts zero cache hits, which concurrent
	// cache traffic could break.
	raw := Node{Kind: KindRaw, Value: RawExpr{Fragment: "id = ?", Args: []any{int64(1)}}}
	bad := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: int64(1)}

	t.Run("update uncacheable renders fresh", func(t *testing.T) {
		renderTwiceNoHit(t, func() (string, []any, error) {
			return Update(sqlited.New(), "users",
				[]Assignment{{Column: "age", Value: int64(99)}}, raw, nil, 0, 0)
		})
	})

	t.Run("update miss error propagates", func(t *testing.T) {
		if _, _, err := Update(sqlited.New(), "users",
			[]Assignment{{Column: "age", Value: int64(99)}}, bad, nil, 0, 0); err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})

	t.Run("delete uncacheable renders fresh", func(t *testing.T) {
		renderTwiceNoHit(t, func() (string, []any, error) {
			return Delete(sqlited.New(), "users", raw, nil, 0, 0)
		})
	})

	t.Run("delete miss error propagates", func(t *testing.T) {
		if _, _, err := Delete(sqlited.New(), "users", bad, nil, 0, 0); err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})
}
