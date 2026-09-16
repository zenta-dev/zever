package orm

import (
	"context"
	"fmt"
	"iter"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/render"
)

// Stream runs q against exec and yields every matching row one at a time,
// via iter.Seq2[PT, error] -- range-over-func's early-return cleanup closes
// the underlying rows cursor even if the consumer breaks out of the loop
// early, so no cursor is ever leaked. Stream owns the full
// db.Rows lifecycle: the iterator body's deferred rows.Close() runs whenever
// iteration ends -- early break or full consumption alike -- and a `closed`
// guard keeps Close from being invoked twice on the full-consumption path
// (the explicit close there exists to surface a close-time error to the
// consumer). All is a thin materializing wrapper over Stream, so the two
// share one execution path.
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

		query, args, err := render.SelectMods(d, q.table.Name(), q.selectColumns(), toRenderNode[T](q.where.Render()), toRenderOrder(q.order), q.limit, q.offset, q.selectModifiers())
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

		closed := false
		defer func() {
			if !closed {
				_ = rows.Close()
			}
		}()

		for rows.Next() {
			var v T

			p := PT(&v)
			if err := p.Scan(rows); err != nil {
				yield(nil, fmt.Errorf("orm: Query.Stream: scan: %w", err))

				return
			}

			if !yield(p, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, fmt.Errorf("orm: Query.Stream: %w", err))

			return
		}

		closeErr := rows.Close()
		closed = true

		if closeErr != nil {
			yield(nil, fmt.Errorf("orm: Query.Stream: %w", closeErr))
		}
	}
}
