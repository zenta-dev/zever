package outboxtest_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/outbox/outboxtest"
)

// stubStore publishes synchronously on Record, mirroring the memory adapter,
// so the kit proves its non-durable path.
type stubStore struct {
	pub outbox.Publisher
}

func (s *stubStore) Record(ctx context.Context, _ db.Tx, msg outbox.Message) error {
	if err := msg.Validate(); err != nil {
		return err
	}

	return s.pub.Publish(ctx, msg)
}

func (s *stubStore) Start(context.Context) error { return nil }
func (s *stubStore) Status() outbox.Status       { return outbox.Status{} }
func (s *stubStore) Close() error                { return nil }
func (s *stubStore) Name() string                { return "stub" }

func TestConformanceStub(t *testing.T) {
	t.Parallel()

	outboxtest.Conformance(t, func(_ *testing.T, pub *outboxtest.Recorder) outboxtest.Harness {
		return outboxtest.Harness{
			Store: &stubStore{pub: pub},
			InTx:  func(ctx context.Context, fn func(context.Context, db.Tx) error) error { return fn(ctx, nil) },
		}
	})
}
