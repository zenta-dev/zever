package orm

import (
	"errors"
	"fmt"
	"strings"
)

// RawExpr is the erased payload of an NRaw node, built by UnsafeRaw:
// a caller-supplied SQL fragment plus positional bound arguments. render
// inserts the fragment verbatim (rewriting only its `?` markers to the
// dialect's numbered placeholders) and binds the args as driver arguments
// -- args are NEVER string-formatted into the SQL text, so a bound value
// containing `'`, `;` or a NUL byte alters only its own value, never the
// SQL structure.
//
// RawExpr mirrors render.RawExpr; toRenderNode converts between the two
// with a plain field-for-field literal (the Value field is an `any`, so the
// dynamic type must be converted, unlike Node's statically-typed JSON/
// FTS pointers).
type RawExpr struct {
	Fragment string
	Args     []any
}

// UnsafeIdent builds a Column bound to ident, but ONLY when ident appears
// in allowlist -- the schema-derived identifier set a caller passes, in
// practice codegen's Columns() for the entity. It is the narrow escape
// hatch for a genuinely dynamic identifier (e.g. a user-chosen sort/filter
// column from a bounded UI), and it exists precisely so that dynamic
// identifiers are never assembled by string concatenation elsewhere. Any
// ident outside the allowlist returns an error, never a forged column.
//
// The returned Column renders unqualified (its table is empty), which is
// correct for the query's own table -- the codegen'd NewColumn path remains
// the way to get a table-qualified column for join queries.
//
// UnsafeIdent is unreachable accidentally through the fluent chain: callers
// must explicitly name it, pass a real allowlist, and handle the error.
// Like UnsafeRaw, it is a security escape hatch: the fragment-free
// identifier still comes from a bounded set, never raw caller input.
func UnsafeIdent[T any, V any](ident string, allowlist []string) (Column[T, V], error) {
	if ident == "" {
		return Column[T, V]{}, errors.New("orm: UnsafeIdent: empty ident")
	}

	for _, allowed := range allowlist {
		if ident == allowed {
			return NewColumn[T, V]("", ident), nil
		}
	}

	return Column[T, V]{}, fmt.Errorf("orm: UnsafeIdent: ident %q not in allowlist (allowlist: %s)", ident, strings.Join(allowlist, ", "))
}

// UnsafeRaw builds a Predicate from a caller-supplied SQL fragment and
// positional bound arguments. The fragment is inserted into the SQL text
// VERBATIM -- it must be a constant in caller code, never attacker input --
// and each `?` marker in it becomes the dialect's numbered placeholder. The
// args are always placeholder-bound, never string-formatted, so an
// attacker-controlled value can never alter the SQL structure.
//
// The fragment is written with `?` markers regardless of dialect; render
// renumbers them to the resolved dialect's placeholder style ($N on
// Postgres) in clause order. A fragment with no `?` marker binds nothing.
//
// UnsafeRaw is the deliberate escape hatch for SQL the typed fluent API
// cannot express. It is unreachable accidentally (callers must explicitly
// invoke it); every audited raw-SQL site stays grep-able.
func UnsafeRaw[T any](fragment string, args ...any) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NRaw, Value: RawExpr{Fragment: fragment, Args: args}}}
}
