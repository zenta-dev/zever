package job

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/core/queue"
)

type traceStubSpan struct {
	trace.Span
	sc trace.SpanContext
}

func (s *traceStubSpan) SpanContext() trace.SpanContext { return s.sc }

func (s *traceStubSpan) End(...trace.SpanEndOption) {}

type traceStubTracer struct {
	trace.Tracer
	mu      sync.Mutex
	counter byte
	names   []string
}

func (t *traceStubTracer) Start(ctx context.Context, name string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	t.mu.Lock()
	t.counter++
	c := t.counter
	if c == 0 {
		c = 1
		t.counter = 1
	}
	t.names = append(t.names, name)
	t.mu.Unlock()

	var tid trace.TraceID
	tid[0] = 0x4b
	tid[1] = 0xf9
	tid[15] = c
	var sid trace.SpanID
	sid[0] = 0xab
	sid[7] = c
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
	span := &traceStubSpan{sc: sc}

	return trace.ContextWithSpan(ctx, span), span
}

type traceStubProvider struct {
	trace.TracerProvider
	tracer *traceStubTracer
}

func (p *traceStubProvider) Tracer(_ string, _ ...trace.TracerOption) trace.Tracer {
	return p.tracer
}

func installTraceStub(t *testing.T) *traceStubTracer {
	t.Helper()

	prev := otel.GetTracerProvider()
	st := &traceStubTracer{}
	otel.SetTracerProvider(&traceStubProvider{tracer: st})
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	return st
}

func TestScheduleFireStartsNewRootSpan(t *testing.T) {
	Reset()
	st := installTraceStub(t)
	if err := Register("trace-root-job", func(context.Context, string) error { return nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}

	var mu sync.Mutex
	var traceIDs []string
	var headers []queue.Headers
	sq := &stubQueue{
		pushFn: func(ctx context.Context, _ string, _ queue.Payload, h queue.Headers) error {
			mu.Lock()
			defer mu.Unlock()
			traceIDs = append(traceIDs, trace.SpanContextFromContext(ctx).TraceID().String())
			cp := make(queue.Headers, len(h))
			for k, v := range h {
				cp[k] = v
			}
			headers = append(headers, cp)

			return nil
		},
	}
	s := NewScheduler(&Dispatcher{Q: sq}, NewUniqueLocker(&fakeCache{}))
	s.Locker = nil
	fixed := time.Date(2026, time.January, 1, 0, 7, 0, 0, time.UTC)
	s.now = func() time.Time { return fixed }
	sched, err := cron.ParseStandard("0 * * * *")
	if err != nil {
		t.Fatalf("ParseStandard: %v", err)
	}

	// Caller trace must be discarded: fireWithSchedule always roots at
	// Background, so no caller ctx is stored on the scheduler.
	s.fireWithSchedule("trace-root-job", "arg", "0 * * * *", sched)
	s.now = func() time.Time { return fixed.Add(2 * time.Hour) }
	s.fireWithSchedule("trace-root-job", "arg", "0 * * * *", sched)

	mu.Lock()
	defer mu.Unlock()
	if len(traceIDs) != 2 {
		t.Fatalf("fires = %d, want 2", len(traceIDs))
	}
	for i, id := range traceIDs {
		if id == "" || id == "00000000000000000000000000000000" {
			t.Errorf("fire %d traceID = %q, want valid", i, id)
		}
	}
	if traceIDs[0] == traceIDs[1] {
		t.Errorf("traceIDs %q == %q, want distinct across fires", traceIDs[0], traceIDs[1])
	}
	if len(st.names) != 2 {
		t.Fatalf("spans = %d, want 2", len(st.names))
	}
	for i, name := range st.names {
		if name != "schedule.trace-root-job" {
			t.Errorf("span %d name = %q, want %q", i, name, "schedule.trace-root-job")
		}
	}
	for i, h := range headers {
		tp := h["traceparent"]
		if tp == "" {
			t.Errorf("fire %d headers missing traceparent: %v", i, h)
		}
	}
}
