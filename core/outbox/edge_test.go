package outbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

func TestParseAdapterErrorType(t *testing.T) {
	t.Parallel()

	_, err := outbox.ParseAdapter("")
	if !errors.Is(err, outbox.ErrInvalidAdapter) {
		t.Fatalf("err = %v, want ErrInvalidAdapter", err)
	}

	var invalidErr outbox.InvalidAdapterError
	if !errors.As(err, &invalidErr) {
		t.Fatalf("err = %v, want InvalidAdapterError", err)
	}

	if invalidErr.Adapter != "" {
		t.Errorf("Adapter = %q, want empty", invalidErr.Adapter)
	}
}

func TestAdapterStringCustom(t *testing.T) {
	t.Parallel()

	if got := outbox.Adapter("custom").String(); got != "custom" {
		t.Errorf("String() = %q, want custom", got)
	}
}

func TestOpenEmptyAdapter(t *testing.T) {
	t.Parallel()

	_, err := outbox.Open(outbox.Adapter(""), outbox.Options{})
	if !errors.Is(err, outbox.ErrUnknownAdapter) {
		t.Fatalf("err = %v, want ErrUnknownAdapter", err)
	}

	var unknownErr outbox.UnknownAdapterError
	if !errors.As(err, &unknownErr) {
		t.Fatalf("err = %v, want UnknownAdapterError", err)
	}

	if unknownErr.Adapter != "" {
		t.Errorf("Adapter = %q, want empty", unknownErr.Adapter)
	}
}

func TestOpenFactoryError(t *testing.T) {
	t.Parallel()

	adapter := uniqueAdapter("stub-err")
	sentinel := errors.New("boom")

	if err := outbox.Register(adapter, func(outbox.Options) (outbox.Store, error) {
		return nil, sentinel
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, err := outbox.Open(adapter, outbox.Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped sentinel", err)
	}

	if !strings.Contains(err.Error(), "outbox: open") {
		t.Errorf("err = %v, want prefix %q", err, "outbox: open")
	}
}

func TestOpenSharedUnknownAdapter(t *testing.T) {
	t.Parallel()

	_, err := outbox.OpenShared(outbox.Adapter("nope-shared"), nil, outbox.Options{})
	if !errors.Is(err, outbox.ErrUnknownAdapter) {
		t.Fatalf("err = %v, want ErrUnknownAdapter", err)
	}
}

func TestOpenSharedInvalidOptions(t *testing.T) {
	t.Parallel()

	adapter := uniqueAdapter("stub-shared-opts")

	if err := outbox.RegisterShared(adapter, func(db.DB, outbox.Options) (outbox.Store, error) {
		return &stubStore{name: "stub"}, nil
	}); err != nil {
		t.Fatalf("RegisterShared() error = %v", err)
	}

	_, err := outbox.OpenShared(adapter, nil, outbox.Options{Table: "bad name"})
	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpenSharedFactoryError(t *testing.T) {
	t.Parallel()

	adapter := uniqueAdapter("stub-shared-err")
	sentinel := errors.New("boom")

	if err := outbox.RegisterShared(adapter, func(db.DB, outbox.Options) (outbox.Store, error) {
		return nil, sentinel
	}); err != nil {
		t.Fatalf("RegisterShared() error = %v", err)
	}

	_, err := outbox.OpenShared(adapter, nil, outbox.Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wrapped sentinel", err)
	}

	if !strings.Contains(err.Error(), "outbox: open shared") {
		t.Errorf("err = %v, want prefix %q", err, "outbox: open shared")
	}
}

func TestMessageValidateErrorType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		msg    outbox.Message
		reason string
	}{
		{"empty id", outbox.Message{Topic: "t"}, "id must be non-empty"},
		{"long id", outbox.Message{ID: strings.Repeat("x", outbox.MaxIDLen+1), Topic: "t"}, "id exceeds"},
		{"empty topic", outbox.Message{ID: "e1"}, "topic must be non-empty"},
	}

	for _, tc := range tests {
		err := tc.msg.Validate()
		if !errors.Is(err, outbox.ErrInvalidMessage) {
			t.Errorf("%s: err = %v, want ErrInvalidMessage", tc.name, err)
		}

		var msgErr outbox.InvalidMessageError
		if !errors.As(err, &msgErr) {
			t.Fatalf("%s: err = %v, want InvalidMessageError", tc.name, err)
		}

		if !strings.Contains(msgErr.Reason, tc.reason) {
			t.Errorf("%s: Reason = %q, want containing %q", tc.name, msgErr.Reason, tc.reason)
		}
	}
}

func TestMessageCloneNilFields(t *testing.T) {
	t.Parallel()

	clone := outbox.Message{}.Clone()
	if clone.Payload != nil {
		t.Errorf("Payload = %v, want nil", clone.Payload)
	}

	if clone.Headers != nil {
		t.Errorf("Headers = %v, want nil", clone.Headers)
	}
}

func TestOptionsValidateJoinsErrors(t *testing.T) {
	t.Parallel()

	err := outbox.Options{
		Table:        "bad name",
		InboxTable:   "1bad",
		PollInterval: -1,
		BatchSize:    -1,
		MaxAttempts:  -1,
		Retention:    -1,
		LockSeconds:  -1,
		Publisher:    "kafka",
	}.Validate()
	if err == nil {
		t.Fatal("Validate() = nil error, want joined violations")
	}

	for _, want := range []string{"table", "inbox_table", "poll_interval", "batch_size", "max_attempts", "retention", "lock_seconds", "publisher"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want containing %q", err, want)
		}
	}

	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Errorf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestPublisherFuncError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")

	var p outbox.Publisher = outbox.PublisherFunc(func(context.Context, outbox.Message) error {
		return sentinel
	})

	if err := p.Publish(t.Context(), outbox.Message{ID: "1", Topic: "t"}); !errors.Is(err, sentinel) {
		t.Fatalf("Publish() = %v, want sentinel", err)
	}
}

func TestDefaultPublisherUnset(t *testing.T) {
	t.Parallel()

	if got := outbox.Default().Publisher; got != "" {
		t.Errorf("Publisher = %q, want empty", got)
	}
}
