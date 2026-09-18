package db

import (
	"context"
	"fmt"
)

// IsolationLevel defines transaction isolation levels.
type IsolationLevel int

const (
	// ReadCommitted is the default isolation level.
	ReadCommitted IsolationLevel = iota
	// RepeatableRead provides repeatable-read isolation.
	RepeatableRead
	// Serializable provides serializable isolation.
	Serializable
)

// String returns the SQL representation of the isolation level.
func (l IsolationLevel) String() string {
	switch l {
	case ReadCommitted:
		return "read committed"
	case RepeatableRead:
		return "repeatable read"
	case Serializable:
		return "serializable"
	default:
		return fmt.Sprintf("IsolationLevel(%d)", int(l))
	}
}

// TxOptions holds transaction configuration.
type TxOptions struct {
	// Isolation is the transaction isolation level.
	Isolation IsolationLevel
	// ReadOnly marks the transaction as read-only.
	ReadOnly bool
	// Deferrable marks a read-only Postgres transaction as deferrable.
	Deferrable bool
}

// Tx is a transaction. It embeds DB for queries within the transaction
// and adds commit, rollback, and savepoint support.
type Tx interface {
	// DB runs queries inside the transaction.
	DB
	// Commit commits the transaction.
	Commit(ctx context.Context) error
	// Rollback aborts the transaction.
	Rollback(ctx context.Context) error
	// Savepoint creates a named savepoint within the transaction.
	Savepoint(ctx context.Context, name string) error
	// RollbackTo rolls back to a named savepoint.
	RollbackTo(ctx context.Context, name string) error
}

// Transactor is implemented by adapters that support transactions.
type Transactor interface {
	// BeginTx starts a transaction with the given options.
	BeginTx(ctx context.Context, opts *TxOptions) (Tx, error)
}

type txContextKey struct{}

// WithTxIntoContext returns a new context that carries tx.
func WithTxIntoContext(ctx context.Context, tx Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

// TxFromContext retrieves a Tx from context if present.
func TxFromContext(ctx context.Context) (Tx, bool) {
	v := ctx.Value(txContextKey{})
	if v == nil {
		return nil, false
	}

	tx, ok := v.(Tx)

	return tx, ok
}

// WithTx runs fn inside a transaction, committing on success and rolling
// back on error or panic. Nested transactions are rejected. Adapters that
// do not implement Transactor fail with ErrTxUnsupported.
func WithTx(ctx context.Context, db DB, opts *TxOptions, fn func(context.Context, Tx) error) (err error) {
	if _, ok := TxFromContext(ctx); ok {
		return fmt.Errorf("%w", ErrNestedTx)
	}

	tr, ok := db.(Transactor)
	if !ok {
		return fmt.Errorf("%w by adapter %T", ErrTxUnsupported, db)
	}

	tx, err := tr.BeginTx(ctx, opts)
	if err != nil {
		return &TxError{Op: "begin", Err: err}
	}

	defer func() {
		if r := recover(); r != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				panic(fmt.Errorf("db: panic recovered: %v (rollback also failed: %w)", r, rbErr))
			}

			panic(r)
		}
	}()

	txCtx := WithTxIntoContext(ctx, tx)

	if err := fn(txCtx, tx); err != nil {
		_ = tx.Rollback(ctx)

		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return &TxError{Op: "commit", Err: err}
	}

	return nil
}
