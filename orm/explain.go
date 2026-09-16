package orm

import (
	"context"
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// explain runs `EXPLAIN [ANALYZE] <stmt>` for a statement produced by
// render over the dialect resolved from exec, returning the planner's
// output one line per result row. It is the shared entry point for every
// queryable type's Explain/ExplainAnalyze: the caller passes a closure
// that renders its own SELECT shape, so the EXPLAIN passthrough --
// "Explain renders exactly the statement All/Stream/Scan execute, prefixed
// with EXPLAIN" -- holds uniformly. The ANALYZE capability gate
// (dialect.ExplainDialect, see ExplainAnalyze's doc comment) is checked
// before anything is issued.
func explain(ctx context.Context, exec db.DB, analyze bool, tag string, render func(d dialect.Dialect) (string, []any, error)) ([]string, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	if analyze {
		ed, ok := d.(dialect.ExplainDialect)
		if !ok || !ed.SupportsExplainAnalyze() {
			return nil, fmt.Errorf("orm: %w: dialect %q does not support EXPLAIN ANALYZE", dialect.ErrUnsupportedByDialect, d.Name())
		}
	}

	stmt, args, err := render(d)
	if err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	return explainExec(ctx, exec, analyze, tag, stmt, encodeArgs(d, args))
}

// explainExec runs `EXPLAIN [ANALYZE] <stmt>` against exec and scans every
// result row into one line of plan text. Each dialect returns different
// EXPLAIN columns (SQLite QUERY PLAN/bytecode tables, Postgres's single
// QUERY PLAN text column), so a row is scanned into len(columns) *any
// holders -- database/sql always accepts a raw value into an *any
// destination -- and the non-nil values are space-joined into one line.
func explainExec(ctx context.Context, exec db.DB, analyze bool, tag, stmt string, args []any) ([]string, error) {
	prefix := "EXPLAIN"
	if analyze {
		prefix = "EXPLAIN ANALYZE"
	}

	rows, err := exec.Query(ctx, prefix+" "+stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	var lines []string

	for rows.Next() {
		holders := make([]any, len(cols))
		dest := make([]any, len(cols))

		for i := range holders {
			holders[i] = new(any)
			dest[i] = holders[i]
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("orm: %s: scan: %w", tag, err)
		}

		var parts []string

		for _, h := range holders {
			v, ok := h.(*any)
			if !ok || v == nil || *v == nil {
				continue
			}

			parts = append(parts, fmt.Sprint(*v))
		}

		lines = append(lines, strings.Join(parts, " "))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	return lines, nil
}

// Explain runs q's underlying SELECT under EXPLAIN against exec and returns
// the planner's output, one line per result row. It is a passthrough:
// Explain renders exactly the statement Stream/All execute and prefixes it
// with EXPLAIN, so the plan reflects the query the caller would actually
// run. Plain EXPLAIN is universal SQL (every supported dialect accepts it),
// so no capability gate is needed here.
func (q Query[T, PT]) Explain(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, false, "Query.Explain", func(d dialect.Dialect) (string, []any, error) {
		if err := q.validateSelectLocking(d); err != nil {
			return "", nil, err
		}

		return render.SelectMods(d, q.table.Name(), q.table.Columns(), toRenderNode[T](q.where.Render()), toRenderOrder(q.order), q.limit, q.offset, q.selectModifiers())
	})
}

// ExplainAnalyze runs q's underlying SELECT under EXPLAIN ANALYZE against
// exec, returning the plan output one line per result row. Unlike plain
// EXPLAIN, EXPLAIN ANALYZE actually executes the statement, and it is not
// universal SQL -- SQLite has no EXPLAIN ANALYZE at any version
// (dialect.ExplainDialect). A dialect lacking the capability returns a
// typed dialect.ErrUnsupportedByDialect rather than sending SQL the server
// would reject.
func (q Query[T, PT]) ExplainAnalyze(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, true, "Query.Explain", func(d dialect.Dialect) (string, []any, error) {
		if err := q.validateSelectLocking(d); err != nil {
			return "", nil, err
		}

		return render.SelectMods(d, q.table.Name(), q.table.Columns(), toRenderNode[T](q.where.Render()), toRenderOrder(q.order), q.limit, q.offset, q.selectModifiers())
	})
}

// explainMutate is the shared mutation twin of the Query Explain
// passthrough: check render (which re-runs the statement's own capability
// gates and returns the exact statement text and bound args) then issue it
// under the EXPLAIN prefix. Plain EXPLAIN is universal SQL and ungated
// here, while ExplainAnalyze goes through the shared dialect.ExplainDialect
// gate in explain().
func explainMutate(
	ctx context.Context, exec db.DB, analyze bool, tag string,
	render func(d dialect.Dialect) (string, []any, error),
) ([]string, error) {
	return explain(ctx, exec, analyze, tag, render)
}
