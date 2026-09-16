package orm

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/zenta-dev/zever/db"
)

// spDepthKey is the context key WithNestedTx uses to track how many
// savepoint levels deep the current call chain already is, so nested
// savepoint names never collide. It is package-private -- callers never
// construct or read it directly, only WithNestedTx does.
type spDepthKey struct{}

// nextSavepointName reads the current nesting depth out of ctx (0 if
// absent), returns the next depth's savepoint name plus that depth, for
// the caller to stash back into the context it passes to fn. Names are
// "sp_1", "sp_2", ... -- collision-free per nesting depth, and valid
// under db/sqlite's validateSavepointName contract (starts with a letter
// or underscore, followed by letters/digits/underscores only; Postgres
// accepts the same shape unquoted).
func nextSavepointName(ctx context.Context) (name string, depth int) {
	depth, _ = ctx.Value(spDepthKey{}).(int)
	depth++

	return "sp_" + strconv.Itoa(depth), depth
}

// WithNestedTx runs fn within a transaction, transparently degrading a
// would-be nested transaction into a SAVEPOINT instead of failing the way
// db.WithTx does.
//
// Detection uses db.TxFromContext -- the exact same context-based
// mechanism db.WithTx itself uses to reject nesting -- so the two
// functions agree on what "already inside a transaction" means:
//
//   - ctx carries no Tx yet: WithNestedTx behaves exactly like
//     db.WithTx (BeginTx, fn, Commit on success / Rollback on error or
//     panic). The Tx db.WithTx stashes into fn's context is then visible
//     to any WithNestedTx call fn itself makes, so a single level of real
//     nesting attempted by caller code becomes the savepoint path below
//     without caller code needing to know which case it's in.
//   - ctx already carries a Tx (this is itself a nested call): WithNestedTx
//     issues a SAVEPOINT on that same Tx, runs fn with a context recording
//     one more nesting depth (so a further nested call gets its own,
//     distinct savepoint name), and RELEASEs the savepoint on fn's success
//     or RollbackTo's it on fn's error -- never touching the outer
//     transaction itself, which the caller of the outermost WithNestedTx
//     still commits or rolls back on its own success/failure.
func WithNestedTx(ctx context.Context, exec db.DB, fn func(ctx context.Context, tx db.Tx) error) error {
	tx, ok := db.TxFromContext(ctx)
	if !ok {
		return db.WithTx(ctx, exec, nil, fn)
	}

	name, depth := nextSavepointName(ctx)
	nextCtx := context.WithValue(ctx, spDepthKey{}, depth)

	if err := tx.Savepoint(ctx, name); err != nil {
		return fmt.Errorf("orm: WithNestedTx: savepoint %s: %w", name, err)
	}

	if err := fn(nextCtx, tx); err != nil {
		if rbErr := tx.RollbackTo(ctx, name); rbErr != nil {
			return errors.Join(err, fmt.Errorf("orm: WithNestedTx: rollback to savepoint %s: %w", name, rbErr))
		}

		return err
	}

	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+name); err != nil {
		return fmt.Errorf("orm: WithNestedTx: release savepoint %s: %w", name, err)
	}

	return nil
}
