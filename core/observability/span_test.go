package observability

import (
	"context"
	"sync"
	"testing"
)

type stubSpan struct{}

func (stubSpan) SetAttributes(...Attr) {}
func (stubSpan) RecordError(error)     {}
func (stubSpan) End()                  {}

type stubTracer struct {
	mu      sync.Mutex
	started int
}

func (s *stubTracer) Start(context.Context, string) (context.Context, Span) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started++
	return context.Background(), stubSpan{}
}

func (s *stubTracer) Shutdown(context.Context) error { return nil }

type starterTracer struct {
	mu      sync.Mutex
	gotName string
	gotCfg  SpanConfig
}

func (s *starterTracer) StartSpan(_ context.Context, name string, opts ...SpanStartOption) (context.Context, Span) {
	cfg := NewSpanConfig(opts...)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gotName = name
	s.gotCfg = cfg
	return context.Background(), stubSpan{}
}

func (s *starterTracer) Start(ctx context.Context, name string) (context.Context, Span) {
	return s.StartSpan(ctx, name)
}

func (s *starterTracer) Shutdown(context.Context) error { return nil }

func TestStartSpan_fallbackIgnoresOpts(t *testing.T) {
	t.Parallel()

	tr := &stubTracer{}

	ctx, span := StartSpan(t.Context(), tr, "op",
		WithSpanKind(SpanKindClient),
		WithAttributes(String("k", "v")),
		WithLinks(SpanLink{TraceID: "aa", SpanID: "bb"}),
	)
	if ctx == nil {
		t.Error("StartSpan() ctx = nil")
	}
	if span == nil {
		t.Fatal("StartSpan() span = nil")
	}

	tr.mu.Lock()
	started := tr.started
	tr.mu.Unlock()

	if started != 1 {
		t.Errorf("Start() calls = %d, want 1 (fallback path)", started)
	}
}

func TestStartSpan_spanStarterReceivesOpts(t *testing.T) {
	t.Parallel()

	tr := &starterTracer{}

	_, span := StartSpan(t.Context(), tr, "op",
		WithSpanKind(SpanKindProducer),
		WithAttributes(String("k", "v"), Int("n", 2)),
		WithLinks(SpanLink{TraceID: "aa", SpanID: "bb", Attrs: []Attr{String("lk", "lv")}}),
	)
	if span == nil {
		t.Fatal("StartSpan() span = nil")
	}

	tr.mu.Lock()
	name := tr.gotName
	cfg := tr.gotCfg
	tr.mu.Unlock()

	if name != "op" {
		t.Errorf("name = %q, want op", name)
	}
	if cfg.Kind != SpanKindProducer {
		t.Errorf("kind = %v, want SpanKindProducer", cfg.Kind)
	}
	if len(cfg.Attrs) != 2 {
		t.Fatalf("attrs len = %d, want 2", len(cfg.Attrs))
	}
	if cfg.Attrs[0].Key != "k" {
		t.Errorf("attrs[0].Key = %q, want k", cfg.Attrs[0].Key)
	}
	if len(cfg.Links) != 1 {
		t.Fatalf("links len = %d, want 1", len(cfg.Links))
	}
	if cfg.Links[0].TraceID != "aa" || cfg.Links[0].SpanID != "bb" {
		t.Errorf("link ids = %q/%q, want aa/bb", cfg.Links[0].TraceID, cfg.Links[0].SpanID)
	}
	if len(cfg.Links[0].Attrs) != 1 || cfg.Links[0].Attrs[0].Key != "lk" {
		t.Errorf("link attrs = %v, want one lk attr", cfg.Links[0].Attrs)
	}
}

func TestStartSpan_nilTracer_returnsNilSpan(t *testing.T) {
	t.Parallel()

	ctx, span := StartSpan(t.Context(), nil, "op", WithSpanKind(SpanKindServer))
	if ctx == nil {
		t.Error("StartSpan() ctx = nil")
	}
	if span != nil {
		t.Errorf("StartSpan() span = %v, want nil", span)
	}
}

func TestNewSpanConfig_optionConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []SpanStartOption
		want SpanConfig
	}{
		{name: "nil opts", opts: nil, want: SpanConfig{}},
		{name: "nil option skipped", opts: []SpanStartOption{nil, WithSpanKind(SpanKindConsumer), nil}, want: SpanConfig{Kind: SpanKindConsumer}},
		{name: "kind only", opts: []SpanStartOption{WithSpanKind(SpanKindServer)}, want: SpanConfig{Kind: SpanKindServer}},
		{
			name: "attrs accumulate",
			opts: []SpanStartOption{WithAttributes(String("a", "1")), WithAttributes(Int("b", 2))},
			want: SpanConfig{Attrs: []Attr{String("a", "1"), Int("b", 2)}},
		},
		{
			name: "links accumulate",
			opts: []SpanStartOption{WithLinks(SpanLink{TraceID: "a", SpanID: "b"}), WithLinks(SpanLink{TraceID: "c", SpanID: "d"})},
			want: SpanConfig{Links: []SpanLink{{TraceID: "a", SpanID: "b"}, {TraceID: "c", SpanID: "d"}}},
		},
		{
			name: "combined",
			opts: []SpanStartOption{WithSpanKind(SpanKindClient), WithAttributes(Bool("x", true)), WithLinks(SpanLink{SpanID: "z"})},
			want: SpanConfig{Kind: SpanKindClient, Attrs: []Attr{Bool("x", true)}, Links: []SpanLink{{SpanID: "z"}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := NewSpanConfig(tc.opts...)
			if got.Kind != tc.want.Kind {
				t.Errorf("Kind = %v, want %v", got.Kind, tc.want.Kind)
			}
			if len(got.Attrs) != len(tc.want.Attrs) {
				t.Errorf("Attrs len = %d, want %d", len(got.Attrs), len(tc.want.Attrs))
			}
			if len(got.Links) != len(tc.want.Links) {
				t.Errorf("Links len = %d, want %d", len(got.Links), len(tc.want.Links))
			}
		})
	}
}

func TestSpanKindString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind SpanKind
		want string
	}{
		{SpanKindInternal, "internal"},
		{SpanKindServer, "server"},
		{SpanKindClient, "client"},
		{SpanKindProducer, "producer"},
		{SpanKindConsumer, "consumer"},
		{SpanKind(99), "internal"},
	}

	for _, tc := range tests {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("SpanKind(%d).String() = %q, want %q", int(tc.kind), got, tc.want)
		}
	}
}
