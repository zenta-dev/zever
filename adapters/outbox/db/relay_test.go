package db

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestRetryThenSuccess(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	pub.failNext(1)

	d := mustNew(t, Options{Publisher: pub, MaxAttempts: 5})

	mustRecord(t, d, outbox.Message{ID: "retry", Topic: "t", Payload: []byte("x")})

	d.pollOnce(t.Context()) // attempt 1 fails
	d.pollOnce(t.Context()) // attempt 2 succeeds

	got := pub.messages()
	if len(got) != 1 {
		t.Fatalf("published %d, want 1 after retry", len(got))
	}

	if got[0].Attempts < 2 {
		t.Errorf("Attempts = %d, want >= 2", got[0].Attempts)
	}

	if st := d.Status(); st.Failed != 0 {
		t.Errorf("Status().Failed = %d, want 0", st.Failed)
	}
}

func TestDLQAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	pub.failNext(100)

	d := mustNew(t, Options{Publisher: pub, MaxAttempts: 2})

	mustRecord(t, d, outbox.Message{ID: "dlq", Topic: "t", Payload: []byte("x")})

	d.pollOnce(t.Context())
	d.pollOnce(t.Context())
	d.pollOnce(t.Context())

	if n := len(pub.messages()); n != 0 {
		t.Fatalf("published %d, want 0 (all failed)", n)
	}

	st := d.Status()
	if st.Failed != 1 {
		t.Errorf("Status().Failed = %d, want 1", st.Failed)
	}

	if st.Pending != 0 {
		t.Errorf("Status().Pending = %d, want 0 after DLQ", st.Pending)
	}

	if st.LastError == "" {
		t.Error("Status().LastError is empty, want the publish error")
	}
}

func TestRetentionCleanup(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub, Retention: time.Minute})

	mustRecord(t, d, outbox.Message{ID: "old", Topic: "t"})
	d.pollOnce(t.Context())

	if n := countRows(t, d); n != 1 {
		t.Fatalf("rows = %d, want 1 processed", n)
	}

	// Age the processed row past retention deterministically.
	_, err := d.conn.Exec(t.Context(),
		`UPDATE `+quoteIdent(DefaultTable)+` SET processed_at = ?`, d.ts(time.Now().UTC().Add(-time.Hour)))
	if err != nil {
		t.Fatalf("age processed_at error = %v", err)
	}

	d.cleanup(t.Context())

	if n := countRows(t, d); n != 0 {
		t.Fatalf("rows = %d, want 0 after retention cleanup", n)
	}
}

func TestStatusStalled(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	mustRecord(t, d, outbox.Message{ID: "stuck", Topic: "t", CreatedAt: time.Now().UTC().Add(-10 * time.Minute)})

	st := d.Status()
	if st.Pending != 1 {
		t.Fatalf("Status().Pending = %d, want 1", st.Pending)
	}

	if !st.Stalled {
		t.Errorf("Status().Stalled = false, want true for a 10m-old pending row")
	}
}

func TestStatusNotStalledWhenFresh(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	mustRecord(t, d, outbox.Message{ID: "fresh", Topic: "t"})

	if st := d.Status(); st.Stalled {
		t.Errorf("Status().Stalled = true, want false for a fresh row")
	}
}

func TestCloseStopsRelay(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	if err := d.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := d.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}

	if err := d.Start(t.Context()); err == nil {
		t.Error("Start() after Close = nil, want error")
	}
}

func TestStartPublishesAsync(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	mustRecord(t, d, outbox.Message{ID: "async", Topic: "t", Payload: []byte("x")})

	if err := d.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()

	for len(pub.messages()) == 0 {
		select {
		case <-deadline:
			t.Fatal("relay did not publish within deadline")
		case <-ticker.C:
		}
	}
}
