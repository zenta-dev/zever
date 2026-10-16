package orm

import (
	"context"
	"fmt"
	"iter"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// Stream runs q against exec and yields every matching row one at a time,
// via iter.Seq2[PT, error] -- range-over-func's early-return cleanup closes
// the underlying rows cursor even if the consumer breaks out of the loop
// early, so no cursor is ever leaked. Stream owns the full
// db.Rows lifecycle: each chunk's cursor is closed exactly once, whether
// iteration ends by early break (close without surfacing its error) or by
// full consumption (a close-time error is surfaced to the consumer). All
// is a thin materializing wrapper over Stream, so the two share one
// execution path.
//
// When q's where holds a value-list IN larger than the resolved dialect's
// per-statement parameter budget, Stream fans out into one statement per
// IN chunk (see planInChunks) and yields the concatenated rows. Fan-out
// engages only for plain unordered unbounded selects; with ORDER BY, LIMIT,
// OFFSET, a row lock or TABLESAMPLE the row set cannot be reconstructed
// from independent statements, so Stream fails closed with
// ErrInListTooLarge instead of returning misordered or truncated rows.
//
// Chunked reads run several statements and are NOT snapshot-consistent:
// rows written or deleted between chunks can be missed or seen inconsistently
// unless the caller passes a transaction (db.Tx) whose isolation level
// provides a stable snapshot. Repeated IN values are removed (order-preserving)
// before chunking, so a duplicated value never yields its row twice.
func (q Query[T, PT]) Stream(ctx context.Context, exec db.DB) iter.Seq2[PT, error] {
	return func(yield func(PT, error) bool) {
		d, err := resolveDialect(exec)
		if err != nil {
			yield(nil, err)

			return
		}

		err = q.validateSelectLocking(d)
		if err != nil {
			yield(nil, err)

			return
		}

		wheres, err := q.streamWheres(d, "orm: Query.Stream")
		if err != nil {
			yield(nil, err)

			return
		}

		for _, w := range wheres {
			query, args, err := render.SelectMods(d, q.table.Name(), q.selectColumns(), w, toRenderOrder(q.order), q.limit, q.offset, q.selectModifiers())
			if err != nil {
				yield(nil, fmt.Errorf("orm: Query.Stream: %w", err))

				return
			}

			encoded := encodeArgs(d, args)
			logQuery(query, encoded)

			rows, err := queryRows(ctx, exec, query, encoded)
			if err != nil {
				yield(nil, fmt.Errorf("orm: Query.Stream: %w", err))

				return
			}

			if stop := streamRows[T, PT](rows, yield); stop {
				return
			}
		}
	}
}

// streamWheres returns one render Node per statement Stream must execute:
// the query's own where when it fits the dialect's parameter budget, or
// one Node per IN chunk when planInChunks fans out. Fan-out with
// result-shaping modifiers is rejected with ErrInListTooLarge (see
// Stream's doc comment for why each modifier is unsafe across chunks).
// tag is the caller's "orm: ..." prefix for wrapped errors, so shared
// callers (Stream, DebugSQL) keep their own error attribution.
func (q Query[T, PT]) streamWheres(d dialect.Dialect, tag string) ([]render.Node, error) {
	plans, err := planInChunks(q.where.n, dialect.MaxParams(d))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tag, err)
	}

	if len(plans) == 1 {
		return []render.Node{toRenderNode[T](q.where.n)}, nil
	}

	if q.limit > 0 || q.offset > 0 || len(q.order) > 0 || q.lock != LockNone || q.tablesample != nil {
		return nil, fmt.Errorf("%s: %w", tag, ErrInListTooLarge)
	}

	wheres := make([]render.Node, len(plans))
	for i, p := range plans {
		wheres[i] = toRenderNode[T](p)
	}

	return wheres, nil
}

// streamRows drains one chunk cursor, yielding each scanned row. It
// reports whether iteration must stop (true): the consumer broke out
// early, or an error was yielded. Either way the cursor is closed exactly
// once before it returns. A scan, iteration or close-time error is yielded
// and ends iteration.
func streamRows[T any, PT ptrScanner[T]](rows db.Rows, yield func(PT, error) bool) (stop bool) {
	for rows.Next() {
		var v T

		p := PT(&v)
		if err := p.Scan(rows); err != nil {
			_ = rows.Close()
			yield(nil, fmt.Errorf("orm: Query.Stream: scan: %w", err))

			return true
		}

		if !yield(p, nil) {
			_ = rows.Close()

			return true
		}
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		yield(nil, fmt.Errorf("orm: Query.Stream: %w", err))

		return true
	}

	if err := rows.Close(); err != nil {
		yield(nil, fmt.Errorf("orm: Query.Stream: %w", err))
	}

	return false
}
