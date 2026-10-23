package outbox_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// adapterSeq numbers the names uniqueAdapter hands out.
var adapterSeq atomic.Int64

// uniqueAdapter returns prefix plus a per-call suffix. The adapter registry
// is process-global with no unregister, so a fixed name would make every
// -count iteration after the first fail with ErrDuplicateAdapter.
type stubStore struct {
	name string
}

func (s *stubStore) Record(context.Context, db.Tx, outbox.Message) error { return nil }
func (s *stubStore) Start(context.Context) error                         { return nil }
func (s *stubStore) Status() outbox.Status                               { return outbox.Status{} }
func (s *stubStore) Close() error                                        { return nil }
func (s *stubStore) Name() string                                        { return s.name }

func TestPublisherFunc(t *testing.T) {
	var got outbox.Message

	var p outbox.Publisher = outbox.PublisherFunc(func(_ context.Context, m outbox.Message) error {
		got = m

		return nil
	})

	if err := p.Publish(context.Background(), outbox.Message{ID: "1", Topic: "t"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if got.ID != "1" {
		t.Errorf("got ID = %q, want 1", got.ID)
	}
}
