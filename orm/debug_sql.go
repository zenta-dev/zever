package orm

import (
	"fmt"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// DebugStmt is one rendered statement: the SQL text with the dialect's
// placeholders plus the bound args exactly as the driver would receive
// them (post dialect-encoding, e.g. sqlite timestamps as RFC3339Nano
// text). Args are never interpolated into SQL.
type DebugStmt struct {
	SQL  string
	Args []any
}

// DebugSQL renders q into the statements Stream would execute against the
// named dialect, without executing anything. It runs the same capability
// gates and the same IN-chunk fan-out as Stream, so the output is exactly
// what All/Stream would hand the driver -- usually one statement, one per
// IN chunk for oversized value-list IN filters.
//
// The named dialect is resolved with dialect.For (no live connection
// needed); an unknown name is an error, never a silent fallback. Args may
// hold secrets (passwords, tokens in filter values): print them only in
// development, never in production logs. For a live-execution trace of
// every statement orm issues, use SetQueryLogger instead; for the
// database's own plan for the statement, use Explain.
func (q Query[T, PT]) DebugSQL(dialectName string) ([]DebugStmt, error) {
	d, err := dialect.For(dialectName)
	if err != nil {
		return nil, fmt.Errorf("orm: Query.DebugSQL: %w", err)
	}

	if err = q.validateSelectLocking(d); err != nil {
		return nil, fmt.Errorf("orm: Query.DebugSQL: %w", err)
	}

	wheres, err := q.streamWheres(d, "orm: Query.DebugSQL")
	if err != nil {
		return nil, err
	}

	out := make([]DebugStmt, 0, len(wheres))

	for _, w := range wheres {
		query, args, err := render.SelectMods(d, q.table.Name(), q.selectColumns(), w, toRenderOrder(q.order), q.limit, q.offset, q.selectModifiers())
		if err != nil {
			return nil, fmt.Errorf("orm: Query.DebugSQL: %w", err)
		}

		out = append(out, DebugStmt{SQL: query, Args: encodeArgs(d, args)})
	}

	return out, nil
}
