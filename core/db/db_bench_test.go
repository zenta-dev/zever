package db

import (
	"context"
	"testing"
	"time"
)

// BenchmarkRegister measures registry insertion for a fresh adapter key.
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
