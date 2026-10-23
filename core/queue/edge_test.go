package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
