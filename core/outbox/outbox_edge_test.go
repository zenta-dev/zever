package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

var edgeSeq atomic.Int64

func freshOutboxAdapter() outbox.Adapter {
	return outbox.Adapter(fmt.Sprintf("edge-%d", 1000+int(edgeSeq.Add(1))))
}

func TestOpen_factoryError_wrappedWithPrefixAndNil(t *testing.T) {
	t.Parallel()

	a := freshOutboxAdapter()
	sentinel := errors.New("boom")

	if err := outbox.Register(a, func(outbox.Options) (outbox.Store, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := outbox.Open(a, outbox.Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open() err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "outbox: open") {
		t.Fatalf("Open() err %q missing %q prefix", err.Error(), "outbox: open")
	}

	if got != nil {
		t.Fatalf("Open() value = %v, want nil on error", got)
	}
}

func TestOpenShared_unknownAdapter(t *testing.T) {
	t.Parallel()

	got, err := outbox.OpenShared(outbox.Adapter("edge-nope"), nil, outbox.Options{})
	if !errors.Is(err, outbox.ErrUnknownAdapter) {
		t.Fatalf("OpenShared() err = %v, want ErrUnknownAdapter", err)
	}

	if got != nil {
		t.Fatalf("OpenShared() value = %v, want nil on error", got)
	}
}

func TestOpenShared_invalidOptions(t *testing.T) {
	t.Parallel()

	a := freshOutboxAdapter()
	if err := outbox.RegisterShared(a, func(db.DB, outbox.Options) (outbox.Store, error) {
		return &stubStore{}, nil
	}); err != nil {
		t.Fatalf("RegisterShared() error = %v", err)
	}

	got, err := outbox.OpenShared(a, nil, outbox.Options{Table: "bad name"})
	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("OpenShared() err = %v, want ErrInvalidOptions", err)
	}

	if got != nil {
		t.Fatalf("OpenShared() value = %v, want nil on error", got)
	}
}

func TestOpenShared_factoryError_wrappedWithPrefix(t *testing.T) {
	t.Parallel()

	a := freshOutboxAdapter()
	sentinel := errors.New("shared boom")

	if err := outbox.RegisterShared(a, func(db.DB, outbox.Options) (outbox.Store, error) {
		return nil, sentinel
	}); err != nil {
		t.Fatalf("RegisterShared() error = %v", err)
	}

	got, err := outbox.OpenShared(a, nil, outbox.Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("OpenShared() err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "outbox: open shared") {
		t.Fatalf("OpenShared() err %q missing %q prefix", err.Error(), "outbox: open shared")
	}

	if got != nil {
		t.Fatalf("OpenShared() value = %v, want nil on error", got)
	}
}

func TestRegister_concurrent_uniqueAdapters(t *testing.T) {
	t.Parallel()

	const n = 16

	var wg sync.WaitGroup
	errs := make(chan error, n)
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshOutboxAdapter()
			if err := outbox.Register(a, func(outbox.Options) (outbox.Store, error) {
				return &stubStore{}, nil
			}); err != nil {
				errs <- fmt.Errorf("Register(%v) error = %w", a, err)
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

func TestRegisterShared_duplicate(t *testing.T) {
	t.Parallel()

	a := freshOutboxAdapter()
	factory := func(db.DB, outbox.Options) (outbox.Store, error) { return &stubStore{}, nil }

	if err := outbox.RegisterShared(a, factory); err != nil {
		t.Fatalf("first RegisterShared() error = %v", err)
	}

	if err := outbox.RegisterShared(a, factory); !errors.Is(err, outbox.ErrDuplicateAdapter) {
		t.Fatalf("second RegisterShared() err = %v, want ErrDuplicateAdapter", err)
	}
}

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

func TestParseAdapter_whitespaceAccepted(t *testing.T) {
	t.Parallel()

	got, err := outbox.ParseAdapter(" ")
	if err != nil {
		t.Fatalf("ParseAdapter(\" \") err = %v, want nil", err)
	}

	if got != outbox.Adapter(" ") {
		t.Fatalf("ParseAdapter(\" \") = %q, want %q", got, " ")
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
