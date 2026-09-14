package stdout_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/observability"
	"github.com/zenta-dev/zever/observability/stdout"
)

func validOptions() observability.Options {
	return observability.Options{ServiceName: "svc", SampleRatio: 1}
}

func openBuffered(t *testing.T, mut func(*observability.Options)) (observability.Provider, *bytes.Buffer) {
	t.Helper()

	opts := validOptions()
	if mut != nil {
		mut(&opts)
	}

	var buf bytes.Buffer
	p, err := stdout.NewWithWriter(opts, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() error = %v", err)
	}

	return p, &buf
}

func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var v map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &v); err != nil {
		t.Fatalf("decode line %q: %v", buf.String(), err)
	}
	return v
}

func TestNew_invalidOptions_returnsError(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.ServiceName = ""

	if _, err := stdout.New(opts); err == nil {
		t.Fatal("New() error = nil, want invalid options")
	} else if !errors.Is(err, observability.ErrInvalidOptions) {
		t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
	}
}

func TestSpan_end_emitsJSONLine(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)

	ctx, span := p.Tracer("scope").Start(context.Background(), "op")
	span.SetAttributes(observability.String("k", "v"))
	span.End()
	_ = ctx

	v := decodeLine(t, buf)
	if v["name"] != "op" {
		t.Errorf("name = %v, want op", v["name"])
	}
	if v["trace_id"] == "" || v["trace_id"] == nil {
		t.Error("trace_id missing")
	}
	if v["span_id"] == "" || v["span_id"] == nil {
		t.Error("span_id missing")
	}
	if attrs, ok := v["attrs"].(map[string]any); !ok || attrs["k"] != "v" {
		t.Errorf("attrs = %v, want k=v", v["attrs"])
	}
}

func TestSpan_childInheritsTraceID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p, err := stdout.NewWithWriter(validOptions(), &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() error = %v", err)
	}

	pc, ps := p.Tracer("s").Start(context.Background(), "parent")
	cc, cs := p.Tracer("s").Start(pc, "child")
	cs.End()
	ps.End()
	_ = cc

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}

	var first, second map[string]any
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if err := json.Unmarshal(lines[1], &second); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if first["trace_id"] != second["trace_id"] {
		t.Errorf("child trace %v != parent %v", first["trace_id"], second["trace_id"])
	}
	if first["span_id"] == second["span_id"] {
		t.Error("span ids equal, want distinct")
	}
}

func TestSpan_recordError_included(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)

	_, span := p.Tracer("s").Start(context.Background(), "op")
	span.RecordError(errors.New("boom"))
	span.RecordError(nil)
	span.End()

	if v := decodeLine(t, buf); v["error"] != "boom" {
		t.Errorf("error = %v, want boom", v["error"])
	}
}

func TestMetrics_quietByDefault_verboseEmits(t *testing.T) {
	t.Parallel()

	p, buf := openBuffered(t, nil)
	m := p.Meter("s")

	if err := m.Counter(context.Background(), "c", 1); err != nil {
		t.Fatalf("Counter() error = %v", err)
	}
	if err := m.Gauge(context.Background(), "g", 2); err != nil {
		t.Fatalf("Gauge() error = %v", err)
	}
	if err := m.Histogram(context.Background(), "h", 3); err != nil {
		t.Fatalf("Histogram() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("quiet buf len = %d, want 0", buf.Len())
	}

	pv, pbuf := openBuffered(t, func(o *observability.Options) { o.Verbose = true })
	mv := pv.Meter("s")
	if err := mv.Counter(context.Background(), "c", 1, observability.Int("n", 2)); err != nil {
		t.Fatalf("Counter() error = %v", err)
	}
	if pbuf.Len() == 0 {
		t.Fatal("verbose buf empty, want metric line")
	}
}

func TestProvider_shutdown_nilError(t *testing.T) {
	t.Parallel()

	p, _ := openBuffered(t, nil)

	ctx := context.Background()
	if err := p.Tracer("s").Shutdown(ctx); err != nil {
		t.Errorf("Tracer.Shutdown() = %v, want nil", err)
	}
	if err := p.Meter("s").Shutdown(ctx); err != nil {
		t.Errorf("Metrics.Shutdown() = %v, want nil", err)
	}
	if err := p.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() = %v, want nil", err)
	}
}

func TestSpan_concurrentSafe(t *testing.T) {
	t.Parallel()

	p, _ := openBuffered(t, nil)
	tr := p.Tracer("s")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, s := tr.Start(context.Background(), "op")
			s.SetAttributes(observability.Bool("b", true))
			s.End()
		}()
	}
	wg.Wait()
}
