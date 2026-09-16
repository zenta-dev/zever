package db

import (
	"context"
	"fmt"
	"sync"
)

// DB is the interface that database adapters must implement.
//
// DB is intentionally open (no unexported methods) so third-party adapters
// outside this module can implement it and register via Register.
type DB interface {
	// Query runs a query and returns matching rows.
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	// Exec runs a query and returns the number of rows it affected.
	Exec(ctx context.Context, query string, args ...any) (int64, error)
	// Ping verifies the connection is alive.
	Ping(ctx context.Context) error
	// Close releases database resources. It checks nothing itself at the
	// core level; adapters enforce their own close semantics.
	Close(ctx context.Context) error
	// Dialect reports the SQL dialect name matching the registered adapter.
	Dialect() string
}

// Rows is an iterator over query results.
type Rows interface {
	// Next advances to the next row, reporting whether one is available.
	Next() bool
	// Scan copies the current row into dest values.
	Scan(dest ...any) error
	// Close releases the rows iterator.
	Close() error
	// Columns returns the result set column names.
	Columns() ([]string, error)
	// Err reports the iteration error, if any.
	Err() error
}

// Preparer is implemented by adapters that support server-side prepared statements.
type Preparer interface {
	// Prepare creates a reusable statement for query.
	Prepare(ctx context.Context, query string) (Stmt, error)
}

// Stmt is a prepared statement bound to one SQL text, reusable across calls.
type Stmt interface {
	// Query runs the statement and returns matching rows.
	Query(ctx context.Context, args ...any) (Rows, error)
	// Exec runs the statement and returns the number of rows it affected.
	Exec(ctx context.Context, args ...any) (int64, error)
	// Close releases the prepared statement.
	Close() error
}

// Factory creates a DB from typed options.
type Factory func(opts Options) (DB, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[adapter]; dup {
		return &DuplicateAdapterError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates a DB for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (DB, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	db, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", adapter, err)
	}

	return db, nil
}
