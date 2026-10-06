package cdc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestRegisterWiresCDCAdapter(t *testing.T) {
	Register()
	Register()

	s, err := outbox.Open(outbox.CDC, outbox.Options{
		DSN:         "postgres://localhost/db",
		Slot:        "s",
		Publication: "p",
		Prefix:      "x",
	})
	if err != nil {
		t.Fatalf("Open(cdc) error = %v", err)
	}

	if s.Name() != string(outbox.CDC) {
		t.Errorf("Name() = %q, want %q", s.Name(), outbox.CDC)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if _, err := outbox.Open(outbox.CDC, outbox.Options{}); err == nil {
		t.Error("Open(cdc, empty) = nil error, want error")
	}
}

func TestNewCustomMaxAttempts(t *testing.T) {
	t.Parallel()

	s, err := New(Options{Options: outbox.Options{
		DSN:         "postgres://localhost/db",
		Slot:        "s",
		Publication: "p",
		Prefix:      "x",
		MaxAttempts: 2,
	}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	st, ok := s.(*store)
	if !ok {
		t.Fatalf("New() = %T, want *store", s)
	}

	if st.maxAttempts != 2 {
		t.Errorf("maxAttempts = %d, want 2", st.maxAttempts)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestNewInvalidCoreOptions(t *testing.T) {
	t.Parallel()

	_, err := New(Options{Options: outbox.Options{
		DSN:          "postgres://localhost/db",
		Slot:         "s",
		Publication:  "p",
		Prefix:       "x",
		PollInterval: -time.Second,
	}})
	if err == nil {
		t.Fatal("New(negative poll) = nil error, want error")
	}

	if !strings.Contains(err.Error(), "cdc:") {
		t.Errorf("New() error = %v, want cdc prefix", err)
	}
}

func TestValidateJoinsAllViolations(t *testing.T) {
	t.Parallel()

	err := Options{}.Validate()
	if err == nil {
		t.Fatal("Validate(empty) = nil error, want joined error")
	}

	for _, want := range []string{"dsn", "slot", "publication", "prefix"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error = %v, want containing %q", err, want)
		}
	}
}

func TestValidateWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts Options
	}{
		{"slot", Options{Options: outbox.Options{DSN: "postgres://x", Slot: "  ", Publication: "p", Prefix: "x"}}},
		{"publication", Options{Options: outbox.Options{DSN: "postgres://x", Slot: "s", Publication: "  ", Prefix: "x"}}},
		{"prefix", Options{Options: outbox.Options{DSN: "postgres://x", Slot: "s", Publication: "p", Prefix: "  "}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.opts.Validate(); err == nil {
				t.Errorf("Validate(%s whitespace) = nil error, want error", tt.name)
			}
		})
	}
}

func TestRecordSuccess(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix}
	tx := &fakeTx{}

	err := s.Record(t.Context(), tx, outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("p")})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if tx.execs != 1 {
		t.Errorf("Exec calls = %d, want 1", tx.execs)
	}
}

func TestStartInvalidDSN(t *testing.T) {
	t.Parallel()

	s, err := New(Options{
		Options:   outbox.Options{DSN: "://invalid", Slot: "s", Publication: "p", Prefix: "x"},
		Publisher: &stubPublisher{},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = s.Start(t.Context())
	if err == nil {
		t.Fatal("Start(bad dsn) = nil error, want error")
	}

	if !strings.Contains(err.Error(), "cdc: parse dsn") {
		t.Errorf("Start() error = %v, want containing %q", err, "cdc: parse dsn")
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestStartConnectRefused(t *testing.T) {
	t.Parallel()

	s, err := New(Options{
		Options:   outbox.Options{DSN: "postgres://127.0.0.1:1/db?sslmode=disable&connect_timeout=2", Slot: "s", Publication: "p", Prefix: "x"},
		Publisher: &stubPublisher{},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	err = s.Start(ctx)
	if err == nil {
		t.Fatal("Start(refused) = nil error, want error")
	}

	if !strings.Contains(err.Error(), "cdc: connect") {
		t.Errorf("Start() error = %v, want containing %q", err, "cdc: connect")
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestStartIdempotentWhenStarted(t *testing.T) {
	t.Parallel()

	s := &store{started: true, publisher: &stubPublisher{}}

	if err := s.Start(t.Context()); err != nil {
		t.Errorf("Start(started) error = %v, want nil", err)
	}
}

func TestCloseRunsCancelAndDone(t *testing.T) {
	t.Parallel()

	cancelled := false
	done := make(chan struct{})
	close(done)

	s := &store{cancel: func() { cancelled = true }, done: done}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if !cancelled {
		t.Error("Close() did not call cancel, want cancel called")
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}

func TestSetRelayErrorNilKeepsLastErrorEmpty(t *testing.T) {
	t.Parallel()

	s := &store{}
	s.setRelayError(nil)

	if st := s.Status(); st.LastError != "" {
		t.Errorf("Status().LastError = %q, want empty", st.LastError)
	}
}

func TestStatusReportsRelayErrorAndCounters(t *testing.T) {
	t.Parallel()

	s := &store{}
	s.processed.Add(2)
	s.failed.Add(1)
	s.setRelayError(errors.New("boom"))

	st := s.Status()

	if st.Processed != 2 {
		t.Errorf("Status().Processed = %d, want 2", st.Processed)
	}

	if st.Failed != 1 {
		t.Errorf("Status().Failed = %d, want 1", st.Failed)
	}

	if st.Pending != 0 {
		t.Errorf("Status().Pending = %d, want 0", st.Pending)
	}

	if !strings.Contains(st.LastError, "boom") {
		t.Errorf("Status().LastError = %q, want containing boom", st.LastError)
	}
}

func TestStoreConcurrentUse(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: recordingSleep(new([]time.Duration))}

	var wg sync.WaitGroup

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_ = s.Status()
			s.setRelayError(errors.New("boom"))
			s.processed.Add(1)
			_ = quoteLiteral("slot")
		}()
	}

	wg.Wait()

	if st := s.Status(); st.Processed != 8 {
		t.Errorf("Status().Processed = %d, want 8", st.Processed)
	}
}

func TestSleepCtxPositiveCompletes(t *testing.T) {
	t.Parallel()

	if err := sleepCtx(t.Context(), time.Millisecond); err != nil {
		t.Errorf("sleepCtx(1ms) = %v, want nil", err)
	}
}

func TestLoopCancelledClosesDone(t *testing.T) {
	t.Parallel()

	s := &store{}
	done := make(chan struct{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	s.loop(ctx, nil, done)

	select {
	case <-done:
	default:
		t.Error("loop(cancelled) did not close done, want closed")
	}
}

func TestHandleKeepaliveMalformed(t *testing.T) {
	t.Parallel()

	s := &store{}
	s.handleKeepalive(t.Context(), nil, []byte{1, 2, 3})

	if st := s.Status(); st.LastError == "" {
		t.Error("Status().LastError is empty, want parse error")
	}
}

func TestHandleKeepaliveNoReplyRequested(t *testing.T) {
	t.Parallel()

	s := &store{publisher: &stubPublisher{}}
	s.handleKeepalive(t.Context(), nil, make([]byte, 17))

	if st := s.Status(); st.LastError != "" {
		t.Errorf("Status().LastError = %q, want empty", st.LastError)
	}
}

func TestHandleXLogDataMalformed(t *testing.T) {
	t.Parallel()

	s := &store{publisher: &stubPublisher{}}
	s.handleXLogData(t.Context(), nil, []byte{1, 2})

	if st := s.Status(); st.LastError == "" {
		t.Error("Status().LastError is empty, want parse error")
	}
}

func TestHandleXLogDataBadWAL(t *testing.T) {
	t.Parallel()

	s := &store{publisher: &stubPublisher{}}
	s.handleXLogData(t.Context(), nil, xlogEnvelopeBytes([]byte{0xFF}))

	if st := s.Status(); st.LastError == "" {
		t.Error("Status().LastError is empty, want WAL parse error")
	}
}

func TestHandleXLogDataNonMessageSkipped(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, publisher: &stubPublisher{}}
	begin := append([]byte{'B'}, make([]byte, 20)...)
	s.handleXLogData(t.Context(), nil, xlogEnvelopeBytes(begin))

	if st := s.Status(); st.LastError != "" {
		t.Errorf("Status().LastError = %q, want empty", st.LastError)
	}

	if st := s.Status(); st.Processed != 0 {
		t.Errorf("Status().Processed = %d, want 0", st.Processed)
	}
}

func TestHandleXLogDataDecodeError(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, publisher: &stubPublisher{}}
	wal := ldmWALBytes(DefaultPrefix, []byte("not json"), 2)
	s.handleXLogData(t.Context(), nil, xlogEnvelopeBytes(wal))

	if st := s.Status(); st.LastError == "" {
		t.Error("Status().LastError is empty, want decode error")
	}
}

func TestHandleXLogDataExhaustedStaysUnacked(t *testing.T) {
	t.Parallel()

	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 1,
		sleep:       recordingSleep(new([]time.Duration)),
		publisher:   &stubPublisher{failures: 10},
	}

	msg := outbox.Message{ID: "evt-1", Topic: "orders"}
	wal := ldmWALBytes(DefaultPrefix, mustPayload(t, msg), 2)
	s.handleXLogData(t.Context(), nil, xlogEnvelopeBytes(wal))

	if st := s.Status(); st.Failed != 1 {
		t.Errorf("Status().Failed = %d, want 1", st.Failed)
	}

	if got := s.lastLSN.Load(); got != 0 {
		t.Errorf("lastLSN = %d, want 0 (unacknowledged)", got)
	}
}

func TestQuoteLiteralEdge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "''"},
		{name: "plain", in: "slot", want: "'slot'"},
		{name: "quotes", in: "a''b", want: "'a''''b'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := quoteLiteral(tt.in); got != tt.want {
				t.Errorf("quoteLiteral(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
