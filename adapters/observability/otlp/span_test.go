package otlp

import (
	"bytes"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/core/observability"
)

func TestStartSpan_implementsSpanStarter(t *testing.T) {
	t.Parallel()

	var _ observability.SpanStarter = (*tracer)(nil)
}

func TestStartSpan_kindMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind observability.SpanKind
		want trace.SpanKind
	}{
		{observability.SpanKindInternal, trace.SpanKindInternal},
		{observability.SpanKindServer, trace.SpanKindServer},
		{observability.SpanKindClient, trace.SpanKindClient},
		{observability.SpanKindProducer, trace.SpanKindProducer},
		{observability.SpanKindConsumer, trace.SpanKindConsumer},
	}

	for _, tc := range tests {
		t.Run(tc.want.String(), func(t *testing.T) {
			t.Parallel()

			sr := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
			t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

			tr := &tracer{tp: tp, tracer: tp.Tracer("scope")}

			_, sp := tr.StartSpan(t.Context(), "op", observability.WithSpanKind(tc.kind))
			sp.End()

			ended := sr.Ended()
			if len(ended) != 1 {
				t.Fatalf("ended spans = %d, want 1", len(ended))
			}
			if got := ended[0].SpanKind(); got != tc.want {
				t.Errorf("SpanKind = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStartSpan_attrsAndLinks(t *testing.T) {
	t.Parallel()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	tr := &tracer{tp: tp, tracer: tp.Tracer("scope")}

	_, sp := tr.StartSpan(t.Context(), "op",
		observability.WithAttributes(observability.String("k", "v"), observability.Int("n", 3)),
		observability.WithLinks(observability.SpanLink{
			TraceID: strings.Repeat("a", 32),
			SpanID:  strings.Repeat("b", 16),
			Attrs:   []observability.Attr{observability.String("lk", "lv")},
		}),
	)
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}

	found := map[string]attribute.Value{}
	for _, kv := range ended[0].Attributes() {
		found[string(kv.Key)] = kv.Value
	}
	if got := found["k"].AsString(); got != "v" {
		t.Errorf("attr k = %q, want v", got)
	}
	if got := found["n"].AsInt64(); got != 3 {
		t.Errorf("attr n = %d, want 3", got)
	}

	links := ended[0].Links()
	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}
	if got := links[0].SpanContext.TraceID(); got != trace.TraceID(bytes.Repeat([]byte{0xaa}, 16)) {
		t.Errorf("link TraceID = %v, want 16xaa", got)
	}
	linkAttrs := map[string]attribute.Value{}
	for _, kv := range links[0].Attributes {
		linkAttrs[string(kv.Key)] = kv.Value
	}
	if got := linkAttrs["lk"].AsString(); got != "lv" {
		t.Errorf("link attr lk = %q, want lv", got)
	}
}

func TestStartSpan_invalidLinkSkipped(t *testing.T) {
	t.Parallel()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	tr := &tracer{tp: tp, tracer: tp.Tracer("scope")}

	_, sp := tr.StartSpan(t.Context(), "op",
		observability.WithLinks(observability.SpanLink{TraceID: "zz", SpanID: "bb"}),
	)
	sp.End()

	ended := sr.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	if got := len(ended[0].Links()); got != 0 {
		t.Errorf("links = %d, want 0 (invalid hex skipped)", got)
	}
}

func TestStart_delegatesToStartSpan(t *testing.T) {
	t.Parallel()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	tr := &tracer{tp: tp, tracer: tp.Tracer("scope")}

	_, sp := tr.Start(t.Context(), "op")
	sp.End()

	if ended := sr.Ended(); len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
}
