package db

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestSetPublisher_startFailsWithoutPublisher(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})

	err := d.Start(t.Context())
	if err == nil {
		t.Fatal("Start() without a publisher = nil, want error")
	}

	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Errorf("Start() error = %v, want it to wrap outbox.ErrInvalidOptions", err)
	}
}

func TestSetPublisher_attachedPublisherDrainsPending(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{})

	d.SetPublisher(pub)

	mustRecord(t, d, outbox.Message{ID: "late", Topic: "orders.created", Payload: []byte("x")})
	d.pollOnce(t.Context())

	got := pub.messages()
	if len(got) != 1 {
		t.Fatalf("published %d, want 1", len(got))
	}

	if got[0].ID != "late" || got[0].Topic != "orders.created" {
		t.Errorf("published %+v, want the recorded message", got[0])
	}

	if err := d.Start(t.Context()); err != nil {
		t.Errorf("Start() after SetPublisher = %v, want nil", err)
	}
}

// A nil Publisher is ignored, so the wiring mistake cannot detach a publisher
// that is already working.
func TestSetPublisher_nilIgnored(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{})

	d.SetPublisher(pub)
	d.SetPublisher(nil)

	mustRecord(t, d, outbox.Message{ID: "kept", Topic: "t"})
	d.pollOnce(t.Context())

	if n := len(pub.messages()); n != 1 {
		t.Fatalf("published %d, want 1: nil SetPublisher detached the publisher", n)
	}
}

// The relay reads the publisher per message, so the most recent attach wins.
// Repointing a running relay is only correct when both destinations are the
// same transport, which is why OutboxRelay attaches before Start.
func TestSetPublisher_latestAttachWins(t *testing.T) {
	t.Parallel()

	first := &recordingPublisher{}
	second := &recordingPublisher{}
	d := mustNew(t, Options{})

	d.SetPublisher(first)
	d.SetPublisher(second)

	mustRecord(t, d, outbox.Message{ID: "one", Topic: "t"})
	d.pollOnce(t.Context())

	if n := len(first.messages()); n != 0 {
		t.Errorf("first publisher received %d, want 0 after reattach", n)
	}

	if n := len(second.messages()); n != 1 {
		t.Errorf("second publisher received %d, want 1", n)
	}
}

// Concurrent attach + drain exercises the race detector on the publisher field.
func TestSetPublisher_concurrentWithRelay(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	const workers = 8

	var wg sync.WaitGroup

	for range workers {
		wg.Add(2)

		go func() {
			defer wg.Done()

			d.SetPublisher(pub)
		}()

		go func() {
			defer wg.Done()

			d.pollOnce(t.Context())
		}()
	}

	wg.Wait()
}
