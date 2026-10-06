package memory_test

import (
	"context"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox/outboxtest"

	"github.com/zenta-dev/zever/adapters/outbox/memory"
)

func TestConformance(t *testing.T) {
	t.Parallel()

	outboxtest.Conformance(t, func(_ *testing.T, pub *outboxtest.Recorder) outboxtest.Harness {
		s, err := memory.New(memory.Options{Publisher: pub})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return outboxtest.Harness{
			Store: s,
			InTx: func(ctx context.Context, fn func(context.Context, coredb.Tx) error) error {
				return fn(ctx, nil)
			},
		}
	})
}
