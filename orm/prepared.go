package orm

import (
	"context"

	"github.com/zenta-dev/zever/core/db"
)

// queryRows runs query against exec, preferring a prepared statement when
// exec implements db.Preparer -- probed the same way db.WithTx probes
// db.Transactor -- and falling back to exec.Query directly otherwise, so a
// non-Preparer adapter behaves exactly as it did before prepared-statement
// reuse existed.
//
// The Stmt's Close is deferred immediately. That is safe for every
// Preparer-backed adapter in this module (db/sqlite): its Stmt.Close is a
// no-op, because the statement is owned by an adapter-level LRU cache and
// must stay live for the next caller that reuses the same SQL text (see
// db/sqlite's Prepare). And database/sql Rows obtained from a *sql.Stmt
// remain valid after the *sql.Stmt is closed, so streaming callers are
// unaffected either way.
func queryRows(ctx context.Context, exec db.DB, query string, args []any) (db.Rows, error) {
	p, ok := exec.(db.Preparer)
	if !ok {
		return exec.Query(ctx, query, args...)
	}

	stmt, err := p.Prepare(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stmt.Close() }()

	return stmt.Query(ctx, args...)
}

// execQuery runs a write query against exec, preferring a prepared
// statement when exec implements db.Preparer and falling back to exec.Exec
// directly otherwise. See queryRows for why the deferred Stmt.Close is
// safe.
func execQuery(ctx context.Context, exec db.DB, query string, args []any) (int64, error) {
	p, ok := exec.(db.Preparer)
	if !ok {
		return exec.Exec(ctx, query, args...)
	}

	stmt, err := p.Prepare(ctx, query)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()

	return stmt.Exec(ctx, args...)
}
