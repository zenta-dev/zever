package db

import (
	"context"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox/outboxtest"
)

// TestConformance runs the outbox conformance kit against the sqlite-backed
// driver with durable behavior enabled.
func TestConformance(t *testing.T) {
	outboxtest.Conformance(t, func(t *testing.T, pub *outboxtest.Recorder) outboxtest.Harness {
		t.Helper()

		d := mustNew(t, Options{Publisher: pub, MaxAttempts: 2, Retention: time.Hour})

		return outboxtest.Harness{
			Store: d,
			InTx: func(ctx context.Context, fn func(ctx context.Context, tx coredb.Tx) error) error {
				return coredb.WithTx(ctx, d.conn, nil, fn)
			},
			Inbox:   d,
			Durable: true,
		}
	})
}
