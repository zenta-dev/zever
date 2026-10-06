package queue

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

func TestOpenShared_success(t *testing.T) {
	t.Parallel()

	a := freshQueueAdapter()
	var gotOpts Options
	if err := RegisterShared(a, func(_ coredb.DB, opts Options) (Queue, error) {
		gotOpts = opts
		return stubQueue{}, nil
	}); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}

	want := Options{PollTimeout: time.Second}
	q, err := OpenShared(a, nil, want)
	if err != nil {
		t.Fatalf("OpenShared(%v) error = %v", a, err)
	}
	if q == nil {
		t.Fatal("OpenShared returned nil Queue")
	}
	if gotOpts != want {
		t.Fatalf("factory opts = %+v, want %+v", gotOpts, want)
	}
}

func TestRegisterShared_nilFactory(t *testing.T) {
	t.Parallel()

	a := freshQueueAdapter()
	if err := RegisterShared(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("RegisterShared(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegisterShared_duplicate(t *testing.T) {
	t.Parallel()

	a := freshQueueAdapter()
	stub := func(coredb.DB, Options) (Queue, error) { return stubQueue{}, nil }
	if err := RegisterShared(a, stub); err != nil {
		t.Fatalf("first RegisterShared(%v) error = %v", a, err)
	}
	if err := RegisterShared(a, stub); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second RegisterShared(%v) err = %v, want ErrDuplicate", a, err)
	}
}

func TestOpenShared_unknown(t *testing.T) {
	t.Parallel()

	_, err := OpenShared(Adapter("test-9500"), nil, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("OpenShared(unknown) err = %v, want ErrUnknownAdapter", err)
	}
}

func TestOpenShared_factoryError_wrapped(t *testing.T) {
	t.Parallel()

	a := freshQueueAdapter()
	sentinel := errors.New("shared boom")
	if err := RegisterShared(a, func(coredb.DB, Options) (Queue, error) { return nil, sentinel }); err != nil {
		t.Fatalf("RegisterShared(%v) error = %v", a, err)
	}
	_, err := OpenShared(a, nil, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("OpenShared err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "queue: open shared") {
		t.Fatalf("OpenShared err %q missing %q", err.Error(), "queue: open shared")
	}
}

func TestParseMessageID_boundaries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty", "", true},
		{"whitespace", " ", true},
		{"uppercase uuid", "018F9F5C-7B3A-7C2E-9A4B-1F2E3D4C5B6A", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id, err := ParseMessageID(tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidMessageID) {
					t.Fatalf("ParseMessageID(%q) err = %v, want ErrInvalidMessageID", tc.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMessageID(%q) error = %v", tc.in, err)
			}
			if id == (MessageID{}) {
				t.Fatalf("ParseMessageID(%q) returned zero ID", tc.in)
			}
		})
	}
}

func TestMessageID_zero_string(t *testing.T) {
	t.Parallel()

	zero := MessageID{}
	if got := zero.String(); got != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("zero MessageID.String() = %q", got)
	}
}

func TestClone_nil_and_empty(t *testing.T) {
	t.Parallel()

	var nilHeaders Headers
	if got := nilHeaders.Clone(); got != nil {
		t.Fatalf("nil Headers.Clone() = %v, want nil", got)
	}

	emptyHeaders := Headers{}
	if got := emptyHeaders.Clone(); got == nil || len(got) != 0 {
		t.Fatalf("empty Headers.Clone() = %v, want empty non-nil", got)
	}

	var nilPayload Payload
	if got := nilPayload.Clone(); got != nil {
		t.Fatalf("nil Payload.Clone() = %v, want nil", got)
	}

	emptyPayload := Payload{}
	if got := emptyPayload.Clone(); got == nil || len(got) != 0 {
		t.Fatalf("empty Payload.Clone() = %v, want empty non-nil", got)
	}
}

func TestMessageClone_nil_members(t *testing.T) {
	t.Parallel()

	msg := Message{Topic: "jobs"}
	cloned := msg.Clone()
	if cloned.Topic != "jobs" {
		t.Fatalf("Clone Topic = %q, want jobs", cloned.Topic)
	}
	if cloned.Payload != nil || cloned.Headers != nil {
		t.Fatalf("Clone of nil members = %+v, want nil payload/headers", cloned)
	}
}

func TestOptionsValidate_always_nil(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts Options
	}{
		{"zero", Options{}},
		{"negative buffer", Options{Buffer: -1}},
		{"negative visibility", Options{VisibilityTimeout: -time.Second}},
		{"max buffer", Options{Buffer: int(^uint(0) >> 1)}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.opts.Validate(); err != nil {
				t.Fatalf("Validate() err = %v, want nil", err)
			}
		})
	}
}

func TestRegisterShared_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshQueueAdapter()
			if err := RegisterShared(a, func(coredb.DB, Options) (Queue, error) { return stubQueue{}, nil }); err != nil {
				t.Errorf("RegisterShared(%v) error = %v", a, err)
				return
			}
			if _, err := OpenShared(a, nil, Options{}); err != nil {
				t.Errorf("OpenShared(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}

func TestRegisterOpen_concurrent(t *testing.T) {
	t.Parallel()

	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			a := freshQueueAdapter()
			if err := Register(a, func(Options) (Queue, error) { return stubQueue{}, nil }); err != nil {
				t.Errorf("Register(%v) error = %v", a, err)
				return
			}
			if _, err := Open(a, Options{}); err != nil {
				t.Errorf("Open(%v) error = %v", a, err)
			}
		}()
	}

	wg.Wait()
}

func TestStubQueue_contract(t *testing.T) {
	t.Parallel()

	var q Queue = stubQueue{}
	ctx := context.Background()
	if err := q.Push(ctx, "t", nil, nil); err != nil {
		t.Errorf("Push error = %v", err)
	}
	if _, err := q.Pop(ctx, "t"); err != nil {
		t.Errorf("Pop error = %v", err)
	}
	if ok, err := q.IsEmpty(ctx, "t"); err != nil || !ok {
		t.Errorf("IsEmpty = %v,%v want true,nil", ok, err)
	}
}
