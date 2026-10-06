package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

func mustNew(t *testing.T, opts Options) *driver {
	t.Helper()

	if opts.Path == "" && opts.DSN == "" {
		// Fresh file per test: ":memory:" sqlite uses shared cache
		// (process-global), so fixed IDs would collide across reruns.
		opts.Path = filepath.Join(t.TempDir(), "outbox.db")
	}

	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Millisecond
	}

	s, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("New() returned %T, want *driver", s)
	}

	t.Cleanup(func() { _ = s.Close() })

	return d
}

func mustRecord(t *testing.T, d *driver, msg outbox.Message) {
	t.Helper()

	err := withTx(t, d, func(ctx context.Context, tx coredb.Tx) error {
		return d.Record(ctx, tx, msg)
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
}

func withTx(t *testing.T, d *driver, fn func(ctx context.Context, tx coredb.Tx) error) error {
	t.Helper()

	return coredb.WithTx(t.Context(), d.conn, nil, fn)
}

func countRows(t *testing.T, d *driver, table string) int {
	t.Helper()

	rows, err := d.conn.Query(t.Context(), `SELECT COUNT(*) FROM `+quoteIdent(table))
	if err != nil {
		t.Fatalf("count query error = %v", err)
	}

	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		t.Fatalf("count query returned no rows: %v", rows.Err())
	}

	var n int64
	if err := rows.Scan(&n); err != nil {
		t.Fatalf("count scan error = %v", err)
	}

	return int(n)
}

func TestRecordRequiresTx(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	err := d.Record(t.Context(), nil, outbox.Message{ID: "1", Topic: "t"})
	if !errors.Is(err, outbox.ErrTxRequired) {
		t.Fatalf("Record(nil tx) = %v, want ErrTxRequired", err)
	}
}

func TestRecordIsTransactional(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	ctx := t.Context()

	tr, ok := d.conn.(coredb.Transactor)
	if !ok {
		t.Fatal("conn does not implement Transactor")
	}

	tx, err := tr.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}

	if err := d.Record(ctx, tx, outbox.Message{ID: "r", Topic: "t", Payload: []byte("x")}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	if n := countRows(t, d, DefaultTable); n != 0 {
		t.Fatalf("rows = %d, want 0 after rollback", n)
	}
}

func TestRecordValidatesMessage(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	err := withTx(t, d, func(ctx context.Context, tx coredb.Tx) error {
		return d.Record(ctx, tx, outbox.Message{})
	})
	if !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("Record(invalid) = %v, want ErrInvalidMessage", err)
	}
}

func TestRecordAndPoll(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	mustRecord(t, d, outbox.Message{ID: "m1", Topic: "orders", Payload: []byte("p"), Headers: map[string]string{"k": "v"}})

	d.pollOnce(t.Context())

	got := pub.messages()
	if len(got) != 1 {
		t.Fatalf("published %d, want 1", len(got))
	}

	if got[0].ID != "m1" || got[0].Topic != "orders" {
		t.Errorf("published = %+v, want m1/orders", got[0])
	}

	if string(got[0].Payload) != "p" {
		t.Errorf("Payload = %q, want p", got[0].Payload)
	}

	if got[0].Headers["k"] != "v" {
		t.Errorf("Headers[k] = %q, want v", got[0].Headers["k"])
	}

	if st := d.Status(); st.Processed != 1 || st.Pending != 0 {
		t.Errorf("Status = %+v, want processed 1 pending 0", st)
	}
}

func TestPollIdempotentAfterProcessed(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	mustRecord(t, d, outbox.Message{ID: "once", Topic: "t"})
	d.pollOnce(t.Context())
	d.pollOnce(t.Context())

	if n := len(pub.messages()); n != 1 {
		t.Fatalf("published %d, want 1 (processed rows are not re-claimed)", n)
	}
}

func TestStartRequiresPublisher(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	if err := d.Start(t.Context()); !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("Start() = %v, want ErrInvalidOptions", err)
	}
}

func TestStartIdempotent(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	if err := d.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := d.Start(t.Context()); err != nil {
		t.Fatalf("Start() second error = %v", err)
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	if got := mustNew(t, Options{}).Name(); got != "db" {
		t.Errorf("Name() = %q, want db", got)
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{"empty valid", Options{}, false},
		{"bad table", Options{Table: "no-dashes!"}, true},
		{"bad inbox", Options{InboxTable: "1bad"}, true},
		{"negative poll", Options{PollInterval: -time.Second}, true},
		{"negative batch", Options{BatchSize: -1}, true},
		{"negative attempts", Options{MaxAttempts: -1}, true},
		{"negative retention", Options{Retention: -time.Hour}, true},
		{"negative lock", Options{LockSeconds: -1}, true},
		{"negative maxconns", Options{Options: coredb.Options{MaxConns: -1}}, true},
	}

	for _, tc := range tests {
		err := tc.opts.Validate()
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: Validate() = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestOpenFromDBDoesNotOwnConn(t *testing.T) {
	t.Parallel()

	conn, err := openSQLite(t)
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}

	s, err := OpenFromDB(conn, Options{})
	if err != nil {
		t.Fatalf("OpenFromDB() error = %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// The borrowed conn must still be usable after the store closes.
	if err := conn.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() after Close error = %v, want borrowed conn open", err)
	}

	if err := conn.Close(t.Context()); err != nil {
		t.Fatalf("conn.Close() error = %v", err)
	}
}

func TestOpenFromDBNilConn(t *testing.T) {
	t.Parallel()

	if _, err := OpenFromDB(nil, Options{}); err == nil {
		t.Fatal("OpenFromDB(nil) = nil error, want error")
	}
}

func TestRegisterOpensViaCoreOptions(t *testing.T) {
	Register()

	s, err := outbox.Open(outbox.DB, outbox.Options{DSN: filepath.Join(t.TempDir(), "reg.db")})
	if err != nil {
		t.Fatalf("Open(db) error = %v", err)
	}

	defer func() { _ = s.Close() }()

	if s.Name() != "db" {
		t.Errorf("Name() = %q, want db", s.Name())
	}
}
