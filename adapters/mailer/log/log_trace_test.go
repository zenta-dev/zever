package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/trace"

	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	"github.com/zenta-dev/zever/core/mailer"
)

func traceCtx(t *testing.T, traceHex, spanHex string) context.Context {
	t.Helper()

	tid, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}
	sid, err := trace.SpanIDFromHex(spanHex)
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	return trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}))
}

func TestSendIncludesTraceID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(mailer.Options{Host: "smtp.example.com", Port: 587}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()

	msg := &mailer.Mail{
		From:    mailer.Address{Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Subject: "hi",
		Body:    "hello",
	}
	ctx := traceCtx(t, "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")
	if err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if decoded["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id = %v, want 4bf92f3577b34da6a3ce929d0e0e4736", decoded["trace_id"])
	}
}

func TestSendOmitsTraceIDWithoutSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	m, err := mailerlog.NewWithWriter(mailer.Options{Host: "smtp.example.com", Port: 587}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer m.Close()

	msg := &mailer.Mail{
		From:    mailer.Address{Address: "from@example.com"},
		To:      []mailer.Address{{Address: "to@example.com"}},
		Subject: "hi",
		Body:    "hello",
	}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if _, has := decoded["trace_id"]; has {
		t.Errorf("trace_id present without span: %v", decoded["trace_id"])
	}
}
