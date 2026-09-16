package orm

import (
	"context"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
)

// ormTestCleaner is the subset of *testing.T/*testing.B a fixture helper
// needs, so seed helpers serve both tests and benchmarks.
type ormTestCleaner interface {
	Helper()
	Fatalf(string, ...any)
	Cleanup(func())
}

// openORMTestDB opens a fresh in-memory sqlite database, registers a
// cleanup closing it, and returns it. Each call gets an isolated database
// (the sqlite adapter pins :memory: to a single pooled connection), so
// parallel tests stay independent. It mirrors the core query_helpers_test.go
// seeds, which open the adapter directly rather than through the registry.
func openORMTestDB(ctx context.Context, t ormTestCleaner) db.DB {
	t.Helper()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	return conn
}

// ormRecordingExec is a db.DB that captures every query/args pair instead
// of executing anything, reporting a caller-chosen dialect name. It is the
// harness for proving builders render a fixed SQL template with payloads
// only among the bound arguments -- no database required.
type ormRecordingExec struct {
	dialectName string
	queries     []string
	argsList    [][]any
}

func (r *ormRecordingExec) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	r.queries = append(r.queries, query)
	r.argsList = append(r.argsList, args)

	return emptyRows{}, nil
}

func (r *ormRecordingExec) Exec(_ context.Context, query string, args ...any) (int64, error) {
	r.queries = append(r.queries, query)
	r.argsList = append(r.argsList, args)

	return 0, nil
}

func (r *ormRecordingExec) Ping(context.Context) error  { return nil }
func (r *ormRecordingExec) Close(context.Context) error { return nil }
func (r *ormRecordingExec) Dialect() string             { return r.dialectName }

// last returns the most recently captured query and bound args.
func (r *ormRecordingExec) last() (string, []any) {
	n := len(r.queries)

	return r.queries[n-1], r.argsList[n-1]
}

// ormStubDB is a db.DB reporting a caller-chosen dialect and serving fixed
// stubRows (or injected query/exec errors) -- the harness for execution
// error paths without a live database.
type ormStubDB struct {
	mockExec

	rows     *stubRows
	queryErr error
	execErr  error
}

func (s *ormStubDB) Query(context.Context, string, ...any) (db.Rows, error) {
	if s.queryErr != nil {
		return nil, s.queryErr
	}

	return s.rows, nil
}

func (s *ormStubDB) Exec(context.Context, string, ...any) (int64, error) {
	if s.execErr != nil {
		return 0, s.execErr
	}

	return 0, nil
}

// ormCaptureDB wraps a real db.DB, recording every query/args pair while
// delegating execution. It replaces log-capture assertions with a
// transport-level record, so SQL-text tests run against a live database.
type ormCaptureDB struct {
	db.DB
	queries  []string
	argsList [][]any
}

func (c *ormCaptureDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	c.queries = append(c.queries, query)
	c.argsList = append(c.argsList, args)

	return c.DB.Query(ctx, query, args...)
}

func (c *ormCaptureDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	c.queries = append(c.queries, query)
	c.argsList = append(c.argsList, args)

	return c.DB.Exec(ctx, query, args...)
}

// lastQuery returns the most recently captured query text and bound args.
func (c *ormCaptureDB) lastQuery() (string, []any) {
	n := len(c.queries)

	return c.queries[n-1], c.argsList[n-1]
}
