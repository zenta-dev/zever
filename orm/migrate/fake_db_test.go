package migrate

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/zenta-dev/zever/db"
)

// fakeRows is a scripted db.Rows.
type fakeRows struct {
	vals     [][]any
	pos      int
	started  bool
	scanErr  error
	iterErr  error
	closeErr error
	closed   bool
}

func (r *fakeRows) Next() bool {
	if !r.started {
		r.started = true
	} else {
		r.pos++
	}

	return r.pos < len(r.vals)
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}

	row := r.vals[r.pos]
	if len(dest) != len(row) {
		return fmt.Errorf("fakeRows: want %d dests, got %d values", len(row), len(dest))
	}

	for i, d := range dest {
		if err := assignValue(d, row[i]); err != nil {
			return err
		}
	}

	return nil
}

func (r *fakeRows) Close() error {
	r.closed = true
	return r.closeErr
}

func (r *fakeRows) Columns() ([]string, error) { return nil, nil }

func (r *fakeRows) Err() error { return r.iterErr }

// assignValue stores v into the pointer dst, coercing numerics and
// []byte-to-string the way database drivers do.
func assignValue(dst, v any) error {
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return errors.New("fakeRows: dest is not a pointer")
	}

	if v == nil {
		dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
		return nil
	}

	vv := reflect.ValueOf(v)
	if vv.Type().AssignableTo(dv.Elem().Type()) {
		dv.Elem().Set(vv)
		return nil
	}

	if b, ok := v.([]byte); ok && dv.Elem().Kind() == reflect.String {
		dv.Elem().SetString(string(b))
		return nil
	}

	if vv.CanConvert(dv.Elem().Type()) {
		switch dv.Elem().Kind() { //nolint:exhaustive // only numeric kinds are convertible here; everything else falls through to the error below
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			dv.Elem().Set(vv.Convert(dv.Elem().Type()))
			return nil
		}
	}

	return fmt.Errorf("fakeRows: cannot assign %T to %T", v, dst)
}

// fakeDB is a scripted db.DB. It deliberately does not implement
// db.Transactor, so code under test takes the unguarded path; wrap it in
// txFakeDB for the transactional path.
type fakeDB struct {
	dialect string
	onQuery func(query string, args []any) (db.Rows, error)
	onExec  func(query string, args []any) (int64, error)
	queries []string
	execs   []string
}

func (f *fakeDB) Dialect() string { return f.dialect }

func (f *fakeDB) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	f.queries = append(f.queries, query)
	if f.onQuery == nil {
		return nil, errors.New("fakeDB: unexpected Query " + query)
	}

	return f.onQuery(query, args)
}

func (f *fakeDB) Exec(_ context.Context, query string, args ...any) (int64, error) {
	f.execs = append(f.execs, query)
	if f.onExec == nil {
		return 0, errors.New("fakeDB: unexpected Exec " + query)
	}

	return f.onExec(query, args)
}

func (f *fakeDB) Ping(context.Context) error { return nil }

func (f *fakeDB) Close(context.Context) error { return nil }

// txFakeDB wraps a fakeDB with a scripted db.Transactor.
type txFakeDB struct {
	*fakeDB
	tx       *fakeTx
	beginErr error
}

func (f *txFakeDB) BeginTx(_ context.Context, _ *db.TxOptions) (db.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}

	if f.tx == nil {
		f.tx = &fakeTx{db: f.fakeDB}
	}

	return f.tx, nil
}

// fakeTx is a scripted db.Tx over a fakeDB.
type fakeTx struct {
	db          *fakeDB
	commitErr   error
	rollbackErr error
	committed   bool
	rolledBack  bool
}

func (t *fakeTx) Dialect() string { return t.db.dialect }

func (t *fakeTx) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	return t.db.Query(ctx, query, args...)
}

func (t *fakeTx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	return t.db.Exec(ctx, query, args...)
}

func (t *fakeTx) Ping(ctx context.Context) error { return t.db.Ping(ctx) }

func (t *fakeTx) Close(ctx context.Context) error { return t.db.Close(ctx) }

func (t *fakeTx) Commit(context.Context) error {
	t.committed = true
	return t.commitErr
}

func (t *fakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	return t.rollbackErr
}

func (t *fakeTx) Savepoint(context.Context, string) error { return nil }

func (t *fakeTx) RollbackTo(context.Context, string) error { return nil }
