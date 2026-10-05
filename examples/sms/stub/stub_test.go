package stub_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/examples/sms"
	"github.com/zenta-dev/zever/examples/sms/stub"
)

// recorder exposes the stub's inspection methods without exporting the
// concrete driver type.
type recorder interface {
	Sent() []stub.Message
	From() string
}

// registerOnce registers the stub factory exactly once for the process; the
// registry is process-wide, so repeated registration would error.
var registerOnce sync.Once

// ensureRegistered registers the stub backend once and returns its result.
func ensureRegistered(t *testing.T) {
	t.Helper()

	registerOnce.Do(func() {
		if err := stub.Register(); err != nil {
			t.Fatalf("Register: %v", err)
		}
	})
}

// openStub opens the registered stub backend with a valid sender.
func openStub(t *testing.T) sms.SMS {
	t.Helper()

	ensureRegistered(t)

	svc, err := sms.Open(sms.Stub, sms.Options{From: "+15550000"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return svc
}

// TestOpenValidatesOptions proves the stub constructor rejects empty options.
func TestOpenValidatesOptions(t *testing.T) {
	t.Parallel()

	if _, err := stub.Open(sms.Options{}); !errors.Is(err, sms.ErrInvalidOptions) {
		t.Fatalf("Open empty = %v, want ErrInvalidOptions", err)
	}
}

// TestSendAndInspect covers the happy path plus the recorded-message copy
// contract.
func TestSendAndInspect(t *testing.T) {
	t.Parallel()

	svc := openStub(t)

	rec, ok := svc.(recorder)
	if !ok {
		t.Fatal("stub backend does not expose Sent/From")
	}

	if got := rec.From(); got != "+15550000" {
		t.Fatalf("From() = %q, want +15550000", got)
	}

	if err := svc.Send(t.Context(), "+15550001", "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	sent := rec.Sent()
	if len(sent) != 1 || sent[0].To != "+15550001" || sent[0].Body != "hello" {
		t.Fatalf("Sent() = %+v, want one message to +15550001", sent)
	}

	sent[0].Body = "mutated"

	if again := rec.Sent(); again[0].Body != "hello" {
		t.Fatalf("Sent() returned a shared slice: body = %q, want hello", again[0].Body)
	}
}

// TestSendEmptyArgs pins the empty-boundary error paths.
func TestSendEmptyArgs(t *testing.T) {
	t.Parallel()

	svc := openStub(t)

	tests := []struct {
		name string
		to   string
		body string
	}{
		{name: "empty to", to: "", body: "hi"},
		{name: "empty body", to: "+15550001", body: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := svc.Send(t.Context(), tt.to, tt.body); !errors.Is(err, sms.ErrInvalidOptions) {
				t.Fatalf("Send = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

// TestCloseRejectsSend proves Close is terminal.
func TestCloseRejectsSend(t *testing.T) {
	t.Parallel()

	svc := openStub(t)

	if err := svc.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := svc.Send(t.Context(), "+15550001", "hi"); !errors.Is(err, sms.ErrClosed) {
		t.Fatalf("Send after Close = %v, want ErrClosed", err)
	}
}

// BenchmarkStubSend measures recording one message.
func BenchmarkStubSend(b *testing.B) {
	svc, err := stub.Open(sms.Options{From: "+15550000"})
	if err != nil {
		b.Fatalf("Open: %v", err)
	}

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if err := svc.Send(ctx, "+15550001", "hello"); err != nil {
			b.Fatalf("Send: %v", err)
		}
	}
}
