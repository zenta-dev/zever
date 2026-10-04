package db

import "testing"

func TestEdgeTxFromContext_nilTxStored(t *testing.T) {
	t.Parallel()

	ctx := WithTxIntoContext(t.Context(), nil)

	tx, ok := TxFromContext(ctx)
	if ok || tx != nil {
		t.Fatalf("TxFromContext(nil tx) = (%v, %v), want (nil, false)", tx, ok)
	}
}

func TestEdgeTxContext_overwriteLastWins(t *testing.T) {
	t.Parallel()

	first := &fakeTx{}
	second := &fakeTx{}
	ctx := WithTxIntoContext(WithTxIntoContext(t.Context(), first), second)

	got, ok := TxFromContext(ctx)
	if !ok {
		t.Fatal("expected tx in context")
	}

	if got != Tx(second) {
		t.Fatal("overwrite did not win")
	}
}

func TestEdgeOptions_Validate_negativeDurations(t *testing.T) {
	t.Parallel()

	if err := (Options{MaxConnLifetime: -1}).Validate(); err == nil {
		t.Fatal("negative MaxConnLifetime err = nil, want error")
	}

	if err := (Options{MaxConnIdleTime: -1}).Validate(); err == nil {
		t.Fatal("negative MaxConnIdleTime err = nil, want error")
	}
}
