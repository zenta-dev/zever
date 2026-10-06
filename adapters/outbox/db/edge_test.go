package db

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

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

// stubTx is a minimal coredb.Tx for error-path tests.
type stubTx struct {
	execErr error
	execs   int
}

func (t *stubTx) Query(context.Context, string, ...any) (coredb.Rows, error) {
	return nil, errors.New("stub: query")
}

func (t *stubTx) Exec(context.Context, string, ...any) (int64, error) {
	t.execs++

	return 0, t.execErr
}

func (t *stubTx) Ping(context.Context) error   { return nil }
func (t *stubTx) Close(context.Context) error  { return nil }
func (t *stubTx) Dialect() string              { return "sqlite" }
func (t *stubTx) Commit(context.Context) error { return nil }

func (t *stubTx) Rollback(context.Context) error { return nil }

func (t *stubTx) Savepoint(context.Context, string) error { return nil }

func (t *stubTx) RollbackTo(context.Context, string) error { return nil }

// stubRows is a minimal coredb.Rows whose Scan fails.
type stubRows struct{ err error }

func (r stubRows) Next() bool { return false }

func (r stubRows) Scan(...any) error { return r.err }

func (r stubRows) Close() error { return nil }

func (r stubRows) Columns() ([]string, error) { return nil, nil }

func (r stubRows) Err() error { return nil }

func TestRecordCancelledContext(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := d.Record(ctx, &stubTx{}, outbox.Message{ID: "1", Topic: "t"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Record(cancelled) = %v, want context.Canceled", err)
	}
}

func TestRecordExecError(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	err := d.Record(t.Context(), &stubTx{execErr: errors.New("boom")}, outbox.Message{ID: "1", Topic: "t"})
	if err == nil {
		t.Fatal("Record() = nil error, want error")
	}

	if !strings.Contains(err.Error(), "db: record") {
		t.Errorf("err = %v, want containing %q", err, "db: record")
	}
}

func TestProcessCancelledContext(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := d.Process(ctx, &stubTx{}, "evt", func(context.Context, coredb.Tx) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Process(cancelled) = %v, want context.Canceled", err)
	}
}

func TestProcessExecError(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	err := d.Process(t.Context(), &stubTx{execErr: errors.New("boom")}, "evt", func(context.Context, coredb.Tx) error { return nil })
	if err == nil {
		t.Fatal("Process() = nil error, want error")
	}

	if !strings.Contains(err.Error(), "db: process") {
		t.Errorf("err = %v, want containing %q", err, "db: process")
	}
}

func TestNewFromDBNilConn(t *testing.T) {
	t.Parallel()

	if _, err := NewFromDB(nil, Options{}); err == nil {
		t.Fatal("NewFromDB(nil) = nil error, want error")
	}
}

func TestOpenFromDBInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := OpenFromDB(stubDB{dialect: "sqlite"}, Options{Table: "bad name"})
	if err == nil {
		t.Fatal("OpenFromDB() = nil error, want error")
	}
}

func TestEnsureSchemaError(t *testing.T) {
	t.Parallel()

	_, err := OpenFromDB(stubDB{dialect: "sqlite"}, Options{})
	if err == nil {
		t.Fatal("OpenFromDB() = nil error, want error")
	}

	if !strings.Contains(err.Error(), "ensure schema") {
		t.Errorf("err = %v, want containing %q", err, "ensure schema")
	}
}

func TestClaimUnsupportedDialect(t *testing.T) {
	t.Parallel()

	d := &driver{conn: stubDB{dialect: "mysql"}}

	if _, err := d.claim(t.Context()); err == nil {
		t.Error("claim() = nil error, want unsupported dialect")
	}
}

func TestScanClaimedError(t *testing.T) {
	t.Parallel()

	if _, err := scanClaimed(stubRows{err: errors.New("boom")}); err == nil {
		t.Fatal("scanClaimed() = nil error, want error")
	}
}

func TestRelayErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func(d *driver)
	}{
		{"markProcessed", func(d *driver) { d.markProcessed(t.Context(), "evt", time.Now()) }},
		{"markFailed", func(d *driver) { d.markFailed(t.Context(), "evt", errors.New("boom")) }},
		{"markRetry", func(d *driver) { d.markRetry(t.Context(), "evt", errors.New("boom"), time.Now()) }},
		{"cleanup", func(d *driver) { d.cleanup(t.Context()) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d := &driver{conn: stubDB{dialect: "sqlite"}, table: DefaultTable}

			tc.fn(d)

			if st := d.Status(); st.LastError == "" {
				t.Error("Status().LastError is empty, want relay error recorded")
			}
		})
	}
}

func TestCoerceTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    time.Time
		wantErr bool
	}{
		{"nil", nil, time.Time{}, false},
		{"time", time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), false},
		{"string", "2026-10-06T12:00:00.000000000Z", time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), false},
		{"bytes", []byte("2026-10-06T12:00:00.000000000Z"), time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), false},
		{"bad string", "not a time", time.Time{}, true},
		{"unsupported", 42, time.Time{}, true},
	}

	for _, tc := range tests {
		got, err := coerceTime(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: coerceTime() = nil error, want error", tc.name)
			}

			continue
		}

		if err != nil {
			t.Errorf("%s: coerceTime() error = %v", tc.name, err)

			continue
		}

		if !got.Equal(tc.want) {
			t.Errorf("%s: coerceTime() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCoerceInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    int64
		wantErr bool
	}{
		{"int64", int64(7), 7, false},
		{"int32", int32(7), 7, false},
		{"int", int(7), 7, false},
		{"float64", float64(7), 7, false},
		{"string", "7", 0, true},
	}

	for _, tc := range tests {
		got, err := coerceInt(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: coerceInt() = nil error, want error", tc.name)
			}

			continue
		}

		if err != nil {
			t.Errorf("%s: coerceInt() error = %v", tc.name, err)

			continue
		}

		if got != tc.want {
			t.Errorf("%s: coerceInt() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestCoerceString(t *testing.T) {
	t.Parallel()

	if got := coerceString(nil); got != "" {
		t.Errorf("coerceString(nil) = %q, want empty", got)
	}

	if got := coerceString("x"); got != "x" {
		t.Errorf("coerceString(x) = %q, want x", got)
	}

	if got := coerceString([]byte("x")); got != "x" {
		t.Errorf("coerceString(bytes) = %q, want x", got)
	}

	if got := coerceString(42); got != "42" {
		t.Errorf("coerceString(42) = %q, want 42", got)
	}
}

func TestDecodeHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{"empty", "", nil},
		{"null", "null", nil},
		{"corrupt", "{not json", nil},
		{"valid", `{"k":"v"}`, map[string]string{"k": "v"}},
	}

	for _, tc := range tests {
		if got := decodeHeaders(tc.input); !maps.Equal(got, tc.want) {
			t.Errorf("%s: decodeHeaders() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestQuoteIdent(t *testing.T) {
	t.Parallel()

	if got := quoteIdent("outbox"); got != `"outbox"` {
		t.Errorf("quoteIdent() = %q, want %q", got, `"outbox"`)
	}

	if got := quoteIdent(`we"ird`); got != `"we""ird"` {
		t.Errorf("quoteIdent() = %q, want %q", got, `"we""ird"`)
	}
}

func TestTimestampType(t *testing.T) {
	t.Parallel()

	if got := timestampType("postgres"); got != "TIMESTAMPTZ" {
		t.Errorf("timestampType(postgres) = %q, want TIMESTAMPTZ", got)
	}

	if got := timestampType("sqlite"); got != "TEXT" {
		t.Errorf("timestampType(sqlite) = %q, want TEXT", got)
	}
}

func TestTsSqliteFormat(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	in := time.Date(2026, 10, 6, 12, 30, 45, 0, time.UTC)
	want := "2026-10-06T12:30:45.000000000Z"

	if got := d.ts(in); got != want {
		t.Errorf("ts() = %v, want %q", got, want)
	}
}

func TestConcurrentRecordAndStatus(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	const workers = 8

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				id := fmt.Sprintf("conc-%d-%d", n, j)

				if err := withTx(t, d, func(ctx context.Context, tx coredb.Tx) error {
					return d.Record(ctx, tx, outbox.Message{ID: id, Topic: "t"})
				}); err != nil {
					t.Errorf("Record(%s) error = %v", id, err)
					return
				}

				_ = d.Status()
			}
		}(i)
	}

	wg.Wait()

	if n := countRows(t, d); n != workers*10 {
		t.Errorf("rows = %d, want %d", n, workers*10)
	}
}
