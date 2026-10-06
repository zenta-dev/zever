package cdc

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/outbox/outboxtest"
)

const (
	conformancePrefix      = "zever_cdc_conf"
	conformanceSlot        = "zever_cdc_conf_slot"
	conformancePublication = "zever_cdc_conf_pub"
)

// TestConformance runs the outbox conformance kit against a live postgres with
// wal_level=logical. CDC has no pending table, so the durable retry and DLQ
// subtests skip; RecordPublish and Close run end to end. Set POSTGRES_DSN to
// run; skipped otherwise.
func TestConformance(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set POSTGRES_DSN to run cdc conformance")
	}

	requireWalLevelLogical(t, dsn)

	prod := openStubDB(t, dsn)

	if _, err := prod.Exec(t.Context(), `CREATE PUBLICATION `+conformancePublication); err != nil {
		skipOnPermissionError(t, err)
		t.Fatalf("create publication error = %v", err)
	}

	var slotSeq atomic.Int64

	outboxtest.Conformance(t, func(t *testing.T, pub *outboxtest.Recorder) outboxtest.Harness {
		t.Helper()

		n := slotSeq.Add(1)

		s, err := New(Options{
			Options: outbox.Options{
				DSN:         dsn,
				Prefix:      conformancePrefix,
				Slot:        fmt.Sprintf("%s_%d", conformanceSlot, n),
				Publication: conformancePublication,
			},
			Publisher: pub,
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		conn := openStubDB(t, dsn)

		return outboxtest.Harness{
			Store: s,
			InTx: func(ctx context.Context, fn func(context.Context, db.Tx) error) error {
				return db.WithTx(ctx, conn, nil, fn)
			},
		}
	})
}
