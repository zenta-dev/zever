package orm

import (
	"context"
	"errors"
	"iter"
	"runtime"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
)

// TestQueryStreamMatchesAll proves Stream yields exactly the rows All does,
// in the same order, for the same filtered query -- the two share one
// execution path.
func TestQueryStreamMatchesAll(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	q := From(widgets).Where(widgetQty.Gte(20)).OrderBy(widgetID.Asc())

	want, err := q.All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	var got []*widget

	for row, err := range q.Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		got = append(got, row)
	}

	if len(got) != len(want) {
		t.Fatalf("Stream yielded %d rows, All returned %d", len(got), len(want))
	}

	for i := range want {
		if got[i].ID != want[i].ID || got[i].Name != want[i].Name || got[i].Quantity != want[i].Quantity {
			t.Fatalf("row %d mismatch: Stream=%+v All=%+v", i, got[i], want[i])
		}
	}
}

// TestQueryStreamEmpty proves Stream with no matching rows yields nothing
// and no error.
func TestQueryStreamEmpty(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	n := 0

	for _, err := range From(widgets).Where(widgetID.Eq("nope")).Stream(ctx, conn) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		n++
	}

	if n != 0 {
		t.Fatalf("Stream yielded %d rows for a no-match filter, want 0", n)
	}
}

// closeCountingRows wraps a db.Rows and counts Close calls, to prove
// Stream owns the cursor lifecycle.
type closeCountingRows struct {
	db.Rows

	closeCalls int
}

func (r *closeCountingRows) Err() error { return nil }
func (r *closeCountingRows) Close() error {
	r.closeCalls++

	return r.Rows.Close()
}

// closeCountingDB wraps a db.DB and hands back a closeCountingRows for
// every Query, so a test can observe exactly when (and how often) Stream
// closes the cursor.
type closeCountingDB struct {
	db.DB

	rows *closeCountingRows
}

func (c *closeCountingDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	rows, err := c.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	c.rows = &closeCountingRows{Rows: rows}

	return c.rows, nil
}

// TestQueryStreamEarlyBreakClosesCursor is the range-over-func cleanup
// guarantee: when the consumer breaks out of the loop after the first row,
// the underlying cursor is closed exactly once -- never leaked, never
// double-closed.
func TestQueryStreamEarlyBreakClosesCursor(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	counting := &closeCountingDB{DB: conn}

	n := 0

	for _, err := range From(widgets).OrderBy(widgetID.Asc()).Stream(ctx, counting) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		n++
		if n == 1 {
			break
		}
	}

	if counting.rows == nil {
		t.Fatalf("no cursor was ever acquired")
	}

	if n != 1 {
		t.Fatalf("iterated %d rows before break, want 1", n)
	}

	if counting.rows.closeCalls != 1 {
		t.Fatalf("cursor Close called %d times after early break, want exactly 1", counting.rows.closeCalls)
	}
}

// TestQueryStreamFullConsumptionClosesOnce proves the full-consumption path
// also closes the cursor exactly once -- the explicit close that surfaces a
// close-time error is the single close, and the deferred cleanup sees the
// `closed` guard and does not close a second time.
func TestQueryStreamFullConsumptionClosesOnce(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	counting := &closeCountingDB{DB: conn}

	for _, err := range From(widgets).Stream(ctx, counting) {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
	}

	if counting.rows == nil {
		t.Fatalf("no cursor was ever acquired")
	}

	if counting.rows.closeCalls != 1 {
		t.Fatalf("cursor Close called %d times after full consumption, want exactly 1", counting.rows.closeCalls)
	}
}

// stubRows is a hand-rolled db.Rows yielding a fixed set of already
// materialized rows, used to exercise Stream's error paths without a real
// database.
type stubRows struct {
	cols       []string
	values     [][]any
	idx        int
	scanErr    error
	iterErr    error
	closeErr   error
	closeCalls int
}

func (r *stubRows) Next() bool {
	r.idx++

	return r.idx <= len(r.values)
}

func (r *stubRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}

	for i := range dest {
		if i < len(r.values[r.idx-1]) {
			if d, ok := dest[i].(*any); ok {
				*d = r.values[r.idx-1][i]
			}
		}
	}

	return nil
}

func (r *stubRows) Err() error { return r.iterErr }
func (r *stubRows) Close() error {
	r.closeCalls++

	return r.closeErr
}

func (r *stubRows) Columns() ([]string, error) { return r.cols, nil }

// stubDB is a db.DB over a caller-chosen stubRows, for error-path tests.
type stubDB struct {
	db.DB

	rows *stubRows
}

func (s *stubDB) Query(context.Context, string, ...any) (db.Rows, error) { return s.rows, nil }
func (s *stubDB) Dialect() string                                        { return "sqlite" }

// TestQueryStreamScanError proves a per-row scan failure surfaces through
// the iterator as an error (wrapped with the scan tag) and that the cursor
// is still closed exactly once.
func TestQueryStreamScanError(t *testing.T) {
	ctx := context.Background()

	rows := &stubRows{
		cols:    []string{"id", "name", "quantity", "bio"},
		values:  [][]any{{"w1", "Alpha", int64(10), nil}},
		scanErr: errors.New("boom"),
	}

	conn := &stubDB{rows: rows}

	var gotErr error
	yielded := 0

	for _, err := range From(widgets).Stream(ctx, conn) {
		gotErr = err
		yielded++
		break
	}

	if gotErr == nil {
		t.Fatalf("Stream with a failing Scan yielded no error")
	}

	if !strings.Contains(gotErr.Error(), "scan") {
		t.Fatalf("scan error not wrapped: %v", gotErr)
	}

	if yielded != 1 {
		t.Fatalf("yielded %d values, want exactly 1 (the error)", yielded)
	}

	if rows.closeCalls != 1 {
		t.Fatalf("cursor Close called %d times after scan error, want exactly 1", rows.closeCalls)
	}
}

// TestQueryStreamRowsErr proves a rows-iteration error surfaces through the
// iterator wrapped with the Stream tag.
func TestQueryStreamRowsErr(t *testing.T) {
	ctx := context.Background()

	rows := &stubRows{
		cols:    []string{"id", "name", "quantity", "bio"},
		iterErr: errors.New("iteration boom"),
	}

	conn := &stubDB{rows: rows}

	var gotErr error

	for _, err := range From(widgets).Stream(ctx, conn) {
		gotErr = err
		break
	}

	if gotErr == nil {
		t.Fatalf("Stream with a failing rows.Err yielded no error")
	}

	if !strings.Contains(gotErr.Error(), "iteration boom") {
		t.Fatalf("rows error not wrapped: %v", gotErr)
	}
}

// TestQueryStreamCloseErr proves a close-time error surfaces through the
// iterator on the full-consumption path.
func TestQueryStreamCloseErr(t *testing.T) {
	ctx := context.Background()

	rows := &stubRows{
		cols:     []string{"id", "name", "quantity", "bio"},
		closeErr: errors.New("close boom"),
	}

	conn := &stubDB{rows: rows}

	var gotErr error

	for _, err := range From(widgets).Stream(ctx, conn) {
		gotErr = err
		break
	}

	if gotErr == nil {
		t.Fatalf("Stream with a failing Close yielded no error")
	}

	if !strings.Contains(gotErr.Error(), "close boom") {
		t.Fatalf("close error not wrapped: %v", gotErr)
	}
}

// TestQueryStreamRenderError proves a render-time failure (here a tuple
// arity mismatch) surfaces on the first yield wrapped with the Stream tag.
func TestQueryStreamRenderError(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	oneColumn := From(widgetOrders).Columns(orderWidgetID.Col())

	var gotErr error

	for _, err := range From(widgets).
		Where(NewTuple(widgetID.Col(), widgetQty.Col()).In(oneColumn)).
		Stream(ctx, conn) {
		gotErr = err
		break
	}

	if gotErr == nil {
		t.Fatal("Stream with an arity mismatch succeeded, want an error")
	}

	if !strings.Contains(gotErr.Error(), "Query.Stream") {
		t.Fatalf("err = %v, want the Query.Stream tag", gotErr)
	}
}

// TestQueryStreamQueryError proves a rows-acquisition failure surfaces on
// the first yield wrapped with the Stream tag.
func TestQueryStreamQueryError(t *testing.T) {
	ctx := context.Background()

	var gotErr error

	for _, err := range From(widgets).Stream(ctx, errQueryDB{}) {
		gotErr = err
		break
	}

	if gotErr == nil {
		t.Fatal("Stream with a failing Query succeeded, want an error")
	}

	if !strings.Contains(gotErr.Error(), "Query.Stream") {
		t.Fatalf("err = %v, want the Query.Stream tag", gotErr)
	}
}

// TestQueryStreamDialectError proves an unresolvable dialect surfaces on
// the first yield rather than panicking or silently falling back.
func TestQueryStreamDialectError(t *testing.T) {
	ctx, _ := newWidgetsDB(t)

	var gotErr error

	for _, err := range From(widgets).Stream(ctx, fakeDB{}) {
		gotErr = err
		break
	}

	if gotErr == nil {
		t.Fatalf("Stream with an unsupported dialect name succeeded, want an error")
	}
}

// TestQueryStreamLockingError proves a locking request the dialect rejects
// surfaces on the first yield.
func TestQueryStreamLockingError(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var gotErr error

	for _, err := range From(widgets).ForUpdate().Stream(ctx, conn) {
		gotErr = err
		break
	}

	if gotErr == nil {
		t.Fatalf("Stream with FOR UPDATE on sqlite succeeded, want an error")
	}
}

// sampleLiveHeap forces a full GC and returns the live heap size. The GC
// is synchronous, so a sample never depends on goroutine scheduling: under
// CPU contention a background sampler goroutine stalls, letting transient
// per-row garbage accumulate between collections and inflate HeapAlloc
// readings by megabytes (observed on CI). Row-count-driven sampling below
// bounds uncollected garbage by rows-per-sample instead.
func sampleLiveHeap() uint64 {
	runtime.GC()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return m.HeapAlloc
}

// baseLiveHeap records the live heap before a measurement so peaks report
// net of pre-existing state (notably the seeded :memory: sqlite database,
// which lives in the Go heap).
func baseLiveHeap() uint64 {
	return sampleLiveHeap()
}

// peakLiveHeapStream drains seq, discarding every row, and returns the
// highest live-heap sample net of base. Samples fire every sampleRows
// rows, so the result reflects retained (not transient) memory at any
// row count: a discarded-row stream reports a ~flat peak while a
// retaining consumer would grow with the row count.
func peakLiveHeapStream(t *testing.T, base uint64, seq iter.Seq2[*widget, error]) uint64 {
	t.Helper()

	const sampleRows = 1000

	var (
		peak uint64
		n    int64
	)

	sample := func() {
		if live := sampleLiveHeap(); live > peak {
			peak = live
		}
	}

	for row, err := range seq {
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}

		_ = row.Quantity

		n++
		if n%sampleRows == 0 {
			sample()
		}
	}

	sample()

	if peak < base {
		return 0
	}

	return peak - base
}

// peakLiveHeapHeld materializes run's result set and returns its live-heap
// footprint net of base. The result is held live through the sample
// (KeepAlive), so one synchronous sample suffices -- no sampler involved.
func peakLiveHeapHeld(t *testing.T, base uint64, run func() (any, error)) uint64 {
	t.Helper()

	held, err := run()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	live := sampleLiveHeap()
	runtime.KeepAlive(held)

	if live < base {
		return 0
	}

	return live - base
}

// TestQueryStreamPeakMemoryFlatVersusAll is the flat-memory proof: Stream's
// peak live heap must NOT grow with the row count (10k vs 100k), while
// All's materializing slice must grow.
func TestQueryStreamPeakMemoryFlatVersusAll(t *testing.T) {
	ctx, conn := newBigWidgetsDB(t, 100000)

	// Warm the connection so driver/cursor buffers are in a steady state
	// before measurement.
	if _, err := From(widgets).Limit(1).All(ctx, conn); err != nil {
		t.Fatalf("warmup: %v", err)
	}

	stream10 := peakLiveHeapStream(t, baseLiveHeap(), From(widgets).Limit(10000).Stream(ctx, conn))

	stream100 := peakLiveHeapStream(t, baseLiveHeap(), From(widgets).Stream(ctx, conn))

	all10 := peakLiveHeapHeld(t, baseLiveHeap(), func() (any, error) {
		return From(widgets).Limit(10000).All(ctx, conn)
	})

	all100 := peakLiveHeapHeld(t, baseLiveHeap(), func() (any, error) {
		return From(widgets).All(ctx, conn)
	})

	t.Logf("stream peak live heap: 10k=%dB 100k=%dB", stream10, stream100)
	t.Logf("all    peak live heap: 10k=%dB 100k=%dB", all10, all100)

	const slack = 1 << 20 // 1 MiB

	// Stream: peak live heap stays ~flat between 10k and 100k rows.
	if stream100 > stream10+slack {
		t.Fatalf("Stream peak live heap grew from %dB (10k rows) to %dB (100k rows), want <= %dB (flat)", stream10, stream100, stream10+slack)
	}

	// All: peak live heap grows with the row count.
	if all100 < all10*3 {
		t.Fatalf("All peak live heap grew from %dB (10k) to %dB (100k), want > 3x growth", all10, all100)
	}

	// The point of streaming: at 100k rows, Stream's live footprint is a
	// small constant while All materializes the whole result set.
	if all100 < stream100*4 {
		t.Fatalf("All peak live heap (%dB) not clearly larger than Stream's (%dB) at 100k rows", all100, stream100)
	}
}
