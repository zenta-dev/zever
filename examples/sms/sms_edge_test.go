package sms_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/examples/sms"
)

// edgeAdapter is a unique adapter name so this test binary's process-wide
// registry is not disturbed by other registrations.
const edgeAdapter = sms.Adapter("edge-test")

// edgeFactory returns a no-op SMS backend.
func edgeFactory(sms.Options) (sms.SMS, error) { return edgeDriver{}, nil }

type edgeDriver struct{}

func (edgeDriver) Send(context.Context, string, string) error { return nil }
func (edgeDriver) Close(context.Context) error                { return nil }

// TestParseAdapter pins the accept-any-non-empty contract and the empty
// rejection, including the typed error's sentinel.
func TestParseAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    sms.Adapter
		wantErr bool
	}{
		{name: "empty rejected", in: "", wantErr: true},
		{name: "stub accepted", in: "stub", want: sms.Stub},
		{name: "custom accepted", in: "twilio", want: sms.Adapter("twilio")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := sms.ParseAdapter(tt.in)
			if tt.wantErr {
				if !errors.Is(err, sms.ErrInvalidAdapter) {
					t.Fatalf("ParseAdapter(%q) err = %v, want ErrInvalidAdapter", tt.in, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseAdapter(%q) err = %v, want nil", tt.in, err)
			}

			if got != tt.want {
				t.Fatalf("ParseAdapter(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestOptionsValidate pins the sender-identity requirement.
func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	if err := (sms.Options{}).Validate(); !errors.Is(err, sms.ErrInvalidOptions) {
		t.Fatalf("Options{}.Validate() = %v, want ErrInvalidOptions", err)
	}

	if err := (sms.Options{From: "+15550000"}).Validate(); err != nil {
		t.Fatalf("Options{From}.Validate() = %v, want nil", err)
	}
}

// TestRegisterAndOpen covers the factory registry lifecycle: nil factory,
// duplicate registration, successful open, and unknown-adapter lookup.
func TestRegisterAndOpen(t *testing.T) {
	t.Parallel()

	if err := sms.Register(edgeAdapter, edgeFactory); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := sms.Register(edgeAdapter, edgeFactory); !errors.Is(err, sms.ErrDuplicate) {
		t.Fatalf("duplicate Register = %v, want ErrDuplicate", err)
	}

	if err := sms.Register("nil-factory", nil); !errors.Is(err, sms.ErrNilFactory) {
		t.Fatalf("nil factory Register = %v, want ErrNilFactory", err)
	}

	svc, err := sms.Open(edgeAdapter, sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if svc == nil {
		t.Fatal("Open returned nil SMS")
	}

	if _, err := sms.Open("definitely-unregistered", sms.Options{From: "+15550000"}); !errors.Is(err, sms.ErrUnknownAdapter) {
		t.Fatalf("Open unknown = %v, want ErrUnknownAdapter", err)
	}
}

// TestOpenValidatesOptionsBeforeLookup proves an invalid-options error wins
// over the unknown-adapter error, so callers fix config before selection.
func TestOpenValidatesOptionsBeforeLookup(t *testing.T) {
	t.Parallel()

	_, err := sms.Open("definitely-unregistered", sms.Options{})
	if !errors.Is(err, sms.ErrInvalidOptions) {
		t.Fatalf("Open = %v, want ErrInvalidOptions", err)
	}
}

// BenchmarkParseAdapter measures adapter-name parsing on the config load
// path.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := sms.ParseAdapter("stub"); err != nil {
			b.Fatalf("ParseAdapter: %v", err)
		}
	}
}

// BenchmarkOpen measures a successful factory lookup and open.
func BenchmarkOpen(b *testing.B) {
	if err := sms.Register(edgeAdapter, edgeFactory); err != nil {
		b.Fatalf("Register: %v", err)
	}

	opts := sms.Options{From: "+15550000"}
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		svc, err := sms.Open(edgeAdapter, opts)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}

		if err := svc.Close(ctx); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}
