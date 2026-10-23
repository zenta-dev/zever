package outbox_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

var edgeSeq atomic.Int64

func TestMessage_Validate_emptyTopic(t *testing.T) {
	t.Parallel()

	err := (outbox.Message{ID: "e1"}).Validate()
	if !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("Validate() err = %v, want ErrInvalidMessage", err)
	}
}

func TestMessage_IDMultibyteByteLengthBoundary(t *testing.T) {
	t.Parallel()

	atLimit := outbox.Message{ID: strings.Repeat("é", 127) + "a", Topic: "t"}
	if len(atLimit.ID) != outbox.MaxIDLen {
		t.Fatalf("test setup: ID byte length = %d, want %d", len(atLimit.ID), outbox.MaxIDLen)
	}

	if err := atLimit.Validate(); err != nil {
		t.Fatalf("Validate() at byte limit err = %v, want nil", err)
	}

	overLimit := outbox.Message{ID: strings.Repeat("é", 128), Topic: "t"}
	if err := overLimit.Validate(); !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("Validate() over byte limit err = %v, want ErrInvalidMessage", err)
	}
}

func TestMessage_Clone_nilMembersStayNil(t *testing.T) {
	t.Parallel()

	clone := (outbox.Message{ID: "e1", Topic: "t"}).Clone()
	if clone.Payload != nil {
		t.Errorf("Clone() Payload = %v, want nil", clone.Payload)
	}

	if clone.Headers != nil {
		t.Errorf("Clone() Headers = %v, want nil", clone.Headers)
	}
}

func TestMessage_Clone_emptyNonNilPreserved(t *testing.T) {
	t.Parallel()

	orig := outbox.Message{ID: "e1", Topic: "t", Payload: []byte{}, Headers: map[string]string{}}
	clone := orig.Clone()

	if clone.Payload == nil {
		t.Error("Clone() Payload = nil, want non-nil empty")
	}

	if clone.Headers == nil {
		t.Error("Clone() Headers = nil, want non-nil empty")
	}
}

func TestOptions_multipleViolationsJoined(t *testing.T) {
	t.Parallel()

	err := outbox.Options{Table: "bad name", PollInterval: -1}.Validate()
	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
	}

	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		t.Fatalf("Validate() err %T does not unwrap to []error", err)
	}

	if got := len(joined.Unwrap()); got != 2 {
		t.Fatalf("Validate() joined %d violations, want 2", got)
	}
}

func TestOptions_tableNameEdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		table   string
		wantErr bool
	}{
		{"underscore only", "_", false},
		{"leading digit", "1table", true},
		{"non-ascii", "tüné", true},
		{"embedded space", "my table", true},
		{"trailing space", "table ", true},
		{"valid mixed", "outbox_2", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := (outbox.Options{Table: tc.table}).Validate()
			if tc.wantErr && !errors.Is(err, outbox.ErrInvalidOptions) {
				t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}

func TestOptions_inboxTableValidated(t *testing.T) {
	t.Parallel()

	err := (outbox.Options{InboxTable: "bad name"}).Validate()
	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
	}
}

func TestPublisherFunc_errorPassthrough(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("publish failed")
	p := outbox.PublisherFunc(func(context.Context, outbox.Message) error { return sentinel })

	if err := p.Publish(t.Context(), outbox.Message{ID: "1", Topic: "t"}); !errors.Is(err, sentinel) {
		t.Fatalf("Publish() err = %v, want sentinel", err)
	}
}

func TestStatus_zeroValue(t *testing.T) {
	t.Parallel()

	var s outbox.Status
	if s.Pending != 0 || s.Processed != 0 || s.Failed != 0 || s.LastError != "" || s.Stalled {
		t.Fatalf("Status zero value = %+v, want all zero", s)
	}
}

func TestDefault_cdcFieldsZero(t *testing.T) {
	t.Parallel()

	got := outbox.Default()
	if got.NotifyChannel != "" || got.Prefix != "" || got.Slot != "" || got.Publication != "" {
		t.Fatalf("Default() CDC fields = %+v, want all empty", got)
	}

	if got.DedicatedPool {
		t.Fatal("Default() DedicatedPool = true, want false")
	}
}
