package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestRenderRaw proves a KindRaw node renders its fragment verbatim (except
// for `?` markers, rewritten to the dialect's numbered placeholder) with
// args bound positionally -- and that a bound arg containing `'`, `;` or a
// NUL byte can never alter the rendered SQL text, only its own value.
func TestRenderRaw(t *testing.T) {
	t.Parallel()
	payload := "' OR 1=1 --"
	fragment := "name = ?"

	t.Run("sqlite placeholder + bound args", func(t *testing.T) {
		t.Parallel()
		q, args, err := renderExpr(sqlite.New(), Node{Kind: KindRaw, Value: RawExpr{Fragment: fragment, Args: []any{payload}}}, &argCounter{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if q != fragment {
			t.Fatalf("clause = %q, want %q (fragment must render verbatim)", q, fragment)
		}

		if !reflect.DeepEqual(args, []any{payload}) {
			t.Fatalf("args = %#v, want payload bound as the single arg", args)
		}

		if strings.Contains(q, payload) {
			t.Fatalf("clause %q contains the raw payload %q: a bound arg was string-formatted into SQL text", q, payload)
		}
	})

	t.Run("postgres numbered placeholders in clause order", func(t *testing.T) {
		q, args, err := renderExpr(fakePostgres{}, Node{Kind: KindRaw, Value: RawExpr{Fragment: "a = ? AND b = ?", Args: []any{"x", "y"}}}, &argCounter{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := "a = $1 AND b = $2"
		if q != want {
			t.Fatalf("clause = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"x", "y"}) {
			t.Fatalf("args = %#v, want [x y]", args)
		}
	})

	t.Run("numbering continues across a preceding clause", func(t *testing.T) {
		// A Where combining a binary predicate and a raw fragment must
		// number the raw fragment's placeholder after the binary's.
		where := Node{Kind: KindCompound, Compound: CompoundAnd, Children: []Node{
			{Kind: KindBinary, Column: "id", Op: OpEq, Value: "w1"},
			{Kind: KindRaw, Value: RawExpr{Fragment: "name = ?", Args: []any{"'"}}}}}

		q, args, err := renderExpr(fakePostgres{}, where, &argCounter{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `("id" = $1 AND name = $2)`
		if q != want {
			t.Fatalf("clause = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"w1", "'"}) {
			t.Fatalf("args = %#v, want [w1 ']", args)
		}
	})

	t.Run("fragment without markers binds nothing", func(t *testing.T) {
		q, args, err := renderExpr(sqlite.New(), Node{Kind: KindRaw, Value: RawExpr{Fragment: "1 = 1", Args: []any{"ignored"}}}, &argCounter{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if q != "1 = 1" {
			t.Fatalf("clause = %q, want %q", q, "1 = 1")
		}

		if args != nil {
			t.Fatalf("args = %#v, want nil (no ? marker, nothing bound)", args)
		}
	})

	t.Run("more markers than args fails closed without panicking", func(t *testing.T) {
		// A placeholder whose arg position is missing is still emitted: the
		// placeholder/arg mismatch surfaces as a driver error at execution,
		// and renderRaw must never panic or drop the fragment.
		q, args, err := renderExpr(sqlite.New(), Node{Kind: KindRaw, Value: RawExpr{Fragment: "a = ? AND b = ?", Args: []any{"only-one"}}}, &argCounter{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := "a = ? AND b = ?"
		if q != want {
			t.Fatalf("clause = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"only-one"}) {
			t.Fatalf("args = %#v, want the single bound arg", args)
		}
	})

	t.Run("missing RawExpr payload renders nothing", func(t *testing.T) {
		_, _, err := renderExpr(sqlite.New(), Node{Kind: KindRaw, Value: "not-a-raw-expr"}, &argCounter{})

		if err == nil {
			t.Fatalf("err = nil, want an error for a malformed raw node (never a silently empty clause)")
		}
	})

	t.Run("bound arg with semicolon and NUL cannot reach SQL text", func(t *testing.T) {
		evil := "'; DROP TABLE widgets; --\x00"

		q, args, err := renderExpr(sqlite.New(), Node{Kind: KindRaw, Value: RawExpr{Fragment: "name = ?", Args: []any{evil}}}, &argCounter{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if q != "name = ?" {
			t.Fatalf("clause = %q, want %q", q, "name = ?")
		}

		if strings.Contains(q, evil) {
			t.Fatalf("clause %q contains the raw payload: bound args must never be string-formatted", q)
		}

		if !reflect.DeepEqual(args, []any{evil}) {
			t.Fatalf("args = %#v, want the evil payload bound unchanged", args)
		}
	})
}
