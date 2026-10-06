package cdc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

// fakeTx is a minimal db.Tx whose Exec records the last statement. Record
// returns before Exec for invalid messages, so the stub only needs to satisfy
// the interface.
type fakeTx struct {
	execs int
}

func (t *fakeTx) Query(context.Context, string, ...any) (db.Rows, error) {
	return nil, errors.New("stub: query")
}

func (t *fakeTx) Exec(context.Context, string, ...any) (int64, error) {
	t.execs++

	return 0, nil
}

func (t *fakeTx) Ping(context.Context) error { return nil }

func (t *fakeTx) Close(context.Context) error { return nil }

func (t *fakeTx) Dialect() string { return "postgres" }

func (t *fakeTx) Commit(context.Context) error { return nil }

func (t *fakeTx) Rollback(context.Context) error { return nil }

func (t *fakeTx) Savepoint(context.Context, string) error { return nil }

func (t *fakeTx) RollbackTo(context.Context, string) error { return nil }

func TestMessageRoundTrip(t *testing.T) {
	t.Parallel()

	msg := outbox.Message{
		ID:        "evt-1",
		Topic:     "orders",
		Key:       "tenant-1",
		Payload:   []byte(`{"amount":42}`),
		Headers:   map[string]string{"trace": "abc"},
		CreatedAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		Attempts:  2,
	}

	payload, err := encodeMessage(msg)
	if err != nil {
		t.Fatalf("encodeMessage() error = %v", err)
	}

	got, err := decodeMessage(payload)
	if err != nil {
		t.Fatalf("decodeMessage() error = %v", err)
	}

	if got.ID != msg.ID || got.Topic != msg.Topic || got.Key != msg.Key {
		t.Errorf("round trip mismatch: got %+v, want %+v", got, msg)
	}

	if string(got.Payload) != string(msg.Payload) {
		t.Errorf("payload = %q, want %q", got.Payload, msg.Payload)
	}

	if got.Headers["trace"] != "abc" {
		t.Errorf("headers = %v, want trace=abc", got.Headers)
	}

	if !got.CreatedAt.Equal(msg.CreatedAt) {
		t.Errorf("created_at = %v, want %v", got.CreatedAt, msg.CreatedAt)
	}

	if got.Attempts != msg.Attempts {
		t.Errorf("attempts = %d, want %d", got.Attempts, msg.Attempts)
	}
}

func TestEncodeMessageMatchesWireShape(t *testing.T) {
	t.Parallel()

	msg := outbox.Message{ID: "evt-2", Topic: "billing", Payload: []byte("x")}

	payload, err := encodeMessage(msg)
	if err != nil {
		t.Fatalf("encodeMessage() error = %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}

	if wire["id"] != "evt-2" || wire["topic"] != "billing" {
		t.Errorf("wire shape = %v, want id/topic fields", wire)
	}
}

func TestDecodeMessageInvalid(t *testing.T) {
	t.Parallel()

	if _, err := decodeMessage([]byte("not json")); err == nil {
		t.Fatal("decodeMessage(not json) = nil error, want error")
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr string
	}{
		{
			name:    "empty dsn",
			opts:    Options{Options: outbox.Options{Slot: "s", Publication: "p", Prefix: "x"}},
			wantErr: "dsn",
		},
		{
			name:    "empty slot",
			opts:    Options{Options: outbox.Options{DSN: "postgres://x", Publication: "p", Prefix: "x"}},
			wantErr: "slot",
		},
		{
			name:    "empty publication",
			opts:    Options{Options: outbox.Options{DSN: "postgres://x", Slot: "s", Prefix: "x"}},
			wantErr: "publication",
		},
		{
			name:    "empty prefix",
			opts:    Options{Options: outbox.Options{DSN: "postgres://x", Slot: "s", Publication: "p"}},
			wantErr: "prefix",
		},
		{
			name: "valid",
			opts: Options{Options: outbox.Options{DSN: "postgres://x", Slot: "s", Publication: "p", Prefix: "x"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.opts.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("Validate() = nil error, want %q", tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	t.Parallel()

	s, err := New(Options{Options: outbox.Options{DSN: "postgres://localhost/db"}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	st, ok := s.(*store)
	if !ok {
		t.Fatalf("New() = %T, want *store", s)
	}

	if st.prefix != DefaultPrefix {
		t.Errorf("prefix = %q, want %q", st.prefix, DefaultPrefix)
	}

	if st.slot != DefaultSlot {
		t.Errorf("slot = %q, want %q", st.slot, DefaultSlot)
	}

	if st.publication != DefaultPublication {
		t.Errorf("publication = %q, want %q", st.publication, DefaultPublication)
	}

	if st.maxAttempts != outbox.DefaultMaxAttempts {
		t.Errorf("maxAttempts = %d, want %d", st.maxAttempts, outbox.DefaultMaxAttempts)
	}
}

func TestNewInvalidOptions(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("New(empty) = nil error, want error")
	}
}

func TestRecordInvalidMessage(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix}
	tx := &fakeTx{}

	err := s.Record(t.Context(), tx, outbox.Message{})
	if !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("Record() error = %v, want ErrInvalidMessage", err)
	}

	if tx.execs != 0 {
		t.Errorf("Exec calls = %d, want 0 for invalid message", tx.execs)
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	s := &store{}
	if s.Name() != string(outbox.CDC) {
		t.Errorf("Name() = %q, want %q", s.Name(), outbox.CDC)
	}
}
