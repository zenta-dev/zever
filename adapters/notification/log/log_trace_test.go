package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel/trace"

	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	"github.com/zenta-dev/zever/core/notification"
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

func TestNotifyIncludesTraceID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()

	ctx := traceCtx(t, "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")
	if err := n.Notify(ctx, validNotification()); err != nil {
		t.Fatalf("Notify err = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if decoded["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id = %v, want 4bf92f3577b34da6a3ce929d0e0e4736", decoded["trace_id"])
	}
	// Wire Data must not carry the trace: correlation is log-attrs only.
	if data, ok := decoded["data"].(map[string]any); ok {
		if _, has := data["trace_id"]; has {
			t.Error("wire data contains trace_id, want log-attrs only")
		}
	}
}

func TestNotifyOmitsTraceIDWithoutSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()

	if err := n.Notify(t.Context(), validNotification()); err != nil {
		t.Fatalf("Notify err = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if _, has := decoded["trace_id"]; has {
		t.Errorf("trace_id present without span: %v", decoded["trace_id"])
	}
}
