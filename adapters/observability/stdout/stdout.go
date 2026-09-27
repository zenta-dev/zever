package stdout

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/codec"
)

type spanCtxKey struct{}

type spanInfo struct {
	traceID string
	spanID  string
}

type line struct {
	TraceID string `json:"trace_id"`
	SpanID  string `json:"span_id,omitempty"`
	Name    string `json:"name"`
	// DurationMs is int64 milliseconds on the wire, not a time.Duration — intentional.
	DurationMs int64          `json:"duration_ms,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type shared struct {
	mu      sync.Mutex
	w       io.Writer
	codec   codec.Codec[line]
	verbose bool
	limit   int
}

func (s *shared) emit(l line) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.codec.Encode(l)
	if err != nil {
		return
	}
	_, _ = s.w.Write(append(data, '\n'))
}

type provider struct {
	tracer  *tracer
	metrics *metrics
}

type tracer struct {
	sh *shared
}

type span struct {
	sh      *shared
	info    spanInfo
	name    string
	start   time.Time
	mu      sync.Mutex
	attrs   []observability.Attr
	errText string
}

type metrics struct {
	sh *shared
}

// New returns a Provider that writes JSON telemetry lines to os.Stdout.
func New(opts observability.Options) (observability.Provider, error) {
	return NewWithWriter(opts, os.Stdout)
}

// NewWithWriter returns a Provider that writes JSON telemetry lines to w.
func NewWithWriter(opts observability.Options, w io.Writer) (observability.Provider, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	limit := opts.AttrValueLimit
	if limit <= 0 {
		limit = observability.MaxValueLen
	}

	sh := &shared{w: w, codec: codec.JSONCodec[line]{}, verbose: opts.Verbose, limit: limit}

	return &provider{tracer: &tracer{sh: sh}, metrics: &metrics{sh: sh}}, nil
}

func (p *provider) Tracer(string) observability.Tracer { return p.tracer }
func (p *provider) Meter(string) observability.Metrics { return p.metrics }

func (p *provider) Shutdown(context.Context) error { return nil }

func (t *tracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	if ctx == nil {
		ctx = context.Background()
	}

	traceID := ""
	if info, ok := ctx.Value(spanCtxKey{}).(spanInfo); ok {
		traceID = info.traceID
	}
	if traceID == "" {
		traceID = randHex(16)
	}

	info := spanInfo{traceID: traceID, spanID: randHex(8)}
	s := &span{sh: t.sh, info: info, name: name, start: time.Now()}

	return context.WithValue(ctx, spanCtxKey{}, info), s
}

func (t *tracer) Shutdown(context.Context) error { return nil }

func (s *span) SetAttributes(attrs ...observability.Attr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attrs = append(s.attrs, attrs...)
}

func (s *span) RecordError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errText = err.Error()
}

func (s *span) End() {
	s.mu.Lock()
	attrs := append([]observability.Attr(nil), s.attrs...)
	errText := s.errText
	s.mu.Unlock()

	s.sh.emit(line{
		TraceID:    s.info.traceID,
		SpanID:     s.info.spanID,
		Name:       s.name,
		DurationMs: time.Since(s.start).Milliseconds(),
		Attrs:      flatten(attrs, s.sh.limit),
		Error:      errText,
	})
}

func (m *metrics) Counter(_ context.Context, name string, value float64, attrs ...observability.Attr) error {
	return m.record("counter", name, value, attrs)
}

func (m *metrics) Gauge(_ context.Context, name string, value float64, attrs ...observability.Attr) error {
	return m.record("gauge", name, value, attrs)
}

func (m *metrics) Histogram(_ context.Context, name string, value float64, attrs ...observability.Attr) error {
	return m.record("histogram", name, value, attrs)
}

func (m *metrics) record(kind, name string, value float64, attrs []observability.Attr) error {
	if !m.sh.verbose {
		return nil
	}

	flat := flatten(attrs, m.sh.limit)
	flat["kind"] = kind
	flat["value"] = value

	m.sh.emit(line{Name: name, Attrs: flat})
	return nil
}

func (m *metrics) Shutdown(context.Context) error { return nil }

func flatten(attrs []observability.Attr, limit int) map[string]any {
	if len(attrs) == 0 {
		return nil
	}

	out := make(map[string]any, len(attrs))
	for _, a := range attrs {
		if len(a.Key) > observability.MaxKeyLen {
			a.Key = a.Key[:observability.MaxKeyLen]
		}
		out[a.Key] = plainValue(a, limit)
	}
	return out
}

func plainValue(a observability.Attr, limit int) any {
	switch v := a.Value.(type) {
	case observability.StringValue:
		s := v.Value
		if len(s) > limit {
			s = s[:limit]
		}
		return s
	case observability.Int64Value:
		return v.Value
	case observability.Float64Value:
		return v.Value
	case observability.BoolValue:
		return v.Value
	default:
		return nil
	}
}

var fallbackSeq atomic.Uint64

var (
	randReadMu sync.Mutex
	// randRead is crypto/rand.Read as a var so tests can inject failures.
	// Guarded by randReadMu: randHex takes it for the read, tests hold it
	// across override windows, so parallel span tests never race the seam.
	randRead = rand.Read
)

func randHex(n int) string {
	b := make([]byte, n)
	randReadMu.Lock()
	_, err := randRead(b)
	randReadMu.Unlock()
	if err == nil {
		return hex.EncodeToString(b)
	}

	seq := fallbackSeq.Add(1)
	now := uint64(time.Now().UnixNano())

	fallback := make([]byte, n)
	binary.LittleEndian.PutUint64(fallback, now)

	if n > 8 {
		binary.LittleEndian.PutUint64(fallback[8:], seq)
	} else {
		for i := range fallback {
			fallback[i] ^= byte((seq >> (uint(i) % 8 * 8)) & 0xff) //nolint:gosec // intentional truncation: only low 8 bits seed the fallback id
		}
	}

	return hex.EncodeToString(fallback)
}
