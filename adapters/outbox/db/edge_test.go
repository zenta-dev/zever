package db

import (
	"context"
	"errors"
	"strings"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// stubDB is a minimal coredb.DB used to exercise dialect rejection.
type stubDB struct {
	dialect string
}

func (s stubDB) Query(context.Context, string, ...any) (coredb.Rows, error) {
	return nil, errors.New("stub: query")
}

func (s stubDB) Exec(context.Context, string, ...any) (int64, error) {
	return 0, errors.New("stub: exec")
}

func (s stubDB) Ping(context.Context) error { return nil }

func (s stubDB) Close(context.Context) error { return nil }

func (s stubDB) Dialect() string { return s.dialect }

func TestUnsupportedDialect(t *testing.T) {
	t.Parallel()

	_, err := OpenFromDB(stubDB{dialect: "mysql"}, Options{})
	if err == nil {
		t.Fatal("OpenFromDB(mysql) = nil error, want unsupported dialect")
	}

	if !strings.Contains(err.Error(), "unsupported dialect") {
		t.Fatalf("error = %v, want unsupported dialect", err)
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{Table: "bad name"}); err == nil {
		t.Fatal("New(bad table) = nil error, want error")
	}
}

func TestRegisterSharedOpens(t *testing.T) {
	Register()

	conn, err := openSQLite(t)
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}

	defer func() { _ = conn.Close(context.Background()) }()

	s, err := outbox.OpenShared(outbox.DB, conn, outbox.Options{})
	if err != nil {
		t.Fatalf("OpenShared() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	if s.Name() != "db" {
		t.Errorf("Name() = %q, want db", s.Name())
	}
}

func TestOpenFromDBSqliteDialect(t *testing.T) {
	t.Parallel()

	conn, err := openSQLite(t)
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}

	defer func() { _ = conn.Close(context.Background()) }()

	s, err := OpenFromDB(conn, Options{Table: "edge_outbox", InboxTable: "edge_inbox"})
	if err != nil {
		t.Fatalf("OpenFromDB() error = %v", err)
	}

	if _, ok := s.(*driver); !ok {
		t.Fatalf("OpenFromDB() = %T, want *driver", s)
	}
}
