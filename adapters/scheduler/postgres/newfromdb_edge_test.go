package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/orm/dialect"
)

// fakeDialectDB is a coredb.DB whose Dialect falls outside sqlite/postgres,
// letting checkDialect's fail-closed branch be exercised without a live
// server.
type fakeDialectDB struct{ dialect string }

func (f *fakeDialectDB) Query(context.Context, string, ...any) (coredb.Rows, error) {
	return nil, errors.New("postgres: fake query unused")
}

func (f *fakeDialectDB) Exec(context.Context, string, ...any) (int64, error) {
	return 0, errors.New("postgres: fake exec unused")
}

func (f *fakeDialectDB) Ping(context.Context) error  { return nil }
func (f *fakeDialectDB) Close(context.Context) error { return nil }
func (f *fakeDialectDB) Dialect() string             { return f.dialect }

func borrowOpts() Options {
	o := Options{Owner: "owner-borrow"}
	o.Dispatcher = &job.Dispatcher{Q: newStubQueue()}

	return o
}

// TestNewFromDB_nilConn checks the fail-closed nil guard.
func TestNewFromDB_nilConn(t *testing.T) {
	t.Parallel()

	_, err := NewFromDB(nil, borrowOpts())
	if err == nil {
		t.Fatal("NewFromDB(nil) = nil, want error")
	}
}

// TestNewFromDB_unsupportedDialect checks that a dialect outside
// sqlite/postgres fails closed before any state transition.
func TestNewFromDB_unsupportedDialect(t *testing.T) {
	t.Parallel()

	_, err := NewFromDB(&fakeDialectDB{dialect: "mysql"}, borrowOpts())
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("NewFromDB(mysql) = %v, want ErrUnsupportedByDialect", err)
	}
}

// TestNewFromDB_borrowsConnection checks that a driver built over a caller's
// connection does not own it: Close stops ticks but leaves the pool open.
func TestNewFromDB_borrowsConnection(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "borrow.db")})
	if err != nil {
		t.Fatalf("dbsqlite.New: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	s, err := NewFromDB(conn, borrowOpts())
	if err != nil {
		t.Fatalf("NewFromDB: %v", err)
	}

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("NewFromDB type = %T, want *driver", s)
	}

	if d.owns {
		t.Fatal("NewFromDB marked borrowed conn as owned")
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("borrowed conn ping after Close: %v", err)
	}
}

// TestOpenFromDB_delegates checks that OpenFromDB mirrors NewFromDB.
func TestOpenFromDB_delegates(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "delegate.db")})
	if err != nil {
		t.Fatalf("dbsqlite.New: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	s, err := OpenFromDB(conn, borrowOpts())
	if err != nil {
		t.Fatalf("OpenFromDB: %v", err)
	}

	if got := s.Name(); got != "postgres" {
		t.Fatalf("Name() = %q, want postgres", got)
	}
}

// TestIsDuplicateErr checks the dialect-agnostic primary-key/unique matcher.
func TestIsDuplicateErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		msg  string
		want bool
	}{
		{"UNIQUE constraint failed: scheduler_slots.slot", true},
		{"duplicate key value violates unique constraint", true},
		{"PRIMARY KEY must be unique", true},
		{"no such table: scheduler_slots", false},
		{"database is locked", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := isDuplicateErr(errors.New(tc.msg)); got != tc.want {
			t.Errorf("isDuplicateErr(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}
