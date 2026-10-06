package observability

import "context"

// SpanKind classifies the role of a span in a distributed trace.
type SpanKind int

const (
	// SpanKindInternal marks work within a single service, not tied to a request or message.
	SpanKindInternal SpanKind = iota
	// SpanKindServer marks a span handling an inbound request.
	SpanKindServer
	// SpanKindClient marks a span making an outbound request.
	SpanKindClient
	// SpanKindProducer marks a span publishing a message.
	SpanKindProducer
	// SpanKindConsumer marks a span processing a received message.
	SpanKindConsumer
)

// String returns the canonical name of SpanKind.
func (k SpanKind) String() string {
	switch k {
	case SpanKindInternal:
		return "internal"
	case SpanKindServer:
		return "server"
	case SpanKindClient:
		return "client"
	case SpanKindProducer:
		return "producer"
	case SpanKindConsumer:
		return "consumer"
	default:
		return "internal"
	}
}

// SpanLink references a span related to the one being started.
type SpanLink struct {
	// TraceID is the hex-encoded trace ID of the linked span.
	TraceID string
	// SpanID is the hex-encoded span ID of the linked span.
	SpanID string
	// Attrs carries attributes describing the link.
	Attrs []Attr
}

// SpanConfig holds resolved span start options.
type SpanConfig struct {
	// Kind classifies the span role; zero value is SpanKindInternal.
	Kind SpanKind
	// Attrs carries initial span attributes.
	Attrs []Attr
	// Links carries related spans.
	Links []SpanLink
}

// SpanStartOption customizes how a span is started.
type SpanStartOption func(*SpanConfig)

// WithSpanKind sets the span kind.
func WithSpanKind(kind SpanKind) SpanStartOption {
	return func(c *SpanConfig) { c.Kind = kind }
}

// WithAttributes sets initial span attributes.
func WithAttributes(attrs ...Attr) SpanStartOption {
	return func(c *SpanConfig) { c.Attrs = append(c.Attrs, attrs...) }
}

// WithLinks sets related spans.
func WithLinks(links ...SpanLink) SpanStartOption {
	return func(c *SpanConfig) { c.Links = append(c.Links, links...) }
}

// NewSpanConfig resolves opts into a SpanConfig, skipping nil options.
func NewSpanConfig(opts ...SpanStartOption) SpanConfig {
	var cfg SpanConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// SpanStarter is an optional interface Tracer implementers may provide to
// accept span kind, initial attributes, and links. Providers that do not
// implement it keep working: StartSpan falls back to Tracer.Start.
type SpanStarter interface {
	// StartSpan begins a span with the given name and start options,
	// returning the updated context.
	StartSpan(ctx context.Context, name string, opts ...SpanStartOption) (context.Context, Span)
}

// StartSpan begins a span using t. When t implements SpanStarter the opts
// are honored; otherwise it falls back to t.Start(ctx, name) and opts are
// ignored. A nil t returns (ctx, nil) without panicking.
func StartSpan(ctx context.Context, t Tracer, name string, opts ...SpanStartOption) (context.Context, Span) {
	if t == nil {
		return ctx, nil
	}
	if ss, ok := t.(SpanStarter); ok {
		return ss.StartSpan(ctx, name, opts...)
	}
	return t.Start(ctx, name)
}
