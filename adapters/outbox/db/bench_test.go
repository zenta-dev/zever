package db

import (
	"context"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

func benchMessage(id string) outbox.Message {
	return outbox.Message{ID: id, Topic: "orders", Payload: []byte("payload")}
}

func mustDriver(b *testing.B, opts Options) *driver {
	b.Helper()

	if opts.Path == "" && opts.DSN == "" {
		opts.Path = filepath.Join(b.TempDir(), "bench.db")
	}

	s, err := New(opts)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	d, ok := s.(*driver)
	if !ok {
		b.Fatalf("New() = %T, want *driver", s)
	}

	return d
}

func BenchmarkNew(b *testing.B) {
	dir := b.TempDir()

	var seq atomic.Uint64

	b.ReportAllocs()

	for b.Loop() {
		opts := Options{Path: filepath.Join(dir, "bench-"+strconv.FormatUint(seq.Add(1), 10)+".db")}

		s, err := New(opts)
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		_ = s.Close()
	}
}

func BenchmarkRecord(b *testing.B) {
	d := mustDriver(b, Options{})

	b.ReportAllocs()

	var seq atomic.Uint64

	for b.Loop() {
		id := "bench-" + strconv.FormatUint(seq.Add(1), 10)

		err := coredb.WithTx(b.Context(), d.conn, nil, func(ctx context.Context, tx coredb.Tx) error {
			return d.Record(ctx, tx, benchMessage(id))
		})
		if err != nil {
			b.Fatalf("Record() error = %v", err)
		}
	}
}

func BenchmarkPollOnce(b *testing.B) {
	d := mustDriver(b, Options{Publisher: &recordingPublisher{}})

	b.ReportAllocs()

	var seq atomic.Uint64

	for b.Loop() {
		id := "bench-" + strconv.FormatUint(seq.Add(1), 10)

		err := coredb.WithTx(b.Context(), d.conn, nil, func(ctx context.Context, tx coredb.Tx) error {
			return d.Record(ctx, tx, benchMessage(id))
		})
		if err != nil {
			b.Fatalf("Record() error = %v", err)
		}

		d.pollOnce(b.Context())
	}
}

func BenchmarkProcess(b *testing.B) {
	d := mustDriver(b, Options{})

	b.ReportAllocs()

	var seq atomic.Uint64

	for b.Loop() {
		id := "bench-" + strconv.FormatUint(seq.Add(1), 10)

		err := coredb.WithTx(b.Context(), d.conn, nil, func(ctx context.Context, tx coredb.Tx) error {
			return d.Process(ctx, tx, id, func(context.Context, coredb.Tx) error { return nil })
		})
		if err != nil {
			b.Fatalf("Process() error = %v", err)
		}
	}
}

func BenchmarkStatus(b *testing.B) {
	d := mustDriver(b, Options{Publisher: &recordingPublisher{}})

	b.ReportAllocs()

	for b.Loop() {
		_ = d.Status()
	}
}

func BenchmarkQuoteIdent(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = quoteIdent("outbox")
	}
}

func BenchmarkTs(b *testing.B) {
	d := mustDriver(b, Options{})

	in := time.Date(2026, 10, 6, 12, 30, 45, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		_ = d.ts(in)
	}
}

func BenchmarkCoerceTime(b *testing.B) {
	in := "2026-10-06T12:30:45.000000000Z"

	b.ReportAllocs()

	for b.Loop() {
		_, _ = coerceTime(in)
	}
}

func BenchmarkCoerceInt(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_, _ = coerceInt(int64(42))
	}
}

func BenchmarkCoerceString(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = coerceString("value")
	}
}

func BenchmarkDecodeHeaders(b *testing.B) {
	in := `{"k":"v","trace":"abc"}`

	b.ReportAllocs()

	for b.Loop() {
		_ = decodeHeaders(in)
	}
}
