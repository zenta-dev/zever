package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/observability"
)

// TestRegRemovalTypedErrors covers every typed error left uncovered when the
// adapter registry was removed: Register/Open used to exercise the adapter
// errors via duplicate/unknown/invalid paths, so direct unit tests now pin
// the message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSubs []string
		asTarget any
	}{
		{"duplicate", DuplicateAdapterError{Adapter: "db"}, ErrDuplicateAdapter, []string{"db"}, &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "db"}, ErrUnknownAdapter, []string{"db"}, &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, []string{"bogus"}, &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad table"}, ErrInvalidOptions, []string{"bad table"}, &InvalidOptionsError{}},
		{"message", InvalidMessageError{Reason: "empty topic"}, ErrInvalidMessage, []string{"empty topic"}, &InvalidMessageError{}},
		{
			"stalled",
			StalledError{Pending: 3, Oldest: time.Minute, LastError: "boom"},
			ErrStalled,
			[]string{"3", "boom"},
			&StalledError{},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "outbox: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "outbox: ")
			}

			for _, sub := range tc.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("Error() = %q, want substring %q", msg, sub)
				}
			}

			if !errors.Is(tc.err, tc.sentinel) {
				t.Errorf("errors.Is(err, sentinel) = false (err = %v)", tc.err)
			}

			if !errors.As(tc.err, tc.asTarget) {
				t.Errorf("errors.As(%T) = false", tc.err)
			}
		})
	}
}

// TestRegRemovalValidIdentifierEmpty covers the empty-name rejection: table
// names interpolate into DDL, so the empty string must never validate.
// Validate guards empty table names before calling, so this pins the helper
// directly.
func TestRegRemovalValidIdentifierEmpty(t *testing.T) {
	t.Parallel()

	if validIdentifier("") {
		t.Error("validIdentifier(\"\") = true, want false")
	}

	if !validIdentifier("outbox_v2") {
		t.Error("validIdentifier(outbox_v2) = false, want true")
	}

	if validIdentifier("no-dashes") {
		t.Error("validIdentifier(no-dashes) = true, want false")
	}
}

// regRemovalSpanOnlyProvider wires a tracer but no meter: Meter returns a
// nil interface so Recorder.meter reports absent while spans still record.
type regRemovalSpanOnlyProvider struct {
	tracer *fakeTracer
}

func (p *regRemovalSpanOnlyProvider) Tracer(string) observability.Tracer { return p.tracer }

func (p *regRemovalSpanOnlyProvider) Meter(string) observability.Metrics { return nil }

func (p *regRemovalSpanOnlyProvider) Shutdown(context.Context) error { return nil }

// TestRegRemovalPublishSpanMeterNil covers the PublishSpan finisher's
// meter-nil branch: a provider with a tracer but no meter still ends the
// span while skipping duration recording.
func TestRegRemovalPublishSpanMeterNil(t *testing.T) {
	t.Parallel()

	p := &regRemovalSpanOnlyProvider{tracer: &fakeTracer{}}
	r := NewRecorder(p, "db", "outbox", "")

	spanCtx, finish := r.PublishSpan(t.Context(), "orders", "msg-1")
	if spanCtx == nil {
		t.Fatal("PublishSpan() ctx = nil")
	}

	finish(OutcomeOK)

	if got := p.tracer.spanCount(SpanPublish); got != 1 {
		t.Errorf("publish spans = %d, want 1", got)
	}
}
