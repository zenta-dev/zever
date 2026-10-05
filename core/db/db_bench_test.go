package db

import (
	"context"
	"testing"
	"time"
)

// BenchmarkRegister measures registry insertion for a fresh adapter key.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if err := Register(dbFreshAdapter(), func(Options) (DB, error) { return &stubDB{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

// BenchmarkOpen measures validated registry lookup plus the factory call.
func BenchmarkOpen(b *testing.B) {
	adapter := dbFreshAdapter()
	if err := Register(adapter, func(Options) (DB, error) { return &stubDB{dialect: "sqlite"}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Open(adapter, Options{DSN: "postgres://localhost/db"}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

// BenchmarkOptionsValidate measures the adapter-independent range checks.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{MaxConns: 10, MinConns: 2, MaxConnLifetime: time.Hour, MaxConnIdleTime: time.Minute}

	b.ReportAllocs()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

// BenchmarkAdapterString measures canonical adapter naming.
func BenchmarkAdapterString(b *testing.B) {
	a := Postgres

	b.ReportAllocs()

	for b.Loop() {
		_ = a.String()
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := ParseAdapter("postgres"); err != nil {
			b.Fatalf("ParseAdapter err = %v", err)
		}
	}
}

// BenchmarkIsolationLevelString measures isolation-level rendering.
func BenchmarkIsolationLevelString(b *testing.B) {
	level := Serializable

	b.ReportAllocs()

	for b.Loop() {
		_ = level.String()
	}
}

// BenchmarkTxContextRoundTrip measures attaching and reading a Tx from context.
func BenchmarkTxContextRoundTrip(b *testing.B) {
	tx := &fakeTx{}
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		got, ok := TxFromContext(WithTxIntoContext(ctx, tx))
		if !ok || got != Tx(tx) {
			b.Fatal("Tx context round trip failed")
		}
	}
}

// BenchmarkWithTxCommit measures the transaction commit path through WithTx.
func BenchmarkWithTxCommit(b *testing.B) {
	tx := &fakeTx{}
	transactor := &fakeTransactor{tx: tx}
	ctx := b.Context()
	noop := func(context.Context, Tx) error { return nil }

	b.ReportAllocs()

	for b.Loop() {
		if err := WithTx(ctx, transactor, nil, noop); err != nil {
			b.Fatalf("WithTx err = %v", err)
		}
	}
}
