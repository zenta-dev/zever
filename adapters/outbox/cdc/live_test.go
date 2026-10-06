package cdc

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// TestCDCLive exercises the adapter end to end against a live postgres with
// wal_level=logical: it creates a slot and publication, records a message in
// a transaction, and asserts the consumer receives and publishes it. Set
// POSTGRES_DSN to run; skipped otherwise.
func TestCDCLive(t *testing.T) {
	dsn := postgresDSN(t)
	requireWalLevelLogical(t, dsn)

	ctx := t.Context()

	const (
		prefix      = "zever_cdc_live"
		slot        = "zever_cdc_live_slot"
		publication = "zever_cdc_live_pub"
	)

	prod := openStubDB(t, dsn)

	// Create the publication the pgoutput plugin args reference. An empty
	// publication is enough: pg_logical_emit_message is not filtered by it.
	if _, err := prod.Exec(ctx, `CREATE PUBLICATION `+publication); err != nil {
		skipOnPermissionError(t, err)
		t.Fatalf("create publication error = %v", err)
	}

	received := make(chan outbox.Message, 1)
	pub := outbox.PublisherFunc(func(_ context.Context, msg outbox.Message) error {
		received <- msg

		return nil
	})

	s, err := New(Options{
		Options: outbox.Options{
			DSN:         dsn,
			Prefix:      prefix,
			Slot:        slot,
			Publication: publication,
		},
		Publisher: pub,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if startErr := s.Start(ctx); startErr != nil {
		skipOnPermissionError(t, startErr)
		t.Fatalf("Start() error: %v", startErr)
	}

	t.Cleanup(func() { _ = s.Close() })

	msg := outbox.Message{ID: "cdc-live-1", Topic: "orders", Payload: []byte(`{"ok":true}`)}

	err = db.WithTx(ctx, prod, nil, func(ctx context.Context, tx db.Tx) error {
		return s.Record(ctx, tx, msg)
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	select {
	case got := <-received:
		if got.ID != msg.ID || got.Topic != msg.Topic {
			t.Errorf("received = %+v, want ID %q topic %q", got, msg.ID, msg.Topic)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for replicated message")
	}

	if st := s.Status(); st.Processed != 1 {
		t.Errorf("Status().Processed = %d, want 1", st.Processed)
	}
}
